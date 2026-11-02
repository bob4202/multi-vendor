package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"

	"github.com/google/uuid"
)

type storefrontBranchContextKey struct{}

// requireStorefrontKey resolves the branch entirely from the API key - the
// storefront never puts a branch ID in the URL, the key itself scopes every
// request to exactly one branch.
func (s *Server) requireStorefrontKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(auth, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}
		key := strings.TrimPrefix(auth, bearerPrefix)
		if key == "" {
			writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		branch, err := s.queries.GetBranchByStorefrontKeyHash(r.Context(), crypto.HashAPIKey(key))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid api key")
			return
		}

		ctx := context.WithValue(r.Context(), storefrontBranchContextKey{}, branch)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func branchFromContext(ctx context.Context) (db.Branch, bool) {
	branch, ok := ctx.Value(storefrontBranchContextKey{}).(db.Branch)
	return branch, ok
}

func (s *Server) handleStorefrontListProducts(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	if raw := r.URL.Query().Get("ids"); raw != "" {
		ids, err := parseProductIDs(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := s.listBranchProductsByID(r.Context(), branch.ID, ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	productQuery, err := parseProductQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := s.listBranchProducts(r.Context(), branch.ID, productQuery, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// maxLookupIDs is the max number of product IDs we'll accept in a single
// request.
const maxLookupIDs = 100

func parseProductIDs(raw string) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]struct{})
	ids := make([]uuid.UUID, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			return nil, fmt.Errorf("invalid product id: %q", part)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("ids must contain at least one product id")
	}
	if len(ids) > maxLookupIDs {
		return nil, fmt.Errorf("too many ids: max %d", maxLookupIDs)
	}
	return ids, nil
}

func (s *Server) handleStorefrontGetProduct(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	productID, err := productIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := s.getBranchProduct(r.Context(), branch.ID, productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	// The storefront never sees deleted/unsellable items, even by direct id.
	if product.Deleted || product.CanBeSold == nil || !*product.CanBeSold {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) handleStorefrontListCategories(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}

	categories, err := s.listBranchCategories(r.Context(), branch.ID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, categories)
}
