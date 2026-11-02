package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/portal"
)

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// branchResponse never includes the API key - or the storefront key hash,
// only its prefix, so an admin UI can identify which key is active without
// the secret ever being re-exposed after creation.
type branchResponse struct {
	ID                              string   `json:"id"`
	Name                            string   `json:"name"`
	Host                            string   `json:"host"`
	APIUsername                     string   `json:"api_username"`
	IsActive                        bool     `json:"is_active"`
	AllowedSecondaryDepartmentUUIDs []string `json:"allowed_secondary_department_uuids"`
	AllowedProductRefs              []string `json:"allowed_product_refs"`
	StorefrontAPIKeyPrefix          *string  `json:"storefront_api_key_prefix"`
	VendorID                        string   `json:"vendor_id"`
	StorefrontURL                   *string  `json:"storefront_url"`
}

func toBranchResponse(b db.Branch) branchResponse {
	deptUUIDs := b.AllowedSecondaryDepartmentUuids
	if deptUUIDs == nil {
		deptUUIDs = []string{}
	}
	refs := b.AllowedProductRefs
	if refs == nil {
		refs = []string{}
	}
	return branchResponse{
		ID:                              b.ID.String(),
		Name:                            b.Name,
		Host:                            b.Host,
		APIUsername:                     b.ApiUsername,
		IsActive:                        b.IsActive,
		AllowedSecondaryDepartmentUUIDs: deptUUIDs,
		AllowedProductRefs:              refs,
		StorefrontAPIKeyPrefix:          b.StorefrontApiKeyPrefix,
		VendorID:                        b.VendorID.String(),
		StorefrontURL:                   b.StorefrontUrl,
	}
}

// createBranchRequest is reused for both create and update. On update, a nil
// pointer field means "leave unchanged" - same convention as IsActive - so
// an explicit empty array must be sent to clear a filter.
type createBranchRequest struct {
	Name                            string     `json:"name"`
	Host                            string     `json:"host"`
	APIUsername                     string     `json:"api_username"`
	APIKey                          string     `json:"api_key"`
	IsActive                        *bool      `json:"is_active"`
	AllowedSecondaryDepartmentUUIDs *[]string  `json:"allowed_secondary_department_uuids"`
	AllowedProductRefs              *[]string  `json:"allowed_product_refs"`
	VendorID                        *uuid.UUID `json:"vendor_id"` // required on create
	// Base URL of the branch's storefront, for links in customer emails.
	// "" clears it.
	StorefrontURL *string `json:"storefront_url"`
}

