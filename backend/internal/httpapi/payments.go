package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/payment"
)

type paymentSessionRequest struct {
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

type paymentSessionResponse struct {
	RedirectURL string    `json:"redirect_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// POST /v1/store/orders/{orderID}/payment-session {"success_url", "cancel_url"}
// Starts (or returns the open) hosted payment page for an order awaiting
// payment. The storefront redirects the customer to redirect_url.
func (s *Server) handleStorefrontStartPaymentSession(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	orderID, err := uuid.Parse(chiURLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	var req paymentSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isHTTPURL(req.SuccessURL) || !isHTTPURL(req.CancelURL) {
		writeError(w, http.StatusBadRequest, "success_url and cancel_url must be http(s) URLs")
		return
	}

	session, err := s.payments.StartCheckout(r.Context(), payment.CheckoutParams{
		BranchID:   branch.ID,
		OrderID:    orderID,
		SuccessURL: req.SuccessURL,
		CancelURL:  req.CancelURL,
	})
	switch {
	case errors.Is(err, payment.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, payment.ErrNotAwaitingPayment),
		errors.Is(err, payment.ErrPaymentExpired),
		errors.Is(err, payment.ErrCheckoutInProgress):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, payment.ErrProviderNotSetup):
		writeError(w, http.StatusServiceUnavailable, "This shop isn't taking online payments right now.")
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, paymentSessionResponse{
		RedirectURL: session.RedirectURL,
		ExpiresAt:   session.ExpiresAt,
	})
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
