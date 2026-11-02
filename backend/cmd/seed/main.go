// Command seed populates the database with demo vendors and branches, a
// synthetic halal butcher product catalogue (50+ products across 7
// departments), storefront content (descriptions/images), vendor portal
// logins, demo orders and a platform admin login for local development.
//
// It bypasses the inventory sync entirely - these are fixture rows
// for UI integration, not real vendor data, and are never touched by the
// sync engine. Every write is an upsert keyed on a natural key (vendor slug,
// branch host, product PLU, user email, order idempotency key), so the
// command is safe to re-run.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/seed
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
)

type seedDepartment struct {
	uuid  string
	name  string
	blurb string
}

var departments = []seedDepartment{
	{"dept-chicken", "Chicken", "Halal-certified, farm-raised chicken, hand-cut fresh in-store."},
	{"dept-beef", "Beef", "Grass-fed, halal-certified beef, dry-aged for flavour."},
	{"dept-lamb", "Lamb", "Tender, halal-certified lamb sourced from trusted UK farms."},
	{"dept-goat", "Goat & Mutton", "Traditional halal goat and mutton cuts, bone-in for maximum flavour."},
	{"dept-mince", "Mince", "Freshly minced daily, never frozen, halal-certified."},
	{"dept-offal", "Offal", "Traditional halal offal cuts, cleaned and trimmed in-house."},
	{"dept-marinated", "Marinated & Ready to Cook", "Marinated and ready to cook - halal-certified, no prep needed."},
}

type seedProduct struct {
	department string // department uuid, must match a departments[i].uuid
	name       string
	priceP     int64 // base price in pence (branch A)
	costP      int64 // cost price in pence
}

var seedProducts = []seedProduct{
	// Chicken
	{"dept-chicken", "Whole Chicken", 650, 400},
	{"dept-chicken", "Chicken Breast Fillets", 750, 480},
	{"dept-chicken", "Chicken Thighs (Bone-In)", 550, 320},
	{"dept-chicken", "Chicken Drumsticks", 450, 260},
	{"dept-chicken", "Chicken Wings", 400, 220},
	{"dept-chicken", "Chicken Leg Quarters", 500, 300},
	{"dept-chicken", "Chicken Breast Mini Fillets", 700, 430},
	{"dept-chicken", "Chicken Liver", 300, 150},

	// Beef
	{"dept-beef", "Beef Mince (Standard)", 700, 480},
	{"dept-beef", "Beef Brisket", 950, 650},
	{"dept-beef", "Beef Shin", 850, 550},
	{"dept-beef", "Beef Sirloin Steak", 1600, 1100},
	{"dept-beef", "Beef Rib Eye Steak", 1800, 1250},
	{"dept-beef", "Beef Diced (Stewing)", 900, 600},
	{"dept-beef", "Beef Short Ribs", 1100, 750},
	{"dept-beef", "Beef Ox Tail", 800, 500},

	// Lamb
	{"dept-lamb", "Lamb Chops", 1400, 950},
	{"dept-lamb", "Lamb Leg (Whole)", 1600, 1100},
	{"dept-lamb", "Lamb Shoulder", 1300, 900},
	{"dept-lamb", "Lamb Diced (Curry Cut)", 1200, 800},
	{"dept-lamb", "Lamb Shanks", 1250, 850},
	{"dept-lamb", "Lamb Ribs", 1000, 650},
	{"dept-lamb", "Lamb Neck Fillet", 1100, 700},
	{"dept-lamb", "Lamb Kidney", 350, 180},

	// Goat & Mutton
	{"dept-goat", "Goat Meat Diced", 1300, 900},
	{"dept-goat", "Goat Leg", 1500, 1050},
	{"dept-goat", "Goat Shoulder", 1350, 950},
	{"dept-goat", "Mutton Chops", 1450, 1000},
	{"dept-goat", "Mutton Diced", 1250, 850},
	{"dept-goat", "Goat Ribs", 1050, 700},
	{"dept-goat", "Mutton Leg", 1550, 1080},
	{"dept-goat", "Goat Liver", 380, 200},

	// Mince
	{"dept-mince", "Beef Mince Extra Lean", 800, 550},
	{"dept-mince", "Lamb Mince", 1200, 800},
	{"dept-mince", "Chicken Mince", 650, 400},
	{"dept-mince", "Goat Mince", 1250, 850},
	{"dept-mince", "Beef Mince 20% Fat", 650, 430},
	{"dept-mince", "Lamb & Beef Mixed Mince", 950, 620},
	{"dept-mince", "Turkey Mince", 700, 450},
	{"dept-mince", "Mutton Mince", 1200, 800},

	// Offal
	{"dept-offal", "Beef Liver", 320, 160},
	{"dept-offal", "Beef Kidney", 300, 150},
	{"dept-offal", "Beef Tripe", 350, 180},
	{"dept-offal", "Lamb Liver", 380, 200},
	{"dept-offal", "Lamb Heart", 320, 160},
	{"dept-offal", "Beef Heart", 300, 150},
	{"dept-offal", "Lamb Tongue", 450, 250},
	{"dept-offal", "Beef Tongue", 500, 280},

	// Marinated & Ready to Cook
	{"dept-marinated", "Chicken Tikka Marinated", 850, 550},
	{"dept-marinated", "Lamb Tikka Marinated", 1450, 1000},
	{"dept-marinated", "Chicken Tandoori Marinated", 850, 550},
	{"dept-marinated", "Beef Seekh Kebab Mix", 950, 620},
	{"dept-marinated", "Lamb Seekh Kebab Mix", 1300, 900},
	{"dept-marinated", "Chicken Wings Peri Peri Marinated", 550, 320},
	{"dept-marinated", "Marinated Lamb Chops", 1550, 1050},
	{"dept-marinated", "BBQ Marinated Chicken Thighs", 700, 450},
}

