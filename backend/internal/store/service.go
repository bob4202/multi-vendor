package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/db"
)

var (
	// ErrIdempotencyConflict: the same Idempotency-Key was reused with a
	// different order. The handler turns it into 409.
	ErrIdempotencyConflict = errors.New("idempotency key already used for a different order")
	// ErrOrderNotFound: no such order for this branch (404).
	ErrOrderNotFound = errors.New("order not found")
	// ErrOrderNotPending: the order can't be accepted/rejected because it's
	// no longer pending - already handled, or its acceptance window closed (409).
	ErrOrderNotPending = errors.New("order is no longer pending")

	// ErrPaymentProviderNotFound: the branch's vendor has no active payment
	// account, so it can't take orders (card is the only payment method).
	ErrPaymentProviderNotFound = errors.New("payment provider not found")
)

// ValidationError carries every problem found with an order (422).
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("order has %d problem(s)", len(e.Problems))
}

// Order is an order as the rest of the app sees it - no database types.
type Order struct {
	ID              uuid.UUID
	BranchID        uuid.UUID
	Reference       string
	Status          string
	FulfilmentType  string
	FulfilmentDate  string // YYYY-MM-DD
	CustomerName    string
	CustomerEmail   string
	CustomerPhone   string
	Address         *Address // nil for collection
	Notes           *string
	RejectionReason *string
	Subtotal        int64
	DeliveryFee     int64
	Total           int64
	Items           []OrderItem
	CreatedAt       time.Time
	AcceptDeadline  time.Time
	AcceptedAt      *time.Time
	CancelledAt     *time.Time
	UpdatedAt       time.Time

	CustomerAccountID  *uuid.UUID
	AllowSubstitutions bool
	TermsVersion       *string
	// CollectionCode is shown to the customer for collection orders once
	// the shop accepts; staff check it at hand-over.
	CollectionCode *string
	ReadyAt        *time.Time
	CollectBy      *time.Time

	// While awaiting_payment: the order is dropped if not paid by then.
	PaymentDeadline *time.Time
	// The live payment, or else the newest attempt; nil before the first
	// attempt (and for orders placed before card payments).
	Payment *Payment
}

// Payment is the customer-facing summary of an order's card payment.
type Payment struct {
	Status           string // created, authorised, failed, expired, captured, voided, ...
	AmountAuthorised int64
	AmountCaptured   int64
	AmountRefunded   int64
}

// AwaitingCustomer reports whether a substitution is waiting for the
// customer's answer (the order can't be marked ready until it isn't).
func (o Order) AwaitingCustomer() bool {
	for _, item := range o.Items {
		if item.Status == ItemAwaitingCustomer {
			return true
		}
	}
	return false
}

type Address struct {
	Line1    string
	Line2    *string
	City     string
	Postcode string
}

type OrderItem struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	Name      string
	Plu       *string
	UnitPrice int64
	Quantity  int
	LineTotal int64 // as ordered
	// Status and ChargedTotal follow substitutions: what the line costs now.
	Status       string
	ChargedTotal int64
	// Substitution is the latest substitute offered for this line, if any
	// (withdrawn offers are left out).
	Substitution *Substitution
}

// Order item statuses - must match order_items_status_check.
const (
	ItemOK               = "ok"
	ItemAwaitingCustomer = "awaiting_customer"
	ItemSubstituted      = "substituted"
	ItemRemoved          = "removed"
)

// Order Status -
const (
	AwaitingPayment = "awaiting_payment"
)

type Substitution struct {
	ID          uuid.UUID
	ProductID   uuid.UUID
	Name        string
	Plu         *string
	UnitPrice   int64 // the substitute's own price
	Quantity    int
	LineTotal   int64 // what the customer pays for it
	Status      string
	ExpiresAt   time.Time
	RespondedAt *time.Time
}

