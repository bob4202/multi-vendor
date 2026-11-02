package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/db"
)

// Platform admin login for the admin UI. Like the vendor portal, only the
// Next.js server calls these and keeps the token in an httpOnly cookie.

type adminContextKey struct{}

type adminResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func toAdminResponse(a db.PlatformAdmin) adminResponse {
	return adminResponse{ID: a.ID.String(), Name: a.Name, Email: a.Email}
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		writeError(w, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error())
		return
	}

	res, err := s.platform.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	case errors.Is(err, auth.ErrAccountLocked):
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Token     string        `json:"token"`
		ExpiresAt time.Time     `json:"expires_at"`
		Admin     adminResponse `json:"admin"`
	}{res.Token, res.ExpiresAt, toAdminResponse(res.Admin)})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if token := bearerToken(r); token != "" {
		if err := s.platform.Logout(r.Context(), token); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminMe only makes sense with a session; the shared key has no
// person behind it.
func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	admin, ok := r.Context().Value(adminContextKey{}).(db.PlatformAdmin)
	if !ok {
		writeError(w, http.StatusBadRequest, "no admin session (request used X-Admin-Key)")
		return
	}
	writeJSON(w, http.StatusOK, toAdminResponse(admin))
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.platform.Overview(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
