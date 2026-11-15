package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

// ErrNothingToRetry: the order has no payment, or its charge hasn't failed
// (already charged, or a retry is already running) (409).
var ErrNothingToRetry = errors.New("there's no failed charge to try again")

// RetryPaymentInput is a shop's request to try a failed charge again.
type RetryPaymentInput struct {
	BranchID    uuid.UUID
	OrderID     uuid.UUID
	ActorUserID uuid.UUID
}

// QueueRetry puts a capture_failed payment back to authorised and queues a new
// capture of the order's total, as at ready. If it fails again, the worker
// marks it capture_failed with a new history line, as the first time. The
// payment is locked, so two presses at once queue only one capture.
func (s *Service) QueueRetry(ctx context.Context, in RetryPaymentInput) (Order, error) {
	return s.inOrderTx(ctx, in.BranchID, in.OrderID, func(q *db.Queries, order db.Order) error {
		p, err := q.GetLatestPaymentForOrder(ctx, order.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNothingToRetry
		}
		if err != nil {
			return err
		}

		if _, err := q.LockPayment(ctx, p.ID); err != nil {
			return err
		}

		// No rows = it isn't capture_failed.
		if _, err = q.MarkPaymentRetrying(ctx, p.ID); errors.Is(err, pgx.ErrNoRows) {
			return ErrNothingToRetry
		} else if err != nil {
			return err
		}

		actorID := pgtype.UUID{Bytes: in.ActorUserID, Valid: true}
		err = QueueCaptureIfHeld(ctx, q, in.OrderID, actorID, order.Total)

		if err != nil {
			return err
		}

		note := "Trying the charge again"
		return q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
			OrderID:     order.ID,
			FromStatus:  &order.Status,
			ToStatus:    order.Status,
			ActorUserID: actorID,
			Note:        &note,
			Kind:        "payment",
		})

	})

}
