-- Card payments: vendor accounts, payment attempts, webhook events and the
-- capture/void/refund job queue. Never call a provider while a transaction
-- from here is open: write our row, commit, call, then record the result.

-- Vendor payment accounts ---------------------------------------------------------
-- Summary queries never select the encrypted columns; only the *Credentials
-- queries do, for payment.Service to decrypt right before a provider call.

-- name: CreateVendorPaymentAccount :one
-- Step 3 of connecting: the row exists as 'connecting' so its id can go in
-- the webhook URL before the webhook is registered.
INSERT INTO vendor_payment_accounts (
    vendor_id, provider, live, display_name, secret_key_enc, secret_key_last4, created_by
) VALUES (
    sqlc.arg(vendor_id), sqlc.arg(provider), sqlc.arg(live), sqlc.narg(display_name),
    sqlc.arg(secret_key_enc), sqlc.arg(secret_key_last4), sqlc.narg(created_by)
)
RETURNING id;

-- name: SetVendorPaymentAccountWebhook :execrows
-- Step 4: Stripe returns the signing secret only once, so store it at once.
UPDATE vendor_payment_accounts
SET webhook_id = sqlc.arg(webhook_id),
    webhook_secret_enc = sqlc.arg(webhook_secret_enc)
WHERE id = sqlc.arg(id)
  AND status = 'connecting';

-- name: RetireVendorPaymentAccounts :execrows
-- Step 5a, same transaction as ActivateVendorPaymentAccount (and the whole
-- of DELETE /v1/vendor/payments with keep_id = NULL): the current account
-- stops taking new checkouts. Its held payments still finish with its keys.
UPDATE vendor_payment_accounts
SET status = 'disabled'
WHERE vendor_id = sqlc.arg(vendor_id)
  AND status IN ('active', 'invalid')
  AND id IS DISTINCT FROM sqlc.narg(keep_id)::uuid;

-- name: ActivateVendorPaymentAccount :one
-- Step 5b. No row = it wasn't 'connecting' or has no webhook yet.
UPDATE vendor_payment_accounts
SET status = 'active', last_error = NULL
WHERE id = sqlc.arg(id)
  AND status = 'connecting'
  AND webhook_secret_enc IS NOT NULL
RETURNING id, vendor_id, provider, live, display_name, secret_key_last4, status, last_error, created_at;

-- name: MarkVendorPaymentAccountInvalid :execrows
-- A failed connect, or the provider rejected the key later (revoked).
-- Hides card at checkout until the owner reconnects. A disabled account
-- stays disabled: it only finishes old payments.
UPDATE vendor_payment_accounts
SET status = 'invalid', last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id)
  AND status IN ('connecting', 'active');

-- name: GetCurrentVendorPaymentAccount :one
-- GET /v1/vendor/payments and the admin read-only view: the active account,
-- or else the newest invalid one so the owner sees why card is off.
-- No row = card isn't set up (respond null).
SELECT id, vendor_id, provider, live, display_name, secret_key_last4, status, last_error, created_at
FROM vendor_payment_accounts
WHERE vendor_id = sqlc.arg(vendor_id)
  AND status IN ('active', 'invalid')
ORDER BY (status = 'active') DESC, created_at DESC
LIMIT 1;

-- name: ListVendorPaymentAccounts :many
-- Every account a vendor has connected, newest first. A vendor has a
-- handful at most, so this isn't paginated.
SELECT id, vendor_id, provider, live, display_name, secret_key_last4, status, last_error, created_at,
    EXISTS (SELECT 1 FROM payments p
      WHERE p.payment_account_id = a.id
        AND p.status IN ('created', 'authorised', 'capture_failed')) AS has_unsettled_payments
FROM vendor_payment_accounts a
WHERE a.vendor_id = sqlc.arg(vendor_id)
ORDER BY a.created_at DESC;

-- name: GetBranchCheckoutAccount :one
-- Checkout and GET /v1/store/branch: the account that takes this branch's
-- new payments, with its keys. No row = card is off, so no orders can be
-- placed (card is the only payment method).
SELECT a.id, a.provider, a.live, a.secret_key_enc, a.webhook_secret_enc
FROM branches b
JOIN vendor_payment_accounts a ON a.vendor_id = b.vendor_id AND a.status = 'active'
WHERE b.id = sqlc.arg(branch_id);

-- name: GetVendorPaymentAccountCredentials :one
-- Capture, void, refund and webhooks: always the account the payment was
-- made with, whatever its status now. Webhooks also check the provider in
-- the URL matches.
SELECT id, vendor_id, provider, live, status, secret_key_enc, webhook_secret_enc
FROM vendor_payment_accounts
WHERE id = sqlc.arg(id);