// Timeouts are the order rules that come from configuration.
type Timeouts struct {
	Accept       time.Duration // the shop must accept a new order within this
	Substitution time.Duration // the customer must answer a substitution within this
	Collection   time.Duration // a ready order waits this long to be collected
	Grace        time.Duration // ...plus this, before it's a no-show
	Payment      time.Duration // the customer must finish paying within this
}

// Service holds what order handling needs: the pool (for transactions), the
// sqlc queries, and the timeouts.
type Service struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	acceptTimeout time.Duration
	timeouts      Timeouts
	logger        *slog.Logger
	now           func() time.Time // swapped in tests
}

func NewService(pool *pgxpool.Pool, timeouts Timeouts, logger *slog.Logger) *Service {
	return &Service{
		pool:          pool,
		queries:       db.New(pool),
		acceptTimeout: timeouts.Accept,
		timeouts:      timeouts,
		logger:        logger,
		now:           time.Now,
	}
}

// Timeouts returns the configured order timeouts.
func (s *Service) Timeouts() Timeouts {
	return s.timeouts
}

// PlaceOrder validates the order against the branch's real data (prices,
// availability, opening days, delivery rules) and saves it as 'pending'. The
// shop then has acceptTimeout to accept it before it's cancelled.
//
// created is false when this is a replay of an order already placed with the
// same Idempotency-Key (the original order is returned).
func (s *Service) PlaceOrder(ctx context.Context, in PlaceOrderInput) (order Order, created bool, err error) {
	// Refuse the order when the branch can't take card payments. Only active
	// accounts come back, and the schema guarantees those have their keys and
	// webhook, so finding a row is enough. Test keys are allowed for now.
	if _, err := s.queries.GetBranchCheckoutAccount(ctx, in.BranchID); errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, ErrPaymentProviderNotFound
	} else if err != nil {
		return Order{}, false, err
	}

	// Merge before checking, so the 1-99 limit applies to the combined amount.
	in.Items = mergeDuplicateItems(in.Items)
	if problems := checkInput(in); len(problems) > 0 {
		return Order{}, false, &ValidationError{Problems: problems}
	}
	in.CustomerPhone, _ = NormalizePhone(in.CustomerPhone) // checkInput made sure it's valid
	in.CustomerEmail = strings.TrimSpace(in.CustomerEmail)

	hash := requestHash(in)

	// Seen this key before? Then this is a retry - answer it without
	// re-validating (prices may have changed since, but the order stands).
	existing, err := s.queries.GetOrderByBranchAndIdempotencyKey(ctx, db.GetOrderByBranchAndIdempotencyKeyParams{
		BranchID:       in.BranchID,
		IdempotencyKey: in.IdempotencyKey,
	})
	switch {
	case err == nil:
		return s.replay(ctx, existing, hash)
	case !errors.Is(err, pgx.ErrNoRows):
		return Order{}, false, err
	}

	priced, deliveryFee, problems, err := s.validateAgainstBranch(ctx, in)
	if err != nil {
		return Order{}, false, err
	}

	var subtotal int64
	for _, item := range priced {
		subtotal += item.LineTotal
	}
	total := subtotal + deliveryFee
	if len(problems) == 0 && in.ExpectedTotal != nil && *in.ExpectedTotal != total {
		problems = append(problems, Problem{
			Code:    "price_changed",
			Message: "Prices have changed since you started checking out. Please review your order.",
		})
	}
	if len(problems) > 0 {
		return Order{}, false, &ValidationError{Problems: problems}
	}

	return s.insert(ctx, in, hash, priced, subtotal, deliveryFee, total)
}

