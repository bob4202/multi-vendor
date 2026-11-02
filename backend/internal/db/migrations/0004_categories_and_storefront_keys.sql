-- +goose Up
CREATE TABLE categories (
    department_uuid TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE branches
    ADD COLUMN storefront_api_key_hash   BYTEA,
    ADD COLUMN storefront_api_key_prefix TEXT;

CREATE UNIQUE INDEX branches_storefront_api_key_hash_idx
    ON branches (storefront_api_key_hash)
    WHERE storefront_api_key_hash IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS branches_storefront_api_key_hash_idx;
ALTER TABLE branches DROP COLUMN storefront_api_key_prefix, DROP COLUMN storefront_api_key_hash;
DROP TABLE IF EXISTS categories;