-- name: ListRetiredPaymentAccountWebhooks :many
-- Cleanup job: disabled or invalid accounts whose webhook can now be
-- deleted from the provider, because none of their payments can still
-- change. Call DeleteWebhook, then ClearVendorPaymentAccountWebhook.
SELECT a.id, a.provider, a.live, a.secret_key_enc, a.webhook_id
FROM vendor_payment_accounts a
WHERE a.status IN ('disabled', 'invalid')
  AND a.webhook_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM payments p
    WHERE p.payment_account_id = a.id
      AND p.status IN ('created', 'authorised', 'capture_failed'))
  AND NOT EXISTS (SELECT 1 FROM payment_jobs j
    JOIN payments p ON p.id = j.payment_id
    WHERE p.payment_account_id = a.id AND j.status = 'queued')
ORDER BY a.updated_at
LIMIT 20;

-- name: ClearVendorPaymentAccountWebhook :exec
UPDATE vendor_payment_accounts
SET webhook_id = NULL
WHERE id = sqlc.arg(id)
  AND status IN ('disabled', 'invalid');

-- name: ListInvalidPaymentAccounts :many
-- Admin "needs attention": vendors whose card checkout is off because the
-- provider rejected their key. Only the newest invalid row per vendor, and
-- only if they haven't reconnected since.
SELECT DISTINCT ON (a.vendor_id)
    a.id, a.vendor_id, v.name AS vendor_name, a.provider, a.last_error, a.updated_at
FROM vendor_payment_accounts a
JOIN vendors v ON v.id = a.vendor_id
WHERE a.status = 'invalid'
  AND NOT EXISTS (SELECT 1 FROM vendor_payment_accounts x
    WHERE x.vendor_id = a.vendor_id AND x.status = 'active')
ORDER BY a.vendor_id, a.updated_at DESC;

-- Orders (payment side) -------------------------------------------------------------
-- InsertOrder (order.sql) sets status 'awaiting_payment' and payment_deadline.

-- name: MarkOrderPaid :one
-- Webhook 'authorised', same transaction as MarkPaymentAuthorised: the shop
-- sees the order now, and its accept countdown starts now. No row = the
-- order was already dropped (paid too late): queue a void.
UPDATE orders
SET status = 'pending',
    accept_deadline = now() + make_interval(mins => sqlc.arg(accept_minutes)::int)
WHERE id = sqlc.arg(id)
  AND status = 'awaiting_payment'
  AND deleted = false
RETURNING id, branch_id, reference, accept_deadline;

-- name: ExpireUnpaidOrders :many
-- Card orders nobody finished paying for. Open sessions are left to expire
-- at the provider; a payment that still arrives is voided by the webhook.
UPDATE orders
SET status = 'cancelled',
    rejection_reason = 'payment_not_completed',
    cancelled_at = now()
WHERE status = 'awaiting_payment'
  AND payment_deadline <= now()
  AND deleted = false
RETURNING id, branch_id, reference;

-- Payments -------------------------------------------------------------------------
-- Status changes are one query per event, each guarded by the statuses it
-- may move from. No row back = the event is stale or out of order: ignore
-- it (the webhook still answers 200). Run them with the payment locked
-- (LockPayment) in the same transaction as InsertPaymentEvent.

-- name: CreatePayment :one
-- The amount and branch come from the order itself, so they can't drift
-- from what the customer is paying for. No row = the order isn't awaiting
-- payment. A unique violation on payments_one_open_session_idx = another
-- session is still open: use GetOpenPaymentSession.
INSERT INTO payments (order_id, branch_id, provider, payment_account_id, amount_authorised, currency, expires_at)
SELECT o.id, o.branch_id, sqlc.arg(provider), sqlc.arg(payment_account_id), o.total, 'gbp', sqlc.arg(expires_at)
FROM orders o
WHERE o.id = sqlc.arg(order_id)
  AND o.status = 'awaiting_payment'
  AND o.deleted = false
RETURNING *;

-- name: SetPaymentSession :one
-- After the provider created the hosted page.
UPDATE payments
SET provider_ref = sqlc.arg(provider_ref),
    redirect_url = sqlc.arg(redirect_url)
WHERE id = sqlc.arg(id)
  AND status = 'created'
RETURNING *;

-- name: GetOpenPaymentSession :one
-- Reuse the open session instead of starting a second one (double click,
-- back button, two tabs). The minute of slack avoids sending the customer
-- to a page that expires as it loads.
SELECT *
FROM payments
WHERE order_id = sqlc.arg(order_id)
  AND status = 'created'
  AND redirect_url IS NOT NULL
  AND expires_at > now() + interval '1 minute';

