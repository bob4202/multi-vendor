package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"ecommerce/backend/internal/inventory"
)

// Name is what branches store in inventory_provider to use this provider.
const Name = "connect"

// Provider pulls products and customers through the paged REST API.
type Provider struct {
	client *Client
}

// NewFactory returns the factory for the registry. urlTemplate is
// INVENTORY_CONNECT_URL: the API's base URL with {host} for each branch.
func NewFactory(urlTemplate string) inventory.Factory {
	return func(c inventory.Credentials) (inventory.Provider, error) {
		if urlTemplate == "" {
			return nil, errors.New("connect: INVENTORY_CONNECT_URL is not set")
		}
		return &Provider{client: NewClient(urlTemplate, c, nil)}, nil
	}
}

func (p *Provider) Products(ctx context.Context, q inventory.ProductQuery, handle inventory.ProductPage) error {
	params := url.Values{}
	if !q.Since.IsZero() {
		params.Set("modified_since", q.Since.UTC().Format(time.RFC3339))
	}
	if q.Department != "" {
		params.Set("secondary_department_uuid", q.Department)
	}
	return p.client.ForEachPage(ctx, "product", params, func(ctx context.Context, records []json.RawMessage) error {
		products := make([]inventory.Product, 0, len(records))
		for _, raw := range records {
			pr, err := ParseProduct(raw)
			if err != nil {
				return err
			}
			products = append(products, pr)
		}
		return handle(ctx, products)
	})
}

func (p *Provider) Customers(ctx context.Context, since time.Time, handle inventory.CustomerPage) error {
	params := url.Values{}
	if !since.IsZero() {
		params.Set("last_modified", since.UTC().Format(time.RFC3339))
	}
	return p.client.ForEachPage(ctx, "customer", params, func(ctx context.Context, records []json.RawMessage) error {
		customers := make([]inventory.Customer, 0, len(records))
		for _, raw := range records {
			c, err := ParseCustomer(raw)
			if err != nil {
				return err
			}
			customers = append(customers, c)
		}
		return handle(ctx, customers)
	})
}

// apiProduct mirrors the GET /product record fields we keep.
type apiProduct struct {
	UUID                    string  `json:"uuid"`
	Plu                     *string `json:"plu"`
	ProductCode             *string `json:"product_code"`
	Barcode                 *string `json:"barcode"`
	Name                    *string `json:"name"`
	DepartmentUUID          *string `json:"department_uuid"`
	SecondaryDepartmentUUID *string `json:"secondary_department_uuid"`
	ProductTypeUUID         *string `json:"product_type_uuid"`
	VatRateUUID             *string `json:"vat_rate_uuid"`
	PricebaseUUID           *string `json:"pricebase_uuid"`
	OpenPriced              *bool   `json:"open_priced"`
	SupplierUUID            *string `json:"supplier_uuid"`
	Price1                  *int64  `json:"price1"`
	Price2                  *int64  `json:"price2"`
	Price3                  *int64  `json:"price3"`
	Price4                  *int64  `json:"price4"`
	Price5                  *int64  `json:"price5"`
	Price6                  *int64  `json:"price6"`
	Price7                  *int64  `json:"price7"`
	Price8                  *int64  `json:"price8"`
	Price9                  *int64  `json:"price9"`
	Price10                 *int64  `json:"price10"`
	CostPrice               *int64  `json:"cost_price"`
	MinimumStock            *int32  `json:"minimum_stock"`
	MaximumStock            *int32  `json:"maximum_stock"`
	CanBeSold               *bool   `json:"can_be_sold"`
	CanBeSoldSpecialOrders  *bool   `json:"can_be_sold_special_orders"`
	CanBeSupplied           *bool   `json:"can_be_supplied"`
	CanBeProduced           *bool   `json:"can_be_produced"`
	TrackStock              *bool   `json:"track_stock"`
	TrackAvailability       *bool   `json:"track_availability"`
	Deleted                 bool    `json:"deleted"`
	LastModified            *string `json:"last_modified"`
}

