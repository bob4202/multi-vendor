-- name: UpsertCustomer :exec
INSERT INTO customers (
    branch_id, external_id, name, reference, mobile, email,
    loyalty_number, loyalty_number_alt, customer_type_uuid, website_customer_id,
    allow_marketing, deleted, source_last_modified, source_last_ordered,
    raw, synced_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, now(), now()
)
ON CONFLICT (branch_id, external_id) DO UPDATE SET
    name = EXCLUDED.name,
    reference = EXCLUDED.reference,
    mobile = EXCLUDED.mobile,
    email = EXCLUDED.email,
    loyalty_number = EXCLUDED.loyalty_number,
    loyalty_number_alt = EXCLUDED.loyalty_number_alt,
    customer_type_uuid = EXCLUDED.customer_type_uuid,
    website_customer_id = EXCLUDED.website_customer_id,
    allow_marketing = EXCLUDED.allow_marketing,
    deleted = EXCLUDED.deleted,
    source_last_modified = EXCLUDED.source_last_modified,
    source_last_ordered = EXCLUDED.source_last_ordered,
    raw = EXCLUDED.raw,
    synced_at = now(),
    updated_at = now();

-- name: ListCustomers :many
SELECT * FROM customers
WHERE branch_id = $1
  AND (deleted = false OR sqlc.arg(include_deleted)::boolean = true)
ORDER BY name
LIMIT $2 OFFSET $3;

-- name: CountCustomers :one
SELECT count(*) FROM customers WHERE branch_id = $1;
