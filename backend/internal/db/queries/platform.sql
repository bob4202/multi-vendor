-- Platform admins -----------------------------------------------------------------

-- name: CreatePlatformAdmin :one
INSERT INTO platform_admins (email, name, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPlatformAdminByEmail :one
SELECT * FROM platform_admins WHERE email = $1;

-- name: RecordPlatformAdminLoginFailure :exec
-- Same lockout rule as RecordVendorLoginFailure.
UPDATE platform_admins
SET failed_login_count = CASE
        WHEN failed_login_count + 1 >= sqlc.arg(max_attempts)::int THEN 0
        ELSE failed_login_count + 1
    END,
    locked_until = CASE
        WHEN failed_login_count + 1 >= sqlc.arg(max_attempts)::int THEN sqlc.arg(lock_until)::timestamptz
        ELSE locked_until
    END
WHERE id = sqlc.arg(id);

-- name: RecordPlatformAdminLoginSuccess :exec
UPDATE platform_admins
SET failed_login_count = 0, locked_until = NULL, last_login_at = now()
WHERE id = $1;

-- name: UpdatePlatformAdminPasswordHash :exec
UPDATE platform_admins SET password_hash = $2 WHERE id = $1;

-- name: CreatePlatformAdminSession :one
INSERT INTO platform_admin_sessions (admin_id, token_hash, user_agent, expires_at, absolute_expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetActivePlatformAdminSession :one
SELECT sqlc.embed(s), sqlc.embed(a)
FROM platform_admin_sessions s
JOIN platform_admins a ON a.id = s.admin_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND a.is_active;

-- name: TouchPlatformAdminSession :exec
UPDATE platform_admin_sessions
SET last_seen_at = now(),
    expires_at = LEAST(sqlc.arg(expires_at)::timestamptz, absolute_expires_at)
WHERE id = sqlc.arg(id);

-- name: RevokePlatformAdminSession :exec
UPDATE platform_admin_sessions SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- Vendor invites -------------------------------------------------------------------

-- name: ExpireUnusedVendorInvites :exec
-- Called before issuing a new invite, so only the newest link works.
UPDATE vendor_invites SET expires_at = now()
WHERE user_id = $1 AND used_at IS NULL AND expires_at > now();

-- name: CreateVendorInvite :one
INSERT INTO vendor_invites (user_id, token_hash, created_by_admin_id, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetVendorInviteByTokenHash :one
SELECT sqlc.embed(i), sqlc.embed(u), sqlc.embed(v)
FROM vendor_invites i
JOIN vendor_users u ON u.id = i.user_id
JOIN vendors v ON v.id = u.vendor_id
WHERE i.token_hash = $1;

-- name: MarkVendorInviteUsed :execrows
-- Guarded, so two simultaneous accepts can't both succeed.
UPDATE vendor_invites SET used_at = now()
WHERE id = $1 AND used_at IS NULL AND expires_at > now();

-- name: GetVendorUserForVendor :one
SELECT * FROM vendor_users WHERE id = $1 AND vendor_id = $2;

-- Audit trail ----------------------------------------------------------------------

-- name: InsertAdminAuditEvent :exec
INSERT INTO admin_audit_events (admin_id, action, target_type, target_id, summary)
VALUES ($1, $2, $3, $4, $5);

-- name: ListRecentAdminAuditEvents :many
SELECT e.id, e.action, e.target_type, e.target_id, e.summary, e.created_at, a.name AS admin_name
FROM admin_audit_events e
LEFT JOIN platform_admins a ON a.id = e.admin_id
ORDER BY e.created_at DESC, e.id DESC
LIMIT $1;

-- Vendors, as the admin sees them ---------------------------------------------------

-- name: ListVendorsWithCounts :many
SELECT
    v.*,
    (SELECT count(*) FROM branches b WHERE b.vendor_id = v.id)::int AS branch_count,
    (SELECT count(*) FROM vendor_users u WHERE u.vendor_id = v.id)::int AS user_count,
    (SELECT count(*) FROM vendor_users u WHERE u.vendor_id = v.id AND u.password_hash IS NULL)::int AS pending_invites
FROM vendors v
WHERE sqlc.narg(search)::text IS NULL
   OR v.name ILIKE '%' || sqlc.narg(search)::text || '%'
   OR v.slug ILIKE '%' || sqlc.narg(search)::text || '%'
ORDER BY v.name;

-- name: UpdateVendor :one
UPDATE vendors SET name = $2, is_active = $3 WHERE id = $1 RETURNING *;

-- name: ListVendorBranchesForAdmin :many
-- One row per branch with what the admin needs to see at a glance: is it
-- set up (store details, storefront key) and how did the last product sync go.
SELECT
    b.id, b.name, b.host, b.is_active, b.storefront_api_key_prefix,
    EXISTS (SELECT 1 FROM branch_info bi WHERE bi.branch_id = b.id) AS has_store_details,
    ss.last_run_at AS last_sync_at,
    ss.last_run_status AS last_sync_status,
    ss.last_run_error AS last_sync_error
FROM branches b
LEFT JOIN sync_state ss ON ss.branch_id = b.id AND ss.resource = 'product'
WHERE b.vendor_id = $1
ORDER BY b.name;

-- name: ListVendorUsersForAdmin :many
-- invite_expires_at / invite_used_at describe the newest invite, if any.
SELECT
    u.id, u.email, u.name, u.role, u.is_active, u.last_login_at, u.locked_until,
    (u.password_hash IS NOT NULL)::boolean AS has_password,
    li.expires_at AS invite_expires_at,
    li.used_at AS invite_used_at
FROM vendor_users u
LEFT JOIN LATERAL (
    SELECT i.expires_at, i.used_at FROM vendor_invites i
    WHERE i.user_id = u.id
    ORDER BY i.created_at DESC
    LIMIT 1
) li ON true
WHERE u.vendor_id = $1
ORDER BY (u.role = 'owner') DESC, u.name;

-- Overview ---------------------------------------------------------------------------

-- name: GetPlatformCounts :one
SELECT
    (SELECT count(*) FROM vendors WHERE is_active)::int AS active_vendors,
    (SELECT count(*) FROM branches WHERE is_active)::int AS active_branches,
    (SELECT count(*) FROM branches b
        WHERE b.is_active AND b.storefront_api_key_hash IS NOT NULL
          AND EXISTS (SELECT 1 FROM sync_state s WHERE s.branch_id = b.id AND s.resource = 'product' AND s.last_run_status = 'success')
    )::int AS live_branches,
    (SELECT count(*) FROM vendor_users WHERE password_hash IS NULL)::int AS pending_invites,
    (SELECT count(*) FROM sync_state s JOIN branches b ON b.id = s.branch_id
        WHERE b.is_active AND s.last_run_status = 'error')::int AS failed_syncs;

-- name: ListFailedSyncs :many
SELECT b.id AS branch_id, b.name AS branch_name, v.id AS vendor_id, v.name AS vendor_name,
       s.resource, s.last_run_at, s.last_run_error
FROM sync_state s
JOIN branches b ON b.id = s.branch_id
JOIN vendors v ON v.id = b.vendor_id
WHERE b.is_active AND s.last_run_status = 'error'
ORDER BY s.last_run_at DESC;

-- name: ListBranchesNeedingSetup :many
-- Active branches missing a piece needed to go live. Each flag is one
-- "needs attention" item.
SELECT * FROM (
    SELECT
        b.id AS branch_id, b.name AS branch_name, v.id AS vendor_id, v.name AS vendor_name, b.created_at,
        NOT EXISTS (SELECT 1 FROM sync_state s WHERE s.branch_id = b.id) AS never_synced,
        (b.storefront_api_key_hash IS NULL)::boolean AS no_storefront_key,
        NOT EXISTS (SELECT 1 FROM branch_info bi WHERE bi.branch_id = b.id) AS no_store_details
    FROM branches b
    JOIN vendors v ON v.id = b.vendor_id
    WHERE b.is_active AND v.is_active
) t
WHERE never_synced OR no_storefront_key OR no_store_details
ORDER BY created_at DESC;

-- name: ListPendingVendorInvites :many
SELECT u.id AS user_id, u.name AS user_name, u.email, v.id AS vendor_id, v.name AS vendor_name,
       li.expires_at AS invite_expires_at
FROM vendor_users u
JOIN vendors v ON v.id = u.vendor_id
LEFT JOIN LATERAL (
    SELECT i.expires_at FROM vendor_invites i
    WHERE i.user_id = u.id AND i.used_at IS NULL
    ORDER BY i.created_at DESC
    LIMIT 1
) li ON true
WHERE u.password_hash IS NULL AND u.is_active AND v.is_active
ORDER BY u.created_at DESC;

-- name: ListVendorsWithoutOwner :many
-- Active vendors nobody can log in to as owner yet.
SELECT v.id, v.name, v.created_at
FROM vendors v
WHERE v.is_active
  AND NOT EXISTS (
      SELECT 1 FROM vendor_users u
      WHERE u.vendor_id = v.id AND u.role = 'owner' AND u.is_active AND u.password_hash IS NOT NULL
  )
ORDER BY v.created_at DESC;

-- name: ListLockedVendorUsers :many
SELECT u.id AS user_id, u.name AS user_name, u.email, u.locked_until, v.id AS vendor_id, v.name AS vendor_name
FROM vendor_users u
JOIN vendors v ON v.id = u.vendor_id
WHERE u.locked_until > now()
ORDER BY u.locked_until DESC;
