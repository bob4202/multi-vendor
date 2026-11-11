package portal

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/store"
)

// Order statuses - must match orders_status_check in 0007_orders.sql.
const (
	StatusPending        = "pending"
	StatusAccepted       = "accepted"
	StatusPicking        = "picking"
	StatusReady          = "ready"
	StatusOutForDelivery = "out_for_delivery"
	StatusCompleted      = "completed"
	StatusRejected       = "rejected"
	StatusCancelled      = "cancelled"
	StatusNoShow         = "no_show"
)

// ActiveStatuses are orders that still need work; everything else is done.
var (
	ActiveStatuses = []string{StatusPending, StatusAccepted, StatusPicking, StatusReady, StatusOutForDelivery}
	DoneStatuses   = []string{StatusCompleted, StatusRejected, StatusCancelled, StatusNoShow}
)

// transitions is the order lifecycle. Statuses with no entry are terminal.
// onlyFor restricts a step to one fulfilment type (no_show only makes sense
// for collections, out_for_delivery only for deliveries).
var transitions = map[string][]transition{
	StatusPending: {
		{to: StatusAccepted},
		{to: StatusRejected},
		{to: StatusCancelled},
	},
	StatusAccepted: {
		{to: StatusPicking},
		{to: StatusCancelled},
	},
	StatusPicking: {
		{to: StatusReady},
		{to: StatusCancelled},
	},
	StatusReady: {
		{to: StatusCompleted, onlyFor: "collection"},
		{to: StatusNoShow, onlyFor: "collection"},
		{to: StatusOutForDelivery, onlyFor: "delivery"},
	},
	StatusOutForDelivery: {
		{to: StatusCompleted},
	},
}

type transition struct {
	to      string
	onlyFor string // fulfilment type; "" = both
}

// NextStatuses lists the statuses an order can move to from its current
// one, in the order a UI should offer them (primary action first).
func NextStatuses(from, fulfilmentType string) []string {
	next := []string{}
	for _, t := range transitions[from] {
		if t.onlyFor == "" || t.onlyFor == fulfilmentType {
			next = append(next, t.to)
		}
	}
	return next
}

func canTransition(from, to, fulfilmentType string) bool {
	for _, s := range NextStatuses(from, fulfilmentType) {
		if s == to {
			return true
		}
	}
	return false
}

var (
	ErrOrderNotFound     = errors.New("order not found")
	ErrInvalidTransition = errors.New("order can't move to that status from its current one")
	ErrReasonRequired    = errors.New("a reason is required to reject an order")
	// ErrStatusConflict means someone else changed the order first; the
	// client should reload it and decide again.
	ErrStatusConflict = errors.New("order status has changed since it was loaded")
	// ErrAcceptWindowClosed: the order's accept_deadline passed, so it can
	// only be cancelled now (the expiry job will do that shortly).
	ErrAcceptWindowClosed = errors.New("too late to accept: this order wasn't accepted in time and is being cancelled")
	// ErrAwaitingCustomer: a substitution is waiting for the customer's
	// answer, so the order can't be marked ready yet.
	ErrAwaitingCustomer = errors.New("waiting for the customer to answer a substitution")

	ErrCollectionCodeRequired = errors.New("enter the customer's collection code, or a reason to hand over without it")
	ErrCollectionCodeWrong    = errors.New("that collection code doesn't match this order")
	// ErrCollectionCodeLocked: too many wrong codes; only an override with a
	// reason works now.
	ErrCollectionCodeLocked = errors.New("too many wrong codes - hand over with a reason instead (e.g. checked ID)")
	// ErrPaymentNotTaken: the capture failed, so the order can't be handed
	// over (409).
	ErrPaymentNotTaken = errors.New("the card couldn't be charged for this order - don't hand it over")
)

// maxCollectionCodeAttempts wrong codes lock code entry for that order.
const maxCollectionCodeAttempts = 5

// NextStatusesFor is NextStatuses for a loaded order: "ready" is held back
// while the customer still has to answer a substitution.
func NextStatusesFor(o db.Order, awaitingCustomer bool) []string {
	next := NextStatuses(o.Status, o.FulfilmentType)
	if !awaitingCustomer {
		return next
	}
	out := next[:0:0]
	for _, s := range next {
		if s != StatusReady {
			out = append(out, s)
		}
	}
	return out
}

type ChangeStatusInput struct {
	BranchID       uuid.UUID
	OrderID        uuid.UUID
	ActorUserID    uuid.UUID
	ExpectedStatus string
	NewStatus      string
	Reason         string // required for rejected, optional note otherwise

	// Handing over a collection order (ready -> completed) needs the
	// customer's code, or a reason to go without it.
	CollectionCode     string
	CodeOverrideReason string
}

