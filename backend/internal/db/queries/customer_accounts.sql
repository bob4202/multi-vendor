-- Storefront customer accounts: email-code login, sessions, profile, their
-- orders and push subscriptions. Not the inventory-synced `customers` table.

-- name: CreateCustomerLoginCode :one
INSERT INTO customer_login_codes (vendor_id, email, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: LatestCustomerLoginCode :one
SELECT * FROM customer_login_codes
WHERE vendor_id = $1 AND email = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: CountCustomerLoginCodesSince :one
SELECT count(*) FROM customer_login_codes
WHERE vendor_id = $1 AND email = $2 AND created_at > sqlc.arg(since)::timestamptz;

-- name: RecordCustomerLoginCodeAttempt :exec
UPDATE customer_login_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeCustomerLoginCode :execrows
-- Only one request can use a code, even if two arrive together.
UPDATE customer_login_codes SET consumed_at = now()
WHERE id = $1 AND consumed_at IS NULL;

-- name: UpsertCustomerAccount :one
INSERT INTO customer_accounts (vendor_id, email, last_login_at)
VALUES ($1, $2, now())
ON CONFLICT (vendor_id, email) DO UPDATE SET last_login_at = now()
RETURNING *;

-- name: GetCustomerAccount :one
SELECT * FROM customer_accounts WHERE id = $1;

-- name: UpdateCustomerAccount :one
UPDATE customer_accounts
SET name = sqlc.narg(name), phone = sqlc.narg(phone)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: LinkGuestOrdersToCustomer :execrows
-- Once someone proves they own an email, the vendor's guest orders placed
-- with it become theirs.
UPDATE orders o
SET customer_account_id = sqlc.arg(customer_id)
FROM branches b
WHERE b.id = o.branch_id
  AND b.vendor_id = sqlc.arg(vendor_id)
  AND o.customer_account_id IS NULL
  AND lower(o.customer_email) = sqlc.arg(email)::text
  AND o.deleted = false;

-- name: CreateCustomerSession :one
INSERT INTO customer_sessions (customer_id, token_hash, user_agent, expires_at, absolute_expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActiveCustomerSession :one
SELECT sqlc.embed(s), sqlc.embed(c)
FROM customer_sessions s
JOIN customer_accounts c ON c.id = s.customer_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now();

-- name: TouchCustomerSession :exec
UPDATE customer_sessions
SET last_seen_at = now(),
    expires_at = LEAST(sqlc.arg(expires_at)::timestamptz, absolute_expires_at)
WHERE id = sqlc.arg(id);

-- name: RevokeCustomerSession :exec
UPDATE customer_sessions SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: CountCustomerOrders :one
SELECT count(*) FROM orders
WHERE customer_account_id = $1 AND deleted = false;

-- name: ListCustomerOrders :many
SELECT
    o.id, o.reference, o.status, o.fulfilment_type, o.fulfilment_date, o.total, o.created_at,
    b.name AS branch_name,
    (SELECT coalesce(sum(i.quantity), 0)::int FROM order_items i
      WHERE i.order_id = o.id AND i.status <> 'removed') AS item_count,
    (o.status = 'awaiting_payment' OR EXISTS (SELECT 1 FROM order_items i
      WHERE i.order_id = o.id AND i.status = 'awaiting_customer'))::bool AS needs_action
FROM orders o
JOIN branches b ON b.id = o.branch_id
WHERE o.customer_account_id = sqlc.arg(customer_id)
  AND o.deleted = false
ORDER BY o.created_at DESC, o.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: UpsertPushSubscription :one
-- A browser re-subscribing keeps one row per endpoint. Signing in later
-- attaches the account; signing out never detaches it here.
INSERT INTO push_subscriptions (endpoint, p256dh, auth, customer_account_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint) DO UPDATE SET
    p256dh = EXCLUDED.p256dh,
    auth = EXCLUDED.auth,
    customer_account_id = coalesce(EXCLUDED.customer_account_id, push_subscriptions.customer_account_id)
RETURNING *;

-- name: LinkPushSubscriptionToOrder :exec
INSERT INTO order_push_subscriptions (order_id, subscription_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE endpoint = $1;

-- name: ListPushSubscriptionsForOrder :many
-- Everything that should hear about this order: browsers subscribed to it
-- directly, plus every browser of the account that placed it.
SELECT s.* FROM push_subscriptions s
WHERE s.id IN (SELECT ops.subscription_id FROM order_push_subscriptions ops WHERE ops.order_id = sqlc.arg(order_id))
   OR (sqlc.narg(customer_id)::uuid IS NOT NULL AND s.customer_account_id = sqlc.narg(customer_id)::uuid);

-- name: MarkPushSubscriptionSuccess :exec
UPDATE push_subscriptions SET last_success_at = now() WHERE id = $1;

-- name: DeletePushSubscriptionByID :exec
DELETE FROM push_subscriptions WHERE id = $1;