// ParseProduct turns one raw /product record into our shape.
func ParseProduct(raw json.RawMessage) (inventory.Product, error) {
	var p apiProduct
	if err := json.Unmarshal(raw, &p); err != nil {
		return inventory.Product{}, fmt.Errorf("unmarshal product: %w", err)
	}
	if p.UUID == "" {
		return inventory.Product{}, errors.New("product record missing uuid")
	}
	lastModified, err := parseTime(p.LastModified)
	if err != nil {
		return inventory.Product{}, fmt.Errorf("parse product %s last_modified: %w", p.UUID, err)
	}
	return inventory.Product{
		ExternalID:             p.UUID,
		PLU:                    p.Plu,
		ProductCode:            p.ProductCode,
		Barcode:                p.Barcode,
		Name:                   p.Name,
		DepartmentID:           p.DepartmentUUID,
		SecondaryDepartmentID:  p.SecondaryDepartmentUUID,
		ProductTypeID:          p.ProductTypeUUID,
		VatRateID:              p.VatRateUUID,
		PricebaseID:            p.PricebaseUUID,
		SupplierID:             p.SupplierUUID,
		OpenPriced:             p.OpenPriced,
		Prices:                 [10]*int64{p.Price1, p.Price2, p.Price3, p.Price4, p.Price5, p.Price6, p.Price7, p.Price8, p.Price9, p.Price10},
		CostPrice:              p.CostPrice,
		MinimumStock:           p.MinimumStock,
		MaximumStock:           p.MaximumStock,
		CanBeSold:              p.CanBeSold,
		CanBeSoldSpecialOrders: p.CanBeSoldSpecialOrders,
		CanBeSupplied:          p.CanBeSupplied,
		CanBeProduced:          p.CanBeProduced,
		TrackStock:             p.TrackStock,
		TrackAvailability:      p.TrackAvailability,
		Deleted:                p.Deleted,
		LastModified:           lastModified,
		Raw:                    raw,
	}, nil
}

// apiCustomer mirrors the GET /customer record fields we keep. Email only
// lives in the nested address object.
type apiCustomer struct {
	UUID      string  `json:"uuid"`
	Name      *string `json:"name"`
	Reference *string `json:"reference"`
	Mobile    *string `json:"mobile"`
	Address   *struct {
		Email *string `json:"email"`
	} `json:"address"`
	LoyaltyNumber     *string `json:"loyalty_number"`
	LoyaltyNumberAlt  *string `json:"loyalty_number_alt"`
	CustomerTypeUUID  *string `json:"customer_type_uuid"`
	WebsiteCustomerID *string `json:"website_customer_id"`
	AllowMarketing    *bool   `json:"allow_marketing"`
	Deleted           bool    `json:"deleted"`
	LastModified      *string `json:"last_modified"`
	LastOrdered       *string `json:"last_ordered"`
}

// ParseCustomer turns one raw /customer record into our shape.
func ParseCustomer(raw json.RawMessage) (inventory.Customer, error) {
	var c apiCustomer
	if err := json.Unmarshal(raw, &c); err != nil {
		return inventory.Customer{}, fmt.Errorf("unmarshal customer: %w", err)
	}
	if c.UUID == "" {
		return inventory.Customer{}, errors.New("customer record missing uuid")
	}
	lastModified, err := parseTime(c.LastModified)
	if err != nil {
		return inventory.Customer{}, fmt.Errorf("parse customer %s last_modified: %w", c.UUID, err)
	}
	lastOrdered, err := parseTime(c.LastOrdered)
	if err != nil {
		return inventory.Customer{}, fmt.Errorf("parse customer %s last_ordered: %w", c.UUID, err)
	}
	out := inventory.Customer{
		ExternalID:        c.UUID,
		Name:              c.Name,
		Reference:         c.Reference,
		Mobile:            c.Mobile,
		LoyaltyNumber:     c.LoyaltyNumber,
		LoyaltyNumberAlt:  c.LoyaltyNumberAlt,
		CustomerTypeID:    c.CustomerTypeUUID,
		WebsiteCustomerID: c.WebsiteCustomerID,
		AllowMarketing:    c.AllowMarketing,
		Deleted:           c.Deleted,
		LastModified:      lastModified,
		Raw:               raw,
	}
	if c.Address != nil {
		out.Email = c.Address.Email
	}
	if !lastOrdered.IsZero() {
		out.LastOrdered = &lastOrdered
	}
	return out, nil
}

func parseTime(v *string) (time.Time, error) {
	if v == nil || *v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, *v)
}
