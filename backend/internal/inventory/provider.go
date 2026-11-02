// Package inventory defines how we pull products and customers from a
// branch's external inventory system (a till, scale or stock system). Each
// system gets its own Provider (see the connect and fake subpackages); the
// sync engine only talks to this interface, so adding a system never
// touches the sync code.
package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Product is one product record, already turned into our shape. Pointer
// fields are nil when the system doesn't have that value.
type Product struct {
	// ExternalID is the system's own id for this product at this branch.
	ExternalID string
	// PLU, when present, is what links the same product across branches.
	PLU         *string
	ProductCode *string
	Barcode     *string
	Name        *string

	DepartmentID          *string
	SecondaryDepartmentID *string // what branch department filters match on
	ProductTypeID         *string
	VatRateID             *string
	PricebaseID           *string
	SupplierID            *string
	OpenPriced            *bool

	// Prices are price levels 1 to 10, in pence. Prices[0] is the shop price.
	Prices       [10]*int64
	CostPrice    *int64
	MinimumStock *int32
	MaximumStock *int32

	CanBeSold              *bool
	CanBeSoldSpecialOrders *bool
	CanBeSupplied          *bool
	CanBeProduced          *bool
	TrackStock             *bool
	TrackAvailability      *bool
	Deleted                bool

	// LastModified moves the sync cursor forward. Zero when unknown.
	LastModified time.Time
	// Raw is the system's original record, stored for reference. nil = none.
	Raw json.RawMessage
}

// Customer is one customer record from the inventory system (not a
// storefront customer account).
type Customer struct {
	ExternalID        string
	Name              *string
	Reference         *string
	Mobile            *string
	Email             *string
	LoyaltyNumber     *string
	LoyaltyNumberAlt  *string
	CustomerTypeID    *string
	WebsiteCustomerID *string
	AllowMarketing    *bool
	Deleted           bool
	LastModified      time.Time
	LastOrdered       *time.Time
	Raw               json.RawMessage
}

// ProductQuery says which products to fetch.
type ProductQuery struct {
	// Since asks only for products changed after this time. Zero = all.
	Since time.Time
	// Department, when set, is a hint that only this secondary department is
	// wanted. A provider may ignore it: the sync engine filters again anyway.
	Department string
}

// ProductPage and CustomerPage handle one page of records. Returning an
// error stops the fetch.
type (
	ProductPage  func(ctx context.Context, products []Product) error
	CustomerPage func(ctx context.Context, customers []Customer) error
)

// Provider pulls records from one branch's inventory system. Read only:
// nothing is ever written back.
type Provider interface {
	Products(ctx context.Context, q ProductQuery, handle ProductPage) error
	Customers(ctx context.Context, since time.Time, handle CustomerPage) error
}

// Credentials are a branch's connection settings, as entered in the admin
// side. What each field means is up to the provider.
type Credentials struct {
	Host     string
	Username string
	APIKey   string
}

// Factory builds a provider for one branch.
type Factory func(c Credentials) (Provider, error)

// Registry maps provider names (stored on each branch) to factories.
type Registry map[string]Factory

var ErrUnknownProvider = errors.New("inventory: provider not enabled")

func (r Registry) Build(name string, c Credentials) (Provider, error) {
	f, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("%q: %w", name, ErrUnknownProvider)
	}
	return f(c)
}

func (r Registry) Has(name string) bool {
	_, ok := r[name]
	return ok
}

// Names lists the enabled providers, sorted.
func (r Registry) Names() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
