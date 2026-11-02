-- name: NextOrderReference :one
-- Formatted in SQL so the "TH-" prefix lives in one place: TH-10001, ...
SELECT
    ('TH-' || nextval('order_reference_seq'))::text AS reference;

-- name: InsertOrder :one
-- ON CONFLICT DO NOTHING: if this branch already has an order with this
-- idempotency key, no row is inserted and nothing is returned (pgx.ErrNoRows).
-- The service treats that as a replay and loads the existing order with
-- GetOrderByBranchAndIdempotencyKey. This also covers two identical requests
-- racing each other - the loser gets ErrNoRows instead of a unique violation.
INSERT INTO orders (branch_id, reference, fulfilment_type, fulfilment_date, customer_name, customer_email, customer_phone, address_line_1, address_line_2, address_city, address_postcode, notes, subtotal, delivery_fee, total, idempotency_key, request_hash, accept_deadline, customer_account_id, allow_substitutions, terms_version, terms_accepted_at, status, payment_deadline)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
ON CONFLICT (branch_id, idempotency_key)
    DO NOTHING
RETURNING
    *;

-- name: InsertOrderItem :exec
-- A new line costs what was ordered: charged_total starts as line_total.
INSERT INTO order_items (order_id, product_id, product_name, product_plu, unit_price, quantity, line_total, charged_total)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $7);

-- name: GetOrderByBranchAndIdempotencyKey :one
SELECT
    *
FROM
    orders
WHERE
    branch_id = $1
    AND idempotency_key = $2
    AND deleted = FALSE;

-- name: GetOrderByBranchAndID :one
SELECT
    *
FROM
    orders
WHERE
    branch_id = $1
    AND id = $2
    AND deleted = FALSE;

-- name: GetVendorOrderByBranchAndID :one
SELECT
    *
FROM
    orders
WHERE
    branch_id = $1
    AND id = $2
    AND status <> 'awaiting_payment'
    AND rejection_reason IS DISTINCT FROM 'payment_not_completed'
    AND deleted = FALSE;

-- name: ListActiveOrders :many
SELECT
    *
FROM
    orders
WHERE
    branch_id = $1
    AND deleted = FALSE
    AND status IN ('pending', 'accepted', 'picking')
ORDER BY
    created_at ASC,
    id ASC;

-- name: ListOrdersChangedSince :many
SELECT
    *
FROM
    orders
WHERE
    branch_id = $1
    AND deleted = FALSE
    AND updated_at > $2
ORDER BY
    updated_at ASC;

-- name: ListItemsByOrderIDs :many
-- Reads the snapshot columns, not products - an order shows what was bought
-- and charged at the time, even if the product has since changed.
SELECT
    id,
    order_id,
    product_id,
    product_name,
    product_plu,
    unit_price,
    quantity,
    line_total,
    status,
    charged_total
FROM
    order_items
WHERE
    order_id = ANY (sqlc.arg ('order_ids')::uuid[])
ORDER BY
    order_id,
    product_name;

-- name: UpdateOrderStatus :one
UPDATE
    orders
SET
    status = sqlc.arg ('new_status')
WHERE
    id = sqlc.arg ('id')
    AND branch_id = sqlc.arg ('branch_id')
    AND status = sqlc.arg ('expected_status')
    AND deleted = FALSE
RETURNING
    id;

-- name: AcceptOrder :one
-- Only a pending order that is still inside its acceptance window can be
-- accepted - checked here, not just by the expiry job, so a late click can't
-- win the race against it. No row = not found, not pending, or too late.
UPDATE
    orders
SET
    status = 'accepted',
    accepted_at = now(),
    collection_code = sqlc.narg ('collection_code')
WHERE
    id = sqlc.arg ('id')
    AND branch_id = sqlc.arg ('branch_id')
    AND status = 'pending'
    AND accept_deadline > now()
    AND deleted = FALSE
RETURNING
    *;

-- name: RejectOrder :one
UPDATE
    orders
SET
    status = 'rejected',
    rejection_reason = sqlc.narg ('reason'),
    cancelled_at = now()
WHERE
    id = sqlc.arg ('id')
    AND branch_id = sqlc.arg ('branch_id')
    AND status = 'pending'
    AND deleted = FALSE
RETURNING
    *;

-- name: ExpireOverdueOrders :many
-- Cancels every pending order whose acceptance window has passed.
UPDATE
    orders
SET
    status = 'cancelled',
    rejection_reason = 'not_accepted_in_time',
    cancelled_at = now()
WHERE
    status = 'pending'
    AND accept_deadline <= now()
    AND deleted = FALSE
RETURNING
    id,
    branch_id,
    reference;

-- name: GetOrderByID :one
-- For a signed-in customer opening one of their orders from another of the
-- vendor's branches; the caller checks the order is theirs.
SELECT
    *
FROM
    orders
WHERE
    id = $1
    AND deleted = FALSE;