-- name: ExpireStalePaymentSessions :execrows
-- Before a retry: closes this order's session if it has run out (or never
-- got a redirect URL), so CreatePayment can make a new one.
UPDATE payments
SET status = 'expired'
WHERE order_id = sqlc.arg(order_id)
  AND status = 'created'
  AND (redirect_url IS NULL OR expires_at <= now() + interval '1 minute');

-- name: GetPayment :one
SELECT * FROM payments WHERE id = sqlc.arg(id);

-- name: LockPayment :one
-- Serialises everything that changes one payment: webhooks, job results.
SELECT * FROM payments WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: LockPaymentByProviderRef :one
-- For webhooks that only carry the provider's session id.
SELECT * FROM payments
WHERE provider = sqlc.arg(provider) AND provider_ref = sqlc.arg(provider_ref)
FOR UPDATE;

-- name: GetLatestPaymentForOrder :one
-- The "payment" block on one order. Prefers the live payment, then the newest attempt.
SELECT *
FROM payments
WHERE order_id = sqlc.arg(order_id)
ORDER BY (status IN ('authorised', 'captured', 'partially_refunded', 'refunded', 'capture_failed', 'voided')) DESC,
    created_at DESC
LIMIT 1;

-- name: ListLatestPaymentsByOrderIDs :many
-- The "payment" block for a page of orders in one query, like
-- ListItemsByOrderIDs. Same choice per order as GetLatestPaymentForOrder.
SELECT DISTINCT ON (order_id)
    id, order_id, provider, status, failure_code, currency,
    amount_authorised, amount_captured, amount_refunded,
    authorised_at, captured_at, voided_at, expires_at, created_at
FROM payments
WHERE order_id = ANY (sqlc.arg(order_ids)::uuid[])
ORDER BY order_id,
    (status IN ('authorised', 'captured', 'partially_refunded', 'refunded', 'capture_failed', 'voided')) DESC,
    created_at DESC;

-- name: ListPaymentsForOrder :many
-- Every attempt, newest first: vendor order detail and debugging.
SELECT *
FROM payments
WHERE order_id = sqlc.arg(order_id)
ORDER BY created_at DESC;

-- name: MarkPaymentAuthorised :one
-- The card is held. Also accepted from failed/expired: if money was held
-- anyway, we must record it so it can be voided or captured. A unique
-- violation on payments_one_live_idx = the order is already paid by
-- another attempt: queue a void for this one.
UPDATE payments
SET status = 'authorised',
    payment_ref = sqlc.arg(payment_ref),
    amount_authorised = sqlc.arg(amount),
    authorised_at = now(),
    failure_code = NULL
WHERE id = sqlc.arg(id)
  AND status IN ('created', 'failed', 'expired')
RETURNING *;

-- name: SetPaymentRef :execrows
-- Stripe's checkout.session.completed: the PaymentIntent id, before the
-- hold is confirmed.
UPDATE payments
SET payment_ref = sqlc.arg(payment_ref)
WHERE id = sqlc.arg(id)
  AND payment_ref IS NULL;

-- name: MarkPaymentFailed :one
UPDATE payments
SET status = 'failed', failure_code = sqlc.narg(failure_code)
WHERE id = sqlc.arg(id)
  AND status = 'created'
RETURNING *;

-- name: MarkPaymentExpired :one
UPDATE payments
SET status = 'expired'
WHERE id = sqlc.arg(id)
  AND status = 'created'
RETURNING *;

-- name: MarkPaymentCaptured :one
-- From the capture job and from the provider's confirmation webhook; both
-- set the same amount, so whichever comes second changes nothing.
UPDATE payments
SET status = 'captured',
    amount_captured = sqlc.arg(amount),
    captured_at = coalesce(captured_at, now())
WHERE id = sqlc.arg(id)
  AND status IN ('authorised', 'capture_failed', 'captured')
RETURNING *;

-- name: MarkPaymentCaptureFailed :one
-- The capture job gave up (hold expired, card closed, out of retries). The
-- order can't be handed over while the payment is in this state.
UPDATE payments
SET status = 'capture_failed', failure_code = sqlc.narg(failure_code)
WHERE id = sqlc.arg(id)
  AND status = 'authorised'
RETURNING *;

-- name: MarkPaymentVoided :one
UPDATE payments
SET status = 'voided', voided_at = coalesce(voided_at, now())
WHERE id = sqlc.arg(id)
  AND status IN ('created', 'authorised', 'capture_failed')
