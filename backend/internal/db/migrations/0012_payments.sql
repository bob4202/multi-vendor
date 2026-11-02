-- +goose Up

-- Card payments. Every new order is paid by card on the provider's hosted
-- page: the total is held at checkout, captured at 'ready' and released on
-- cancel. Plan: backend/docs/payments-guide.md.

-- Orders ------------------------------------------------------------------------------
-- Card is the only way to pay, so there's no payment method column: an
-- order's payments rows say how it went.
-- awaiting_payment comes before pending: the shop never sees an unpaid order.
ALTER TABLE orders
    ADD COLUMN payment_deadline TIMESTAMPTZ, -- when an unpaid order is dropped
    DROP CONSTRAINT orders_status_check,
    ADD CONSTRAINT orders_status_check CHECK (status IN (
        'awaiting_payment', 'pending', 'accepted', 'picking', 'ready',
        'out_for_delivery', 'completed', 'rejected', 'cancelled', 'no_show'
    )),
    ADD CONSTRAINT orders_payment_deadline_check CHECK (
        status <> 'awaiting_payment' OR payment_deadline IS NOT NULL
    );

-- ExpireUnpaidOrders: only unpaid orders are ever scanned.
CREATE INDEX orders_awaiting_payment_deadline_idx ON orders (payment_deadline)
    WHERE status = 'awaiting_payment' AND deleted = false;

-- "Card held £24.50", "Captured £22.10", "Refunded £5.00" in the order history.
ALTER TABLE order_status_events
    DROP CONSTRAINT order_status_events_kind_check,
    ADD CONSTRAINT order_status_events_kind_check
        CHECK (kind IN ('status', 'substitution', 'removed', 'code_override', 'payment'));

-- Vendor payment accounts ---------------------------------------------------------
-- Each vendor's own provider keys (bring your own). Old rows stay, because
-- held payments still need their keys to capture, void or refund.
CREATE TABLE vendor_payment_accounts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id          UUID NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    provider           TEXT NOT NULL,           -- 'stripe', 'dojo', 'fake'
    live               BOOLEAN NOT NULL,        -- live key or test key
    display_name       TEXT,                    -- business name from the provider
    secret_key_enc     BYTEA NOT NULL,          -- crypto.SecretBox
    secret_key_last4   TEXT NOT NULL,           -- shown instead of the key
    webhook_id         TEXT,                    -- our endpoint in their account
    webhook_secret_enc BYTEA,                   -- its signing secret, encrypted
    status             TEXT NOT NULL DEFAULT 'connecting'
        CONSTRAINT vendor_payment_accounts_status_check
            CHECK (status IN ('connecting', 'active', 'disabled', 'invalid')),
    last_error         TEXT,                    -- e.g. key revoked in Stripe
    created_by         UUID REFERENCES vendor_users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- An account can't take payments before its webhook exists, or nothing
    -- would ever tell us a card was held.
    CONSTRAINT vendor_payment_accounts_active_webhook_check CHECK (
        status <> 'active' OR webhook_secret_enc IS NOT NULL
    )
);

CREATE TRIGGER trg_vendor_payment_accounts_updated_at
    BEFORE UPDATE ON vendor_payment_accounts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- One account takes new checkouts.
CREATE UNIQUE INDEX vendor_payment_accounts_one_active_idx ON vendor_payment_accounts (vendor_id)
    WHERE status = 'active';
-- A vendor's account history, newest first.
CREATE INDEX vendor_payment_accounts_vendor_idx ON vendor_payment_accounts (vendor_id, created_at DESC);

-- Payments ----------------------------------------------------------------------------
-- One row per attempt. A failed or abandoned attempt stays; a retry makes a new row.
-- branch_id is copied from the order so payment lists page by branch without a join.
CREATE TABLE payments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id           UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    branch_id          UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    provider           TEXT NOT NULL,            -- 'stripe', 'dojo', 'fake'
    payment_account_id UUID NOT NULL REFERENCES vendor_payment_accounts(id) ON DELETE RESTRICT,
    provider_ref       TEXT,                     -- checkout session / payment intent
    payment_ref        TEXT,                     -- charge ref used for capture/void/refund
    redirect_url       TEXT,                     -- hosted page, reused while the session is open
    expires_at         TIMESTAMPTZ NOT NULL,     -- when the hosted session stops taking payment
    status             TEXT NOT NULL DEFAULT 'created'
        CONSTRAINT payments_status_check CHECK (status IN (
            'created', 'authorised', 'failed', 'expired',
            'captured', 'voided', 'partially_refunded', 'refunded', 'capture_failed'
        )),
    failure_code       TEXT,                     -- provider's decline code, for "try again"
    currency           TEXT NOT NULL DEFAULT 'gbp',
    -- Asked for while 'created'; what the provider actually held after that.
    amount_authorised  BIGINT NOT NULL CHECK (amount_authorised >= 0),
    amount_captured    BIGINT NOT NULL DEFAULT 0 CHECK (amount_captured >= 0),
    amount_refunded    BIGINT NOT NULL DEFAULT 0 CHECK (amount_refunded >= 0),
    authorised_at      TIMESTAMPTZ,
    captured_at        TIMESTAMPTZ,
    voided_at          TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payments_captured_check CHECK (amount_captured <= amount_authorised),
    CONSTRAINT payments_refunded_check CHECK (amount_refunded <= amount_captured)
);