type seedBranch struct {
	vendor      string // vendor slug, must match a seedVendors[i].slug
	name        string
	host        string
	priceDeltaP int64 // pence added to every product's base price at this branch
	info        seedBranchInfo
}

type seedBranchInfo struct {
	phone, email                    string
	addressLine1, city, postalCode  string
	collection, delivery            bool
	deliveryFeeP, deliveryMinOrderP int64
	deliveryFreeOverP               *int64 // nil = no free-delivery threshold
	deliveryPostcodePrefixes        []string
}

type seedOpeningHour struct {
	dayOfWeek       int16 // ISO: 1 = Monday ... 7 = Sunday
	openAt, closeAt string
}

func pence(p int64) *int64 { return &p }

// Phone numbers are from Ofcom's reserved drama range (020 7946 0xxx), so
// they can never ring a real number.
var seedBranches = []seedBranch{
	{"grocery-shop", "Grocery Stratford", "stratford-demo", 0, seedBranchInfo{
		phone: "020 7946 0101", email: "stratford@groceryshop.example",
		addressLine1: "12 Broadway", city: "London", postalCode: "E15 1NG",
		collection: true, delivery: true,
		deliveryFeeP: 399, deliveryMinOrderP: 2000, deliveryFreeOverP: pence(5000),
		deliveryPostcodePrefixes: []string{"E15", "E20", "E13", "E7"},
	}},
	// Collection-only, no free-delivery threshold - exercises the other
	// checkout branches in the UI.
	{"grocery-shop", "Grocery Ilford", "ilford-demo", 50, seedBranchInfo{
		phone: "020 7946 0202", email: "ilford@groceryshop.example",
		addressLine1: "88 High Road", city: "Ilford", postalCode: "IG1 1DB",
		collection: true, delivery: false,
	}},
	// Owned by a separate vendor - proves one owner can't see another's
	// branch. Phone is in Ofcom's Leeds drama range (0113 496 0xxx).
	{"grocery-shop-leeds", "Grocery Leeds", "leeds-demo", 25, seedBranchInfo{
		phone: "0113 496 0303", email: "leeds@groceryshop.example",
		addressLine1: "5 Harehills Lane", city: "Leeds", postalCode: "LS8 4HA",
		collection: true, delivery: true,
		deliveryFeeP: 299, deliveryMinOrderP: 1500,
		deliveryPostcodePrefixes: []string{"LS7", "LS8", "LS9"},
	}},
}

// Same weekly schedule for every seeded branch: Mon-Sat 9-8, Sun 10-6.
var seedOpeningHours = []seedOpeningHour{
	{1, "09:00", "20:00"}, {2, "09:00", "20:00"}, {3, "09:00", "20:00"},
	{4, "09:00", "20:00"}, {5, "09:00", "20:00"}, {6, "09:00", "20:00"},
	{7, "10:00", "18:00"},
}

