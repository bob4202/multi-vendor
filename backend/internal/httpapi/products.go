package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// productResponse is the shape returned for one product, used by both the
// admin and storefront routes - only the auth/filtering around it differs.
type productResponse struct {
	ID           string   `json:"id"`
	Plu          *string  `json:"plu"`
	Name         *string  `json:"name"`
	Barcode      *string  `json:"barcode"`
	CategoryID   *string  `json:"category_id"`
	CategoryName *string  `json:"category_name"`
	Price        *int64   `json:"price"`
	Description  *string  `json:"description"`
	ImageURLs    []string `json:"image_urls"`
	IsUpsell     bool     `json:"is_upsell"`
	CanBeSold    *bool    `json:"can_be_sold"`
	Deleted      bool     `json:"deleted"`
}

type paginatedProductsResponse struct {
	Data       []productResponse `json:"data"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalCount int64             `json:"total_count"`
	TotalPages int               `json:"total_pages"`
}

type categoryResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func toProductResponse(id uuid.UUID, plu, name, barcode, categoryID, categoryName *string, price *int64, description *string, imageURLs []string, isUpsell bool, canBeSold *bool, deleted bool) productResponse {
	return productResponse{
		ID:           id.String(),
		Plu:          plu,
		Name:         name,
		Barcode:      barcode,
		CategoryID:   categoryID,
		CategoryName: categoryName,
		Price:        price,
		Description:  description,
		ImageURLs:    imageURLs,
		IsUpsell:     isUpsell,
		CanBeSold:    canBeSold,
		Deleted:      deleted,
	}
}

// productQuery holds the shared listing query params.
type productQuery struct {
	Category     *string
	Search       *string
	FeaturedOnly bool
	Sort         string // one of validSorts; the SQL compares against the same strings
	Page         int
	PageSize     int
}

var validSorts = map[string]bool{
	"name_asc":   true,
	"price_asc":  true,
	"price_desc": true,
}

// parseProductQuery reads the shared listing query params: category, q
// (search), featured, sort, page, page_size. page/page_size are clamped to
// sane bounds rather than erroring on a bad value - a storefront filter UI
// shouldn't 400 just because it sent page=0. featured and sort are part of
// the contract, so an unknown value is a 400.
func parseProductQuery(r *http.Request) (productQuery, error) {
	q := r.URL.Query()
	pq := productQuery{Sort: "name_asc", Page: 1, PageSize: defaultPageSize}

	if v := q.Get("category"); v != "" {
		pq.Category = &v
	}
	if v := q.Get("q"); v != "" {
		pq.Search = &v
	}

	if v := q.Get("featured"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return productQuery{}, errors.New("featured must be true or false")
		}
		pq.FeaturedOnly = b
	}

	if v := q.Get("sort"); v != "" {
		if !validSorts[v] {
			return productQuery{}, errors.New("sort must be one of name_asc, price_asc, price_desc")
		}
		pq.Sort = v
	}

	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		pq.Page = v
	}
	if v, err := strconv.Atoi(q.Get("page_size")); err == nil && v > 0 {
		pq.PageSize = v
	}
	if pq.PageSize > maxPageSize {
		pq.PageSize = maxPageSize
	}

	return pq, nil
}

// listBranchProducts is the shared query path for
// GET /v1/admin/branches/{branchID}/products, the vendor products list and
// GET /v1/store/products.
func (s *Server) listBranchProducts(ctx context.Context, branchID uuid.UUID, productQuery productQuery, includeUnavailable bool) (paginatedProductsResponse, error) {
	total, err := s.queries.CountBranchProducts(ctx, db.CountBranchProductsParams{
		BranchID:           branchID,
		Category:           productQuery.Category,
		Search:             productQuery.Search,
		IncludeUnavailable: includeUnavailable,
		FeaturedOnly:       productQuery.FeaturedOnly,
	})
	if err != nil {
		return paginatedProductsResponse{}, err
	}

	rows, err := s.queries.ListBranchProducts(ctx, db.ListBranchProductsParams{
		BranchID:           branchID,
		Category:           productQuery.Category,
		Search:             productQuery.Search,
		IncludeUnavailable: includeUnavailable,
		FeaturedOnly:       productQuery.FeaturedOnly,
		Sort:               productQuery.Sort,
		PageLimit:          int32(productQuery.PageSize),
		PageOffset:         int32((productQuery.Page - 1) * productQuery.PageSize),
	})
	if err != nil {
		return paginatedProductsResponse{}, err
	}

	data := make([]productResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, toProductResponse(
			row.ID, row.Plu, row.Name, row.Barcode, row.CategoryID, row.CategoryName,
			row.Price, row.Description, row.ImageUrls, row.IsUpsell, row.CanBeSold, row.Deleted,
		))
	}

	totalPages := int(total) / productQuery.PageSize
	if int(total)%productQuery.PageSize != 0 {
		totalPages++
	}

	return paginatedProductsResponse{
		Data:       data,
		Page:       productQuery.Page,
		PageSize:   productQuery.PageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

// getBranchProduct is the shared single-product lookup for both
// GET /v1/admin/branches/{branchID}/products/{id} and GET /v1/store/products/{id}.
func (s *Server) getBranchProduct(ctx context.Context, branchID, productID uuid.UUID) (productResponse, error) {
	row, err := s.queries.GetBranchProduct(ctx, db.GetBranchProductParams{BranchID: branchID, ID: productID})
	if err != nil {
		return productResponse{}, err
	}
	return toProductResponse(
		row.ID, row.Plu, row.Name, row.Barcode, row.CategoryID, row.CategoryName,
		row.Price, row.Description, row.ImageUrls, row.IsUpsell, row.CanBeSold, row.Deleted,
	), nil
}

// listBranchCategories is the shared category listing for both
// GET /v1/admin/branches/{branchID}/categories and GET /v1/store/categories.
func (s *Server) listBranchCategories(ctx context.Context, branchID uuid.UUID, includeUnavailable bool) ([]categoryResponse, error) {
	rows, err := s.queries.ListBranchCategories(ctx, db.ListBranchCategoriesParams{
		BranchID:           branchID,
		IncludeUnavailable: includeUnavailable,
	})
	if err != nil {
		return nil, err
	}

	out := make([]categoryResponse, 0, len(rows))
	for _, row := range rows {
		if row.DepartmentUuid == nil {
			continue
		}
		name := *row.DepartmentUuid
		if row.Name != nil {
			name = *row.Name
		}
		out = append(out, categoryResponse{ID: *row.DepartmentUuid, Name: name})
	}
	return out, nil
}

// listBranchProductsByID backs GET /v1/store/products?ids=... (cart
// re-validation). Only products this branch can sell right now come back -
// an absent id is how the storefront knows an item became unavailable.
// Wrapped in the same paginated shape as the listing (one page holding
// everything) so the client parses both the same way.
func (s *Server) listBranchProductsByID(ctx context.Context, branchID uuid.UUID, ids []uuid.UUID) (paginatedProductsResponse, error) {
	rows, err := s.queries.ListBranchProductsByID(ctx, db.ListBranchProductsByIDParams{
		BranchID: branchID,
		Ids:      ids,
	})
	if err != nil {
		return paginatedProductsResponse{}, err
	}

	data := make([]productResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, toProductResponse(
			row.ID, row.Plu, row.Name, row.Barcode, row.CategoryID, row.CategoryName,
			row.Price, row.Description, row.ImageUrls, row.IsUpsell, row.CanBeSold, row.Deleted,
		))
	}

	return paginatedProductsResponse{
		Data:       data,
		Page:       1,
		PageSize:   len(data),
		TotalCount: int64(len(data)),
		TotalPages: 1,
	}, nil
}
