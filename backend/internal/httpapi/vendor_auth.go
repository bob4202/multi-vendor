package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/portal"
)

// Vendor portal auth. Only the Next.js server calls these routes: it keeps
// the session token in an httpOnly cookie and forwards it as a bearer token,
// so the token never reaches browser JavaScript.

type principalContextKey struct{}
type vendorBranchContextKey struct{}

func principalFromContext(ctx context.Context) portal.Principal {
	p, _ := ctx.Value(principalContextKey{}).(portal.Principal)
	return p
}

func vendorBranchFromContext(ctx context.Context) db.ListVendorUserBranchesRow {
	b, _ := ctx.Value(vendorBranchContextKey{}).(db.ListVendorUserBranchesRow)
	return b
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimPrefix(auth, prefix)
}

func (s *Server) requireVendorSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.portal.Authenticate(r.Context(), bearerToken(r))
		if errors.Is(err, portal.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireBranchAccess resolves {branchID} against the branches the user may
// manage. A branch they can't reach is a 404, not a 403, so branch IDs of
// other vendors can't be probed.
func (s *Server) requireBranchAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		branchID, err := branchIDFromRequest(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "branch not found")
			return
		}
		branches, err := s.portal.Branches(r.Context(), principalFromContext(r.Context()).User.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		for _, b := range branches {
			if b.ID == branchID {
				ctx := context.WithValue(r.Context(), vendorBranchContextKey{}, b)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		writeError(w, http.StatusNotFound, "branch not found")
	})
}

func requirePermission(action portal.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !principalFromContext(r.Context()).Can(action) {
				writeError(w, http.StatusForbidden, "your role doesn't allow this")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type vendorUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type vendorResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type vendorBranchResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// permissionsResponse tells the portal which controls to show, so the
// frontend never has to mirror the role table. The API still enforces it.
type permissionsResponse struct {
	UpdateOrderStatus bool `json:"update_order_status"`
	EditStoreInfo     bool `json:"edit_store_info"`
	TriggerSync       bool `json:"trigger_sync"`
	RefundOrders      bool `json:"refund_orders"`
}

type loginResponse struct {
	Token     string             `json:"token"`
	ExpiresAt time.Time          `json:"expires_at"`
	User      vendorUserResponse `json:"user"`
	Vendor    vendorResponse     `json:"vendor"`
}

type meResponse struct {
	User        vendorUserResponse     `json:"user"`
	Vendor      vendorResponse         `json:"vendor"`
	Branches    []vendorBranchResponse `json:"branches"`
	Permissions permissionsResponse    `json:"permissions"`
}

func toVendorUserResponse(u db.VendorUser) vendorUserResponse {
	return vendorUserResponse{ID: u.ID.String(), Name: u.Name, Email: u.Email, Role: u.Role}
}

func toVendorResponse(v db.Vendor) vendorResponse {
	return vendorResponse{ID: v.ID.String(), Name: v.Name}
}

func (s *Server) handleVendorLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		writeError(w, http.StatusUnauthorized, portal.ErrInvalidCredentials.Error())
		return
	}

	res, err := s.portal.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	switch {
	case errors.Is(err, portal.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	case errors.Is(err, portal.ErrAccountLocked):
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		User:      toVendorUserResponse(res.User),
		Vendor:    toVendorResponse(res.Vendor),
	})
}

func (s *Server) handleVendorLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.portal.Logout(r.Context(), bearerToken(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVendorMe(w http.ResponseWriter, r *http.Request) {
	p := principalFromContext(r.Context())
	branches, err := s.portal.Branches(r.Context(), p.User.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]vendorBranchResponse, 0, len(branches))
	for _, b := range branches {
		out = append(out, vendorBranchResponse{ID: b.ID.String(), Name: b.Name, IsActive: b.IsActive})
	}
	writeJSON(w, http.StatusOK, meResponse{
		User:     toVendorUserResponse(p.User),
		Vendor:   toVendorResponse(p.Vendor),
		Branches: out,
		Permissions: permissionsResponse{
			UpdateOrderStatus: p.Can(portal.UpdateOrderStatus),
			EditStoreInfo:     p.Can(portal.EditStoreInfo),
			TriggerSync:       p.Can(portal.TriggerSync),
			RefundOrders:      p.Can(portal.RefundOrders),
		},
	})
}

// vendorBranchID is the branch a /v1/vendor/branches/{branchID} request is
// scoped to, already checked by requireBranchAccess.
func vendorBranchID(r *http.Request) uuid.UUID {
	return vendorBranchFromContext(r.Context()).ID
}