// validateAgainstBranch runs every check that needs the branch's data. It
// collects problems rather than stopping at the first, so the customer sees
// everything at once. err is only for real failures (DB down).
func (s *Service) validateAgainstBranch(ctx context.Context, in PlaceOrderInput) ([]OrderItem, int64, []Problem, error) {
	var problems []Problem

	info, err := s.queries.GetBranchInfo(ctx, in.BranchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, []Problem{{
			Code: "fulfilment_unavailable", Field: "fulfillment_type",
			Message: "This shop isn't taking online orders yet.",
		}}, nil
	}
	if err != nil {
		return nil, 0, nil, err
	}

	if (in.FulfillmentType == "delivery" && !info.Delivery) ||
		(in.FulfillmentType == "collection" && !info.Collection) {
		problems = append(problems, Problem{
			Code: "fulfilment_unavailable", Field: "fulfillment_type",
			Message: fmt.Sprintf("This shop doesn't offer %s.", in.FulfillmentType),
		})
	}

	var dateProblem *Problem
	if in.FulfillmentType == "collection" {
		dateProblem, err = s.checkCollectionToday(ctx, in.BranchID, in.FulfillmentDate)
	} else {
		dateProblem, err = s.checkDate(ctx, in.BranchID, info.Timezone, in.FulfillmentDate)
	}
	if err != nil {
		return nil, 0, nil, err
	}
	if dateProblem != nil {
		problems = append(problems, *dateProblem)
	}

	priced, itemProblems, err := s.priceItems(ctx, in.BranchID, in.Items)
	if err != nil {
		return nil, 0, nil, err
	}
	problems = append(problems, itemProblems...)

	var deliveryFee int64
	if in.FulfillmentType == "delivery" && info.Delivery {
		switch err := checkDeliveryArea(in.Postcode, info.DeliveryPostcodePrefixes); {
		case errors.Is(err, ErrPostcodeOutOfDeliveryArea):
			problems = append(problems, Problem{
				Code: "outside_delivery_area", Field: "postcode",
				Message: "Sorry, we don't deliver to this postcode yet. You can still choose collection.",
			})
		case err != nil:
			problems = append(problems, Problem{
				Code: "invalid", Field: "postcode",
				Message: "Please enter a valid UK postcode.",
			})
		}

		var subtotal int64
		for _, item := range priced {
			subtotal += item.LineTotal
		}
		// Only judge the minimum once the basket itself is valid - an empty
		// subtotal because every item is unavailable isn't "too small".
		if len(itemProblems) == 0 && !minOrderCheck(subtotal, info.DeliveryMinOrder) {
			problems = append(problems, Problem{
				Code: "below_min_order",
				Message: fmt.Sprintf("Delivery needs a minimum order of £%d.%02d.",
					info.DeliveryMinOrder/100, info.DeliveryMinOrder%100),
			})
		}
		deliveryFee = calculateDeliveryFee(subtotal, info.DeliveryFreeOver, info.DeliveryFee)
	}

	return priced, deliveryFee, problems, nil
}

// checkDate checks the fulfilment date in the branch's own timezone: not in
// the past, not too far ahead, and a day the shop is open.
func (s *Service) checkDate(ctx context.Context, branchID uuid.UUID, timezone, dateStr string) (*Problem, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		// checkInput already rejected malformed dates; this is belt and braces.
		return &Problem{Code: "invalid", Field: "fulfillment_date", Message: "Choose a valid date."}, nil
	}

	if !isDateAllowed(date, s.now()) {
		return &Problem{
			Code: "invalid_date", Field: "fulfillment_date",
			Message: "Choose a date between today and two weeks from now.",
		}, nil
	}

	hours, err := s.queries.ListBranchOpeningHours(ctx, branchID)
	if err != nil {
		return nil, err
	}
	pgDate := toPgDate(date)
	exceptions, err := s.queries.ListBranchHoursExceptions(ctx, db.ListBranchHoursExceptionsParams{
		BranchID: branchID, FromDate: pgDate, ToDate: pgDate,
	})
	if err != nil {
		return nil, err
	}
	if !isOpenOn(date, hours, exceptions) {
		return &Problem{
			Code: "invalid_date", Field: "fulfillment_date",
			Message: "The shop is closed on that day. Please choose another date.",
		}, nil
	}
	return nil, nil
}

