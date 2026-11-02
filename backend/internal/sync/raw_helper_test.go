package sync

import (
	"context"
	"time"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory/connect"
)

// upsertRawProduct parses a raw connect-API product record and saves it, so
// tests can describe products as the JSON a real system sends.
func upsertRawProduct(ctx context.Context, q *db.Queries, branch db.Branch, raw []byte) (time.Time, bool, error) {
	p, err := connect.ParseProduct(raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return upsertProduct(ctx, q, branch, p)
}
