-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE branches (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    host           TEXT NOT NULL UNIQUE, -- fills {host} in INVENTORY_API_URL
    api_username   TEXT NOT NULL,
    api_key_cipher BYTEA NOT NULL,       -- AES-256-GCM encrypted inventory system API key
    is_active      BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sync_state (
    branch_id        UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    resource         TEXT NOT NULL, -- 'product' | 'customer'
    cursor           TIMESTAMPTZ,   -- watermark: max source last_modified successfully synced
    last_run_at      TIMESTAMPTZ,
    last_run_status  TEXT,          -- 'success' | 'error'
    last_run_error   TEXT,
    last_run_records INTEGER,
    PRIMARY KEY (branch_id, resource)
);

-- Canonical, deduplicated product catalog. A row here represents one
-- physical product, shared across every branch that stocks it. Records
-- from different branches are merged into the same row when their PLU
-- matches; a product with no PLU can't be matched and gets its own row.
CREATE TABLE products (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plu                         TEXT UNIQUE, -- cross-branch dedup key; NULL = unmatched, not deduplicated
    name                        TEXT,
    product_code                TEXT,
    barcode                     TEXT,
    department_uuid             TEXT,
    secondary_department_uuid   TEXT,
    product_type_uuid           TEXT,
    vat_rate_uuid                TEXT,
    pricebase_uuid               TEXT,
    open_priced                  BOOLEAN,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX products_barcode_idx ON products (barcode);

-- Per-branch link: availability, pricing and stock config differ per
-- branch even for the same product, so they live here rather than on the
-- shared product row.
CREATE TABLE branch_products (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id                   UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id                  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    external_id              TEXT NOT NULL,
    supplier_uuid                TEXT,
    price1                       BIGINT,
    price2                       BIGINT,
    price3                       BIGINT,
    price4                       BIGINT,
    price5                       BIGINT,
    price6                       BIGINT,
    price7                       BIGINT,
    price8                       BIGINT,
    price9                       BIGINT,
    price10                      BIGINT,
    cost_price                   BIGINT,
    minimum_stock                INTEGER,
    maximum_stock                INTEGER,
    can_be_sold                  BOOLEAN,
    can_be_sold_special_orders   BOOLEAN,
    can_be_supplied              BOOLEAN,
    can_be_produced              BOOLEAN,
    track_stock                  BOOLEAN,
    track_availability           BOOLEAN,
    deleted                      BOOLEAN NOT NULL DEFAULT false,
    source_last_modified         TIMESTAMPTZ,
    raw                          JSONB NOT NULL,
    synced_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (branch_id, external_id)
);

CREATE INDEX branch_products_product_idx ON branch_products (product_id);
CREATE INDEX branch_products_branch_deleted_idx ON branch_products (branch_id, deleted);

CREATE TABLE customers (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id             UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    external_id        TEXT NOT NULL,
    name                  TEXT,
    reference             TEXT,
    mobile                TEXT,
    email                 TEXT,
    loyalty_number        TEXT,
    loyalty_number_alt    TEXT,
    customer_type_uuid    TEXT,
    website_customer_id   TEXT,
    allow_marketing       BOOLEAN,
    deleted               BOOLEAN NOT NULL DEFAULT false,
    source_last_modified  TIMESTAMPTZ,
    source_last_ordered   TIMESTAMPTZ,
    raw                   JSONB NOT NULL,
    synced_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (branch_id, external_id)
);

CREATE INDEX customers_branch_deleted_idx ON customers (branch_id, deleted);
CREATE INDEX customers_branch_loyalty_idx ON customers (branch_id, loyalty_number);
CREATE INDEX customers_branch_reference_idx ON customers (branch_id, reference);

-- +goose Down
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS branch_products;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS branches;