// checkCollectionToday: collection orders are for today only, and only
// until the shop's last-orders time.
func (s *Service) checkCollectionToday(ctx context.Context, branchID uuid.UUID, dateStr string) (*Problem, error) {
	day, err := s.CollectionToday(ctx, branchID)
	if err != nil {
		return nil, err
	}
	switch {
	case day == nil:
		return &Problem{
			Code: "ordering_closed", Field: "fulfillment_date",
			Message: "The shop is closed for collection today. Please come back tomorrow.",
		}, nil
	case dateStr != day.Date:
		return &Problem{
			Code: "invalid_date", Field: "fulfillment_date",
			Message: "Collection orders are for today only.",
		}, nil
	case !day.Accepting:
		return &Problem{
			Code: "ordering_closed", Field: "fulfillment_date",
			Message: fmt.Sprintf("Last orders for collection today were at %s. Please come back tomorrow.",
				day.LastOrderAt.Format("15:04")),
		}, nil
	}
	return nil, nil
}

// priceItems checks every basket line against the branch in ONE query and
// prices the ones it can sell. Prices always come from the database - never
// from the request.
func (s *Service) priceItems(ctx context.Context, branchID uuid.UUID, items []ItemInput) ([]OrderItem, []Problem, error) {
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ProductID)
	}

	rows, err := s.queries.ListBranchProductsByID(ctx, db.ListBranchProductsByIDParams{
		BranchID: branchID,
		Ids:      ids,
	})
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[uuid.UUID]db.ListBranchProductsByIDRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	var priced []OrderItem
	var problems []Problem
	for _, item := range items {
		row, ok := byID[item.ProductID]
		// Missing = not sold here, deleted or unsellable (the query filters
		// those out). No price = nothing we can charge.
		if !ok || row.Price == nil {
			problems = append(problems, Problem{
				Code: "unavailable", Field: "items", ProductID: &item.ProductID,
				Message: "This item is no longer available.",
			})
			continue
		}
		name := "Product"
		if row.Name != nil {
			name = *row.Name
		}
		priced = append(priced, OrderItem{
			ProductID: item.ProductID,
			Name:      name,
			Plu:       row.Plu,
			UnitPrice: *row.Price,
			Quantity:  item.Quantity,
			LineTotal: *row.Price * int64(item.Quantity),
		})
	}
	return priced, problems, nil
}

