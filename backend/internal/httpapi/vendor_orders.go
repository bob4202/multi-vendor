package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/portal"
	"ecommerce/backend/internal/store"
)

type orderFulfilmentResponse struct {
	Type string `json:"type"`
	Date string `json:"date"`
}

type orderCustomerResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type vendorOrderAddressResponse struct {
	Line1    *string `json:"line1"`
	Line2    *string `json:"line2"`
	City     *string `json:"city"`
	Postcode *string `json:"postcode"`
}

type vendorOrderSummaryResponse struct {
	ID             string                  `json:"id"`
	Reference      string                  `json:"reference"`
	Status         string                  `json:"status"`
	Fulfilment     orderFulfilmentResponse `json:"fulfilment"`
	Customer       orderCustomerResponse   `json:"customer"`
	ItemCount      int32                   `json:"item_count"`
	Total          int64                   `json:"total"`
	CreatedAt      time.Time               `json:"created_at"`
	AcceptDeadline time.Time               `json:"accept_deadline"` // pending orders are cancelled after this
	// A substitution is waiting for the customer's answer.
	AwaitingCustomer bool `json:"awaiting_customer"`
	// Ready collection orders: collect by then, or it becomes a no-show.
	CollectBy *time.Time `json:"collect_by"`
}

func toVendorOrderSummary(o db.Order, itemCount int32, awaiting bool) vendorOrderSummaryResponse {
	return vendorOrderSummaryResponse{
		ID:               o.ID.String(),
		Reference:        o.Reference,
		Status:           o.Status,
		Fulfilment:       orderFulfilmentResponse{Type: o.FulfilmentType, Date: o.FulfilmentDate.Time.Format(time.DateOnly)},
		Customer:         orderCustomerResponse{Name: o.CustomerName, Email: o.CustomerEmail, Phone: o.CustomerPhone},
		ItemCount:        itemCount,
		Total:            o.Total,
		CreatedAt:        o.CreatedAt.Time,
		AcceptDeadline:   o.AcceptDeadline.Time,
		AwaitingCustomer: awaiting,
		CollectBy:        optionalTime(o.CollectBy.Time, o.CollectBy.Valid),
	}
}

type vendorOrderItemResponse struct {
	ID           string                `json:"id"`
	ProductID    string                `json:"product_id"`
	Name         string                `json:"name"`
	PLU          *string               `json:"plu"`
	UnitPrice    int64                 `json:"unit_price"`
	Quantity     int                   `json:"quantity"`
	LineTotal    int64                 `json:"line_total"`
	Status       string                `json:"status"`
	ChargedTotal int64                 `json:"charged_total"`
	Substitution *substitutionResponse `json:"substitution"`
}

type orderEventResponse struct {
	Kind       string    `json:"kind"` // status | substitution | removed | code_override
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Note       *string   `json:"note"`
	ActorName  *string   `json:"actor_name"`
	CreatedAt  time.Time `json:"created_at"`
}

type vendorOrderDetailResponse struct {
	ID              string                      `json:"id"`
	Reference       string                      `json:"reference"`
	Status          string                      `json:"status"`
	NextStatuses    []string                    `json:"next_statuses"`
	Fulfilment      orderFulfilmentResponse     `json:"fulfilment"`
	Customer        orderCustomerResponse       `json:"customer"`
	Address         *vendorOrderAddressResponse `json:"address"`
	Notes           *string                     `json:"notes"`
	RejectionReason *string                     `json:"rejection_reason"`
	Items           []vendorOrderItemResponse   `json:"items"`
	Subtotal        int64                       `json:"subtotal"`
	DeliveryFee     int64                       `json:"delivery_fee"`
	Total           int64                       `json:"total"`
	Events          []orderEventResponse        `json:"events"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
	AcceptDeadline  time.Time                   `json:"accept_deadline"`
	AcceptedAt      *time.Time                  `json:"accepted_at"`
	CancelledAt     *time.Time                  `json:"cancelled_at"`

	AllowSubstitutions bool       `json:"allow_substitutions"`
	AwaitingCustomer   bool       `json:"awaiting_customer"`
	ReadyAt            *time.Time `json:"ready_at"`
	CollectBy          *time.Time `json:"collect_by"`
	// Staff never see the code itself - they type what the customer shows.
	RequiresCollectionCode bool             `json:"requires_collection_code"`
	Payment                *paymentResponse `json:"payment"`
}

type paginatedVendorOrdersResponse struct {
	Data       []vendorOrderSummaryResponse `json:"data"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalCount int64                        `json:"total_count"`
	TotalPages int                          `json:"total_pages"`
}