// upsellProducts names a handful of products every seeded branch features
// as an upsell, demonstrating branch_product_content.
var upsellProducts = map[string]bool{
	"Chicken Tikka Marinated": true,
	"Beef Rib Eye Steak":      true,
	"Lamb Chops":              true,
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	deptUUIDs := make([]string, len(departments))
	for i, d := range departments {
		deptUUIDs[i] = d.uuid
	}

	for i, d := range departments {
		if err := upsertCategory(ctx, pool, d.uuid, d.name, i); err != nil {
			log.Fatalf("upsert category %s: %v", d.uuid, err)
		}
	}
	log.Printf("categories ready: %d", len(departments))

	vendorIDs := make(map[string]uuid.UUID, len(seedVendors))
	for _, v := range seedVendors {
		id, err := upsertVendor(ctx, pool, v)
		if err != nil {
			log.Fatalf("upsert vendor %s: %v", v.slug, err)
		}
		vendorIDs[v.slug] = id
		log.Printf("vendor ready: %s (%s)", v.name, id)

		added, err := seedPaymentAccount(ctx, pool, id)
		if err != nil {
			log.Fatalf("seed payment account for %s: %v", v.slug, err)
		}
		if added {
			log.Printf("  fake payment account ready (webhook secret: %s)", fakeWebhookSecret)
		} else {
			log.Printf("  payment account: already active, unchanged")
		}
	}

	branchIDs := make(map[string]uuid.UUID, len(seedBranches))
	for _, b := range seedBranches {
		id, err := upsertBranch(ctx, pool, b, vendorIDs[b.vendor], deptUUIDs)
		if err != nil {
			log.Fatalf("upsert branch %s: %v", b.host, err)
		}
		branchIDs[b.host] = id
		log.Printf("branch ready: %s (%s)", b.name, id)

		issued, err := issueStorefrontKeyIfMissing(ctx, pool, id)
		if err != nil {
			log.Fatalf("issue storefront key for %s: %v", b.host, err)
		}
		if issued != "" {
			log.Printf("  storefront api key (shown once): %s", issued)
		} else {
			log.Printf("  storefront api key: already issued, unchanged")
		}

		if err := upsertBranchInfo(ctx, pool, id, b.info); err != nil {
			log.Fatalf("upsert branch_info %s: %v", b.host, err)
		}
		log.Printf("  branch info, opening hours and holiday closure ready")
	}

	blurbByDept := make(map[string]string, len(departments))
	for _, d := range departments {
		blurbByDept[d.uuid] = d.blurb
	}

	productIDs := make([]uuid.UUID, len(seedProducts))
	productCount := 0
	for i, p := range seedProducts {
		plu := fmt.Sprintf("SEED%04d", i+1)
		barcode := fmt.Sprintf("500011%06d", i+1)

		productID, err := upsertProduct(ctx, pool, plu, barcode, p)
		if err != nil {
			log.Fatalf("upsert product %s (%s): %v", plu, p.name, err)
		}
		productIDs[i] = productID

		description := fmt.Sprintf("%s. %s", p.name, blurbByDept[p.department])
		// 1-3 photos per product (varies by PLU) so the storefront carousel
		// has something real to demo, not just single-image products.
		imageCount := 1 + i%3
		imageURLs := make([]string, imageCount)
		for n := range imageURLs {
			imageURLs[n] = fmt.Sprintf("https://picsum.photos/seed/%s-%d/640/480", plu, n+1)
		}
		if err := upsertProductContent(ctx, pool, productID, description, imageURLs); err != nil {
			log.Fatalf("upsert product_content %s: %v", plu, err)
		}

		for _, b := range seedBranches {
			branchID := branchIDs[b.host]
			if err := upsertBranchProduct(ctx, pool, branchID, productID, plu, p, b.priceDeltaP); err != nil {
				log.Fatalf("upsert branch_product %s/%s: %v", b.host, plu, err)
			}
			if upsellProducts[p.name] {
				if err := upsertBranchProductUpsell(ctx, pool, branchID, productID); err != nil {
					log.Fatalf("upsert branch_product_content %s/%s: %v", b.host, plu, err)
				}
			}
		}

		productCount++
	}

	log.Printf("seeded %d branches, %d products (%d branch_product links)",
		len(seedBranches), productCount, len(seedBranches)*productCount)

	if err := seedVendorUsers(ctx, pool, vendorIDs, branchIDs); err != nil {
		log.Fatalf("seed vendor users: %v", err)
	}

	for _, b := range seedBranches {
		n, err := seedOrders(ctx, pool, b, branchIDs[b.host], vendorIDs[b.vendor], productIDs)
		if err != nil {
			log.Fatalf("seed orders for %s: %v", b.host, err)
		}
		log.Printf("orders ready for %s: %d new", b.name, n)
	}

	if err := seedPlatform(ctx, pool, vendorIDs, branchIDs); err != nil {
		log.Fatalf("seed platform admin: %v", err)
	}
}

