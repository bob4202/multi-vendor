package sync

import (
	"context"
	"fmt"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
)

// upsertCustomer saves one inventory-system customer for a branch.
func upsertCustomer(ctx context.Context, q *db.Queries, branch db.Branch, c inventory.Customer) error {
	if c.ExternalID == "" {
		return fmt.Errorf("customer record missing external id")
	}
	lastOrdered := timestamptz(timeOrZero(c.LastOrdered))
	err := q.UpsertCustomer(ctx, db.UpsertCustomerParams{
		BranchID:           branch.ID,
		ExternalID:         c.ExternalID,
		Name:               c.Name,
		Reference:          c.Reference,
		Mobile:             c.Mobile,
		Email:              c.Email,
		LoyaltyNumber:      c.LoyaltyNumber,
		LoyaltyNumberAlt:   c.LoyaltyNumberAlt,
		CustomerTypeUuid:   c.CustomerTypeID,
		WebsiteCustomerID:  c.WebsiteCustomerID,
		AllowMarketing:     c.AllowMarketing,
		Deleted:            c.Deleted,
		SourceLastModified: timestamptz(c.LastModified),
		SourceLastOrdered:  lastOrdered,
		Raw:                rawOrEmpty(c.Raw),
	})
	if err != nil {
		return fmt.Errorf("upsert customer %s: %w", c.ExternalID, err)
	}
	return nil
}
