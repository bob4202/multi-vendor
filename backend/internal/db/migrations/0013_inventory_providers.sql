-- +goose Up

-- Each branch picks which inventory system it syncs from. The name matches a
-- provider compiled into the API (internal/inventory); INVENTORY_PROVIDERS
-- decides which ones a server offers. host/api_username/api_key_cipher are
-- that provider's connection settings.
ALTER TABLE branches
    ADD COLUMN inventory_provider TEXT NOT NULL DEFAULT 'connect'
        CONSTRAINT branches_inventory_provider_check CHECK (inventory_provider ~ '^[a-z0-9_-]+$');

-- A host is only unique within one system.
ALTER TABLE branches DROP CONSTRAINT branches_host_key;
ALTER TABLE branches ADD CONSTRAINT branches_inventory_host_key UNIQUE (inventory_provider, host);

-- +goose Down
ALTER TABLE branches DROP CONSTRAINT branches_inventory_host_key;
ALTER TABLE branches ADD CONSTRAINT branches_host_key UNIQUE (host);
ALTER TABLE branches DROP COLUMN inventory_provider;
