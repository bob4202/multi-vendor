package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"ecommerce/backend/internal/portal"
)

// Public invite routes: the token in the URL is the credential. Unknown
// tokens are 404; used or expired ones 410, so the page can say "ask for a
// new link" rather than "not found".

func (s *Server) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	info, err := s.portal.GetInvite(r.Context(), chi.URLParam(r, "token"))
	if s.writeInviteError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Email      string    `json:"email"`
		Name       string    `json:"name"`
		VendorName string    `json:"vendor_name"`
		ExpiresAt  time.Time `json:"expires_at"`
	}{info.Email, info.Name, info.VendorName, info.ExpiresAt})
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.portal.AcceptInvite(r.Context(), chi.URLParam(r, "token"), req.Password, r.UserAgent())
	if s.writeInviteError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		User:      toVendorUserResponse(res.User),
		Vendor:    toVendorResponse(res.Vendor),
	})
}

func (s *Server) writeInviteError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, portal.ErrInviteNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, portal.ErrInviteExpired):
		writeError(w, http.StatusGone, err.Error())
	default:
		return s.writePortalError(w, r, err)
	}
	return true
}
