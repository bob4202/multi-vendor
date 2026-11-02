-- name: CreateBranch :one
INSERT INTO branches (name, host, api_username, api_key_cipher, is_active, allowed_secondary_department_uuids, allowed_product_refs, vendor_id, storefront_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateBranch :one
UPDATE branches
SET name = $2,
    host = $3,
    api_username = $4,
    api_key_cipher = $5,
    is_active = $6,
    allowed_secondary_department_uuids = $7,
    allowed_product_refs = $8,
    vendor_id = $9,
    storefront_url = $10,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetBranch :one
SELECT * FROM branches WHERE id = $1;

-- name: ListBranches :many
SELECT * FROM branches ORDER BY name;

-- name: ListActiveBranches :many
SELECT * FROM branches WHERE is_active = true ORDER BY name;

-- name: SetBranchActive :exec
UPDATE branches SET is_active = $2, updated_at = now() WHERE id = $1;

-- name: SetBranchStorefrontKey :one
UPDATE branches SET storefront_api_key_hash = $2, storefront_api_key_prefix = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RevokeBranchStorefrontKey :exec
UPDATE branches SET storefront_api_key_hash = NULL, storefront_api_key_prefix = NULL, updated_at = now()
WHERE id = $1;

-- name: GetBranchByStorefrontKeyHash :one
SELECT * FROM branches WHERE storefront_api_key_hash = $1 AND is_active = true;
