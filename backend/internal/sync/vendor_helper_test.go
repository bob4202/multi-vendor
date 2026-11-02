package sync

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
)

// testVendorID returns the id of a vendor for test branches to belong to
// (every branch needs one), creating it on first use.
func testVendorID(t *testing.T, ctx context.Context, q *db.Queries) uuid.UUID {
	t.Helper()
	const slug = "sync-integration-tests"
	for _, v := range mustListVendors(t, ctx, q) {
		if v.Slug == slug {
			return v.ID
		}
	}
	v, err := q.CreateVendor(ctx, db.CreateVendorParams{Slug: slug, Name: "Sync integration tests"})
	if err != nil {
		t.Fatalf("create test vendor: %v", err)
	}
	return v.ID
}

func mustListVendors(t *testing.T, ctx context.Context, q *db.Queries) []db.Vendor {
	t.Helper()
	vendors, err := q.ListVendors(ctx)
	if err != nil {
		t.Fatalf("list vendors: %v", err)
	}
	return vendors
}
