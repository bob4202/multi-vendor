package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ecommerce/backend/internal/customer"
	"ecommerce/backend/internal/db"
)

// Storefront customer accounts (/v1/store/auth, /v1/store/me). The Next
// server keeps the session token in an httpOnly cookie and forwards it in
// X-Customer-Session, next to the storefront key in Authorization.

const customerSessionHeader = "X-Customer-Session"

type customerContextKey struct{}

func customerFromContext(ctx context.Context) (db.CustomerAccount, bool) {
	c, ok := ctx.Value(customerContextKey{}).(db.CustomerAccount)
	return c, ok
}

// withCustomer attaches the signed-in customer, if any. A missing, expired
// or other-vendor session just means "guest" here; routes that need an
// account use requireCustomer.
func (s *Server) withCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(customerSessionHeader)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		account, err := s.customers.Authenticate(r.Context(), token)
		if err != nil && !errors.Is(err, customer.ErrUnauthenticated) {
			s.internalError(w, r, err)
			return
		}
		branch, _ := branchFromContext(r.Context())
		if err == nil && account.VendorID == branch.VendorID {
			r = r.WithContext(context.WithValue(r.Context(), customerContextKey{}, account))
		}
		next.ServeHTTP(w, r)
	})
}

func requireCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := customerFromContext(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "sign in to continue")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type customerResponse struct {
	ID    string  `json:"id"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
}

func toCustomerResponse(c db.CustomerAccount) customerResponse {
	return customerResponse{ID: c.ID.String(), Email: c.Email, Name: c.Name, Phone: c.Phone}
}

// POST /v1/store/auth/code {email}
func (s *Server) handleCustomerSendCode(w http.ResponseWriter, r *http.Request) {
	branch, _ := branchFromContext(r.Context())
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := s.customers.SendCode(r.Context(), branch.VendorID, branch.Name, req.Email)
	var tooSoon *customer.TooSoonError
	switch {
	case errors.Is(err, customer.ErrInvalidEmail):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": err.Error(), "issues": []customer.Issue{{Field: "email", Message: "Enter a valid email address"}},
		})
	case errors.As(err, &tooSoon):
		w.Header().Set("Retry-After", strconv.Itoa(int(tooSoon.RetryAfter.Seconds())))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": err.Error(), "retry_after": int(tooSoon.RetryAfter.Seconds()),
		})
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusAccepted, map[string]any{})
	}
}

// POST /v1/store/auth/verify {email, code}
func (s *Server) handleCustomerVerify(w http.ResponseWriter, r *http.Request) {
	branch, _ := branchFromContext(r.Context())
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.customers.Verify(r.Context(), branch.VendorID, req.Email, req.Code, r.UserAgent())
	switch {
	case errors.Is(err, customer.ErrInvalidCode):
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	case errors.Is(err, customer.ErrTooManyAttempts):
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Token     string           `json:"token"`
		ExpiresAt time.Time        `json:"expires_at"`
		Customer  customerResponse `json:"customer"`
	}{res.Token, res.ExpiresAt, toCustomerResponse(res.Account)})
}

// POST /v1/store/auth/logout
func (s *Server) handleCustomerLogout(w http.ResponseWriter, r *http.Request) {
	if token := r.Header.Get(customerSessionHeader); token != "" {
		if err := s.customers.Logout(r.Context(), token); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/store/me
func (s *Server) handleCustomerMe(w http.ResponseWriter, r *http.Request) {
	c, _ := customerFromContext(r.Context())
	writeJSON(w, http.StatusOK, toCustomerResponse(c))
}

// PATCH /v1/store/me {name?, phone?}
func (s *Server) handleCustomerUpdateMe(w http.ResponseWriter, r *http.Request) {
	c, _ := customerFromContext(r.Context())
	var req struct {
		Name  *string `json:"name"`
		Phone *string `json:"phone"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.customers.UpdateProfile(r.Context(), c.ID, req.Name, req.Phone)
	var invalid *customer.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "please check your details", "issues": invalid.Issues})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toCustomerResponse(updated))
}

type customerOrderSummaryResponse struct {
	ID          string                  `json:"id"`
	Reference   string                  `json:"reference"`
	Status      string                  `json:"status"`
	BranchName  string                  `json:"branch_name"`
	Fulfilment  orderFulfilmentResponse `json:"fulfilment"`
	ItemCount   int32                   `json:"item_count"`
	Total       int64                   `json:"total"`
	CreatedAt   time.Time               `json:"created_at"`
	NeedsAction bool                    `json:"needs_action"` // a substitution is waiting for an answer
}

// GET /v1/store/me/orders?page&page_size
func (s *Server) handleCustomerOrders(w http.ResponseWriter, r *http.Request) {
	c, _ := customerFromContext(r.Context())
	page, pageSize := parsePage(r)
	res, err := s.customers.ListOrders(r.Context(), c.ID, page, pageSize)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	data := make([]customerOrderSummaryResponse, 0, len(res.Orders))
	for _, o := range res.Orders {
		data = append(data, customerOrderSummaryResponse{
			ID: o.ID.String(), Reference: o.Reference, Status: o.Status, BranchName: o.BranchName,
			Fulfilment:  orderFulfilmentResponse{Type: o.FulfilmentType, Date: o.FulfilmentDate.Time.Format(time.DateOnly)},
			ItemCount:   o.ItemCount,
			Total:       o.Total,
			CreatedAt:   o.CreatedAt.Time,
			NeedsAction: o.NeedsAction,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":        data,
		"page":        page,
		"page_size":   pageSize,
		"total_count": res.Total,
		"total_pages": int((res.Total + int64(pageSize) - 1) / int64(pageSize)),
	})
}