// insert saves the order and its lines in one transaction.
func (s *Service) insert(ctx context.Context, in PlaceOrderInput, hash []byte, items []OrderItem, subtotal, deliveryFee, total int64) (Order, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Order{}, false, err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	q := s.queries.WithTx(tx)

	reference, err := q.NextOrderReference(ctx)
	if err != nil {
		return Order{}, false, err
	}

	date, _ := time.Parse("2006-01-02", in.FulfillmentDate) // validated already
	params := db.InsertOrderParams{
		BranchID:       in.BranchID,
		Reference:      reference,
		FulfilmentType: in.FulfillmentType,
		FulfilmentDate: toPgDate(date),
		CustomerName:   strings.TrimSpace(in.CustomerName),
		CustomerEmail:  strings.TrimSpace(in.CustomerEmail),
		CustomerPhone:  strings.TrimSpace(in.CustomerPhone),
		Notes:          optional(in.Notes),
		Subtotal:       subtotal,
		DeliveryFee:    deliveryFee,
		Total:          total,
		IdempotencyKey: in.IdempotencyKey,
		RequestHash:    hash,
		// Required by the schema, but unused while the order awaits payment:
		// expiry only looks at pending orders, and the payment webhook sets a
		// fresh deadline when the card is held.
		AcceptDeadline: pgtype.Timestamptz{Time: s.now().Add(s.acceptTimeout), Valid: true},

		AllowSubstitutions: in.AllowSubstitutions,
		TermsVersion:       optional(in.TermsVersion),
		TermsAcceptedAt:    pgtype.Timestamptz{Time: s.now(), Valid: true},
		Status:             AwaitingPayment,
		PaymentDeadline:    pgtype.Timestamptz{Time: s.now().Add(s.timeouts.Payment), Valid: true},
	}
	if in.CustomerAccountID != nil {
		params.CustomerAccountID = pgtype.UUID{Bytes: *in.CustomerAccountID, Valid: true}
	}
	if in.FulfillmentType == "delivery" {
		params.AddressLine1 = optional(in.AddressLine1)
		params.AddressLine2 = optional(in.AddressLine2)
		params.AddressCity = optional(in.City)
		params.AddressPostcode = optional(strings.ToUpper(in.Postcode))
	}

	row, err := q.InsertOrder(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		// Another request with the same key won the race between our lookup
		// and this insert. Answer with that order instead.
		_ = tx.Rollback(ctx)
		existing, err := s.queries.GetOrderByBranchAndIdempotencyKey(ctx, db.GetOrderByBranchAndIdempotencyKeyParams{
			BranchID: in.BranchID, IdempotencyKey: in.IdempotencyKey,
		})
		if err != nil {
			return Order{}, false, err
		}
		return s.replay(ctx, existing, hash)
	}
	if err != nil {
		return Order{}, false, err
	}

	for _, item := range items {
		if err := q.InsertOrderItem(ctx, db.InsertOrderItemParams{
			OrderID:     row.ID,
			ProductID:   item.ProductID,
			ProductName: item.Name,
			ProductPlu:  item.Plu,
			UnitPrice:   item.UnitPrice,
			Quantity:    int32(item.Quantity),
			LineTotal:   item.LineTotal,
		}); err != nil {
			return Order{}, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, false, err
	}
	order, err := s.withItems(ctx, row)
	return order, true, err
}

// replay answers a repeated Idempotency-Key: the original order if the
// request is the same, a conflict if it isn't.
func (s *Service) replay(ctx context.Context, existing db.Order, hash []byte) (Order, bool, error) {
	if !bytes.Equal(existing.RequestHash, hash) {
		return Order{}, false, ErrIdempotencyConflict
	}
	order, err := s.withItems(ctx, existing)
	return order, false, err
}

// GetOrder returns one of the branch's orders with its lines.
func (s *Service) GetOrder(ctx context.Context, branchID, orderID uuid.UUID) (Order, error) {
	row, err := s.queries.GetOrderByBranchAndID(ctx, db.GetOrderByBranchAndIDParams{BranchID: branchID, ID: orderID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	return s.withItems(ctx, row)
}

// GetCustomerOrder returns an order placed by this customer account, from
// any branch.
func (s *Service) GetCustomerOrder(ctx context.Context, customerID, orderID uuid.UUID) (Order, error) {
	row, err := s.queries.GetOrderByID(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if !row.CustomerAccountID.Valid || uuid.UUID(row.CustomerAccountID.Bytes) != customerID {
		return Order{}, ErrOrderNotFound
	}
	return s.withItems(ctx, row)
}

// ListActiveOrders returns the branch's orders the shop still has to deal
// with (pending / accepted / picking), oldest first, with their lines.
func (s *Service) ListActiveOrders(ctx context.Context, branchID uuid.UUID) ([]Order, error) {
	rows, err := s.queries.ListActiveOrders(ctx, branchID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	itemRows, err := s.queries.ListItemsByOrderIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	itemsByOrder := make(map[uuid.UUID][]OrderItem, len(rows))
	for _, ir := range itemRows {
		itemsByOrder[ir.OrderID] = append(itemsByOrder[ir.OrderID], toOrderItem(ir))
	}

	paymentRows, err := s.queries.ListLatestPaymentsByOrderIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	paymentByOrder := make(map[uuid.UUID]*Payment, len(paymentRows))
	for _, pr := range paymentRows {
		paymentByOrder[pr.OrderID] = &Payment{
			Status: pr.Status, AmountAuthorised: pr.AmountAuthorised,
			AmountCaptured: pr.AmountCaptured, AmountRefunded: pr.AmountRefunded,
		}
	}

	orders := make([]Order, 0, len(rows))
	for _, row := range rows {
		order := toOrder(row, itemsByOrder[row.ID])
		order.Payment = paymentByOrder[row.ID]
		orders = append(orders, order)
	}
	return orders, nil
}

// AcceptOrder confirms a pending order. Fails with ErrOrderNotPending if it
// was already handled or its acceptance window has closed.
func (s *Service) AcceptOrder(ctx context.Context, branchID, orderID uuid.UUID) (Order, error) {
	params := db.AcceptOrderParams{ID: orderID, BranchID: branchID}
	existing, err := s.queries.GetVendorOrderByBranchAndID(ctx, db.GetVendorOrderByBranchAndIDParams{BranchID: branchID, ID: orderID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if existing.FulfilmentType == "collection" {
		code, err := NewCollectionCode()
		if err != nil {
			return Order{}, err
		}
		params.CollectionCode = &code
	}
	row, err := s.queries.AcceptOrder(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, s.whyNotPending(ctx, branchID, orderID)
	}
	if err != nil {
		return Order{}, err
	}
	s.recordStatusEvent(ctx, row.ID, "accepted", nil)
	return s.withItems(ctx, row)
}

// RejectOrder declines a pending order, with an optional reason for the
// customer.
func (s *Service) RejectOrder(ctx context.Context, branchID, orderID uuid.UUID, reason string) (Order, error) {
	row, err := s.queries.RejectOrder(ctx, db.RejectOrderParams{
		ID: orderID, BranchID: branchID, Reason: optional(reason),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, s.whyNotPending(ctx, branchID, orderID)
	}
	if err != nil {
		return Order{}, err
	}
	s.recordStatusEvent(ctx, row.ID, "rejected", optional(reason))
	return s.withItems(ctx, row)
}

// recordStatusEvent adds a pending -> status entry to the order's history
// (order_status_events), which the vendor portal shows as a timeline. No
// actor: these changes come from the admin API or the expiry job, not a
// vendor user. Best effort - the status change itself already happened.
func (s *Service) recordStatusEvent(ctx context.Context, orderID uuid.UUID, to string, note *string) {
	from := "pending"
	if err := s.queries.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID: orderID, FromStatus: &from, ToStatus: to, Note: note, Kind: "status",
	}); err != nil {
		s.logger.Error("record order status event failed", "order_id", orderID, "error", err)
	}
}

// whyNotPending tells "no such order" apart from "it exists but can't be
// changed any more" after a conditional UPDATE matched nothing.
func (s *Service) whyNotPending(ctx context.Context, branchID, orderID uuid.UUID) error {
	_, err := s.queries.GetVendorOrderByBranchAndID(ctx, db.GetVendorOrderByBranchAndIDParams{BranchID: branchID, ID: orderID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}
	return ErrOrderNotPending
}

// ExpireOverdue cancels every pending order whose acceptance window has
// passed, across all branches.
func (s *Service) ExpireOverdue(ctx context.Context) (int, error) {
	rows, err := s.queries.ExpireOverdueOrders(ctx)
	if err != nil {
		return 0, err
	}
	note := "Not accepted in time"
	for _, row := range rows {
		s.logger.Info("order not accepted in time - cancelled",
			"order_id", row.ID, "branch_id", row.BranchID, "reference", row.Reference)
		s.recordStatusEvent(ctx, row.ID, "cancelled", &note)
	}
	return len(rows), nil
}

// ExpireUncollected marks ready collection orders nobody picked up as
// no-shows, once collect_by plus the grace period has passed.
func (s *Service) ExpireUncollected(ctx context.Context) (int, error) {
	rows, err := s.queries.ExpireUncollectedOrders(ctx, int32(s.timeouts.Grace/time.Minute))
	if err != nil {
		return 0, err
	}
	from, note := "ready", "Not collected in time"
	for _, row := range rows {
		s.logger.Info("order not collected in time - no-show",
			"order_id", row.ID, "branch_id", row.BranchID, "reference", row.Reference)
		if err := s.queries.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
			OrderID: row.ID, FromStatus: &from, ToStatus: "no_show", Note: &note, Kind: "status",
		}); err != nil {
			s.logger.Error("record order status event failed", "order_id", row.ID, "error", err)
		}
	}
	return len(rows), nil
}

// RunExpiry runs the order timeouts every interval until ctx is cancelled:
// unaccepted orders, unanswered substitutions and uncollected orders. Run
// it in its own goroutine. Each step is a conditional UPDATE, so several
// API instances running it at once never double-apply a change. AcceptOrder
// also checks the deadline itself, so a slow tick can never let a late
// accept through.
// ExpireUnpaid cancels orders nobody finished paying for by their
// payment_deadline (reason payment_not_completed). The shop never saw them.
// A payment that still arrives afterwards is voided by the webhook.
func (s *Service) ExpireUnpaid(ctx context.Context) (int, error) {
	rows, err := s.queries.ExpireUnpaidOrders(ctx)
	if err != nil {
		return 0, err
	}
	from, note := AwaitingPayment, "Payment not completed in time"
	for _, row := range rows {
		s.logger.Info("order not paid in time - cancelled",
			"order_id", row.ID, "branch_id", row.BranchID, "reference", row.Reference)
		if err := s.queries.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
			OrderID: row.ID, FromStatus: &from, ToStatus: "cancelled", Note: &note, Kind: "status",
		}); err != nil {
			s.logger.Error("record order status event failed", "order_id", row.ID, "error", err)
		}
	}
	return len(rows), nil
}

func (s *Service) RunExpiry(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.ExpireOverdue(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("expire overdue orders failed", "error", err)
		}
		if _, err := s.ExpireSubstitutions(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("expire substitutions failed", "error", err)
		}
		if _, err := s.ExpireUncollected(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("expire uncollected orders failed", "error", err)
		}
		if _, err := s.ExpireUnpaid(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("expire unpaid orders failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) withItems(ctx context.Context, row db.Order) (Order, error) {
	return LoadOrder(ctx, s.queries, row)
}

// LoadOrder attaches an order's lines and their substitutions. Exported so
// the vendor portal shows lines exactly as the customer sees them.
func LoadOrder(ctx context.Context, q *db.Queries, row db.Order) (Order, error) {
	itemRows, err := q.ListItemsByOrderIDs(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return Order{}, err
	}
	subRows, err := q.ListSubstitutionsByOrderIDs(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return Order{}, err
	}
	// Rows come oldest first, so the last one per line wins.
	latest := map[uuid.UUID]*Substitution{}
	for _, sr := range subRows {
		if sr.Status == "withdrawn" {
			delete(latest, sr.OrderItemID)
			continue
		}
		sub := toSubstitution(sr)
		latest[sr.OrderItemID] = &sub
	}
	items := make([]OrderItem, 0, len(itemRows))
	for _, ir := range itemRows {
		item := toOrderItem(ir)
		item.Substitution = latest[ir.ID]
		items = append(items, item)
	}
	order := toOrder(row, items)

	p, err := q.GetLatestPaymentForOrder(ctx, row.ID)
	switch {
	case err == nil:
		order.Payment = &Payment{
			Status: p.Status, AmountAuthorised: p.AmountAuthorised,
			AmountCaptured: p.AmountCaptured, AmountRefunded: p.AmountRefunded,
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return Order{}, err
	}
	return order, nil
}

func toSubstitution(r db.OrderSubstitution) Substitution {
	return Substitution{
		ID:          r.ID,
		ProductID:   r.ProductID,
		Name:        r.ProductName,
		Plu:         r.ProductPlu,
		UnitPrice:   r.UnitPrice,
		Quantity:    int(r.Quantity),
		LineTotal:   r.LineTotal,
		Status:      r.Status,
		ExpiresAt:   r.ExpiresAt.Time,
		RespondedAt: timePtr(r.RespondedAt),
	}
}

// requestHash fingerprints what the customer asked for, so a reused
// Idempotency-Key can be told apart: same order (retry) vs different order
// (conflict). Items are sorted so line order doesn't change the hash.
func requestHash(in PlaceOrderInput) []byte {
	items := slices.Clone(in.Items)
	slices.SortFunc(items, func(a, b ItemInput) int {
		return strings.Compare(a.ProductID.String(), b.ProductID.String())
	})

	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "item:%s:%d\n", item.ProductID, item.Quantity)
	}
	expected := ""
	if in.ExpectedTotal != nil {
		expected = strconv.FormatInt(*in.ExpectedTotal, 10)
	}
	for _, field := range []string{
		in.FulfillmentType, in.FulfillmentDate,
		in.CustomerName, in.CustomerEmail, in.CustomerPhone,
		in.AddressLine1, in.AddressLine2, in.City, strings.ToUpper(in.Postcode),
		in.Notes, expected,
		in.TermsVersion, strconv.FormatBool(in.AllowSubstitutions),
	} {
		b.WriteString(strings.TrimSpace(field))
		b.WriteByte('\n')
	}

	sum := sha256.Sum256([]byte(b.String()))
	return sum[:]
}

func toOrder(row db.Order, items []OrderItem) Order {
	order := Order{
		ID:              row.ID,
		BranchID:        row.BranchID,
		Reference:       row.Reference,
		Status:          row.Status,
		FulfilmentType:  row.FulfilmentType,
		FulfilmentDate:  row.FulfilmentDate.Time.Format("2006-01-02"),
		CustomerName:    row.CustomerName,
		CustomerEmail:   row.CustomerEmail,
		CustomerPhone:   row.CustomerPhone,
		Notes:           row.Notes,
		RejectionReason: row.RejectionReason,
		Subtotal:        row.Subtotal,
		DeliveryFee:     row.DeliveryFee,
		Total:           row.Total,
		Items:           items,
		CreatedAt:       row.CreatedAt.Time,
		AcceptDeadline:  row.AcceptDeadline.Time,
		AcceptedAt:      timePtr(row.AcceptedAt),
		CancelledAt:     timePtr(row.CancelledAt),
		UpdatedAt:       row.UpdatedAt.Time,

		AllowSubstitutions: row.AllowSubstitutions,
		TermsVersion:       row.TermsVersion,
		CollectionCode:     row.CollectionCode,
		ReadyAt:            timePtr(row.ReadyAt),
		CollectBy:          timePtr(row.CollectBy),
		PaymentDeadline:    timePtr(row.PaymentDeadline),
	}
	if row.CustomerAccountID.Valid {
		id := uuid.UUID(row.CustomerAccountID.Bytes)
		order.CustomerAccountID = &id
	}
	if order.Items == nil {
		order.Items = []OrderItem{}
	}
	if row.AddressLine1 != nil {
		order.Address = &Address{
			Line1:    *row.AddressLine1,
			Line2:    row.AddressLine2,
			City:     deref(row.AddressCity),
			Postcode: deref(row.AddressPostcode),
		}
	}
	return order
}

func toOrderItem(r db.ListItemsByOrderIDsRow) OrderItem {
	return OrderItem{
		ID:           r.ID,
		ProductID:    r.ProductID,
		Name:         r.ProductName,
		Plu:          r.ProductPlu,
		UnitPrice:    r.UnitPrice,
		Quantity:     int(r.Quantity),
		LineTotal:    r.LineTotal,
		Status:       r.Status,
		ChargedTotal: r.ChargedTotal,
	}
}

func toPgDate(t time.Time) pgtype.Date {
	y, m, d := t.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// optional turns "" (after trimming) into nil, for nullable columns.
func optional(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
