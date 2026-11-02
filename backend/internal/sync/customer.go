package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"ecommerce/backend/internal/db"
)

type customerSyncer struct{}

func (customerSyncer) Resource() string    { return "customer" }
func (customerSyncer) CursorParam() string { return "last_modified" }
func (customerSyncer) PageLimit() int      { return 200 } // the API's documented max for /customer

// ExtraParams: customer sync has no filter concept, so nothing extra.
func (customerSyncer) ExtraParams(db.Branch) url.Values { return nil }

// apiCustomerAddress mirrors the nested "address" object, which is
// the only place email lives on a customer record.
type apiCustomerAddress struct {
	Email *string `json:"email"`
}

type apiCustomer struct {
	UUID              string              `json:"uuid"`
	Name              *string             `json:"name"`
	Reference         *string             `json:"reference"`
	Mobile            *string             `json:"mobile"`
	Address           *apiCustomerAddress `json:"address"`
	LoyaltyNumber     *string             `json:"loyalty_number"`
	LoyaltyNumberAlt  *string             `json:"loyalty_number_alt"`
	CustomerTypeUUID  *string             `json:"customer_type_uuid"`
	WebsiteCustomerID *string             `json:"website_customer_id"`
	AllowMarketing    *bool               `json:"allow_marketing"`
	Deleted           bool                `json:"deleted"`
	LastModified      *string             `json:"last_modified"`
	LastOrdered       *string             `json:"last_ordered"`
}

func (customerSyncer) Upsert(ctx context.Context, q *db.Queries, branch db.Branch, raw json.RawMessage) (time.Time, bool, error) {
	var c apiCustomer
	if err := json.Unmarshal(raw, &c); err != nil {
		return time.Time{}, false, fmt.Errorf("unmarshal customer: %w", err)
	}
	if c.UUID == "" {
		return time.Time{}, false, fmt.Errorf("customer record missing uuid")
	}

	lastModified, sourceLastModified, err := parseTimestamp(c.LastModified)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse customer last_modified: %w", err)
	}
	_, sourceLastOrdered, err := parseTimestamp(c.LastOrdered)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse customer last_ordered: %w", err)
	}

	var email *string
	if c.Address != nil {
		email = c.Address.Email
	}

	err = q.UpsertCustomer(ctx, db.UpsertCustomerParams{
		BranchID:           branch.ID,
		ExternalID:         c.UUID,
		Name:               c.Name,
		Reference:          c.Reference,
		Mobile:             c.Mobile,
		Email:              email,
		LoyaltyNumber:      c.LoyaltyNumber,
		LoyaltyNumberAlt:   c.LoyaltyNumberAlt,
		CustomerTypeUuid:   c.CustomerTypeUUID,
		WebsiteCustomerID:  c.WebsiteCustomerID,
		AllowMarketing:     c.AllowMarketing,
		Deleted:            c.Deleted,
		SourceLastModified: sourceLastModified,
		SourceLastOrdered:  sourceLastOrdered,
		Raw:                raw,
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("upsert customer %s: %w", c.UUID, err)
	}

	return lastModified, true, nil
}
