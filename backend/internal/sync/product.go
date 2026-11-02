package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/inventory"
)

// departmentHint lets the provider filter on its side when that's safe:
// exactly one department and no product-ref allowlist. Mixing the two on the
// system's side could drop allowlisted products from other departments, so
// in every other case the full filter runs here only (matchesProductFilter).
func departmentHint(branch db.Branch) string {
	if len(branch.AllowedProductRefs) == 0 && len(branch.AllowedSecondaryDepartmentUuids) == 1 {
		return branch.AllowedSecondaryDepartmentUuids[0]
	}
	return ""
}

// productFilterConfigured reports whether this branch wants any products at
// all. A branch with neither filter set gets nothing synced.
func productFilterConfigured(branch db.Branch) bool {
	return len(branch.AllowedSecondaryDepartmentUuids) > 0 || len(branch.AllowedProductRefs) > 0
}

// matchesProductFilter reports whether p is included by this branch's
// filter: the branch's department allowlist and product-ref allowlist
// (matched against either the product's UUID or its PLU) combine with OR.
func matchesProductFilter(branch db.Branch, p inventory.Product) bool {
	if p.SecondaryDepartmentID != nil && containsString(branch.AllowedSecondaryDepartmentUuids, *p.SecondaryDepartmentID) {
		return true
	}
	if containsString(branch.AllowedProductRefs, p.ExternalID) {
		return true
	}
	if p.PLU != nil && *p.PLU != "" && containsString(branch.AllowedProductRefs, *p.PLU) {
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

// upsertProduct saves one product for a branch: the shared catalogue row
// (descriptive fields, deduplicated by PLU) and the branch's own link (price,
// stock, availability). It returns the record's change time, and false when
// the branch's filter leaves it out.
func upsertProduct(ctx context.Context, q *db.Queries, branch db.Branch, p inventory.Product) (time.Time, bool, error) {
	if p.ExternalID == "" {
		return time.Time{}, false, fmt.Errorf("product record missing external id")
	}
	if !matchesProductFilter(branch, p) {
		return p.LastModified, false, nil
	}

	productID, err := resolveProductID(ctx, q, branch.ID, p)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("resolve catalog product for %s: %w", p.ExternalID, err)
	}

	err = q.UpsertBranchProduct(ctx, db.UpsertBranchProductParams{
		BranchID:               branch.ID,
		ProductID:              productID,
		ExternalID:             p.ExternalID,
		SupplierUuid:           p.SupplierID,
		Price1:                 p.Prices[0],
		Price2:                 p.Prices[1],
		Price3:                 p.Prices[2],
		Price4:                 p.Prices[3],
		Price5:                 p.Prices[4],
		Price6:                 p.Prices[5],
		Price7:                 p.Prices[6],
		Price8:                 p.Prices[7],
		Price9:                 p.Prices[8],
		Price10:                p.Prices[9],
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
		SourceLastModified:     timestamptz(p.LastModified),
		Raw:                    rawOrEmpty(p.Raw),
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("upsert branch_product %s: %w", p.ExternalID, err)
	}
	return p.LastModified, true, nil
}

// resolveProductID finds or creates the canonical catalog row for a
// branch's product record. When a PLU is present it's the dedup key across
// branches (INSERT ... ON CONFLICT handles both create and merge in one
// statement). Without a PLU there's nothing to merge on, so we fall back to
// whatever product row this branch's external_id was already linked to
// (updating its descriptive fields), or create a fresh row the first time
// we see it - never a new row on every sync.
func resolveProductID(ctx context.Context, q *db.Queries, branchID uuid.UUID, p inventory.Product) (uuid.UUID, error) {
	if p.PLU != nil && *p.PLU != "" {
		return q.UpsertProductByPLU(ctx, db.UpsertProductByPLUParams{
			Plu:                     p.PLU,
			Name:                    p.Name,
			ProductCode:             p.ProductCode,
			Barcode:                 p.Barcode,
			DepartmentUuid:          p.DepartmentID,
			SecondaryDepartmentUuid: p.SecondaryDepartmentID,
			ProductTypeUuid:         p.ProductTypeID,
			VatRateUuid:             p.VatRateID,
			PricebaseUuid:           p.PricebaseID,
			OpenPriced:              p.OpenPriced,
		})
	}

	existingID, err := q.GetProductIDByBranchExternalID(ctx, db.GetProductIDByBranchExternalIDParams{
		BranchID:   branchID,
		ExternalID: p.ExternalID,
	})
	if err == nil {
		return q.UpdateProductDescriptive(ctx, db.UpdateProductDescriptiveParams{
			ID:                      existingID,
			Name:                    p.Name,
			ProductCode:             p.ProductCode,
			Barcode:                 p.Barcode,
			DepartmentUuid:          p.DepartmentID,
			SecondaryDepartmentUuid: p.SecondaryDepartmentID,
			ProductTypeUuid:         p.ProductTypeID,
			VatRateUuid:             p.VatRateID,
			PricebaseUuid:           p.PricebaseID,
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
		DepartmentUuid:          p.DepartmentID,
		SecondaryDepartmentUuid: p.SecondaryDepartmentID,
		ProductTypeUuid:         p.ProductTypeID,
		VatRateUuid:             p.VatRateID,
		PricebaseUuid:           p.PricebaseID,
		OpenPriced:              p.OpenPriced,
	})
}