RETURNING *;

-- name: SetPaymentRefundedTotal :one
-- Takes the running total refunded, not the amount of one refund: Stripe's
-- charge.refunded reports the total, and GREATEST makes the job and the
-- webhook reporting the same refund count it once.
UPDATE payments
SET amount_refunded = GREATEST(amount_refunded, sqlc.arg(refunded_total)::bigint),
    status = CASE WHEN GREATEST(amount_refunded, sqlc.arg(refunded_total)::bigint) >= amount_captured
        THEN 'refunded' ELSE 'partially_refunded' END
WHERE id = sqlc.arg(id)
  AND status IN ('captured', 'partially_refunded', 'refunded')
  AND sqlc.arg(refunded_total)::bigint <= amount_captured
RETURNING *;

-- name: GetRefundableAmount :one
-- POST …/refunds: what's left to refund once refunds already queued are
-- counted. Run it with the payment locked so two refunds can't both pass.
SELECT p.amount_captured - p.amount_refunded - coalesce((
    SELECT sum(j.amount) FROM payment_jobs j
    WHERE j.payment_id = p.id AND j.kind = 'refund' AND j.status = 'queued'
), 0)::bigint AS refundable
FROM payments p
WHERE p.id = sqlc.arg(id)
  AND p.status IN ('captured', 'partially_refunded');

-- name: ListStalePaymentSessions :many
-- Reconcile job: sessions still 'created' long after they started, in case
-- a webhook was lost. Ask the provider for each one's status.
SELECT p.id, p.order_id, p.provider, p.payment_account_id, p.provider_ref
FROM payments p
WHERE p.status = 'created'
  AND p.created_at <= now() - make_interval(mins => sqlc.arg(older_than_minutes)::int)
  AND p.provider_ref IS NOT NULL
ORDER BY p.created_at
LIMIT 50;

-- Vendor payment list ---------------------------------------------------------------
-- Keyset pagination (newest first): pass the created_at and id of the last
-- row you showed as the cursor, or NULLs for the first page. Ask for
-- page_limit + 1 rows; an extra row means there's a next page. Unlike
-- OFFSET, a page costs the same however deep it is, and rows arriving
-- meanwhile don't shift what the next page shows. Every filter is optional
-- (NULL = no filter).

-- name: ListBranchPayments :many
SELECT
    p.id, p.order_id, p.provider, p.status, p.failure_code, p.currency,
    p.amount_authorised, p.amount_captured, p.amount_refunded,
    p.authorised_at, p.captured_at, p.voided_at, p.created_at,
    o.reference AS order_reference,
    o.customer_name,
    o.status AS order_status
FROM payments p
JOIN orders o ON o.id = p.order_id
WHERE p.branch_id = sqlc.arg(branch_id)
  AND (sqlc.narg(statuses)::text[] IS NULL OR p.status = ANY (sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR p.created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR p.created_at < sqlc.narg(created_to))
  AND (sqlc.narg(reference)::text IS NULL OR o.reference = upper(sqlc.narg(reference)))
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR (p.created_at, p.id) < (sqlc.narg(cursor_created_at), sqlc.narg(cursor_id)::uuid))
ORDER BY p.created_at DESC, p.id DESC
LIMIT sqlc.arg(page_limit)::int;

-- name: SummariseBranchTakings :one
-- Totals for a period by when the money was taken: [captured_from, captured_to).
SELECT
    count(*)::int AS captured_count,
    coalesce(sum(amount_captured), 0)::bigint AS captured_total,
    coalesce(sum(amount_refunded), 0)::bigint AS refunded_total,
    coalesce(sum(amount_captured - amount_refunded), 0)::bigint AS net_total
FROM payments
WHERE branch_id = sqlc.arg(branch_id)
  AND captured_at >= sqlc.arg(captured_from)
  AND captured_at < sqlc.arg(captured_to);

-- name: CountCaptureFailedPayments :one
-- Vendor warning badge: ready orders whose card couldn't be charged.
SELECT count(*)::int
FROM payments p
JOIN orders o ON o.id = p.order_id
WHERE p.branch_id = sqlc.arg(branch_id)
  AND p.status = 'capture_failed'
  AND o.status IN ('ready', 'out_for_delivery');

-- Payment events --------------------------------------------------------------------

-- name: InsertPaymentEvent :execrows
-- 0 rows = already handled: answer 200 and stop. Run it in the same
-- transaction as the status change it causes, so a failure rolls both
-- back and the provider's retry is processed instead of skipped as a
-- duplicate.
INSERT INTO payment_events (provider, event_id, payment_id, kind, payload)
VALUES (sqlc.arg(provider), sqlc.arg(event_id), sqlc.narg(payment_id), sqlc.arg(kind), sqlc.arg(payload))
ON CONFLICT (provider, event_id) DO NOTHING;

