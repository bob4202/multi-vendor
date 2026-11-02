-- +goose Up
ALTER TABLE branches
    ADD COLUMN allowed_secondary_department_uuids TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN allowed_product_refs TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE branches
    DROP COLUMN allowed_product_refs,
    DROP COLUMN allowed_secondary_department_uuids;