-- name: LockOrder :one
-- Serialises changes to one order (substitutions, answers, hand-over).
SELECT
    *
FROM
    orders
WHERE
    id = sqlc.arg ('id')
    AND deleted = FALSE
FOR UPDATE;

-- name: GetOrderItemForOrder :one
SELECT
    *
FROM
    order_items
WHERE
    id = sqlc.arg ('id')
    AND order_id = sqlc.arg ('order_id');

-- name: SetOrderItemState :exec
UPDATE
    order_items
SET
    status = sqlc.arg ('status'),
    charged_total = sqlc.arg ('charged_total')
WHERE
    id = sqlc.arg ('id');

-- name: RecalculateOrderTotals :one
-- The subtotal is what every line costs now. Always touches the row, so the
-- change is broadcast to live screens even when the total stays the same.
UPDATE
    orders o
SET
    subtotal = s.subtotal,
    total = s.subtotal + o.delivery_fee,
    updated_at = now()
FROM (
    SELECT
        coalesce(sum(charged_total), 0)::bigint AS subtotal
    FROM
        order_items
    WHERE
        order_id = sqlc.arg ('id')) s
WHERE
    o.id = sqlc.arg ('id')
RETURNING
    o.*;

-- name: CountOrderItemStates :one
SELECT
    count(*) FILTER (WHERE status <> 'removed')::int AS live,
    count(*) FILTER (WHERE status = 'awaiting_customer')::int AS awaiting
FROM
    order_items
WHERE
    order_id = $1;

-- name: ListSubstitutionsByOrderIDs :many
SELECT
    *
FROM
    order_substitutions
WHERE
    order_id = ANY (sqlc.arg ('order_ids')::uuid[])
ORDER BY
    created_at,
    id;

-- name: InsertSubstitution :one
INSERT INTO order_substitutions (order_id, order_item_id, product_id, product_name, product_plu, unit_price, quantity, line_total, expires_at, created_by_user_id)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING
    *;

-- name: GetSubstitutionForOrder :one
SELECT
    *
FROM
    order_substitutions
WHERE
    id = sqlc.arg ('id')
    AND order_id = sqlc.arg ('order_id');

-- name: GetLiveSubstitutionForItem :one
SELECT
    *
FROM
    order_substitutions
WHERE
    order_item_id = $1
    AND status = 'proposed';

-- name: SetSubstitutionStatus :execrows
-- Only a live proposal can be answered, withdrawn or expired - whoever
-- gets there first wins.
UPDATE
    order_substitutions
SET
    status = sqlc.arg ('status'),
    responded_at = now()
WHERE
    id = sqlc.arg ('id')
    AND status = 'proposed';

-- name: ListDueSubstitutions :many
SELECT
    id,
    order_id
FROM
    order_substitutions
WHERE
    status = 'proposed'
    AND expires_at <= now()
ORDER BY
    expires_at
LIMIT 50;

-- name: ExpireUncollectedOrders :many
-- Ready collection orders nobody picked up: collect_by plus a grace period.
UPDATE
    orders
SET
    status = 'no_show',
    rejection_reason = 'not_collected_in_time'
WHERE
    status = 'ready'
    AND collect_by IS NOT NULL
    AND collect_by + make_interval(mins => sqlc.arg ('grace_minutes')::int) <= now()
    AND deleted = FALSE
RETURNING
    id,
    branch_id,
    reference;

-- name: InsertNotification :exec
INSERT INTO notification_outbox (order_id, kind, detail)
    VALUES ($1, $2, $3);

-- name: ClaimDueNotifications :many
-- Locked until the claiming transaction ends; other instances skip them.
SELECT
    *
FROM
    notification_outbox
WHERE
    sent_at IS NULL
    AND next_attempt_at <= now()
ORDER BY
    id
LIMIT 20
FOR UPDATE
    SKIP LOCKED;

-- name: MarkNotificationSent :exec
UPDATE
    notification_outbox
SET
    sent_at = now(),
    attempts = attempts + 1,
    last_error = sqlc.narg ('last_error')
WHERE
    id = sqlc.arg ('id');

-- name: MarkNotificationRetry :exec
UPDATE
    notification_outbox
SET
    attempts = attempts + 1,
    last_error = sqlc.arg ('last_error'),
    next_attempt_at = sqlc.arg ('next_attempt_at')
WHERE
    id = sqlc.arg ('id');

-- name: GetOrderNotificationContext :one
-- What an email or push about an order needs to say and link to.
SELECT
    b.name AS branch_name,
    b.storefront_url,
    bi.phone AS branch_phone,
    bi.timezone
FROM
    branches b
    LEFT JOIN branch_info bi ON bi.branch_id = b.id
WHERE
    b.id = $1;

-- name: NotifyOutbox :exec
-- Delivered on commit; wakes the notification sender on every instance.
SELECT
    pg_notify('notification_outbox', '');