// ChangeOrderStatus moves an order to a new status and records who did it,
// atomically. ExpectedStatus is the status the user was looking at - if the
// order has moved on since, nothing changes and ErrStatusConflict comes back.
func (s *Service) ChangeOrderStatus(ctx context.Context, in ChangeStatusInput) (db.Order, error) {
	reason := strings.TrimSpace(in.Reason)
	if in.NewStatus == StatusRejected && reason == "" {
		return db.Order{}, ErrReasonRequired
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.Order{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	current, err := q.GetOrderByBranchAndID(ctx, db.GetOrderByBranchAndIDParams{BranchID: in.BranchID, ID: in.OrderID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return db.Order{}, fmt.Errorf("load order: %w", err)
	}
	if current.Status != in.ExpectedStatus {
		return db.Order{}, ErrStatusConflict
	}
	if !canTransition(current.Status, in.NewStatus, current.FulfilmentType) {
		return db.Order{}, ErrInvalidTransition
	}
	if acceptWindowClosed(current, in.NewStatus, s.now()) {
		return db.Order{}, ErrAcceptWindowClosed
	}

	// Don't let unpaid shopping leave the shop. No payment row (orders from
	// before card payments) is fine.
	if in.NewStatus == StatusCompleted || in.NewStatus == StatusOutForDelivery {
		latestPayment, err := q.GetLatestPaymentForOrder(ctx, in.OrderID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.Order{}, fmt.Errorf("load latest payment: %w", err)
		}
		if err == nil && blocksHandover(in.NewStatus, latestPayment.Status) {
			return db.Order{}, ErrPaymentNotTaken
		}
	}

	params := db.TransitionOrderStatusParams{
		ID:             in.OrderID,
		BranchID:       in.BranchID,
		ExpectedStatus: in.ExpectedStatus,
		NewStatus:      in.NewStatus,
	}
	isCollection := current.FulfilmentType == "collection"

	switch in.NewStatus {
	case StatusAccepted:
		if isCollection {
			code, err := store.NewCollectionCode()
			if err != nil {
				return db.Order{}, err
			}
			params.CollectionCode = &code
		}
	case StatusReady:
		counts, err := q.CountOrderItemStates(ctx, current.ID)
		if err != nil {
			return db.Order{}, fmt.Errorf("count items: %w", err)
		}
		if counts.Awaiting > 0 {
			return db.Order{}, ErrAwaitingCustomer
		}
		if isCollection {
			day, err := store.CollectionTodayFor(ctx, q, in.BranchID, s.now())
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return db.Order{}, fmt.Errorf("load today's hours: %w", err)
			}
			by := store.CollectBy(s.now(), s.collectionWindow, day)
			params.CollectBy = pgtype.Timestamptz{Time: by, Valid: true}
		}
	}

	// Hand-over check. Orders accepted before codes existed have none.
	var override *string
	if in.NewStatus == StatusCompleted && isCollection && current.CollectionCode != nil {
		code := strings.TrimSpace(in.CollectionCode)
		overrideReason := strings.TrimSpace(in.CodeOverrideReason)
		switch {
		case code == "" && overrideReason == "":
			return db.Order{}, ErrCollectionCodeRequired
		case code == "":
			override = &overrideReason
		case current.CollectionCodeAttempts >= maxCollectionCodeAttempts:
			return db.Order{}, ErrCollectionCodeLocked
		case subtle.ConstantTimeCompare([]byte(code), []byte(*current.CollectionCode)) != 1:
			// Counted outside the transaction, which is about to roll back.
			if _, err := s.queries.RecordCollectionCodeAttempt(ctx, current.ID); err != nil {
				return db.Order{}, fmt.Errorf("record code attempt: %w", err)
			}
			return db.Order{}, ErrCollectionCodeWrong
		}
	}

	var rejectionReason, note *string
	if reason != "" {
		note = &reason
		if in.NewStatus == StatusRejected {
			rejectionReason = &reason
		}
	}

	params.RejectionReason = rejectionReason
	updated, err := q.TransitionOrderStatus(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		// Changed between our read and our write - or the deadline passed
		// in that instant (the UPDATE re-checks it).
		if acceptWindowClosed(current, in.NewStatus, s.now()) {
			return db.Order{}, ErrAcceptWindowClosed
		}
		return db.Order{}, ErrStatusConflict
	}
	if err != nil {
		return db.Order{}, fmt.Errorf("update status: %w", err)
	}

	from := current.Status
	if err := q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID:     in.OrderID,
		FromStatus:  &from,
		ToStatus:    in.NewStatus,
		ActorUserID: pgtype.UUID{Bytes: in.ActorUserID, Valid: true},
		Note:        note,
		Kind:        "status",
	}); err != nil {
		return db.Order{}, fmt.Errorf("record status event: %w", err)
	}
	if override != nil {
		to := in.NewStatus
		overrideNote := "Handed over without the collection code: " + *override
		if err := q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
			OrderID:     in.OrderID,
			FromStatus:  &to,
			ToStatus:    to,
			ActorUserID: pgtype.UUID{Bytes: in.ActorUserID, Valid: true},
			Note:        &overrideNote,
			Kind:        "code_override",
		}); err != nil {
			return db.Order{}, fmt.Errorf("record code override: %w", err)
		}
	}

	// Rejected or cancelled before the money was taken: queue a void to
	// release the customer's card hold. Same transaction as the status
	// change, so the job exists only if the change is saved.
	if in.NewStatus == StatusRejected || in.NewStatus == StatusCancelled {
		actor := pgtype.UUID{Bytes: in.ActorUserID, Valid: true}
		if err := store.QueueVoidIfHeld(ctx, q, in.OrderID, actor); err != nil {
			return db.Order{}, err
		}
	}

	if in.NewStatus == StatusReady {
		actor := pgtype.UUID{Bytes: in.ActorUserID, Valid: true}
		err := store.QueueCaptureIfHeld(ctx, q, in.OrderID, actor, updated.Total)
		if err != nil {
			return db.Order{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return db.Order{}, err
	}
	return updated, nil
}

// acceptWindowClosed reports whether this change is accepting a pending
// order after its deadline - the same rule AcceptOrder in order.sql applies.
func acceptWindowClosed(o db.Order, newStatus string, now time.Time) bool {
	return o.Status == StatusPending && newStatus == StatusAccepted &&
		o.AcceptDeadline.Valid && !o.AcceptDeadline.Time.After(now)
}

// blocksHandover: the shopping can't leave the shop (collected or out for
// delivery) once taking the money has failed. A capture that's still queued
// doesn't block: the worker usually takes the money within seconds.
func blocksHandover(newStatus, paymentStatus string) bool {
	handover := newStatus == StatusCompleted || newStatus == StatusOutForDelivery
	return handover && paymentStatus == "capture_failed"
}
