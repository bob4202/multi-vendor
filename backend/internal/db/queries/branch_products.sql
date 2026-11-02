-- name: UpsertBranchProduct :exec
INSERT INTO branch_products (
    branch_id, product_id, external_id, supplier_uuid,
    price1, price2, price3, price4, price5, price6, price7, price8, price9, price10,
    cost_price, minimum_stock, maximum_stock,
    can_be_sold, can_be_sold_special_orders, can_be_supplied, can_be_produced,
    track_stock, track_availability, deleted, source_last_modified, raw, synced_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17,
    $18, $19, $20, $21,
    $22, $23, $24, $25, $26, now(), now()
)
ON CONFLICT (branch_id, external_id) DO UPDATE SET
    product_id = EXCLUDED.product_id,
    supplier_uuid = EXCLUDED.supplier_uuid,
    price1 = EXCLUDED.price1,
    price2 = EXCLUDED.price2,
    price3 = EXCLUDED.price3,
    price4 = EXCLUDED.price4,
    price5 = EXCLUDED.price5,
    price6 = EXCLUDED.price6,
    price7 = EXCLUDED.price7,
    price8 = EXCLUDED.price8,
    price9 = EXCLUDED.price9,
    price10 = EXCLUDED.price10,
    cost_price = EXCLUDED.cost_price,
    minimum_stock = EXCLUDED.minimum_stock,
    maximum_stock = EXCLUDED.maximum_stock,
    can_be_sold = EXCLUDED.can_be_sold,
    can_be_sold_special_orders = EXCLUDED.can_be_sold_special_orders,
    can_be_supplied = EXCLUDED.can_be_supplied,
    can_be_produced = EXCLUDED.can_be_produced,
    track_stock = EXCLUDED.track_stock,
    track_availability = EXCLUDED.track_availability,
    deleted = EXCLUDED.deleted,
    source_last_modified = EXCLUDED.source_last_modified,
    raw = EXCLUDED.raw,
    synced_at = now(),
    updated_at = now();
