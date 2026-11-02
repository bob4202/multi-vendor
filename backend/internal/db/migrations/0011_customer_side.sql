-- +goose Up

-- The customer side: customer accounts (email-code login), same-day
-- collection with a collection code, substitutions the customer must
-- approve, the collection window, push subscriptions, and a notification
-- outbox. See README §8.

-- Branch settings ----------------------------------------------------------------

-- Collection orders close this many minutes before the day's last closing
-- time, so the shop has time to pick and the customer time to collect.
ALTER TABLE branch_info
    ADD COLUMN last_order_minutes INTEGER NOT NULL DEFAULT 90
        CHECK (last_order_minutes BETWEEN 0 AND 480);

-- Where this branch's storefront lives, for links in emails. NULL = emails
-- go out without links.
ALTER TABLE branches ADD COLUMN storefront_url TEXT;

-- Customer accounts ----------------------------------------------------------------
-- Not the inventory-synced `customers` table: these are people who sign in
-- to a storefront. One account per email per vendor, so a vendor's customer
-- list is theirs alone.

CREATE TABLE customer_accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id     UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    email         TEXT NOT NULL CHECK (email = lower(email) AND email <> ''),
    name          TEXT,
    phone         TEXT, -- E.164
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vendor_id, email)
);

CREATE TRIGGER trg_customer_accounts_updated_at
    BEFORE UPDATE ON customer_accounts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- One-time login codes. Only a hash is stored. A code dies after expiry,
-- after too many wrong tries, or once used.
CREATE TABLE customer_login_codes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id   UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    code_hash   BYTEA NOT NULL,
    attempts    INTEGER NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX customer_login_codes_lookup_idx ON customer_login_codes (vendor_id, email, created_at DESC);

-- Same shape as vendor_sessions.
CREATE TABLE customer_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id         UUID NOT NULL REFERENCES customer_accounts(id) ON DELETE CASCADE,
    token_hash          BYTEA NOT NULL UNIQUE,
    user_agent          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX customer_sessions_customer_idx ON customer_sessions (customer_id);

-- Orders ----------------------------------------------------------------------------

ALTER TABLE orders
    ADD COLUMN customer_account_id      UUID REFERENCES customer_accounts(id) ON DELETE SET NULL,
    ADD COLUMN allow_substitutions      BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN terms_version            TEXT,
    ADD COLUMN terms_accepted_at        TIMESTAMPTZ,
    -- Shown to the customer once the shop accepts a collection order; staff
    -- check it at hand-over. Wrong tries are counted so it can't be guessed.
    ADD COLUMN collection_code          TEXT CHECK (collection_code ~ '^[0-9]{6}$'),
    ADD COLUMN collection_code_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN ready_at                 TIMESTAMPTZ,
    -- Collect by this time; COLLECTION_GRACE_MINUTES later it's a no-show.
    ADD COLUMN collect_by               TIMESTAMPTZ;

CREATE INDEX orders_customer_account_idx ON orders (customer_account_id, created_at DESC)
    WHERE customer_account_id IS NOT NULL;

-- Linking guest orders to an account on sign-in matches on email.
CREATE INDEX orders_customer_email_idx ON orders (lower(customer_email))
    WHERE customer_account_id IS NULL;

CREATE INDEX orders_branch_collection_code_idx ON orders (branch_id, collection_code)
    WHERE collection_code IS NOT NULL AND status IN ('accepted', 'picking', 'ready');

-- ExpireUncollectedOrders scans only ready collection orders.
CREATE INDEX orders_ready_collect_by_idx ON orders (collect_by)
    WHERE status = 'ready' AND collect_by IS NOT NULL;

-- Order lines can now change after the order is placed: an item can be
-- removed or swapped for a substitute. line_total stays what was originally
-- ordered; charged_total is what the line costs now, and the order's
-- subtotal is the sum of charged_total.
ALTER TABLE order_items
    ADD COLUMN status TEXT NOT NULL DEFAULT 'ok'
        CONSTRAINT order_items_status_check CHECK (status IN ('ok', 'awaiting_customer', 'substituted', 'removed')),
    ADD COLUMN charged_total BIGINT;

UPDATE order_items SET charged_total = line_total;

ALTER TABLE order_items
    ALTER COLUMN charged_total SET NOT NULL,
    ADD CONSTRAINT order_items_charged_total_check CHECK (charged_total >= 0);

-- A substitute the shop offers for an unavailable line. The customer pays
-- line_total = min(original line_total, unit_price * quantity), so the shop
-- covers any increase. At most one live proposal per line.
CREATE TABLE order_substitutions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id           UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    order_item_id      UUID NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
    product_id         UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    product_name       TEXT NOT NULL,
    product_plu        TEXT,
    unit_price         BIGINT NOT NULL CHECK (unit_price >= 0), -- the substitute's own price
    quantity           INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    line_total         BIGINT NOT NULL CHECK (line_total >= 0), -- what the customer pays for it
    status             TEXT NOT NULL DEFAULT 'proposed'
        CONSTRAINT order_substitutions_status_check CHECK (status IN ('proposed', 'accepted', 'declined', 'expired', 'withdrawn')),
    expires_at         TIMESTAMPTZ NOT NULL,
    responded_at       TIMESTAMPTZ,
    created_by_user_id UUID REFERENCES vendor_users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX order_substitutions_one_live_idx ON order_substitutions (order_item_id)
    WHERE status = 'proposed';
