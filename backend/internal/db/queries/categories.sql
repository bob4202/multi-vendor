-- name: UpsertCategory :one
INSERT INTO categories (department_uuid, name, sort_order)
VALUES ($1, $2, $3)
ON CONFLICT (department_uuid) DO UPDATE SET
    name = EXCLUDED.name,
    sort_order = EXCLUDED.sort_order,
    updated_at = now()
RETURNING *;

-- name: ListCategories :many
SELECT * FROM categories ORDER BY sort_order, name;

-- name: ListBranchCategories :many
-- Categories actually in use by one branch's products - not every category
-- that exists, just the ones this branch currently sells (unless
-- include_unavailable is set, e.g. for admin visibility). DISTINCT ON needs
-- its own ORDER BY, so dedup happens in the subquery and the final sort by
-- sort_order/name happens outside it.
SELECT department_uuid, name, sort_order
FROM (
    SELECT DISTINCT ON (products.secondary_department_uuid)
        products.secondary_department_uuid AS department_uuid,
        categories.name,
        COALESCE(categories.sort_order, 0) AS sort_order
    FROM branch_products
    JOIN products ON products.id = branch_products.product_id
    LEFT JOIN categories ON categories.department_uuid = products.secondary_department_uuid
    WHERE branch_products.branch_id = sqlc.arg(branch_id)
      AND products.secondary_department_uuid IS NOT NULL
      AND (sqlc.arg(include_unavailable)::boolean
           OR (branch_products.deleted = false AND branch_products.can_be_sold IS TRUE))
    ORDER BY products.secondary_department_uuid
) branch_categories
ORDER BY sort_order, name;
