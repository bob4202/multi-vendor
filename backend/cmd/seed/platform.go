package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/portal"
)

// Platform admin login, a pending vendor invite and some audit history, so
// the admin side has something to show.

const seedAdminEmail = "admin@groceryshop.example"

// An invited user who hasn't set a password yet - shows up as "invited" on
// the vendor page and in the overview's needs-attention list.
var seedPendingUser = seedUser{
	vendor: "grocery-shop-leeds", email: "manager.leeds@leeds.example",
	name: "Ayesha Raza", role: "manager", branches: []string{"leeds-demo"},
}

func seedPlatform(ctx context.Context, pool *pgxpool.Pool, vendorIDs, branchIDs map[string]uuid.UUID) error {
	password := os.Getenv("SEED_VENDOR_PASSWORD")
	if password == "" {
		password = defaultSeedPassword
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}

	var adminID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO platform_admins (email, name, password_hash)
		VALUES ($1, 'Platform Admin', $2)
		ON CONFLICT (email) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			is_active = true, failed_login_count = 0, locked_until = NULL
		RETURNING id`,
		seedAdminEmail, hash,
	).Scan(&adminID); err != nil {
		return fmt.Errorf("upsert admin: %w", err)
	}
	log.Printf("platform admin login ready: %s (password: %s)", seedAdminEmail, password)

	if err := seedPendingInvite(ctx, pool, vendorIDs, branchIDs, adminID); err != nil {
		return fmt.Errorf("pending invite: %w", err)
	}
	return seedAuditEvents(ctx, pool, adminID, vendorIDs)
}

// seedPendingInvite creates the invited user, and a fresh invite link if
// they don't have a usable one. The link is printed because only its hash
// is stored - re-running the seed won't show it again while it's valid.
func seedPendingInvite(ctx context.Context, pool *pgxpool.Pool, vendorIDs, branchIDs map[string]uuid.UUID, adminID uuid.UUID) error {
	u := seedPendingUser
	vendorID := vendorIDs[u.vendor]

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO vendor_users (vendor_id, email, name, password_hash, role)
		VALUES ($1, $2, $3, NULL, $4)
		ON CONFLICT (email) DO UPDATE SET vendor_id = EXCLUDED.vendor_id, name = EXCLUDED.name, role = EXCLUDED.role
		RETURNING id`,
		vendorID, u.email, u.name, u.role,
	).Scan(&userID); err != nil {
		return err
	}
	for _, host := range u.branches {
		if _, err := tx.Exec(ctx, `
			INSERT INTO vendor_user_branches (vendor_id, user_id, branch_id) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`,
			vendorID, userID, branchIDs[host],
		); err != nil {
			return err
		}
	}

	var hasPassword, hasUsableInvite bool
	if err := tx.QueryRow(ctx, `
		SELECT password_hash IS NOT NULL,
		       EXISTS (SELECT 1 FROM vendor_invites WHERE user_id = $1 AND used_at IS NULL AND expires_at > now())
		FROM vendor_users WHERE id = $1`,
		userID,
	).Scan(&hasPassword, &hasUsableInvite); err != nil {
		return err
	}

	switch {
	case hasPassword:
		log.Printf("invited user %s already accepted their invite", u.email)
	case hasUsableInvite:
		log.Printf("invited user %s: invite already pending (link was printed when it was created)", u.email)
	default:
		token, err := crypto.GenerateToken(portal.InviteTokenPrefix)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO vendor_invites (user_id, token_hash, created_by_admin_id, expires_at)
			VALUES ($1, $2, $3, $4)`,
			userID, crypto.HashAPIKey(token), adminID, time.Now().Add(portal.InviteLifetime),
		); err != nil {
			return err
		}
		log.Printf("invited user %s: open /vendor/invite/%s to accept (shown once)", u.email, token)
	}
	return tx.Commit(ctx)
}

// seedAuditEvents adds a little history the first time only.
func seedAuditEvents(ctx context.Context, pool *pgxpool.Pool, adminID uuid.UUID, vendorIDs map[string]uuid.UUID) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit_events`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	events := []struct {
		ago                     time.Duration
		action, vendor, summary string
	}{
		{72 * time.Hour, "vendor.created", "grocery-shop", "Created vendor Grocery Shop Group"},
		{48 * time.Hour, "vendor.created", "grocery-shop-leeds", "Created vendor Grocery Shop Leeds Ltd"},
		{2 * time.Hour, "user.created", "grocery-shop-leeds", "Added Ayesha Raza (manager.leeds@leeds.example) as manager of Grocery Shop Leeds Ltd"},
	}
	for _, e := range events {
		if _, err := pool.Exec(ctx, `
			INSERT INTO admin_audit_events (admin_id, action, target_type, target_id, summary, created_at)
			VALUES ($1, $2, 'vendor', $3, $4, $5)`,
			adminID, e.action, vendorIDs[e.vendor].String(), e.summary, time.Now().Add(-e.ago),
		); err != nil {
			return err
		}
	}
	return nil
}