// normalizeStorefrontURL trims and drops a trailing slash; "" means none.
// Only http(s) URLs are accepted, since they end up as links in emails.
func normalizeStorefrontURL(raw string) (*string, error) {
	v := strings.TrimRight(strings.TrimSpace(raw), "/")
	if v == "" {
		return nil, nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("storefront_url must be an http(s) URL, e.g. https://shop.example.com")
	}
	return &v, nil
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
	branches, err := s.queries.ListBranches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]branchResponse, 0, len(branches))
	for _, b := range branches {
		out = append(out, toBranchResponse(b))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetBranch(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	branch, err := s.queries.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return
	}
	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (s *Server) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	var req createBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Name == "" || req.Host == "" || req.APIUsername == "" || req.APIKey == "" || req.VendorID == nil {
		writeError(w, http.StatusBadRequest, "name, host, api_username, api_key and vendor_id are required")
		return
	}

	var storefrontURL *string
	if req.StorefrontURL != nil {
		v, err := normalizeStorefrontURL(*req.StorefrontURL)
		if err != nil {
			writeIssues(w, err.Error(), []portal.Issue{{Field: "storefront_url", Message: err.Error()}})
			return
		}
		storefrontURL = v
	}

	cipher, err := s.secretBox.Encrypt(req.APIKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt api key")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	deptUUIDs := []string{}
	if req.AllowedSecondaryDepartmentUUIDs != nil {
		deptUUIDs = *req.AllowedSecondaryDepartmentUUIDs
	}
	productRefs := []string{}
	if req.AllowedProductRefs != nil {
		productRefs = *req.AllowedProductRefs
	}

	branch, err := s.queries.CreateBranch(r.Context(), db.CreateBranchParams{
		Name:                            req.Name,
		Host:                            req.Host,
		ApiUsername:                     req.APIUsername,
		ApiKeyCipher:                    cipher,
		IsActive:                        isActive,
		AllowedSecondaryDepartmentUuids: deptUUIDs,
		AllowedProductRefs:              productRefs,
		VendorID:                        *req.VendorID,
		StorefrontUrl:                   storefrontURL,
	})
	if isForeignKeyViolation(err) {
		writeError(w, http.StatusUnprocessableEntity, "vendor not found")
		return
	}
	if isUniqueViolation(err) {
		writeIssues(w, "host already used", []portal.Issue{{Field: "host", Message: "Another branch already uses this inventory system host"}})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "branch.created", "branch", branch.ID.String(), fmt.Sprintf("Created branch %s", branch.Name))
	writeJSON(w, http.StatusCreated, toBranchResponse(branch))
}

func (s *Server) handleUpdateBranch(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}

	existing, err := s.queries.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return
	}

	var req createBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	name, host, username := existing.Name, existing.Host, existing.ApiUsername
	if req.Name != "" {
		name = req.Name
	}
	if req.Host != "" {
		host = req.Host
	}
	if req.APIUsername != "" {
		username = req.APIUsername
	}

	cipher := existing.ApiKeyCipher
	if req.APIKey != "" {
		cipher, err = s.secretBox.Encrypt(req.APIKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt api key")
			return
		}
	}

	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	deptUUIDs := existing.AllowedSecondaryDepartmentUuids
	if req.AllowedSecondaryDepartmentUUIDs != nil {
		deptUUIDs = *req.AllowedSecondaryDepartmentUUIDs
	}
	productRefs := existing.AllowedProductRefs
	if req.AllowedProductRefs != nil {
		productRefs = *req.AllowedProductRefs
	}
	vendorID := existing.VendorID
	if req.VendorID != nil {
		vendorID = *req.VendorID
	}
	storefrontURL := existing.StorefrontUrl
	if req.StorefrontURL != nil {
		storefrontURL, err = normalizeStorefrontURL(*req.StorefrontURL)
		if err != nil {
			writeIssues(w, err.Error(), []portal.Issue{{Field: "storefront_url", Message: err.Error()}})
			return
		}
	}

	branch, err := s.queries.UpdateBranch(r.Context(), db.UpdateBranchParams{
		ID:                              id,
		Name:                            name,
		Host:                            host,
		ApiUsername:                     username,
		ApiKeyCipher:                    cipher,
		IsActive:                        isActive,
		AllowedSecondaryDepartmentUuids: deptUUIDs,
		AllowedProductRefs:              productRefs,
		VendorID:                        vendorID,
		StorefrontUrl:                   storefrontURL,
	})
	// Moving a branch to another vendor fails while that vendor's staff are
	// still assigned to it (composite FK on vendor_user_branches).
	if isForeignKeyViolation(err) {
		writeError(w, http.StatusConflict, "vendor not found, or staff of the current vendor are still assigned to this branch")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "branch.updated", "branch", branch.ID.String(), fmt.Sprintf("Updated branch %s", branch.Name))
	writeJSON(w, http.StatusOK, toBranchResponse(branch))
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	branch, err := s.queries.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return
	}

	// Sync runs in the background; poll GET /sync-state for the result.
	s.startSync(branch)
	s.audit(r, "branch.sync_started", "branch", branch.ID.String(), fmt.Sprintf("Started a sync for %s", branch.Name))

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync started"})
}

func (s *Server) handleSyncState(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	states, err := s.queries.ListSyncState(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, states)
}

// handleRotateStorefrontKey issues a new storefront API key for a branch,
// replacing any existing one. The plaintext key is returned exactly once -
// only its hash is persisted, so it can never be recovered after this
// response.
func (s *Server) handleRotateStorefrontKey(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	branch, err := s.queries.GetBranch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return
	}

	plaintext, err := crypto.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate api key")
		return
	}
	hash := crypto.HashAPIKey(plaintext)
	prefix := crypto.KeyPrefix(plaintext)

	if _, err := s.queries.SetBranchStorefrontKey(r.Context(), db.SetBranchStorefrontKeyParams{
		ID:                     id,
		StorefrontApiKeyHash:   hash,
		StorefrontApiKeyPrefix: &prefix,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.audit(r, "branch.storefront_key_issued", "branch", id.String(), fmt.Sprintf("Issued a new storefront key for %s", branch.Name))
	writeJSON(w, http.StatusCreated, map[string]string{
		"api_key": plaintext,
		"prefix":  prefix,
		"note":    "this key is shown once and cannot be retrieved again - store it now",
	})
}

func (s *Server) handleRevokeStorefrontKey(w http.ResponseWriter, r *http.Request) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	if err := s.queries.RevokeBranchStorefrontKey(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "branch.storefront_key_revoked", "branch", id.String(), "Revoked the storefront key")
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminListBranchProducts additionally accepts include_unavailable,
// unlike the storefront equivalent, so internal tooling can see
// deleted/unsellable rows too (e.g. to debug why something disappeared).
func (s *Server) handleAdminListBranchProducts(w http.ResponseWriter, r *http.Request) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	productQuery, err := parseProductQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	includeUnavailable := r.URL.Query().Get("include_unavailable") == "true"

	resp, err := s.listBranchProducts(r.Context(), branchID, productQuery, includeUnavailable)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminGetBranchProduct(w http.ResponseWriter, r *http.Request) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	productID, err := productIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}
	product, err := s.getBranchProduct(r.Context(), branchID, productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found for this branch")
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) handleAdminListBranchCategories(w http.ResponseWriter, r *http.Request) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	includeUnavailable := r.URL.Query().Get("include_unavailable") == "true"

	categories, err := s.listBranchCategories(r.Context(), branchID, includeUnavailable)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, categories)
}
