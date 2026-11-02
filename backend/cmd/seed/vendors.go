package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
)

// Vendors, vendor portal logins and demo orders.

type seedVendor struct {
	slug, name string
}

// "grocery-shop" is also the slug migration 0009 gives the default vendor it
// backfills onto pre-existing branches, so seeding adopts that row.
var seedVendors = []seedVendor{
	{"grocery-shop", "Grocery Shop Group"},
	{"grocery-shop-leeds", "Grocery Shop Leeds Ltd"},
}

type seedUser struct {
	vendor   string // vendor slug
	email    string
	name     string
	role     string
	branches []string // branch hosts; owners get every branch implicitly
}

var seedUsers = []seedUser{
	{"grocery-shop", "owner@group.example", "Adam Ahmed", "owner", nil},
	{"grocery-shop", "manager.stratford@group.example", "Sana Malik", "manager", []string{"stratford-demo"}},
	{"grocery-shop", "staff.ilford@group.example", "Bilal Khan", "staff", []string{"ilford-demo"}},
	{"grocery-shop-leeds", "owner@leeds.example", "Imran Hussain", "owner", nil},
}

// Every seeded login shares this password unless SEED_VENDOR_PASSWORD is
// set. Re-running the seed resets it (and clears any lockout).
const defaultSeedPassword = "shop-demo-2026"

