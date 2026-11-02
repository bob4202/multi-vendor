package sync

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
	"ecommerce/backend/internal/inventory/fake"
)

// TestEngineSyncsThroughProvider runs a whole branch sync through a fake
// provider: products matching the branch filter and customers are saved,
// the cursor moves to the newest change, and a second run with nothing new
// writes nothing.
func TestEngineSyncsThroughProvider(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	q := db.New(pool)

	box, err := crypto.NewSecretBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := box.Encrypt("key")
	if err != nil {
		t.Fatal(err)
	}

	str := func(s string) *string { return &s }
	price := int64(450)
	changed := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	suffix := strings.ToLower(uuid.NewString()[:8])

	source := &fake.Provider{
		ProductList: []inventory.Product{
			{ExternalID: "e1-" + suffix, PLU: str("9" + suffix), Name: str("Apples"), SecondaryDepartmentID: str("dept-fruit"),
				Prices: [10]*int64{&price}, LastModified: changed},
			{ExternalID: "e2-" + suffix, Name: str("Shampoo"), SecondaryDepartmentID: str("dept-other"), LastModified: changed},
		},
		CustomerList: []inventory.Customer{
			{ExternalID: "c1-" + suffix, Name: str("Ann"), Email: str("ann@example.com"), LastModified: changed},
		},
	}
	registry := inventory.Registry{"testsource": func(inventory.Credentials) (inventory.Provider, error) { return source, nil }}
	engine := NewEngine(pool, box, registry, slog.New(slog.NewTextHandler(io.Discard, nil)))

	branch, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Engine Branch", Host: "engine-" + suffix, ApiUsername: "u", ApiKeyCipher: cipher, IsActive: true,
		VendorID: testVendorID(t, ctx, q), InventoryProvider: "testsource",
		AllowedSecondaryDepartmentUuids: []string{"dept-fruit"}, AllowedProductRefs: []string{},
	})
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}

	if err := engine.SyncBranch(ctx, branch); err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertBranchProductExists(t, pool, branch.ID, "e1-"+suffix, true)
	assertBranchProductExists(t, pool, branch.ID, "e2-"+suffix, false) // filtered out

	state, err := q.GetSyncState(ctx, db.GetSyncStateParams{BranchID: branch.ID, Resource: resourceProduct})
	if err != nil {
		t.Fatalf("product sync state: %v", err)
	}
	if !state.Cursor.Valid || !state.Cursor.Time.Equal(changed) {
		t.Errorf("product cursor = %v, want %v", state.Cursor.Time, changed)
	}
	if state.LastRunRecords == nil || *state.LastRunRecords != 1 {
		t.Errorf("product records written = %v, want 1", state.LastRunRecords)
	}

	var customers int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM customers WHERE branch_id = $1`, branch.ID).Scan(&customers); err != nil {
		t.Fatal(err)
	}
	if customers != 1 {
		t.Errorf("customers saved = %d, want 1", customers)
	}

	// Nothing changed since the cursor: the second run writes nothing.
	if err := engine.SyncBranch(ctx, branch); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	state, _ = q.GetSyncState(ctx, db.GetSyncStateParams{BranchID: branch.ID, Resource: resourceProduct})
	if state.LastRunRecords == nil || *state.LastRunRecords != 0 {
		t.Errorf("second run records written = %v, want 0", state.LastRunRecords)
	}
}
