package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/payment"
)

var (
	// ErrNotCaptured: the money hasn't been taken yet (or the order has no
	// payment at all), so there is nothing to refund. Cancel instead (409).
	ErrNotCaptured = errors.New("the money hasn't been taken yet; cancel the order instead")
	// ErrInvalidRefundAmount: 0 or less (422 on amount).
	ErrInvalidRefundAmount = errors.New("amount must be more than 0")
	// ErrRefundAmountTooLarge: more than is left to refund (422 on amount).
	ErrRefundAmountTooLarge = errors.New("amount is more than is left to refund")
	// ErrRefundReasonRequired: empty reason (422 on reason).
	ErrRefundReasonRequired = errors.New("reason is required")
)

// RefundInput is a shop's request to give money back on one order.
type RefundInput struct {
	BranchID    uuid.UUID
	OrderID     uuid.UUID
	Amount      int64 // pence
	Reason      string
	ActorUserID uuid.UUID
}

// QueueRefund asks the worker to give money back to the customer. It runs in
// the order's transaction (inOrderTx), which also checks the order is in this
// branch. The payment is locked before counting what's left, so two refunds
// at the same moment can't both pass.
func (s *Service) QueueRefund(ctx context.Context, in RefundInput) (Order, error) {
	reason := strings.TrimSpace(in.Reason)
	if in.Amount <= 0 {
		return Order{}, ErrInvalidRefundAmount
	}
	if reason == "" {
		return Order{}, ErrRefundReasonRequired
	}
	actor := pgtype.UUID{Bytes: in.ActorUserID, Valid: true}

	return s.inOrderTx(ctx, in.BranchID, in.OrderID, func(q *db.Queries, order db.Order) error {
		p, err := q.GetLatestPaymentForOrder(ctx, order.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotCaptured
		}
		if err != nil {
			return err
		}
		if _, err := q.LockPayment(ctx, p.ID); err != nil {
			return err
		}

		// No rows = the payment isn't captured or partially_refunded.
		refundable, err := q.GetRefundableAmount(ctx, p.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotCaptured
		}
		if err != nil {
			return err
		}
		if in.Amount > int64(refundable) {
			return ErrRefundAmountTooLarge
		}

		if _, err := q.EnqueuePaymentJob(ctx, db.EnqueuePaymentJobParams{
			PaymentID:   p.ID,
			Kind:        "refund",
			Amount:      &in.Amount,
			Reason:      &reason,
			RequestedBy: actor,
		}); err != nil {
			return err
		}

		// A refund doesn't change the order's status, so from and to are both
		// the current one; kind "payment" tells the UI to show the note.
		note := "Refund of " + payment.FormatPence(in.Amount) + " requested: " + reason
		return q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
			OrderID:     order.ID,
			FromStatus:  &order.Status,
			ToStatus:    order.Status,
			ActorUserID: actor,
			Note:        &note,
			Kind:        "payment",
		})
	})
}