func upsertBranch(ctx context.Context, pool *pgxpool.Pool, b seedBranch, vendorID uuid.UUID, deptUUIDs []string) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO branches (name, host, api_username, api_key_cipher, is_active, allowed_secondary_department_uuids, allowed_product_refs, vendor_id, storefront_url)
		VALUES ($1, $2, $3, $4, true, $5, '{}', $6, 'http://localhost:3000')
		ON CONFLICT (host) DO UPDATE SET
			name = EXCLUDED.name,
			allowed_secondary_department_uuids = EXCLUDED.allowed_secondary_department_uuids,
			vendor_id = EXCLUDED.vendor_id,
			-- Links in local emails go to the dev storefront.
			storefront_url = coalesce(branches.storefront_url, EXCLUDED.storefront_url),
			updated_at = now()
		RETURNING id`,
		b.name, b.host, "seed-user",
		// Not a real AES-GCM ciphertext - these demo branches have fake
		// hosts and are never expected to actually sync against an inventory system.
		[]byte("seed-placeholder-not-a-real-cipher"),
		deptUUIDs,
		vendorID,
	).Scan(&id)
	return id, err
}

func upsertProduct(ctx context.Context, pool *pgxpool.Pool, plu, barcode string, p seedProduct) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO products (plu, name, product_code, barcode, department_uuid, secondary_department_uuid, open_priced)
		VALUES ($1, $2, $3, $4, $5, $5, false)
		ON CONFLICT (plu) DO UPDATE SET
			name = EXCLUDED.name,
			product_code = EXCLUDED.product_code,
			barcode = EXCLUDED.barcode,
			department_uuid = EXCLUDED.department_uuid,
			secondary_department_uuid = EXCLUDED.secondary_department_uuid,
			updated_at = now()
		RETURNING id`,
		plu, p.name, plu, barcode, p.department,
	).Scan(&id)
	return id, err
}

func upsertProductContent(ctx context.Context, pool *pgxpool.Pool, productID uuid.UUID, description string, imageURLs []string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO product_content (product_id, description, image_urls)
		VALUES ($1, $2, $3)
		ON CONFLICT (product_id) DO UPDATE SET
			description = EXCLUDED.description,
			image_urls = EXCLUDED.image_urls,
			updated_at = now()`,
		productID, description, imageURLs,
	)
	return err
}

func upsertBranchProduct(ctx context.Context, pool *pgxpool.Pool, branchID, productID uuid.UUID, plu string, p seedProduct, priceDeltaP int64) error {
	raw, err := json.Marshal(map[string]any{"seed": true, "plu": plu})
	if err != nil {
		return err
	}
	price := p.priceP + priceDeltaP
	_, err = pool.Exec(ctx, `
		INSERT INTO branch_products (
			branch_id, product_id, external_id, price1, cost_price,
			minimum_stock, maximum_stock, can_be_sold, can_be_sold_special_orders,
			track_stock, track_availability, deleted, source_last_modified, raw
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, true, true, true, false, now(), $8)
		ON CONFLICT (branch_id, external_id) DO UPDATE SET
			price1 = EXCLUDED.price1,
			cost_price = EXCLUDED.cost_price,
			source_last_modified = now(),
			updated_at = now()`,
		branchID, productID, plu, price, p.costP, 5, 100, raw,
	)
	return err
}

func upsertBranchProductUpsell(ctx context.Context, pool *pgxpool.Pool, branchID, productID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO branch_product_content (branch_id, product_id, is_upsell)
		VALUES ($1, $2, true)
		ON CONFLICT (branch_id, product_id) DO UPDATE SET
			is_upsell = true,
			updated_at = now()`,
		branchID, productID,
	)
	return err
}

