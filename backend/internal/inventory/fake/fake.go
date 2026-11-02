// Package fake is an in-memory inventory provider for local development and
// tests. By default it has no records, so "Sync now" succeeds without
// touching anything; tests can fill a Provider with their own records.
package fake

import (
	"context"
	"time"

	"ecommerce/backend/internal/inventory"
)

// Name is what branches store in inventory_provider to use this provider.
const Name = "fake"

// Provider serves its records in one page each. Records changed at or
// before the requested time are left out, like a real incremental sync.
type Provider struct {
	ProductList  []inventory.Product
	CustomerList []inventory.Customer
}

// New is the registry factory: an empty provider for any branch.
func New(inventory.Credentials) (inventory.Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) Products(ctx context.Context, q inventory.ProductQuery, handle inventory.ProductPage) error {
	var out []inventory.Product
	for _, pr := range p.ProductList {
		if newer(pr.LastModified, q.Since) {
			out = append(out, pr)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return handle(ctx, out)
}

func (p *Provider) Customers(ctx context.Context, since time.Time, handle inventory.CustomerPage) error {
	var out []inventory.Customer
	for _, c := range p.CustomerList {
		if newer(c.LastModified, since) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return handle(ctx, out)
}

func newer(changed, since time.Time) bool {
	return since.IsZero() || changed.IsZero() || changed.After(since)
}
