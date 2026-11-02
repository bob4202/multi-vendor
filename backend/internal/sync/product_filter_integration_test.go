package sync

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/db"
)

// TestProductSyncBranchFilter verifies the per-branch inclusion filter:
// products matching either the branch's department allowlist or its
// explicit product-ref allowlist get synced, everything else is skipped
// without error, and a branch with no filter configured gets nothing -
// there is no unfiltered "catalogue" fallback.
func TestProductSyncBranchFilter(t *testing.T) {
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
	syncer := productSyncer{}

	deptBranch, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Dept Filtered Branch", Host: "test-dept-" + t.Name(), ApiUsername: "a", ApiKeyCipher: []byte("x"), IsActive: true, VendorID: vendorID,
		AllowedSecondaryDepartmentUuids: []string{"dept-meat"},
		AllowedProductRefs:              []string{},
	})
	if err != nil {
		t.Fatalf("create dept-filtered branch: %v", err)
	}

	refBranch, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Ref Filtered Branch", Host: "test-ref-" + t.Name(), ApiUsername: "a", ApiKeyCipher: []byte("x"), IsActive: true, VendorID: vendorID,
		AllowedSecondaryDepartmentUuids: []string{"dept-bakery"},
		AllowedProductRefs:              []string{"c1"},
	})
	if err != nil {
		t.Fatalf("create ref-filtered branch: %v", err)
	}

	unfilteredBranch, err := q.CreateBranch(ctx, db.CreateBranchParams{
		Name: "Unfiltered Branch", Host: "test-none-" + t.Name(), ApiUsername: "a", ApiKeyCipher: []byte("x"), IsActive: true, VendorID: vendorID,
		AllowedSecondaryDepartmentUuids: []string{},
		AllowedProductRefs:              []string{},
	})
	if err != nil {
		t.Fatalf("create unfiltered branch: %v", err)
	}

	matchingRaw := []byte(`{"uuid":"m1","plu":"2001","name":"Lamb Chops","secondary_department_uuid":"dept-meat","price1":899,"deleted":false,"last_modified":"2026-01-02T10:00:00Z"}`)
	nonMatchingRaw := []byte(`{"uuid":"n1","plu":"2002","name":"Bread Roll","secondary_department_uuid":"dept-bakery","price1":150,"deleted":false,"last_modified":"2026-01-02T11:00:00Z"}`)
	refMatchRaw := []byte(`{"uuid":"c1","plu":"2003","name":"Custom Cut","secondary_department_uuid":"dept-meat","price1":500,"deleted":false,"last_modified":"2026-01-02T12:00:00Z"}`)

	// (a) matches the department allowlist -> written.
	lm, written, err := syncer.Upsert(ctx, q, deptBranch, json.RawMessage(matchingRaw))
	if err != nil {
		t.Fatalf("upsert matching record: %v", err)
	}
	if !written {
		t.Errorf("expected matching department record to be written")
	}
	if lm.IsZero() {
		t.Errorf("expected non-zero lastModified for a written record")
	}
	assertBranchProductExists(t, pool, deptBranch.ID, "m1", true)

	// (b) matches neither filter -> skipped, no error, cursor-relevant
	// lastModified still returned.
	lm, written, err = syncer.Upsert(ctx, q, deptBranch, json.RawMessage(nonMatchingRaw))
	if err != nil {
		t.Fatalf("upsert non-matching record: %v", err)
	}
	if written {
		t.Errorf("expected non-matching record to be skipped")
	}
	if lm.IsZero() {
		t.Errorf("expected non-zero lastModified even for a skipped record, so the sync cursor can still advance")
	}
	assertBranchProductExists(t, pool, deptBranch.ID, "n1", false)

	// (c) outside the department allowlist but inside the product-ref
	// allowlist -> written (OR semantics).
	_, written, err = syncer.Upsert(ctx, q, refBranch, json.RawMessage(refMatchRaw))
	if err != nil {
		t.Fatalf("upsert ref-matching record: %v", err)
	}
	if !written {
		t.Errorf("expected product-ref allowlist match to be written even though department doesn't match")
	}
	assertBranchProductExists(t, pool, refBranch.ID, "c1", true)

	// (d) branch with no filter configured -> nothing written.
	_, written, err = syncer.Upsert(ctx, q, unfilteredBranch, json.RawMessage(matchingRaw))
	if err != nil {
		t.Fatalf("upsert on unfiltered branch: %v", err)
	}
	if written {
		t.Errorf("expected a branch with no filter configured to skip every product")
	}
	if syncer.FilterConfigured(unfilteredBranch) {
		t.Errorf("expected FilterConfigured to be false for a branch with empty filters")
	}
	assertBranchProductExists(t, pool, unfilteredBranch.ID, "m1", false)
}

func assertBranchProductExists(t *testing.T, pool *pgxpool.Pool, branchID uuid.UUID, externalID string, wantExists bool) {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM branch_products WHERE branch_id = $1 AND external_id = $2`,
		branchID, externalID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count branch_products for %s: %v", externalID, err)
	}
	exists := count > 0
	if exists != wantExists {
		t.Errorf("branch_products row for external_id=%s: got exists=%v, want %v", externalID, exists, wantExists)
	}
}