CREATE INDEX order_substitutions_order_idx ON order_substitutions (order_id, created_at);
CREATE INDEX order_substitutions_expiry_idx ON order_substitutions (expires_at)
    WHERE status = 'proposed';

-- History entries that aren't status changes (a substitution, a removed
-- line, a collection-code override) keep from/to as the current status.
ALTER TABLE order_status_events
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'status'
        CONSTRAINT order_status_events_kind_check CHECK (kind IN ('status', 'substitution', 'removed', 'code_override'));

-- Push subscriptions -----------------------------------------------------------------
-- A browser's Web Push endpoint. Linked to a customer account (every order
-- they place) and/or to individual guest orders.

CREATE TABLE push_subscriptions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint            TEXT NOT NULL UNIQUE,
    p256dh              TEXT NOT NULL,
    auth                TEXT NOT NULL,
    customer_account_id UUID REFERENCES customer_accounts(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at     TIMESTAMPTZ
);

CREATE INDEX push_subscriptions_customer_idx ON push_subscriptions (customer_account_id)
    WHERE customer_account_id IS NOT NULL;

CREATE TABLE order_push_subscriptions (
    order_id        UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    subscription_id UUID NOT NULL REFERENCES push_subscriptions(id) ON DELETE CASCADE,
    PRIMARY KEY (order_id, subscription_id)
);

-- Notification outbox ------------------------------------------------------------------
-- Rows are written in the same transaction as the change they announce, and
-- one worker sends them (email + push), so a notification is never lost or
-- sent twice by several API instances.

CREATE TABLE notification_outbox (
    id              BIGSERIAL PRIMARY KEY,
    order_id        UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL, -- 'status_<status>', 'substitution_proposed', 'item_removed'
    detail          JSONB NOT NULL DEFAULT '{}',
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notification_outbox_due_idx ON notification_outbox (next_attempt_at)
    WHERE sent_at IS NULL;

-- Triggers ----------------------------------------------------------------------------
-- Every order change is broadcast (NOTIFY is delivered on commit), so live
-- screens update no matter which code path or API instance made it. Status
-- changes also queue a notification here, which covers the background jobs
-- (timeouts, no-shows) without each one remembering to.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION orders_after_write()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('order_events', json_build_object(
        'order_id', NEW.id,
        'branch_id', NEW.branch_id,
        'reference', NEW.reference,
        'status', NEW.status,
        'old_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status END,
        'op', lower(TG_OP)
    )::text);

    IF TG_OP = 'UPDATE' AND NEW.status IS DISTINCT FROM OLD.status THEN
        INSERT INTO notification_outbox (order_id, kind)
        VALUES (NEW.id, 'status_' || NEW.status);
        PERFORM pg_notify('notification_outbox', '');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_orders_after_write
    AFTER INSERT OR UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION orders_after_write();

-- +goose Down
DROP TRIGGER IF EXISTS trg_orders_after_write ON orders;
DROP FUNCTION IF EXISTS orders_after_write();
DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS order_push_subscriptions;
DROP TABLE IF EXISTS push_subscriptions;
ALTER TABLE order_status_events DROP COLUMN IF EXISTS kind;
DROP TABLE IF EXISTS order_substitutions;
ALTER TABLE order_items
    DROP CONSTRAINT IF EXISTS order_items_charged_total_check,
    DROP COLUMN IF EXISTS charged_total,
    DROP COLUMN IF EXISTS status;
DROP INDEX IF EXISTS orders_ready_collect_by_idx;
DROP INDEX IF EXISTS orders_branch_collection_code_idx;
DROP INDEX IF EXISTS orders_customer_email_idx;
DROP INDEX IF EXISTS orders_customer_account_idx;
ALTER TABLE orders
    DROP COLUMN IF EXISTS collect_by,
    DROP COLUMN IF EXISTS ready_at,
    DROP COLUMN IF EXISTS collection_code_attempts,
    DROP COLUMN IF EXISTS collection_code,
    DROP COLUMN IF EXISTS terms_accepted_at,
    DROP COLUMN IF EXISTS terms_version,
    DROP COLUMN IF EXISTS allow_substitutions,
    DROP COLUMN IF EXISTS customer_account_id;
DROP TABLE IF EXISTS customer_sessions;
DROP TABLE IF EXISTS customer_login_codes;
DROP TABLE IF EXISTS customer_accounts;
ALTER TABLE branches DROP COLUMN IF EXISTS storefront_url;
ALTER TABLE branch_info DROP COLUMN IF EXISTS last_order_minutes;