func upsertVendor(ctx context.Context, pool *pgxpool.Pool, v seedVendor) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO vendors (slug, name) VALUES ($1, $2)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`,
		v.slug, v.name,
	).Scan(&id)
	return id, err
}

func seedVendorUsers(ctx context.Context, pool *pgxpool.Pool, vendorIDs, branchIDs map[string]uuid.UUID) error {
	password := os.Getenv("SEED_VENDOR_PASSWORD")
	if password == "" {
		password = defaultSeedPassword
	}

	for _, u := range seedUsers {
		hash, err := crypto.HashPassword(password)
		if err != nil {
			return err
		}
		vendorID := vendorIDs[u.vendor]

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		var userID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO vendor_users (vendor_id, email, name, password_hash, role)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (email) DO UPDATE SET
				vendor_id = EXCLUDED.vendor_id,
				name = EXCLUDED.name,
				password_hash = EXCLUDED.password_hash,
				role = EXCLUDED.role,
				is_active = true,
				failed_login_count = 0,
				locked_until = NULL
			RETURNING id`,
			vendorID, u.email, u.name, hash, u.role,
		).Scan(&userID)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("upsert %s: %w", u.email, err)
		}

		// Replace branch assignments wholesale so edits to seedUsers stick.
		if _, err := tx.Exec(ctx, `DELETE FROM vendor_user_branches WHERE user_id = $1`, userID); err != nil {
			tx.Rollback(ctx)
			return err
		}
		for _, host := range u.branches {
			if _, err := tx.Exec(ctx, `
				INSERT INTO vendor_user_branches (vendor_id, user_id, branch_id) VALUES ($1, $2, $3)`,
				vendorID, userID, branchIDs[host],
			); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("assign %s to %s: %w", u.email, host, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	log.Printf("vendor portal logins ready (password: %s):", password)
	for _, u := range seedUsers {
		scope := "all branches of " + u.vendor
		if len(u.branches) > 0 {
			scope = fmt.Sprint(u.branches)
		}
		log.Printf("  %-34s %-8s %s", u.email, u.role, scope)
	}
	return nil
}

type seedCustomer struct {
	name, email, phone string
}

// Phone numbers are from Ofcom's reserved mobile drama range (07700 900xxx).
var seedCustomers = []seedCustomer{
	{"Aisha Rahman", "aisha.rahman@example.com", "07700 900101"},
	{"Yusuf Patel", "yusuf.patel@example.com", "07700 900102"},
	{"Fatima Begum", "fatima.begum@example.com", "07700 900103"},
	{"Omar Siddiqui", "omar.siddiqui@example.com", "07700 900104"},
	{"Zainab Ali", "zainab.ali@example.com", "07700 900105"},
	{"Hamza Qureshi", "hamza.qureshi@example.com", "07700 900106"},
}

type seedOrderLine struct {
	product  int // index into seedProducts
	quantity int
}

type seedOrder struct {
	key        string // idempotency key suffix, unique per branch
	status     string
	fulfilment string // collection | delivery
	dayOffset  int    // fulfilment date relative to today
	age        time.Duration
	customer   int // index into seedCustomers
	lines      []seedOrderLine
	notes      string // from the customer
	reason     string // rejection reason / cancellation note
}

// One order in every status, for both fulfilment types. Delivery orders are
// skipped for branches that don't deliver.
var seedOrderTemplates = []seedOrder{
	{"pending-1", "pending", "collection", 0, time.Hour, 0, []seedOrderLine{{0, 1}, {16, 2}}, "", ""},
	{"pending-2", "pending", "delivery", 1, 30 * time.Minute, 1, []seedOrderLine{{8, 2}, {48, 1}}, "Please ring the bell, flat 3", ""},
	{"accepted-1", "accepted", "collection", 0, 3 * time.Hour, 2, []seedOrderLine{{11, 2}}, "", ""},
	{"picking-1", "picking", "delivery", 0, 5 * time.Hour, 3, []seedOrderLine{{19, 1}, {32, 2}, {55, 1}}, "", ""},
	{"ready-1", "ready", "collection", 0, 6 * time.Hour, 4, []seedOrderLine{{1, 3}}, "Cut into curry pieces please", ""},
	{"out-1", "out_for_delivery", "delivery", 0, 8 * time.Hour, 5, []seedOrderLine{{12, 1}, {49, 2}}, "", ""},
	{"completed-1", "completed", "collection", -1, 30 * time.Hour, 0, []seedOrderLine{{24, 1}, {17, 1}}, "", ""},
	{"completed-2", "completed", "delivery", -2, 54 * time.Hour, 2, []seedOrderLine{{9, 1}, {3, 4}}, "", ""},
	{"rejected-1", "rejected", "collection", -1, 28 * time.Hour, 3, []seedOrderLine{{25, 1}}, "", "Goat leg out of stock until Friday"},
	{"cancelled-1", "cancelled", "delivery", -3, 75 * time.Hour, 4, []seedOrderLine{{5, 2}}, "", "Customer phoned to cancel"},
	{"noshow-1", "no_show", "collection", -2, 50 * time.Hour, 1, []seedOrderLine{{20, 1}}, "", ""},
}

// statusPath is the sequence of status changes that leads to each final
// status, so the seeded audit trail looks like real usage.
func statusPath(final, fulfilment string) []string {
	switch final {
	case "accepted":
		return []string{"accepted"}
	case "picking":
		return []string{"accepted", "picking"}
	case "ready":
		return []string{"accepted", "picking", "ready"}
	case "out_for_delivery":
		return []string{"accepted", "picking", "ready", "out_for_delivery"}
	case "completed":
		if fulfilment == "delivery" {
			return []string{"accepted", "picking", "ready", "out_for_delivery", "completed"}
		}
		return []string{"accepted", "picking", "ready", "completed"}
	case "rejected":
		return []string{"rejected"}
	case "cancelled":
		return []string{"accepted", "cancelled"}
	case "no_show":
		return []string{"accepted", "picking", "ready", "no_show"}
	}
	return nil // pending
}

// seedOrders inserts any of the demo orders this branch doesn't have yet and
// returns how many it created. Existing seeded orders are left untouched, so
// status changes made in the portal survive a re-seed.
func seedOrders(ctx context.Context, pool *pgxpool.Pool, b seedBranch, branchID, vendorID uuid.UUID, productIDs []uuid.UUID) (int, error) {
	var actorID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT id FROM vendor_users WHERE vendor_id = $1 AND role = 'owner' ORDER BY email LIMIT 1`,
		vendorID,
	).Scan(&actorID); err != nil {
		return 0, fmt.Errorf("find owner for status events: %w", err)
	}

	created := 0
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, o := range seedOrderTemplates {
		if o.fulfilment == "delivery" && !b.info.delivery {
			continue
		}
		ok, err := insertSeedOrder(ctx, pool, b, branchID, actorID, productIDs, o, today)
		if err != nil {
			return created, fmt.Errorf("order %s: %w", o.key, err)
		}
		if ok {
			created++
		}
	}
	return created, nil
}

func insertSeedOrder(ctx context.Context, pool *pgxpool.Pool, b seedBranch, branchID, actorID uuid.UUID, productIDs []uuid.UUID, o seedOrder, today time.Time) (bool, error) {
	var subtotal int64
	for _, l := range o.lines {
		subtotal += (seedProducts[l.product].priceP + b.priceDeltaP) * int64(l.quantity)
	}
	var deliveryFee int64
	var line1, city, postcode *string
	if o.fulfilment == "delivery" {
		free := b.info.deliveryFreeOverP != nil && subtotal >= *b.info.deliveryFreeOverP
		if !free {
			deliveryFee = b.info.deliveryFeeP
		}
		l1, c, pc := "1 Example Street", b.info.city, b.info.deliveryPostcodePrefixes[0]+" 1AA"
		line1, city, postcode = &l1, &c, &pc
	}

	path := statusPath(o.status, o.fulfilment)
	placedAt := time.Now().Add(-o.age)
	updatedAt := placedAt.Add(time.Duration(len(path)) * 20 * time.Minute)
	if updatedAt.After(time.Now()) {
		updatedAt = time.Now()
	}

	customer := seedCustomers[o.customer]
	key := "seed-" + o.key
	hash := sha256.Sum256([]byte(key))
	var notes, rejection *string
	if o.notes != "" {
		notes = &o.notes
	}
	if o.status == "rejected" {
		rejection = &o.reason
	}

	// Acceptance timestamps (migration 0008). Real orders get
	// ORDER_ACCEPT_TIMEOUT_MINUTES to be accepted; seeded pending ones get an
	// hour from seeding, so the expiry job doesn't cancel the demo's "New"
	// orders before anyone has looked at them.
	// Past orders were accepted 20 minutes in (see the history below), so
	// give them a window that covers that.
	acceptDeadline := placedAt.Add(30 * time.Minute)
	if o.status == "pending" {
		acceptDeadline = time.Now().Add(time.Hour)
	}
	var acceptedAt, cancelledAt *time.Time
	for i, step := range path {
		at := placedAt.Add(time.Duration(i+1) * 20 * time.Minute)
		switch step {
		case "accepted":
			acceptedAt = &at
		case "rejected", "cancelled":
			cancelledAt = &at
		}
	}

	// Open collection orders carry the code the customer shows at the
	// counter; ready ones also have a collect-by time.
	var collectionCode *string
	var readyAt, collectBy *time.Time
	if o.fulfilment == "collection" && slices.Contains([]string{"accepted", "picking", "ready"}, o.status) {
		code := fmt.Sprintf("%06d", rand.IntN(1_000_000))
		collectionCode = &code
	}
	if o.fulfilment == "collection" && o.status == "ready" {
		at, by := updatedAt, updatedAt.Add(60*time.Minute)
		readyAt, collectBy = &at, &by
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var orderID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (
			branch_id, reference, status, fulfilment_type, fulfilment_date,
			customer_name, customer_email, customer_phone,
			address_line_1, address_city, address_postcode,
			notes, rejection_reason, subtotal, delivery_fee, total,
			idempotency_key, request_hash, created_at, updated_at,
			accept_deadline, accepted_at, cancelled_at,
			collection_code, ready_at, collect_by, terms_version, terms_accepted_at
		) VALUES (
			$1, 'TH-' || nextval('order_reference_seq'), $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13, $14, $13::bigint + $14::bigint,
			$15, $16, $17, $18,
			$19, $20, $21,
			$22, $23, $24, 'seed', $17
		)
		ON CONFLICT (branch_id, idempotency_key) DO NOTHING
		RETURNING id`,
		branchID, o.status, o.fulfilment, today.AddDate(0, 0, o.dayOffset),
		customer.name, customer.email, customer.phone,
		line1, city, postcode,
		notes, rejection, subtotal, deliveryFee,
		key, hash[:], placedAt, updatedAt,
		acceptDeadline, acceptedAt, cancelledAt,
		collectionCode, readyAt, collectBy,
	).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // already seeded
	}
	if err != nil {
		return false, err
	}

	for i, l := range o.lines {
		p := seedProducts[l.product]
		price := p.priceP + b.priceDeltaP
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, product_plu, unit_price, quantity, line_total, charged_total)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
			orderID, productIDs[l.product], p.name, fmt.Sprintf("SEED%04d", l.product+1), price, l.quantity, price*int64(l.quantity),
		); err != nil {
			return false, fmt.Errorf("item %d: %w", i, err)
		}
	}

	from := "pending"
	for i, to := range path {
		var note *string
		if i == len(path)-1 && o.reason != "" {
			note = &o.reason
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_status_events (order_id, from_status, to_status, actor_user_id, note, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			orderID, from, to, actorID, note, placedAt.Add(time.Duration(i+1)*20*time.Minute),
		); err != nil {
			return false, fmt.Errorf("status event %s: %w", to, err)
		}
		from = to
	}

	return true, tx.Commit(ctx)
}
