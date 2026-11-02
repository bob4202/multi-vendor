-- +goose Up

-- Multi-vendor accounts. A vendor is a business (one owner, possibly several
-- branches); vendor_users are the people who log in to the vendor portal.
-- Platform billing (monthly fees, per-order cuts, Stripe Connect accounts)
-- is deliberately not modelled yet - it will get its own tables keyed on
-- vendor_id, so fee models can change without touching these.

CREATE TABLE vendors (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_vendors_updated_at
    BEFORE UPDATE ON vendors
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Every branch belongs to exactly one vendor. Existing branches (created
-- before vendors existed) are assigned to a single default vendor so the
-- column can be NOT NULL from the start.
ALTER TABLE branches ADD COLUMN vendor_id UUID REFERENCES vendors(id) ON DELETE RESTRICT;

INSERT INTO vendors (slug, name)
SELECT 'grocery-shop', 'Grocery Shop'
WHERE EXISTS (SELECT 1 FROM branches);

UPDATE branches
SET vendor_id = (SELECT id FROM vendors WHERE slug = 'grocery-shop')
WHERE vendor_id IS NULL;

ALTER TABLE branches ALTER COLUMN vendor_id SET NOT NULL;

-- Target for the composite foreign key on vendor_user_branches, which is
-- what stops a user from ever being given another vendor's branch.
ALTER TABLE branches ADD CONSTRAINT branches_id_vendor_id_key UNIQUE (id, vendor_id);

CREATE INDEX branches_vendor_idx ON branches (vendor_id);

-- vendor_users: people who log in -----------------------------------------------
-- Roles: owner   - every branch of the vendor, every permission
--        manager - assigned branches: orders, store info, product sync
--        staff   - assigned branches: orders only
-- Emails are stored lowercased (enforced by CHECK) so a plain UNIQUE works
-- and lookups are exact matches. One email = one login across the platform.

CREATE TABLE vendor_users (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vendor_id          UUID NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    email              TEXT NOT NULL UNIQUE CHECK (email = lower(email) AND email <> ''),
    name               TEXT NOT NULL,
    password_hash      TEXT NOT NULL, -- argon2id PHC string
    role               TEXT NOT NULL
        CONSTRAINT vendor_users_role_check CHECK (role IN ('owner', 'manager', 'staff')),
    is_active          BOOLEAN NOT NULL DEFAULT true,
    failed_login_count INTEGER NOT NULL DEFAULT 0,
    locked_until       TIMESTAMPTZ,
    last_login_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vendor_users_id_vendor_id_key UNIQUE (id, vendor_id)
);

CREATE INDEX vendor_users_vendor_idx ON vendor_users (vendor_id);

CREATE TRIGGER trg_vendor_users_updated_at
    BEFORE UPDATE ON vendor_users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- vendor_user_branches: which branches a manager/staff member can manage ---------
-- Owners implicitly manage every branch of their vendor and need no rows.
-- Both foreign keys include vendor_id, so the user and the branch must
-- belong to the same vendor - tenancy enforced by the database itself.

CREATE TABLE vendor_user_branches (
    vendor_id UUID NOT NULL,
    user_id   UUID NOT NULL,
    branch_id UUID NOT NULL,
    PRIMARY KEY (user_id, branch_id),
    FOREIGN KEY (user_id, vendor_id) REFERENCES vendor_users (id, vendor_id) ON DELETE CASCADE,
    FOREIGN KEY (branch_id, vendor_id) REFERENCES branches (id, vendor_id) ON DELETE CASCADE
);

CREATE INDEX vendor_user_branches_branch_idx ON vendor_user_branches (branch_id);

-- vendor_sessions: one row per login --------------------------------------------
-- Only a SHA-256 hash of the token is stored (same approach as storefront
-- keys), so a database dump can't be replayed as live sessions.
-- expires_at slides forward on use; absolute_expires_at never moves.

CREATE TABLE vendor_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES vendor_users(id) ON DELETE CASCADE,
    token_hash          BYTEA NOT NULL UNIQUE,
    user_agent          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX vendor_sessions_user_idx ON vendor_sessions (user_id);

-- order_status_events: audit trail of every status change -------------------------
-- actor_user_id is NULL for changes not made by a vendor user (e.g. the
-- system, or a user who has since been deleted).

CREATE TABLE order_status_events (
    id            BIGSERIAL PRIMARY KEY,
    order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status   TEXT,
    to_status     TEXT NOT NULL,
    actor_user_id UUID REFERENCES vendor_users(id) ON DELETE SET NULL,
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX order_status_events_order_idx ON order_status_events (order_id, created_at);

-- Order lists in the vendor portal page through all of a branch's orders,
-- newest first.
CREATE INDEX orders_branch_created_idx ON orders (branch_id, created_at DESC)
    WHERE deleted = false;

-- +goose Down
DROP INDEX IF EXISTS orders_branch_created_idx;
DROP TABLE IF EXISTS order_status_events;
DROP TABLE IF EXISTS vendor_sessions;
DROP TABLE IF EXISTS vendor_user_branches;
DROP TABLE IF EXISTS vendor_users;
DROP INDEX IF EXISTS branches_vendor_idx;
ALTER TABLE branches DROP CONSTRAINT IF EXISTS branches_id_vendor_id_key;
ALTER TABLE branches DROP COLUMN IF EXISTS vendor_id;
DROP TABLE IF EXISTS vendors;
