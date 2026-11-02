-- Order queries for the vendor portal. Placing orders lives in order.sql;
-- these only read orders and move them through their status lifecycle.

-- name: CountVendorOrders :one
-- Orders dropped unpaid (payment_not_completed) never reached the shop, so
-- they stay out of its lists, like awaiting_payment ones.
SELECT count(*)
FROM orders
WHERE branch_id = sqlc.arg(branch_id)
  AND deleted = false
  AND status = ANY (sqlc.arg(statuses)::text[])
  AND rejection_reason IS DISTINCT FROM 'payment_not_completed';

-- name: ListVendorOrders :many
-- queue = true sorts like a work queue (soonest fulfilment first, then
-- oldest order); otherwise newest orders come first.
SELECT
    o.*,
    (SELECT coalesce(sum(i.quantity), 0)::int FROM order_items i
      WHERE i.order_id = o.id AND i.status <> 'removed') AS item_count,
    EXISTS (SELECT 1 FROM order_items i
      WHERE i.order_id = o.id AND i.status = 'awaiting_customer') AS awaiting_customer
FROM orders o
WHERE o.branch_id = sqlc.arg(branch_id)
  AND o.deleted = false
  AND o.status = ANY (sqlc.arg(statuses)::text[])
  AND o.rejection_reason IS DISTINCT FROM 'payment_not_completed'
ORDER BY
    CASE WHEN sqlc.arg(queue)::bool THEN o.fulfilment_date END ASC,
    CASE WHEN sqlc.arg(queue)::bool THEN o.created_at END ASC,
    o.created_at DESC,
    o.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: TransitionOrderStatus :one
-- Optimistic concurrency: only applies if the order is still in
-- expected_status, so two people acting on the same order can't both win.
-- Accepting also has to beat the order's accept_deadline (same rule as
-- AcceptOrder in order.sql), so a late click can't win against the expiry
-- job. No row back (pgx.ErrNoRows) means the order moved on, its acceptance
-- window closed, or it doesn't exist.
UPDATE orders
SET status = sqlc.arg(new_status),
    rejection_reason = coalesce(sqlc.narg(rejection_reason), rejection_reason),
    accepted_at = CASE WHEN sqlc.arg(new_status) = 'accepted' THEN now() ELSE accepted_at END,
    cancelled_at = CASE WHEN sqlc.arg(new_status) IN ('rejected', 'cancelled') THEN now() ELSE cancelled_at END,
    collection_code = coalesce(sqlc.narg(collection_code), collection_code),
    ready_at = CASE WHEN sqlc.arg(new_status) = 'ready' THEN now() ELSE ready_at END,
    collect_by = coalesce(sqlc.narg(collect_by), collect_by)
WHERE id = sqlc.arg(id)
  AND branch_id = sqlc.arg(branch_id)
  AND status = sqlc.arg(expected_status)
  AND deleted = false
  AND (sqlc.arg(new_status) <> 'accepted' OR accept_deadline > now())
RETURNING *;

-- name: InsertOrderStatusEvent :exec
INSERT INTO order_status_events (order_id, from_status, to_status, actor_user_id, note, kind)
VALUES ($1, $2, $3, $4, $5, sqlc.arg(kind));

-- name: ListOrderStatusEvents :many
SELECT e.id, e.from_status, e.to_status, e.note, e.kind, e.created_at, u.name AS actor_name
FROM order_status_events e
LEFT JOIN vendor_users u ON u.id = e.actor_user_id
WHERE e.order_id = $1
ORDER BY e.created_at, e.id;

-- name: FindOpenOrdersByCollectionCode :many
-- Staff typing a customer's code at the counter.
SELECT
    o.*,
    (SELECT coalesce(sum(i.quantity), 0)::int FROM order_items i
      WHERE i.order_id = o.id AND i.status <> 'removed') AS item_count,
    EXISTS (SELECT 1 FROM order_items i
      WHERE i.order_id = o.id AND i.status = 'awaiting_customer') AS awaiting_customer
FROM orders o
WHERE o.branch_id = sqlc.arg(branch_id)
  AND o.collection_code = sqlc.arg(code)::text
  AND o.status IN ('accepted', 'picking', 'ready')
  AND o.deleted = false
ORDER BY o.created_at DESC;

-- name: RecordCollectionCodeAttempt :one
UPDATE orders SET collection_code_attempts = collection_code_attempts + 1
WHERE id = $1
RETURNING collection_code_attempts;