// handleVendorListOrders: ?status=active (default, oldest first - a work
// queue) | done | all (newest first), plus page/page_size.
func (s *Server) handleVendorListOrders(w http.ResponseWriter, r *http.Request) {
	branchID := vendorBranchID(r)
	q := r.URL.Query()

	var statuses []string
	queue := false
	switch q.Get("status") {
	case "", "active":
		statuses, queue = portal.ActiveStatuses, true
	case "done":
		statuses = portal.DoneStatuses
	case "all":
		statuses = append(append([]string{}, portal.ActiveStatuses...), portal.DoneStatuses...)
	default:
		writeError(w, http.StatusBadRequest, "status must be active, done or all")
		return
	}

	page, pageSize := parsePage(r)
	total, err := s.queries.CountVendorOrders(r.Context(), db.CountVendorOrdersParams{BranchID: branchID, Statuses: statuses})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	rows, err := s.queries.ListVendorOrders(r.Context(), db.ListVendorOrdersParams{
		BranchID:   branchID,
		Statuses:   statuses,
		Queue:      queue,
		PageLimit:  int32(pageSize),
		PageOffset: int32((page - 1) * pageSize),
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	data := make([]vendorOrderSummaryResponse, 0, len(rows))
	for _, o := range rows {
		data = append(data, toVendorOrderSummary(vendorRowOrder(o), o.ItemCount, o.AwaitingCustomer))
	}

	writeJSON(w, http.StatusOK, paginatedVendorOrdersResponse{
		Data:       data,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

func (s *Server) handleVendorGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	order, err := s.queries.GetVendorOrderByBranchAndID(r.Context(), db.GetVendorOrderByBranchAndIDParams{BranchID: vendorBranchID(r), ID: orderID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.writeOrderDetail(w, r, http.StatusOK, order)
}

func (s *Server) handleVendorChangeOrderStatus(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	var req struct {
		Status             string `json:"status"`
		ExpectedStatus     string `json:"expected_status"`
		Reason             string `json:"reason"`
		CollectionCode     string `json:"collection_code"`
		CodeOverrideReason string `json:"code_override_reason"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Status == "" || req.ExpectedStatus == "" {
		writeError(w, http.StatusBadRequest, "status and expected_status are required")
		return
	}

	order, err := s.portal.ChangeOrderStatus(r.Context(), portal.ChangeStatusInput{
		BranchID:       vendorBranchID(r),
		OrderID:        orderID,
		ActorUserID:    principalFromContext(r.Context()).User.ID,
		ExpectedStatus: req.ExpectedStatus,
		NewStatus:      req.Status,
		Reason:         req.Reason,

		CollectionCode:     req.CollectionCode,
		CodeOverrideReason: req.CodeOverrideReason,
	})
	switch {
	case errors.Is(err, portal.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, portal.ErrStatusConflict), errors.Is(err, portal.ErrAcceptWindowClosed):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, portal.ErrInvalidTransition):
		writeIssues(w, err.Error(), []portal.Issue{{Field: "status", Message: err.Error()}})
		return
	case errors.Is(err, portal.ErrReasonRequired):
		writeIssues(w, err.Error(), []portal.Issue{{Field: "reason", Message: err.Error()}})
		return
	case errors.Is(err, portal.ErrAwaitingCustomer):
		writeIssues(w, err.Error(), []portal.Issue{{Field: "status", Message: "Wait for the customer to answer the substitution first."}})
		return
	case errors.Is(err, portal.ErrCollectionCodeRequired), errors.Is(err, portal.ErrCollectionCodeWrong),
		errors.Is(err, portal.ErrCollectionCodeLocked):
		writeIssues(w, err.Error(), []portal.Issue{{Field: "collection_code", Message: err.Error()}})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	s.writeOrderDetail(w, r, http.StatusOK, order)
}

func (s *Server) writeOrderDetail(w http.ResponseWriter, r *http.Request, status int, o db.Order) {
	full, err := store.LoadOrder(r.Context(), s.queries, o)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	events, err := s.queries.ListOrderStatusEvents(r.Context(), o.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	resp := vendorOrderDetailResponse{
		ID:              o.ID.String(),
		Reference:       o.Reference,
		Status:          o.Status,
		NextStatuses:    portal.NextStatusesFor(o, full.AwaitingCustomer()),
		Fulfilment:      orderFulfilmentResponse{Type: o.FulfilmentType, Date: o.FulfilmentDate.Time.Format(time.DateOnly)},
		Customer:        orderCustomerResponse{Name: o.CustomerName, Email: o.CustomerEmail, Phone: o.CustomerPhone},
		Notes:           o.Notes,
		RejectionReason: o.RejectionReason,
		Items:           make([]vendorOrderItemResponse, 0, len(full.Items)),
		Subtotal:        o.Subtotal,
		DeliveryFee:     o.DeliveryFee,
		Total:           o.Total,
		Events:          make([]orderEventResponse, 0, len(events)),
		CreatedAt:       o.CreatedAt.Time,
		UpdatedAt:       o.UpdatedAt.Time,
		AcceptDeadline:  o.AcceptDeadline.Time,
		AcceptedAt:      optionalTime(o.AcceptedAt.Time, o.AcceptedAt.Valid),
		CancelledAt:     optionalTime(o.CancelledAt.Time, o.CancelledAt.Valid),

		AllowSubstitutions:     o.AllowSubstitutions,
		AwaitingCustomer:       full.AwaitingCustomer(),
		ReadyAt:                full.ReadyAt,
		CollectBy:              full.CollectBy,
		RequiresCollectionCode: o.FulfilmentType == "collection" && o.CollectionCode != nil,
		Payment:                toPaymentResponse(full.Payment),
	}
	if o.FulfilmentType == "delivery" {
		resp.Address = &vendorOrderAddressResponse{Line1: o.AddressLine1, Line2: o.AddressLine2, City: o.AddressCity, Postcode: o.AddressPostcode}
	}
	for _, it := range full.Items {
		resp.Items = append(resp.Items, vendorOrderItemResponse{
			ID: it.ID.String(), ProductID: it.ProductID.String(), Name: it.Name, PLU: it.Plu,
			UnitPrice: it.UnitPrice, Quantity: it.Quantity, LineTotal: it.LineTotal,
			Status: it.Status, ChargedTotal: it.ChargedTotal, Substitution: toSubstitutionResponse(it.Substitution),
		})
	}
	for _, e := range events {
		resp.Events = append(resp.Events, orderEventResponse{
			Kind: e.Kind, FromStatus: e.FromStatus, ToStatus: e.ToStatus, Note: e.Note, ActorName: e.ActorName, CreatedAt: e.CreatedAt.Time,
		})
	}
	writeJSON(w, status, resp)
}

// parsePage reads page/page_size, clamped like parseProductQuery.
func parsePage(r *http.Request) (page, pageSize int) {
	q := r.URL.Query()
	page = 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	pageSize = defaultPageSize
	if v, err := strconv.Atoi(q.Get("page_size")); err == nil && v > 0 {
		pageSize = min(v, maxPageSize)
	}
	return page, pageSize
}

// vendorRowOrder lifts the order columns out of a list row.
func vendorRowOrder(o db.ListVendorOrdersRow) db.Order {
	return db.Order{
		ID: o.ID, Reference: o.Reference, Status: o.Status, FulfilmentType: o.FulfilmentType,
		FulfilmentDate: o.FulfilmentDate, CustomerName: o.CustomerName, CustomerEmail: o.CustomerEmail,
		CustomerPhone: o.CustomerPhone, Total: o.Total, CreatedAt: o.CreatedAt,
		AcceptDeadline: o.AcceptDeadline, CollectBy: o.CollectBy,
	}
}

// GET /orders/lookup?code=123456 - staff typing the code a customer shows
// at the counter. Only open collection orders match.
func (s *Server) handleVendorLookupOrder(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		writeIssues(w, "enter the 6-digit code", []portal.Issue{{Field: "code", Message: "Enter the 6-digit code"}})
		return
	}
	rows, err := s.queries.FindOpenOrdersByCollectionCode(r.Context(), db.FindOpenOrdersByCollectionCodeParams{
		BranchID: vendorBranchID(r), Code: code,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "no open order has that code")
		return
	}
	data := make([]vendorOrderSummaryResponse, 0, len(rows))
	for _, o := range rows {
		data = append(data, toVendorOrderSummary(db.Order{
			ID: o.ID, Reference: o.Reference, Status: o.Status, FulfilmentType: o.FulfilmentType,
			FulfilmentDate: o.FulfilmentDate, CustomerName: o.CustomerName, CustomerEmail: o.CustomerEmail,
			CustomerPhone: o.CustomerPhone, Total: o.Total, CreatedAt: o.CreatedAt,
			AcceptDeadline: o.AcceptDeadline, CollectBy: o.CollectBy,
		}, o.ItemCount, o.AwaitingCustomer))
	}
	// Codes are random, so two open orders sharing one is rare - but if it
	// happens, staff pick by name.
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// POST /orders/{orderID}/items/{itemID}/unavailable
// {"substitute": {"product_id", "quantity"}} offers a substitute; {"substitute": null} removes the line.
func (s *Server) handleVendorItemUnavailable(w http.ResponseWriter, r *http.Request) {
	orderID, err1 := uuid.Parse(chi.URLParam(r, "orderID"))
	itemID, err2 := uuid.Parse(chi.URLParam(r, "itemID"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	var req struct {
		Substitute *struct {
			ProductID uuid.UUID `json:"product_id"`
			Quantity  int       `json:"quantity"`
		} `json:"substitute"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in := store.MarkUnavailableInput{
		BranchID: vendorBranchID(r), OrderID: orderID, ItemID: itemID,
		ActorUserID: principalFromContext(r.Context()).User.ID,
	}
	if req.Substitute != nil {
		in.Substitute = &store.SubstituteInput{ProductID: req.Substitute.ProductID, Quantity: req.Substitute.Quantity}
	}
	order, err := s.orders.MarkItemUnavailable(r.Context(), in)
	s.writeItemChange(w, r, order, err)
}

// DELETE /orders/{orderID}/items/{itemID}/substitution - take back an
// unanswered offer.
func (s *Server) handleVendorWithdrawSubstitution(w http.ResponseWriter, r *http.Request) {
	orderID, err1 := uuid.Parse(chi.URLParam(r, "orderID"))
	itemID, err2 := uuid.Parse(chi.URLParam(r, "itemID"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	order, err := s.orders.WithdrawSubstitution(r.Context(), vendorBranchID(r), orderID, itemID, principalFromContext(r.Context()).User.ID)
	s.writeItemChange(w, r, order, err)
}

func (s *Server) writeItemChange(w http.ResponseWriter, r *http.Request, order store.Order, err error) {
	switch {
	case errors.Is(err, store.ErrOrderNotFound), errors.Is(err, store.ErrItemNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, store.ErrItemAlreadyChanged), errors.Is(err, store.ErrOrderNotChangeable),
		errors.Is(err, store.ErrSubstitutionClosed), errors.Is(err, store.ErrNoLiveSubstitution):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, store.ErrSubstitutionsNotAllowed), errors.Is(err, store.ErrSubstituteUnavailable),
		errors.Is(err, store.ErrSubstituteSameProduct), errors.Is(err, store.ErrInvalidQuantity):
		field := "substitute.product_id"
		if errors.Is(err, store.ErrInvalidQuantity) {
			field = "substitute.quantity"
		}
		writeIssues(w, err.Error(), []portal.Issue{{Field: field, Message: err.Error()}})
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	row, err := s.queries.GetVendorOrderByBranchAndID(r.Context(), db.GetVendorOrderByBranchAndIDParams{BranchID: vendorBranchID(r), ID: order.ID})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.writeOrderDetail(w, r, http.StatusOK, row)
}
