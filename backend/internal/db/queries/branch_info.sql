-- name: GetBranchInfo :one
SELECT * FROM branch_info WHERE branch_id = $1;

-- name: UpsertBranchInfo :one
INSERT INTO branch_info (
    branch_id, phone, email, address_line_1, address_line_2, city, postal_code, country, timezone,
    collection, delivery, delivery_fee, delivery_free_over, delivery_min_order, delivery_postcode_prefixes,
    last_order_minutes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15,
    $16
)
ON CONFLICT (branch_id) DO UPDATE SET
    phone = EXCLUDED.phone,
    email = EXCLUDED.email,
    address_line_1 = EXCLUDED.address_line_1,
    address_line_2 = EXCLUDED.address_line_2,
    city = EXCLUDED.city,
    postal_code = EXCLUDED.postal_code,
    country = EXCLUDED.country,
    timezone = EXCLUDED.timezone,
    collection = EXCLUDED.collection,
    delivery = EXCLUDED.delivery,
    delivery_fee = EXCLUDED.delivery_fee,
    delivery_free_over = EXCLUDED.delivery_free_over,
    delivery_min_order = EXCLUDED.delivery_min_order,
    delivery_postcode_prefixes = EXCLUDED.delivery_postcode_prefixes,
    last_order_minutes = EXCLUDED.last_order_minutes
RETURNING *;

-- name: ListBranchOpeningHours :many
SELECT * FROM branch_opening_hours
WHERE branch_id = $1
ORDER BY day_of_week, open_time;

-- Replacing the weekly schedule is delete-all + insert, run in one tx.
-- name: DeleteBranchOpeningHours :exec
DELETE FROM branch_opening_hours WHERE branch_id = $1;

-- name: CreateBranchOpeningHour :one
INSERT INTO branch_opening_hours (branch_id, day_of_week, open_time, close_time)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListBranchHoursExceptions :many
SELECT * FROM branch_hours_exception
WHERE branch_id = @branch_id
  AND date BETWEEN @from_date::date AND @to_date::date
ORDER BY date, open_time NULLS FIRST;

-- name: CreateBranchHoursException :one
INSERT INTO branch_hours_exception (branch_id, date, closed, open_time, close_time, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteBranchHoursException :exec
DELETE FROM branch_hours_exception WHERE id = $1 AND branch_id = $2;
