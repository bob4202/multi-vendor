-- +goose Up

-- Storefront orders. Everything a past order needs to be shown (or disputed)
-- is snapshotted onto the order itself: customer contact, delivery address,
-- and each item's name/price at the time of ordering - so later edits to
-- products, prices or anything else never rewrite order history.
-- Money is in pence (BIGINT, matching branch_products.price*).

-- Human-readable references: TH-10001, TH-10002, ... A sequence is
-- concurrency-safe without locking; a rolled-back order just leaves a gap,
-- which is fine for a reference number.
CREATE SEQUENCE order_reference_seq START 10001;

CREATE TABLE orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- RESTRICT, not CASCADE: orders are financial records and must outlive
    -- any attempt to delete the branch they belong to.
    branch_id       UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    reference       TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'pending'
        CONSTRAINT orders_status_check CHECK (status IN (
            'pending', 'accepted', 'picking', 'ready', 'out_for_delivery',
            'completed', 'rejected', 'cancelled', 'no_show'
        )),
    -- No default: an order must say how it's fulfilled, never silently
    -- become a delivery.
    fulfilment_type TEXT NOT NULL
        CONSTRAINT orders_fulfilment_type_check CHECK (fulfilment_type IN ('delivery', 'collection')),
    fulfilment_date DATE NOT NULL,

    -- Customer snapshot (web customers aren't inventory-system customers, so these
    -- live here rather than referencing the synced `customers` table).
    customer_name   TEXT NOT NULL,
    customer_email  TEXT NOT NULL,
    customer_phone  TEXT NOT NULL,

    -- Delivery address snapshot - NULL for collection orders.
    address_line_1  TEXT,
    address_line_2  TEXT,
    address_city    TEXT,
    address_postcode TEXT,

    notes            TEXT,
    rejection_reason TEXT,

    subtotal        BIGINT NOT NULL CHECK (subtotal >= 0),
    delivery_fee    BIGINT NOT NULL CHECK (delivery_fee >= 0),
    total           BIGINT NOT NULL CHECK (total = subtotal + delivery_fee),

    -- Idempotency: the key the client sent, plus a hash of the request body
    -- it was first used with. Same key + same hash = replay (return the
    -- original order); same key + different hash = 409 conflict.
    idempotency_key TEXT NOT NULL,
    request_hash    BYTEA NOT NULL,

    deleted         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT orders_delivery_address_check CHECK (
        fulfilment_type <> 'delivery'
        OR (address_line_1 IS NOT NULL AND address_city IS NOT NULL AND address_postcode IS NOT NULL)
    )
);

CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Needed by ON CONFLICT (branch_id, idempotency_key) in InsertOrder. Scoped
-- per branch: keys are client-generated, so two storefronts may collide.
CREATE UNIQUE INDEX orders_branch_idempotency_key_uidx ON orders (branch_id, idempotency_key);

-- ListActiveOrders (live display)
CREATE INDEX orders_branch_status_created_idx ON orders (branch_id, status, created_at)
    WHERE deleted = false;

-- ListOrdersChangedSince (polling)
CREATE INDEX orders_branch_updated_idx ON orders (branch_id, updated_at)
    WHERE deleted = false;

-- Order lines are written once with the order and never edited (cancel the
-- order instead), so no soft-delete flag or updated_at.
CREATE TABLE order_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    -- RESTRICT: deleting a product must not erase it from past orders.
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    -- Snapshot at order time, so receipts survive renames and price changes.
    product_name TEXT NOT NULL,
    product_plu  TEXT,
    unit_price   BIGINT NOT NULL CHECK (unit_price >= 0),
    quantity     INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    line_total   BIGINT NOT NULL CHECK (line_total = unit_price * quantity),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One line per product per order. The service merges duplicate product IDs
-- before inserting, so hitting this is a bug, not something to upsert over.
CREATE UNIQUE INDEX order_items_order_product_uidx ON order_items (order_id, product_id);

-- ON DELETE RESTRICT check on products
CREATE INDEX order_items_product_id_idx ON order_items (product_id);

-- +goose Down
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP SEQUENCE IF EXISTS order_reference_seq;
