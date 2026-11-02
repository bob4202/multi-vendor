package sync

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/db"
)

// TestCrossBranchProductDedup verifies the behavior the product model was
// specifically redesigned for: two branches selling the same physical
// product (matched by PLU) collapse into one catalog row with branch-
// specific pricing, while a product with no PLU never merges with anything
// and re-syncing it doesn't create duplicate catalog rows.
func TestCrossBranchProductDedup(t *testing.T) {
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
	vendorID := testVendorID(t, ctx, q)

	// This test isn't exercising the per-branch product filter, so give both
	// branches an explicit product-ref allowlist covering every record they
	// upsert below - a branch with no filter configured skips every product.
	branchA, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Test Branch A", Host: "test-branch-a-" + t.Name(), ApiUsername: "a", ApiKeyCipher: []byte("x"), IsActive: true, VendorID: vendorID,
		AllowedSecondaryDepartmentUuids: []string{},
		AllowedProductRefs:              []string{"a1", "a2"},
	})
	if err != nil {
		t.Fatalf("create branch A: %v", err)
	}
	branchB, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Test Branch B", Host: "test-branch-b-" + t.Name(), ApiUsername: "b", ApiKeyCipher: []byte("x"), IsActive: true, VendorID: vendorID,
		AllowedSecondaryDepartmentUuids: []string{},
		AllowedProductRefs:              []string{"b1"},
	})
	if err != nil {
		t.Fatalf("create branch B: %v", err)
	}

	rawA1 := []byte(`{"uuid":"a1","plu":"1001","name":"Chicken Breast 1kg","barcode":"5000112","product_code":"CB1","department_uuid":"dept-meat","price1":599,"cost_price":400,"can_be_sold":true,"deleted":false,"last_modified":"2026-01-01T10:00:00Z"}`)
	rawB1 := []byte(`{"uuid":"b1","plu":"1001","name":"Chicken Breast 1kg","barcode":"5000112","product_code":"CB1","department_uuid":"dept-meat","price1":649,"cost_price":420,"can_be_sold":true,"deleted":false,"last_modified":"2026-01-01T11:00:00Z"}`)
	rawA2NoPLU := []byte(`{"uuid":"a2","plu":null,"name":"Glasgow Special Pie","department_uuid":"dept-bakery","price1":250,"cost_price":150,"can_be_sold":true,"deleted":false,"last_modified":"2026-01-01T10:00:00Z"}`)

	syncer := productSyncer{}

	if _, _, err := syncer.Upsert(ctx, q, branchA, json.RawMessage(rawA1)); err != nil {
		t.Fatalf("upsert A1: %v", err)
	}
	if _, _, err := syncer.Upsert(ctx, q, branchB, json.RawMessage(rawB1)); err != nil {
		t.Fatalf("upsert B1: %v", err)
	}
	if _, _, err := syncer.Upsert(ctx, q, branchA, json.RawMessage(rawA2NoPLU)); err != nil {
		t.Fatalf("upsert A2 (1st sync): %v", err)
	}
	// Simulate a re-sync of the same no-PLU record.
	if _, _, err := syncer.Upsert(ctx, q, branchA, json.RawMessage(rawA2NoPLU)); err != nil {
		t.Fatalf("upsert A2 (2nd sync): %v", err)
	}

	var pluProductCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products WHERE plu = '1001'`).Scan(&pluProductCount); err != nil {
		t.Fatalf("count plu products: %v", err)
	}
	if pluProductCount != 1 {
		t.Errorf("expected exactly 1 canonical product for PLU 1001 across both branches, got %d", pluProductCount)
	}

	var linkedBranches int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM branch_products bp
		JOIN products p ON p.id = bp.product_id
		WHERE p.plu = '1001'`).Scan(&linkedBranches); err != nil {
		t.Fatalf("count branch links: %v", err)
	}
	if linkedBranches != 2 {
		t.Errorf("expected 2 branch_products rows (one per branch) linked to the shared product, got %d", linkedBranches)
	}

	var priceA, priceB int64
	if err := pool.QueryRow(ctx, `
		SELECT price1 FROM branch_products bp JOIN products p ON p.id = bp.product_id
		WHERE p.plu = '1001' AND bp.branch_id = $1`, branchA.ID).Scan(&priceA); err != nil {
		t.Fatalf("get price A: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT price1 FROM branch_products bp JOIN products p ON p.id = bp.product_id
		WHERE p.plu = '1001' AND bp.branch_id = $1`, branchB.ID).Scan(&priceB); err != nil {
		t.Fatalf("get price B: %v", err)
	}
	if priceA != 599 || priceB != 649 {
		t.Errorf("expected branch-specific prices 599/649, got %d/%d", priceA, priceB)
	}

	var noPLUProductCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products WHERE plu IS NULL AND name = 'Glasgow Special Pie'`).Scan(&noPLUProductCount); err != nil {
		t.Fatalf("count no-plu products: %v", err)
	}
	if noPLUProductCount != 1 {
		t.Errorf("expected re-syncing a no-PLU record to update the same row, not create a duplicate; got %d rows", noPLUProductCount)
	}
}
