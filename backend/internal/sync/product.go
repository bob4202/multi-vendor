package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

type productSyncer struct{}

func (productSyncer) Resource() string    { return "product" }
func (productSyncer) CursorParam() string { return "modified_since" }
func (productSyncer) PageLimit() int      { return 100 } // the API's documented max for /product

// ExtraParams pushes the department filter server-side as an optimization,
// but only when it's unambiguous: a single configured department and no
// product-ref allowlist. Mixing the two server-side would risk silently
// excluding allowlisted products that live in other departments - in every
// other case the authoritative filter check happens client-side in Upsert.
func (productSyncer) ExtraParams(branch db.Branch) url.Values {
	if len(branch.AllowedProductRefs) == 0 && len(branch.AllowedSecondaryDepartmentUuids) == 1 {
		v := url.Values{}
		v.Set("secondary_department_uuid", branch.AllowedSecondaryDepartmentUuids[0])
		return v
	}
	return nil
}

// FilterConfigured reports whether this branch wants any products at all.
// A branch with neither filter set gets nothing synced - there is no
// unfiltered "catalogue" mode.
func (productSyncer) FilterConfigured(branch db.Branch) bool {
	return len(branch.AllowedSecondaryDepartmentUuids) > 0 || len(branch.AllowedProductRefs) > 0
}

// matchesProductFilter reports whether p is included by this branch's
// filter: the branch's department allowlist and product-ref allowlist
// (matched against either the product's UUID or its PLU) combine with OR.
func matchesProductFilter(branch db.Branch, p apiProduct) bool {
	if p.SecondaryDepartmentUUID != nil && containsString(branch.AllowedSecondaryDepartmentUuids, *p.SecondaryDepartmentUUID) {
		return true
	}
	if containsString(branch.AllowedProductRefs, p.UUID) {
		return true
	}
	if p.Plu != nil && *p.Plu != "" && containsString(branch.AllowedProductRefs, *p.Plu) {
		return true
	}
	return false
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// apiProduct mirrors the GET /product response fields we materialize
// as columns, split across the shared `products` catalog row (descriptive,
// same everywhere) and the per-branch `branch_products` link (price, stock,
// availability). Everything else stays in the `raw` JSONB column.
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

func (productSyncer) Upsert(ctx context.Context, q *db.Queries, branch db.Branch, raw json.RawMessage) (time.Time, bool, error) {
	var p apiProduct
	if err := json.Unmarshal(raw, &p); err != nil {
		return time.Time{}, false, fmt.Errorf("unmarshal product: %w", err)
	}
	if p.UUID == "" {
		return time.Time{}, false, fmt.Errorf("product record missing uuid")
	}

	lastModified, sourceLastModified, err := parseTimestamp(p.LastModified)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse product last_modified: %w", err)
	}

	if !matchesProductFilter(branch, p) {
		return lastModified, false, nil
	}

	productID, err := resolveProductID(ctx, q, branch.ID, p)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("resolve catalog product for %s: %w", p.UUID, err)
	}

	err = q.UpsertBranchProduct(ctx, db.UpsertBranchProductParams{
		BranchID:               branch.ID,
		ProductID:              productID,
		ExternalID:             p.UUID,
		SupplierUuid:           p.SupplierUUID,
		Price1:                 p.Price1,
		Price2:                 p.Price2,
		Price3:                 p.Price3,
		Price4:                 p.Price4,
		Price5:                 p.Price5,
		Price6:                 p.Price6,
		Price7:                 p.Price7,
		Price8:                 p.Price8,
		Price9:                 p.Price9,
		Price10:                p.Price10,
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
		SourceLastModified:     sourceLastModified,
		Raw:                    raw,
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("upsert branch_product %s: %w", p.UUID, err)
	}

	return lastModified, true, nil
}

// resolveProductID finds or creates the canonical catalog row for a
// branch's product record. When a PLU is present it's the dedup key across
// branches (INSERT ... ON CONFLICT handles both create and merge in one
// statement). Without a PLU there's nothing to merge on, so we fall back to
// whatever product row this branch's external_id was already linked to
// (updating its descriptive fields), or create a fresh row the first time
// we see it - never a new row on every sync.
func resolveProductID(ctx context.Context, q *db.Queries, branchID uuid.UUID, p apiProduct) (uuid.UUID, error) {
	if p.Plu != nil && *p.Plu != "" {
		return q.UpsertProductByPLU(ctx, db.UpsertProductByPLUParams{
			Plu:                     p.Plu,
			Name:                    p.Name,
			ProductCode:             p.ProductCode,
			Barcode:                 p.Barcode,
			DepartmentUuid:          p.DepartmentUUID,
			SecondaryDepartmentUuid: p.SecondaryDepartmentUUID,
			ProductTypeUuid:         p.ProductTypeUUID,
			VatRateUuid:             p.VatRateUUID,
			PricebaseUuid:           p.PricebaseUUID,
			OpenPriced:              p.OpenPriced,
		})
	}

	existingID, err := q.GetProductIDByBranchExternalID(ctx, db.GetProductIDByBranchExternalIDParams{
		BranchID:   branchID,
		ExternalID: p.UUID,
	})
	if err == nil {
		return q.UpdateProductDescriptive(ctx, db.UpdateProductDescriptiveParams{
			ID:                      existingID,
			Name:                    p.Name,
			ProductCode:             p.ProductCode,
			Barcode:                 p.Barcode,
			DepartmentUuid:          p.DepartmentUUID,
			SecondaryDepartmentUuid: p.SecondaryDepartmentUUID,
			ProductTypeUuid:         p.ProductTypeUUID,
			VatRateUuid:             p.VatRateUUID,
			PricebaseUuid:           p.PricebaseUUID,
			OpenPriced:              p.OpenPriced,
		})
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("lookup existing branch_product link: %w", err)
	}

	return q.CreateProductWithoutPLU(ctx, db.CreateProductWithoutPLUParams{
		Name:                    p.Name,
		ProductCode:             p.ProductCode,
		Barcode:                 p.Barcode,
		DepartmentUuid:          p.DepartmentUUID,
		SecondaryDepartmentUuid: p.SecondaryDepartmentUUID,
		ProductTypeUuid:         p.ProductTypeUUID,
		VatRateUuid:             p.VatRateUUID,
		PricebaseUuid:           p.PricebaseUUID,
		OpenPriced:              p.OpenPriced,
	})
}

func parseTimestamp(v *string) (time.Time, pgtype.Timestamptz, error) {
	var ts pgtype.Timestamptz
	if v == nil || *v == "" {
		return time.Time{}, ts, nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return time.Time{}, ts, err
	}
	if err := ts.Scan(t); err != nil {
		return time.Time{}, ts, err
	}
	return t, ts, nil
}