func upsertCategory(ctx context.Context, pool *pgxpool.Pool, departmentUUID, name string, sortOrder int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO categories (department_uuid, name, sort_order)
		VALUES ($1, $2, $3)
		ON CONFLICT (department_uuid) DO UPDATE SET
			name = EXCLUDED.name,
			sort_order = EXCLUDED.sort_order,
			updated_at = now()`,
		departmentUUID, name, sortOrder,
	)
	return err
}

// issueStorefrontKeyIfMissing generates and stores a new storefront API key
// only if this branch doesn't already have one, returning the plaintext key
// when it issued a new one (empty string if a key already existed) - so
// re-running the seed never invalidates a key already wired into a frontend.
func issueStorefrontKeyIfMissing(ctx context.Context, pool *pgxpool.Pool, branchID uuid.UUID) (string, error) {
	plaintext, err := crypto.GenerateAPIKey()
	if err != nil {
		return "", err
	}
	hash := crypto.HashAPIKey(plaintext)
	prefix := crypto.KeyPrefix(plaintext)

	tag, err := pool.Exec(ctx, `
		UPDATE branches
		SET storefront_api_key_hash = $2, storefront_api_key_prefix = $3, updated_at = now()
		WHERE id = $1 AND storefront_api_key_hash IS NULL`,
		branchID, hash, prefix,
	)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", nil
	}
	return plaintext, nil
}

// upsertBranchInfo writes the branch's storefront details and replaces its
// weekly schedule wholesale (delete + insert in one tx, so a re-run with a
// changed seedOpeningHours never leaves stale periods behind). It also adds
// one closed-all-day exception (next Christmas Day) to demo the override.
func upsertBranchInfo(ctx context.Context, pool *pgxpool.Pool, branchID uuid.UUID, info seedBranchInfo) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO branch_info (
			branch_id, phone, email, address_line_1, city, postal_code,
			collection, delivery, delivery_fee, delivery_free_over, delivery_min_order, delivery_postcode_prefixes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (branch_id) DO UPDATE SET
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			address_line_1 = EXCLUDED.address_line_1,
			city = EXCLUDED.city,
			postal_code = EXCLUDED.postal_code,
			collection = EXCLUDED.collection,
			delivery = EXCLUDED.delivery,
			delivery_fee = EXCLUDED.delivery_fee,
			delivery_free_over = EXCLUDED.delivery_free_over,
			delivery_min_order = EXCLUDED.delivery_min_order,
			delivery_postcode_prefixes = EXCLUDED.delivery_postcode_prefixes`,
		branchID, info.phone, info.email, info.addressLine1, info.city, info.postalCode,
		info.collection, info.delivery, info.deliveryFeeP, info.deliveryFreeOverP, info.deliveryMinOrderP,
		append([]string{}, info.deliveryPostcodePrefixes...), // non-nil: column is NOT NULL
	)
	if err != nil {
		return fmt.Errorf("branch_info: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM branch_opening_hours WHERE branch_id = $1`, branchID); err != nil {
		return fmt.Errorf("clear opening hours: %w", err)
	}
	for _, h := range seedOpeningHours {
		if _, err := tx.Exec(ctx, `
			INSERT INTO branch_opening_hours (branch_id, day_of_week, open_time, close_time)
			VALUES ($1, $2, $3::time, $4::time)`,
			branchID, h.dayOfWeek, h.openAt, h.closeAt,
		); err != nil {
			return fmt.Errorf("opening hours day %d: %w", h.dayOfWeek, err)
		}
	}

	// Next 25 December (this year's if it hasn't passed yet). ON CONFLICT
	// targets the partial unique index on closed days.
	if _, err := tx.Exec(ctx, `
		INSERT INTO branch_hours_exception (branch_id, date, closed, note)
		VALUES (
			$1,
			CASE WHEN make_date(EXTRACT(YEAR FROM current_date)::int, 12, 25) >= current_date
				THEN make_date(EXTRACT(YEAR FROM current_date)::int, 12, 25)
				ELSE make_date(EXTRACT(YEAR FROM current_date)::int + 1, 12, 25)
			END,
			true,
			'Christmas Day'
		)
		ON CONFLICT (branch_id, date) WHERE closed DO NOTHING`,
		branchID,
	); err != nil {
		return fmt.Errorf("hours exception: %w", err)
	}

	return tx.Commit(ctx)
}
