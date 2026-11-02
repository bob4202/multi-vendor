package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/portal"
)

// Store info, closures, products and sync for one branch in the vendor
// portal. Branch access and role checks happen in middleware (router.go).

type vendorStoreInfoResponse struct {
	Name string `json:"name"`
	portal.StoreInfo
}

func (s *Server) handleVendorGetStoreInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.portal.GetStoreInfo(r.Context(), vendorBranchID(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vendorStoreInfoResponse{Name: vendorBranchFromContext(r.Context()).Name, StoreInfo: info})
}

func (s *Server) handleVendorSaveStoreInfo(w http.ResponseWriter, r *http.Request) {
	var req portal.StoreInfo
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := s.portal.SaveStoreInfo(r.Context(), vendorBranchID(r), req)
	if s.writePortalError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, vendorStoreInfoResponse{Name: vendorBranchFromContext(r.Context()).Name, StoreInfo: info})
}

func (s *Server) handleVendorListClosures(w http.ResponseWriter, r *http.Request) {
	closures, err := s.portal.ListClosures(r.Context(), vendorBranchID(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, closures)
}

func (s *Server) handleVendorAddClosure(w http.ResponseWriter, r *http.Request) {
	var req portal.Closure
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	closure, err := s.portal.AddClosure(r.Context(), vendorBranchID(r), req)
	if s.writePortalError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, closure)
}

func (s *Server) handleVendorDeleteClosure(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "closureID"), 10, 32)
	if err != nil {
		writeError(w, http.StatusNotFound, "closure not found")
		return
	}
	if err := s.portal.DeleteClosure(r.Context(), vendorBranchID(r), int32(id)); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleVendorListProducts shows everything synced for the branch, including
// unsellable and deleted rows, so staff can see why something isn't online.
func (s *Server) handleVendorListProducts(w http.ResponseWriter, r *http.Request) {
	productQuery, err := parseProductQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := s.listBranchProducts(r.Context(), vendorBranchID(r), productQuery, true)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleVendorListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.listBranchCategories(r.Context(), vendorBranchID(r), true)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (s *Server) handleVendorSyncState(w http.ResponseWriter, r *http.Request) {
	states, err := s.queries.ListSyncState(r.Context(), vendorBranchID(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if states == nil {
		states = []db.SyncState{}
	}
	writeJSON(w, http.StatusOK, states)
}

func (s *Server) handleVendorTriggerSync(w http.ResponseWriter, r *http.Request) {
	branch, err := s.queries.GetBranch(r.Context(), vendorBranchID(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.startSync(branch)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync started"})
}

// startSync runs a branch sync in the background; callers poll sync-state.
// A detached context is used deliberately: the request context is cancelled
// the moment the handler returns, which would kill the sync instantly.
func (s *Server) startSync(branch db.Branch) {
	go func() {
		if err := s.engine.SyncBranch(context.Background(), branch); err != nil {
			s.logger.Error("manual sync failed", "branch_id", branch.ID, "error", err)
		}
	}()
}

// writePortalError maps portal validation and conflict errors to responses.
// It returns false (writing nothing) when err is nil.
func (s *Server) writePortalError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var verr *portal.ValidationError
	switch {
	case errors.As(err, &verr):
		writeIssues(w, "please fix the highlighted fields", verr.Issues)
	case errors.Is(err, portal.ErrClosureExists):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.internalError(w, r, err)
	}
	return true
}