CREATE TRIGGER trg_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Webhooks find their row by the provider's session id.
CREATE UNIQUE INDEX payments_provider_ref_uidx ON payments (provider, provider_ref);
-- At most one live (held or paid) payment per order.
CREATE UNIQUE INDEX payments_one_live_idx ON payments (order_id)
    WHERE status IN ('authorised', 'captured', 'partially_refunded', 'capture_failed');
-- At most one open hosted session per order, so two tabs can't both pay.
CREATE UNIQUE INDEX payments_one_open_session_idx ON payments (order_id)
    WHERE status = 'created';
-- Every attempt for an order, newest first (order pages, list batches).
CREATE INDEX payments_order_idx ON payments (order_id, created_at DESC);
-- Vendor payment list: keyset pagination by branch, newest first.
CREATE INDEX payments_branch_created_idx ON payments (branch_id, created_at DESC, id DESC);
-- Takings totals by the day money was taken.
CREATE INDEX payments_branch_captured_idx ON payments (branch_id, captured_at)
    WHERE captured_at IS NOT NULL;
-- Reconcile job: sessions still open long after they started.
CREATE INDEX payments_open_created_idx ON payments (created_at)
    WHERE status = 'created';
-- Payments that still need their account's keys (can a retired webhook go?).
CREATE INDEX payments_account_unsettled_idx ON payments (payment_account_id)
    WHERE status IN ('created', 'authorised', 'capture_failed');

-- Payment events ----------------------------------------------------------------------
-- Every webhook we accept, for dedupe and debugging.
CREATE TABLE payment_events (
    provider    TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    payment_id  UUID REFERENCES payments(id) ON DELETE RESTRICT,
    kind        TEXT NOT NULL,
    payload     JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, event_id)
);

CREATE INDEX payment_events_payment_idx ON payment_events (payment_id, received_at)
    WHERE payment_id IS NOT NULL;

-- Payment jobs ------------------------------------------------------------------------
-- Work for the provider. The worker leases a job (pushes next_attempt_at
-- forward) instead of holding a transaction open during the provider call;
-- a crashed worker's job comes back when the lease runs out. The job id is
-- the provider's idempotency key, so running it twice is safe.
CREATE TABLE payment_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id      UUID NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    kind            TEXT NOT NULL
        CONSTRAINT payment_jobs_kind_check CHECK (kind IN ('capture', 'void', 'refund')),
    amount          BIGINT,          -- capture and refund
    reason          TEXT,            -- refund
    requested_by    UUID REFERENCES vendor_users(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'queued'
        CONSTRAINT payment_jobs_status_check CHECK (status IN ('queued', 'done', 'failed', 'cancelled')),
    provider_ref    TEXT,            -- the provider's refund id
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT payment_jobs_amount_check CHECK (
        (kind = 'void' AND amount IS NULL) OR (kind <> 'void' AND amount > 0)
    )
);

CREATE INDEX payment_jobs_due_idx ON payment_jobs (next_attempt_at)
    WHERE status = 'queued';
CREATE INDEX payment_jobs_payment_idx ON payment_jobs (payment_id, created_at);
-- A double click or a retried status change can't queue a second capture or void.
CREATE UNIQUE INDEX payment_jobs_one_queued_idx ON payment_jobs (payment_id, kind)
    WHERE status = 'queued' AND kind IN ('capture', 'void');
-- Alerts: jobs that gave up.
CREATE INDEX payment_jobs_failed_idx ON payment_jobs (created_at DESC, id DESC)
    WHERE status = 'failed';

-- +goose Down
DROP TABLE IF EXISTS payment_jobs;
DROP TABLE IF EXISTS payment_events;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS vendor_payment_accounts;
ALTER TABLE order_status_events
    DROP CONSTRAINT order_status_events_kind_check,
    ADD CONSTRAINT order_status_events_kind_check
        CHECK (kind IN ('status', 'substitution', 'removed', 'code_override'));
DROP INDEX IF EXISTS orders_awaiting_payment_deadline_idx;
-- Fails if any order is still awaiting_payment; cancel those first.
ALTER TABLE orders
    DROP CONSTRAINT orders_payment_deadline_check,
    DROP CONSTRAINT orders_status_check,
    ADD CONSTRAINT orders_status_check CHECK (status IN (
        'pending', 'accepted', 'picking', 'ready', 'out_for_delivery',
        'completed', 'rejected', 'cancelled', 'no_show'
    )),
    DROP COLUMN IF EXISTS payment_deadline;
