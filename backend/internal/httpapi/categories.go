package httpapi

import (
	"encoding/json"
	"net/http"

	"ecommerce/backend/internal/db"
)

type upsertCategoryRequest struct {
	DepartmentUUID string `json:"department_uuid"`
	Name           string `json:"name"`
	SortOrder      int32  `json:"sort_order"`
}

func toCategoryResponse(c db.Category) categoryResponse {
	return categoryResponse{ID: c.DepartmentUuid, Name: c.Name}
}

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.queries.ListCategories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]categoryResponse, 0, len(categories))
	for _, c := range categories {
		out = append(out, toCategoryResponse(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUpsertCategory creates or updates a category in one idempotent call
// (ON CONFLICT (department_uuid) DO UPDATE) - there's no separate PUT since
// this is a small admin-managed lookup table, not a synced resource.
func (s *Server) handleUpsertCategory(w http.ResponseWriter, r *http.Request) {
	var req upsertCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.DepartmentUUID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "department_uuid and name are required")
		return
	}

	category, err := s.queries.UpsertCategory(r.Context(), db.UpsertCategoryParams{
		DepartmentUuid: req.DepartmentUUID,
		Name:           req.Name,
		SortOrder:      req.SortOrder,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toCategoryResponse(category))
}
