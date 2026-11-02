package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/store"
)

const (
	maxOrderBodyBytes    = 64 << 10 // 64 KB - a 50-line basket is a few KB
	maxIdempotencyKeyLen = 128
)

// --- request ----------------------------------------------------------------

// placeOrderRequest is the JSON the storefront sends. It's kept separate from
// store.PlaceOrderInput: this mirrors the wire format (strings, nesting), the
// handler turns it into clean, typed input for the service.
type placeOrderRequest struct {
	Items []struct {
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
	Fulfilment struct {
		Type string `json:"type"`
		Date string `json:"date"`
	} `json:"fulfilment"`
	Customer struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
	} `json:"customer"`
	Address *struct {
		Line1    string `json:"line1"`
		Line2    string `json:"line2"`
		City     string `json:"city"`
		Postcode string `json:"postcode"`
	} `json:"address"`
	Notes         string `json:"notes"`
	ExpectedTotal *int64 `json:"expected_total"`
	// The version of the terms the customer ticked. Required.
	TermsVersion string `json:"terms_version"`
	// Defaults to true when left out.
	AllowSubstitutions *bool `json:"allow_substitutions"`
}

type rejectOrderRequest struct {
	Reason string `json:"reason"`
}

// --- response ---------------------------------------------------------------

type orderResponse struct {
	ID         string `json:"id"`
	Reference  string `json:"reference"`
	Status     string `json:"status"`
	Fulfilment struct {
		Type string `json:"type"`
		Date string `json:"date"`
	} `json:"fulfilment"`
	Customer struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
	} `json:"customer"`
	Address         *orderAddressResponse `json:"address"`
	Notes           *string               `json:"notes"`
	RejectionReason *string               `json:"rejection_reason"`
	Items           []orderItemResponse   `json:"items"`
	Subtotal        int64                 `json:"subtotal"`
	DeliveryFee     int64                 `json:"delivery_fee"`
	Total           int64                 `json:"total"`
	CreatedAt       time.Time             `json:"created_at"`
	AcceptDeadline  time.Time             `json:"accept_deadline"`
	AcceptedAt      *time.Time            `json:"accepted_at"`
	CancelledAt     *time.Time            `json:"cancelled_at"`

	BranchName         string `json:"branch_name"`
	AllowSubstitutions bool   `json:"allow_substitutions"`
	AwaitingCustomer   bool   `json:"awaiting_customer"`
	// Only while there's something to collect: accepted, picking or ready.
	CollectionCode *string    `json:"collection_code"`
	ReadyAt        *time.Time `json:"ready_at"`
	CollectBy      *time.Time `json:"collect_by"`
	// Only while awaiting_payment: the order is dropped if not paid by then.
	PaymentDeadline *time.Time       `json:"payment_deadline"`
	Payment         *paymentResponse `json:"payment"`
}

// paymentResponse is the "payment" block on an order (storefront and vendor).
type paymentResponse struct {
	Status           string `json:"status"`
	AmountAuthorised int64  `json:"amount_authorised"`
	AmountCaptured   int64  `json:"amount_captured"`
	AmountRefunded   int64  `json:"amount_refunded"`
}

func toPaymentResponse(p *store.Payment) *paymentResponse {
	if p == nil {
		return nil
	}
	return &paymentResponse{
		Status: p.Status, AmountAuthorised: p.AmountAuthorised,
		AmountCaptured: p.AmountCaptured, AmountRefunded: p.AmountRefunded,
	}
}

type orderAddressResponse struct {
	Line1    string  `json:"line1"`
	Line2    *string `json:"line2"`
	City     string  `json:"city"`
	Postcode string  `json:"postcode"`
}

type orderItemResponse struct {
	ID           string                `json:"id"`
	ProductID    string                `json:"product_id"`
	Name         string                `json:"name"`
	UnitPrice    int64                 `json:"unit_price"`
	Quantity     int                   `json:"quantity"`
	LineTotal    int64                 `json:"line_total"`
	Status       string                `json:"status"`
	ChargedTotal int64                 `json:"charged_total"`
	Substitution *substitutionResponse `json:"substitution"`
}

type substitutionResponse struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	Name      string    `json:"name"`
	Quantity  int       `json:"quantity"`
	UnitPrice int64     `json:"unit_price"` // the substitute's own price
	LineTotal int64     `json:"line_total"` // what the customer pays for it
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

func toSubstitutionResponse(sub *store.Substitution) *substitutionResponse {
	if sub == nil {
		return nil
	}
	return &substitutionResponse{
		ID: sub.ID.String(), ProductID: sub.ProductID.String(), Name: sub.Name,
		Quantity: sub.Quantity, UnitPrice: sub.UnitPrice, LineTotal: sub.LineTotal,
		Status: sub.Status, ExpiresAt: sub.ExpiresAt,
	}
}