-- name: ListPaymentEvents :many
SELECT provider, event_id, kind, payload, received_at
FROM payment_events
WHERE payment_id = sqlc.arg(payment_id)
ORDER BY received_at;

-- Payment jobs ----------------------------------------------------------------------

-- name: EnqueuePaymentJob :execrows
-- Same transaction as the order status change that causes it. 0 rows = the
-- same capture or void is already queued, which is fine.
INSERT INTO payment_jobs (payment_id, kind, amount, reason, requested_by)
VALUES (sqlc.arg(payment_id), sqlc.arg(kind), sqlc.narg(amount), sqlc.narg(reason), sqlc.narg(requested_by))
ON CONFLICT (payment_id, kind) WHERE status = 'queued' AND kind IN ('capture', 'void')
DO NOTHING;

-- name: ClaimDuePaymentJobs :many
-- Leases due jobs for lease_seconds and returns them; commit straight away,
-- then call the provider outside any transaction. Another worker skips
-- them until the lease runs out, so a crashed worker's jobs come back.
-- Oldest first, so a refund never overtakes the capture it depends on.
UPDATE payment_jobs j
SET attempts = j.attempts + 1,
    next_attempt_at = now() + make_interval(secs => sqlc.arg(lease_seconds)::int)
FROM payments p, vendor_payment_accounts a
WHERE j.id IN (
    SELECT id FROM payment_jobs
    WHERE status = 'queued' AND next_attempt_at <= now()
    ORDER BY created_at
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE SKIP LOCKED
)
  AND p.id = j.payment_id
  AND a.id = p.payment_account_id
RETURNING j.id, j.payment_id, j.kind, j.amount, j.reason, j.attempts,
    p.order_id, p.payment_ref, p.status AS payment_status, p.amount_captured, p.amount_refunded,
    a.id AS account_id, a.provider, a.live, a.secret_key_enc;

-- name: MarkPaymentJobDone :exec
UPDATE payment_jobs
SET status = 'done', provider_ref = sqlc.narg(provider_ref), last_error = NULL, finished_at = now()
WHERE id = sqlc.arg(id) AND status = 'queued';

-- name: MarkPaymentJobRetry :exec
-- attempts was already counted when the job was claimed.
UPDATE payment_jobs
SET last_error = sqlc.arg(last_error), next_attempt_at = sqlc.arg(next_attempt_at)
WHERE id = sqlc.arg(id) AND status = 'queued';

-- name: MarkPaymentJobFailed :exec
-- Out of retries, or the provider said it can never work (e.g. hold expired).
UPDATE payment_jobs
SET status = 'failed', last_error = sqlc.arg(last_error), finished_at = now()
WHERE id = sqlc.arg(id) AND status = 'queued';

-- name: CancelQueuedPaymentJobs :execrows
-- e.g. a capture still queued when the order is cancelled: drop it and
-- queue a void instead.
UPDATE payment_jobs
SET status = 'cancelled', finished_at = now()
WHERE payment_id = sqlc.arg(payment_id)
  AND kind = sqlc.arg(kind)
  AND status = 'queued';

-- name: ListPaymentJobsForPayment :many
-- Vendor order detail: refunds and their state ("Refund £5.00 pending").
SELECT j.id, j.kind, j.amount, j.reason, j.status, j.attempts, j.last_error,
    j.created_at, j.finished_at, u.name AS requested_by_name
FROM payment_jobs j
LEFT JOIN vendor_users u ON u.id = j.requested_by
WHERE j.payment_id = sqlc.arg(payment_id)
ORDER BY j.created_at;

-- name: ListFailedPaymentJobs :many
-- Admin alerts, newest first, keyset paginated like ListBranchPayments.
SELECT j.id, j.payment_id, j.kind, j.amount, j.attempts, j.last_error, j.created_at,
    o.id AS order_id, o.reference AS order_reference, o.branch_id, b.name AS branch_name
FROM payment_jobs j
JOIN payments p ON p.id = j.payment_id
JOIN orders o ON o.id = p.order_id
JOIN branches b ON b.id = o.branch_id
WHERE j.status = 'failed'
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR (j.created_at, j.id) < (sqlc.narg(cursor_created_at), sqlc.narg(cursor_id)::uuid))
ORDER BY j.created_at DESC, j.id DESC
LIMIT sqlc.arg(page_limit)::int;
