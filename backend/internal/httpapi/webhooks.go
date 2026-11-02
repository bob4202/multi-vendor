package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"ecommerce/backend/internal/payment"
)

// Stripe's events are a few KB; this leaves plenty of room.
const maxWebhookBody = 1 << 20

// POST /v1/webhooks/{provider}/{accountID} - a vendor's payment provider
// telling us a card was held, declined, captured... No storefront key: the
// provider adapter checks the signature with that account's webhook secret.
func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chiURLParam(r, "accountID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// The signature covers the exact bytes, so read them before anything
	// decodes the body.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}

	err = s.payments.HandleWebhook(r.Context(), chiURLParam(r, "provider"), accountID, r, body)
	switch {
	case errors.Is(err, payment.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, payment.ErrBadWebhook):
		s.logger.Warn("payment webhook rejected", "account_id", accountID, "error", err)
		writeError(w, http.StatusBadRequest, "invalid webhook")
		return
	case err != nil:
		// 500 makes the provider retry later.
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"received": true})
}
