-- +goose Up

-- Storefront-editorial content, kept entirely separate from the inventory
-- sync mirror (products/branch_products) so a vendor re-sync can never
-- clobber a description or image someone set here. product_content is
-- shared across every branch selling the product; branch_product_content
-- holds per-branch overrides (a store's own blurb/photo for a shared
-- product) and upsell flags - a row here is optional, its absence just
-- means "use the shared content, not featured as an upsell".
CREATE TABLE product_content (
    product_id   UUID PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    description  TEXT,
    image_url    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE branch_product_content (
    branch_id             UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id            UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    description_override  TEXT,
    image_url_override    TEXT,
    is_upsell             BOOLEAN NOT NULL DEFAULT false,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (branch_id, product_id)
);

CREATE INDEX branch_product_content_upsell_idx ON branch_product_content (branch_id) WHERE is_upsell = true;

-- +goose Down
DROP TABLE IF EXISTS branch_product_content;
DROP TABLE IF EXISTS product_content;
