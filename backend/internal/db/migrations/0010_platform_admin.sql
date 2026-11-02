-- +goose Up

-- Platform admins: the people who run the marketplace itself (onboard
-- vendors, set up branches). Same security model as vendor users - argon2id
-- passwords, lockout, hashed session tokens - but a separate table, so a
-- vendor account can never be promoted into an admin by a bad UPDATE.

CREATE TABLE platform_admins (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email              TEXT NOT NULL UNIQUE CHECK (email = lower(email) AND email <> ''),
    name               TEXT NOT NULL,
    password_hash      TEXT NOT NULL, -- argon2id PHC string
    is_active          BOOLEAN NOT NULL DEFAULT true,
    failed_login_count INTEGER NOT NULL DEFAULT 0,
    locked_until       TIMESTAMPTZ,
    last_login_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_platform_admins_updated_at
    BEFORE UPDATE ON platform_admins
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE platform_admin_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id            UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    token_hash          BYTEA NOT NULL UNIQUE,
    user_agent          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX platform_admin_sessions_admin_idx ON platform_admin_sessions (admin_id);

-- Invited vendor users have no password until they accept their invite.
ALTER TABLE vendor_users ALTER COLUMN password_hash DROP NOT NULL;

-- vendor_invites: one-time links to set a password ------------------------------
-- Only the SHA-256 of the token is stored. Issuing a new invite for a user
-- expires their older unused ones (done by the service, in one tx).

CREATE TABLE vendor_invites (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES vendor_users(id) ON DELETE CASCADE,
    token_hash          BYTEA NOT NULL UNIQUE,
    created_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    used_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX vendor_invites_user_idx ON vendor_invites (user_id, created_at DESC);

-- admin_audit_events: who did what on the platform -------------------------------
-- admin_id NULL = done with the shared ADMIN_API_KEY (scripts), or by an
-- admin who has since been deleted. target_id is TEXT so any kind of row
-- (UUID vendors, int closures...) can be referenced.

CREATE TABLE admin_audit_events (
    id          BIGSERIAL PRIMARY KEY,
    admin_id    UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    summary     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_events_created_idx ON admin_audit_events (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS admin_audit_events;
DROP TABLE IF EXISTS vendor_invites;
-- Invited users without a password can't satisfy NOT NULL again; they never
-- finished signing up, so remove them.
DELETE FROM vendor_users WHERE password_hash IS NULL;
ALTER TABLE vendor_users ALTER COLUMN password_hash SET NOT NULL;
DROP TABLE IF EXISTS platform_admin_sessions;
DROP TABLE IF EXISTS platform_admins;
