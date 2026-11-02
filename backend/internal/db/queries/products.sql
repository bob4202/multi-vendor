-- name: UpsertProductByPLU :one
-- Finds-or-creates the canonical product for a PLU, refreshing its
-- descriptive fields from whichever branch synced most recently.
INSERT INTO products (plu, name, product_code, barcode, department_uuid, secondary_department_uuid, product_type_uuid, vat_rate_uuid, pricebase_uuid, open_priced)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (plu)
    DO UPDATE SET
        name = EXCLUDED.name,
        product_code = EXCLUDED.product_code,
        barcode = EXCLUDED.barcode,
        department_uuid = EXCLUDED.department_uuid,
        secondary_department_uuid = EXCLUDED.secondary_department_uuid,
        product_type_uuid = EXCLUDED.product_type_uuid,
        vat_rate_uuid = EXCLUDED.vat_rate_uuid,
        pricebase_uuid = EXCLUDED.pricebase_uuid,
        open_priced = EXCLUDED.open_priced,
        updated_at = now()
    RETURNING
        id;

-- name: CreateProductWithoutPLU :one
-- Used when the source record has no PLU, so it can't be matched to an
-- existing product; each such record gets its own catalog row.
INSERT INTO products (plu, name, product_code, barcode, department_uuid, secondary_department_uuid, product_type_uuid, vat_rate_uuid, pricebase_uuid, open_priced)
    VALUES (NULL, $1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING
    id;

-- name: UpdateProductDescriptive :one
-- Refreshes descriptive fields on a product row already linked via
-- branch_products, for records that still have no PLU to match on.
UPDATE
    products
SET
    name = $2,
    product_code = $3,
    barcode = $4,
    department_uuid = $5,
    secondary_department_uuid = $6,
    product_type_uuid = $7,
    vat_rate_uuid = $8,
    pricebase_uuid = $9,
    open_priced = $10,
    updated_at = now()
WHERE
    id = $1
RETURNING
    id;

-- name: GetProductIDByBranchExternalID :one
SELECT
    product_id
FROM
    branch_products
WHERE
    branch_id = $1
    AND external_id = $2;

-- name: ListBranchProducts :many
-- Storefront/admin product listing for one branch: filters by category
-- (secondary_department_uuid) and/or a name search, merges editorial
-- content (branch override wins over the shared product content), and
-- resolves the category's display name. include_unavailable=false (the
-- storefront path) restricts to sellable, non-deleted rows.
SELECT
    products.id,
    products.plu,
    products.name,
    products.barcode,
    products.secondary_department_uuid AS category_id,
    categories.name AS category_name,
    branch_products.price1 AS price,
    branch_products.can_be_sold,
    branch_products.deleted,
    COALESCE(branch_product_content.description_override, product_content.description) AS description,
    COALESCE(branch_product_content.image_urls_override, product_content.image_urls) AS image_urls,
    COALESCE(branch_product_content.is_upsell, FALSE) AS is_upsell
FROM
    branch_products
    JOIN products ON products.id = branch_products.product_id
    LEFT JOIN categories ON categories.department_uuid = products.secondary_department_uuid
    LEFT JOIN product_content ON product_content.product_id = products.id
    LEFT JOIN branch_product_content ON branch_product_content.branch_id = branch_products.branch_id
        AND branch_product_content.product_id = products.id
WHERE
    branch_products.branch_id = sqlc.arg (branch_id)
    AND (sqlc.narg (category)::text IS NULL
        OR products.secondary_department_uuid = sqlc.narg (category))
    AND (sqlc.narg (search)::text IS NULL
        OR products.name ILIKE '%' || sqlc.narg (search)::text || '%')
    AND (sqlc.arg (include_unavailable)::boolean
        OR (branch_products.deleted = FALSE
            AND branch_products.can_be_sold IS TRUE))
    AND (NOT sqlc.arg (featured_only)::boolean
        OR COALESCE(branch_product_content.is_upsell, FALSE))
ORDER BY
    CASE WHEN sqlc.arg (sort)::text = 'price_asc' THEN
        branch_products.price1
    END ASC NULLS LAST,
    CASE WHEN sqlc.arg (sort)::text = 'price_desc' THEN
        branch_products.price1
    END DESC NULLS LAST,
    products.name,
    products.id
LIMIT sqlc.arg (page_limit)
OFFSET sqlc.arg (page_offset);

-- name: CountBranchProducts :one
SELECT
    count(*)
FROM
    branch_products
    JOIN products ON products.id = branch_products.product_id
    LEFT JOIN branch_product_content ON branch_product_content.branch_id = branch_products.branch_id
        AND branch_product_content.product_id = products.id
WHERE
    branch_products.branch_id = sqlc.arg (branch_id)
    AND (sqlc.narg (category)::text IS NULL
        OR products.secondary_department_uuid = sqlc.narg (category))
    AND (sqlc.narg (search)::text IS NULL
        OR products.name ILIKE '%' || sqlc.narg (search)::text || '%')
    AND (sqlc.arg (include_unavailable)::boolean
        OR (branch_products.deleted = FALSE
            AND branch_products.can_be_sold IS TRUE))
    AND (NOT sqlc.arg (featured_only)::boolean
        OR COALESCE(branch_product_content.is_upsell, FALSE));

-- name: GetBranchProduct :one
-- Single-product detail, same shape/joins as ListBranchProducts.
SELECT
    products.id,
    products.plu,
    products.name,
    products.barcode,
    products.secondary_department_uuid AS category_id,
    categories.name AS category_name,
    branch_products.price1 AS price,
    branch_products.can_be_sold,
    branch_products.deleted,
    COALESCE(branch_product_content.description_override, product_content.description) AS description,
    COALESCE(branch_product_content.image_urls_override, product_content.image_urls) AS image_urls,
    COALESCE(branch_product_content.is_upsell, FALSE) AS is_upsell
FROM
    branch_products
    JOIN products ON products.id = branch_products.product_id
    LEFT JOIN categories ON categories.department_uuid = products.secondary_department_uuid
    LEFT JOIN product_content ON product_content.product_id = products.id
    LEFT JOIN branch_product_content ON branch_product_content.branch_id = branch_products.branch_id
        AND branch_product_content.product_id = products.id
WHERE
    branch_products.branch_id = $1
    AND products.id = $2;

-- name: ListBranchProductsByID :many
-- Storefront/admin product listing for one branch: filters by ID
SELECT
    products.id,
    products.plu,
    products.name,
    products.barcode,
    products.secondary_department_uuid AS category_id,
    categories.name AS category_name,
    branch_products.price1 AS price,
    branch_products.can_be_sold,
    branch_products.deleted,
    COALESCE(branch_product_content.description_override, product_content.description) AS description,
    COALESCE(branch_product_content.image_urls_override, product_content.image_urls) AS image_urls,
    COALESCE(branch_product_content.is_upsell, FALSE) AS is_upsell
FROM
    branch_products
    JOIN products ON products.id = branch_products.product_id
    LEFT JOIN categories ON categories.department_uuid = products.secondary_department_uuid
    LEFT JOIN product_content ON product_content.product_id = products.id
    LEFT JOIN branch_product_content ON branch_product_content.branch_id = branch_products.branch_id
        AND branch_product_content.product_id = products.id
WHERE
    branch_products.branch_id = @branch_id
    AND products.id = ANY (@ids::uuid[])
    AND branch_products.deleted = FALSE
    AND branch_products.can_be_sold = TRUE;

