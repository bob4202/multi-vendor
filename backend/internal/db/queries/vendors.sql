-- Vendors ----------------------------------------------------------------------

-- name: CreateVendor :one
INSERT INTO vendors (slug, name) VALUES ($1, $2) RETURNING *;

-- name: GetVendor :one
SELECT * FROM vendors WHERE id = $1;

-- name: ListVendors :many
SELECT * FROM vendors ORDER BY name;

-- Vendor users -------------------------------------------------------------------

-- name: CreateVendorUser :one
INSERT INTO vendor_users (vendor_id, email, name, password_hash, role)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetVendorUser :one
SELECT * FROM vendor_users WHERE id = $1;

-- name: GetVendorUserByEmail :one
-- email must already be lowercased (the column CHECK guarantees stored
-- emails are).
SELECT * FROM vendor_users WHERE email = $1;

-- name: ListVendorUsers :many
SELECT * FROM vendor_users WHERE vendor_id = $1 ORDER BY name;

-- name: RecordVendorLoginFailure :exec
-- Counts a failed password attempt. Reaching max_attempts locks the account
-- until lock_until and resets the counter, so once the lock expires the user
-- gets a fresh set of attempts rather than being re-locked on the next typo.
UPDATE vendor_users
SET failed_login_count = CASE
        WHEN failed_login_count + 1 >= sqlc.arg(max_attempts)::int THEN 0
        ELSE failed_login_count + 1
    END,
    locked_until = CASE
        WHEN failed_login_count + 1 >= sqlc.arg(max_attempts)::int THEN sqlc.arg(lock_until)::timestamptz
        ELSE locked_until
    END
WHERE id = sqlc.arg(id);

-- name: RecordVendorLoginSuccess :exec
UPDATE vendor_users
SET failed_login_count = 0, locked_until = NULL, last_login_at = now()
WHERE id = $1;

-- name: UpdateVendorUserPasswordHash :exec
UPDATE vendor_users SET password_hash = $2 WHERE id = $1;

-- name: AssignVendorUserBranch :exec
-- The composite foreign keys reject a branch that belongs to another vendor.
INSERT INTO vendor_user_branches (vendor_id, user_id, branch_id)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, branch_id) DO NOTHING;

-- name: ListVendorUserBranches :many
-- Branches this user may manage: every branch of their vendor for an owner,
-- only explicitly assigned ones for managers and staff.
SELECT b.id, b.name, b.is_active
FROM branches b
JOIN vendor_users u ON u.vendor_id = b.vendor_id
WHERE u.id = $1
  AND (
      u.role = 'owner'
      OR EXISTS (
          SELECT 1 FROM vendor_user_branches ub
          WHERE ub.user_id = u.id AND ub.branch_id = b.id
      )
  )
ORDER BY b.name;

-- Sessions -----------------------------------------------------------------------

-- name: CreateVendorSession :one
INSERT INTO vendor_sessions (user_id, token_hash, user_agent, expires_at, absolute_expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActiveVendorSession :one
-- A session only counts while it's unrevoked, unexpired, and both its user
-- and vendor are still active - deactivating either logs everyone out.
SELECT sqlc.embed(s), sqlc.embed(u), sqlc.embed(v)
FROM vendor_sessions s
JOIN vendor_users u ON u.id = s.user_id
JOIN vendors v ON v.id = u.vendor_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND u.is_active
  AND v.is_active;

-- name: TouchVendorSession :exec
-- Slides the idle expiry forward, never past the absolute expiry.
UPDATE vendor_sessions
SET last_seen_at = now(),
    expires_at = LEAST(sqlc.arg(expires_at)::timestamptz, absolute_expires_at)
WHERE id = sqlc.arg(id);

-- name: RevokeVendorSession :exec
UPDATE vendor_sessions SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;