// showCollectionCode: the code only matters while there's something to
// collect.
func showCollectionCode(o store.Order) *string {
	switch o.Status {
	case "accepted", "picking", "ready":
		return o.CollectionCode
	}
	return nil
}

type issueResponse struct {
	Code      string `json:"code"`
	Field     string `json:"field,omitempty"`
	ProductID string `json:"product_id,omitempty"`
	Message   string `json:"message"`
}

func toOrderResponse(o store.Order, branchName string) orderResponse {
	resp := orderResponse{
		BranchName:         branchName,
		AllowSubstitutions: o.AllowSubstitutions,
		AwaitingCustomer:   o.AwaitingCustomer(),
		CollectionCode:     showCollectionCode(o),
		ReadyAt:            o.ReadyAt,
		CollectBy:          o.CollectBy,
		Payment:            toPaymentResponse(o.Payment),

		ID:              o.ID.String(),
		Reference:       o.Reference,
		Status:          o.Status,
		Notes:           o.Notes,
		RejectionReason: o.RejectionReason,
		Items:           make([]orderItemResponse, 0, len(o.Items)),
		Subtotal:        o.Subtotal,
		DeliveryFee:     o.DeliveryFee,
		Total:           o.Total,
		CreatedAt:       o.CreatedAt,
		AcceptDeadline:  o.AcceptDeadline,
		AcceptedAt:      o.AcceptedAt,
		CancelledAt:     o.CancelledAt,
	}
	if o.Status == store.AwaitingPayment {
		resp.PaymentDeadline = o.PaymentDeadline
	}
	resp.Fulfilment.Type = o.FulfilmentType
	resp.Fulfilment.Date = o.FulfilmentDate
	resp.Customer.Name = o.CustomerName
	resp.Customer.Email = o.CustomerEmail
	resp.Customer.Phone = o.CustomerPhone
	if o.Address != nil {
		resp.Address = &orderAddressResponse{
			Line1: o.Address.Line1, Line2: o.Address.Line2,
			City: o.Address.City, Postcode: o.Address.Postcode,
		}
	}
	for _, item := range o.Items {
		resp.Items = append(resp.Items, orderItemResponse{
			ID:           item.ID.String(),
			ProductID:    item.ProductID.String(),
			Name:         item.Name,
			UnitPrice:    item.UnitPrice,
			Quantity:     item.Quantity,
			LineTotal:    item.LineTotal,
			Status:       item.Status,
			ChargedTotal: item.ChargedTotal,
			Substitution: toSubstitutionResponse(item.Substitution),
		})
	}
	return resp
}

// writeOrderError maps the order service's errors onto HTTP statuses. Anything
// unexpected is logged and reported as a generic 500.
func (s *Server) writeOrderError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *store.ValidationError
	switch {
	case errors.As(err, &validation):
		issues := make([]issueResponse, 0, len(validation.Problems))
		for _, p := range validation.Problems {
			issue := issueResponse{Code: p.Code, Field: p.Field, Message: p.Message}
			if p.ProductID != nil {
				issue.ProductID = p.ProductID.String()
			}
			issues = append(issues, issue)
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "the order can't be placed as it is",
			"issues": issues,
		})
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrOrderNotPending):
		writeError(w, http.StatusConflict, "this order can no longer be accepted or rejected")
	case errors.Is(err, store.ErrPaymentProviderNotFound):
		// Shown to the customer as is by the checkout page.
		writeError(w, http.StatusServiceUnavailable, "This shop isn't taking online orders right now. Please try again later.")
	default:
		s.internalError(w, r, err)
	}
}

// --- storefront -------------------------------------------------------------

// POST /v1/store/orders
func (s *Server) handleStorefrontPlaceOrder(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > maxIdempotencyKeyLen {
		writeError(w, http.StatusBadRequest, "Idempotency-Key header is required (max 128 characters)")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxOrderBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req placeOrderRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	in := store.PlaceOrderInput{
		BranchID:        branch.ID,
		IdempotencyKey:  key,
		FulfillmentType: req.Fulfilment.Type,
		FulfillmentDate: req.Fulfilment.Date,
		CustomerName:    req.Customer.Name,
		CustomerEmail:   req.Customer.Email,
		CustomerPhone:   req.Customer.Phone,
		Notes:           req.Notes,
		ExpectedTotal:   req.ExpectedTotal,

		TermsVersion:       req.TermsVersion,
		AllowSubstitutions: req.AllowSubstitutions == nil || *req.AllowSubstitutions,
	}
	if c, ok := customerFromContext(r.Context()); ok {
		in.CustomerAccountID = &c.ID
	}
	for _, item := range req.Items {
		id, err := uuid.Parse(item.ProductID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid product_id: "+item.ProductID)
			return
		}
		in.Items = append(in.Items, store.ItemInput{ProductID: id, Quantity: item.Quantity})
	}
	if req.Address != nil {
		in.AddressLine1 = req.Address.Line1
		in.AddressLine2 = req.Address.Line2
		in.City = req.Address.City
		in.Postcode = req.Address.Postcode
	}

	order, created, err := s.orders.PlaceOrder(r.Context(), in)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK // replay of an order already placed with this key
	}

	//TODO: Send notification to the user

	writeJSON(w, status, toOrderResponse(order, branch.Name))
}

// GET /v1/store/orders/{orderID} - the customer's order status page polls this.
func (s *Server) handleStorefrontGetOrder(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	orderID, err := uuid.Parse(chiURLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	order, branchName, err := s.storefrontOrder(r, branch, orderID)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order, branchName))
}

// POST /v1/store/orders/{orderID}/substitutions/{substitutionID} {"accept": bool}
func (s *Server) handleStorefrontRespondSubstitution(w http.ResponseWriter, r *http.Request) {
	branch, _ := branchFromContext(r.Context())
	orderID, err1 := uuid.Parse(chiURLParam(r, "orderID"))
	subID, err2 := uuid.Parse(chiURLParam(r, "substitutionID"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var req struct {
		Accept *bool `json:"accept"`
	}
	if err := decodeJSON(w, r, &req); err != nil || req.Accept == nil {
		writeError(w, http.StatusBadRequest, "accept (true or false) is required")
		return
	}
	order, branchName, err := s.storefrontOrder(r, branch, orderID)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}
	updated, err := s.orders.RespondToSubstitution(r.Context(), order.BranchID, order.ID, subID, *req.Accept)
	switch {
	case errors.Is(err, store.ErrSubstitutionNotFound), errors.Is(err, store.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, store.ErrSubstitutionClosed):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(updated, branchName))
}

// storefrontOrder loads an order the storefront may show: one of this
// branch's, or - for a signed-in customer - one of their own from any of the
// vendor's branches ("My orders" lists them all).
func (s *Server) storefrontOrder(r *http.Request, branch db.Branch, orderID uuid.UUID) (store.Order, string, error) {
	order, err := s.orders.GetOrder(r.Context(), branch.ID, orderID)
	if err == nil {
		return order, branch.Name, nil
	}
	c, signedIn := customerFromContext(r.Context())
	if !errors.Is(err, store.ErrOrderNotFound) || !signedIn {
		return store.Order{}, "", err
	}
	order, err = s.orders.GetCustomerOrder(r.Context(), c.ID, orderID)
	if err != nil {
		return store.Order{}, "", err
	}
	other, err := s.queries.GetBranch(r.Context(), order.BranchID)
	if err != nil {
		return store.Order{}, "", err
	}
	return order, other.Name, nil
}

// --- admin (shop staff) -----------------------------------------------------

// GET /v1/admin/branches/{branchID}/orders - the shop's live order list.
func (s *Server) handleAdminListActiveOrders(w http.ResponseWriter, r *http.Request) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return
	}
	orders, err := s.orders.ListActiveOrders(r.Context(), branchID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	resp := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, toOrderResponse(o, ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// POST /v1/admin/branches/{branchID}/orders/{orderID}/accept
func (s *Server) handleAdminAcceptOrder(w http.ResponseWriter, r *http.Request) {
	branchID, orderID, ok := branchAndOrderIDs(w, r)
	if !ok {
		return
	}
	order, err := s.orders.AcceptOrder(r.Context(), branchID, orderID)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order, ""))
}

// POST /v1/admin/branches/{branchID}/orders/{orderID}/reject  {"reason": "..."}
func (s *Server) handleAdminRejectOrder(w http.ResponseWriter, r *http.Request) {
	branchID, orderID, ok := branchAndOrderIDs(w, r)
	if !ok {
		return
	}
	var req rejectOrderRequest
	if r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	order, err := s.orders.RejectOrder(r.Context(), branchID, orderID, req.Reason)
	if err != nil {
		s.writeOrderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order, ""))
}

func branchAndOrderIDs(w http.ResponseWriter, r *http.Request) (branchID, orderID uuid.UUID, ok bool) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid branch id")
		return uuid.Nil, uuid.Nil, false
	}
	orderID, err = uuid.Parse(chiURLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return uuid.Nil, uuid.Nil, false
	}
	return branchID, orderID, true
}
