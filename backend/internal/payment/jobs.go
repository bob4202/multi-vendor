package payment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

const (
	BatchSize      = 5
	IntervalSecond = 15
	LeaseSecond    = 300
)

// retryDelays is how long to wait before each retry of a failed job. Once
// they're used up, the job is marked failed.
var retryDelays = [...]time.Duration{time.Minute, 3 * time.Minute, 5 * time.Minute}

// errNoPaymentRef: the payment never got a provider reference, so there is
// nothing to void, capture or refund and retrying can't help.
var errNoPaymentRef = errors.New("payment has no provider reference")

// errInvalidAmount: a capture or refund job without an amount above zero. The
// job was queued wrong, so retrying can't help.
var errInvalidAmount = errors.New("job has no valid amount")

func (s *Service) RunJobs(ctx context.Context) {

	ticker := time.NewTicker(IntervalSecond * time.Second)
	defer ticker.Stop()

	for {
		err := s.claimJobs(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Error("payment claiming fail", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

	}

}

func (s *Service) claimJobs(ctx context.Context) error {
	jobs, err := s.queries.ClaimDuePaymentJobs(ctx, db.ClaimDuePaymentJobsParams{
		LeaseSeconds: LeaseSecond,
		BatchSize:    BatchSize,
	})
	if err != nil {
		return err
	}

	for _, job := range jobs {
		s.logger.Info("claimed payment job", "id", job.ID, "kind", job.Kind, "payment_id", job.PaymentID)
		switch job.Kind {
		case "capture":
			if captureErr := s.captureJob(ctx, job); captureErr != nil {
				s.failOrRetry(ctx, job, captureErr)
			}
		case "void":
			if voidErr := s.voidJob(ctx, job); voidErr != nil {
				s.failOrRetry(ctx, job, voidErr)
			}
		case "refund":
			if refundErr := s.refundJob(ctx, job); refundErr != nil {
				s.failOrRetry(ctx, job, refundErr)
			}
		default:
			s.logger.Info("not supported at the moment", "kind", job.Kind)
		}
	}
	return nil

}

// refundJob gives the job's amount back to the customer, then adds it to the
// payment's refunded total and marks the job done, in one transaction. The
// total is read from the locked payment, not the job: two refunds claimed in
// the same batch would both see the same old total, and one would be lost.
// The job id is the idempotency key, so a retry never pays out twice.
func (s *Service) refundJob(ctx context.Context, job db.ClaimDuePaymentJobsRow) error {
	if job.PaymentRef == nil {
		return errNoPaymentRef
	}
	if job.Amount == nil || *job.Amount <= 0 {
		return errInvalidAmount
	}

	sk, err := s.secret.Decrypt(job.SecretKeyEnc)
	if err != nil {
		return err
	}
	provider, err := s.providers.Build(job.Provider, Credentials{
		AccountID: job.AccountID,
		SecretKey: sk,
		Live:      job.Live,
	})
	if err != nil {
		return err
	}

	refundRef, err := provider.Refund(ctx, *job.PaymentRef, *job.Amount, job.ID.String())
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	q := s.queries.WithTx(tx)

	payment, err := q.LockPayment(ctx, job.PaymentID)
	if err != nil {
		return err
	}

	total := payment.AmountRefunded + *job.Amount
	_, err = q.SetPaymentRefundedTotal(ctx, db.SetPaymentRefundedTotalParams{RefundedTotal: total, ID: payment.ID})
	if err != nil {
		return err
	}
	if err := q.MarkPaymentJobDone(ctx, db.MarkPaymentJobDoneParams{ProviderRef: &refundRef, ID: job.ID}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil

}

// captureJob takes the job's amount from the customer's card hold, then marks
// the payment captured and the job done. The job id is the idempotency key, so
// a retry never takes the money twice.
func (s *Service) captureJob(ctx context.Context, job db.ClaimDuePaymentJobsRow) error {
	if job.PaymentRef == nil {
		return errNoPaymentRef
	}
	if job.Amount == nil || *job.Amount <= 0 {
		return errInvalidAmount
	}

	sk, err := s.secret.Decrypt(job.SecretKeyEnc)
	if err != nil {
		return err
	}
	provider, err := s.providers.Build(job.Provider, Credentials{
		AccountID: job.AccountID,
		SecretKey: sk,
		Live:      job.Live,
	})
	if err != nil {
		return err
	}

	if err = provider.Capture(ctx, *job.PaymentRef, *job.Amount, job.ID.String()); err != nil {
		return err
	}
	_, err = s.queries.MarkPaymentCaptured(ctx, db.MarkPaymentCapturedParams{Amount: *job.Amount, ID: job.PaymentID})
	if err != nil {
		return err
	}
	err = s.queries.MarkPaymentJobDone(ctx, db.MarkPaymentJobDoneParams{ID: job.ID})
	if err != nil {
		return err
	}

	return nil

}

func (s *Service) voidJob(ctx context.Context, job db.ClaimDuePaymentJobsRow) error {
	if job.PaymentRef == nil {
		return errNoPaymentRef
	}

	sk, err := s.secret.Decrypt(job.SecretKeyEnc)
	if err != nil {
		return err
	}
	provider, err := s.providers.Build(job.Provider, Credentials{
		AccountID: job.AccountID,
		SecretKey: sk,
		Live:      job.Live,
	})
	if err != nil {
		return err
	}
	err = provider.Void(ctx, *job.PaymentRef, job.ID.String())
	if err != nil {
		return err
	}

	_, err = s.queries.MarkPaymentVoided(ctx, job.PaymentID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	err = s.queries.MarkPaymentJobDone(ctx, db.MarkPaymentJobDoneParams{ID: job.ID})
	if err != nil {
		return err
	}
	return nil

}

// failOrRetry handles a job that just failed. Errors that can never succeed
// (the provider doesn't know the payment, the money was already taken, there
// is no reference) give up straight away; anything else (network trouble,
// provider outage) is retried with a growing wait until retryDelays runs out.
func (s *Service) failOrRetry(ctx context.Context, job db.ClaimDuePaymentJobsRow, jobErr error) {
	message := jobErr.Error()
	permanent := isPermanentJobError(jobErr)
	if giveUp(job.Attempts, jobErr) {
		s.logger.Error(
			"payment job gave up",
			"id",
			job.ID,
			"kind",
			job.Kind,
			"payment_id",
			job.PaymentID,
			"attempts",
			job.Attempts,
			"permanent",
			permanent,
			"error",
			jobErr,
		)
		if job.Kind == "capture" {
			if err := s.captureGiveUp(ctx, job, message); err != nil {
				s.logger.Error("capture giveup job failed", "id", job.ID, "payment id", job.PaymentID, "error", err)
			}
		} else {
			err := s.queries.MarkPaymentJobFailed(ctx, db.MarkPaymentJobFailedParams{LastError: &message, ID: job.ID})
			if err != nil {
				s.logger.Error("marking payment job failed", "id", job.ID, "error", err)
			}
		}
		return
	}

	delay := retryDelays[job.Attempts-1]
	s.logger.Warn("payment job failed, will retry",
		"id", job.ID, "kind", job.Kind, "attempts", job.Attempts, "retry_in", delay, "error", jobErr)
	err := s.queries.MarkPaymentJobRetry(ctx, db.MarkPaymentJobRetryParams{
		LastError:     &message,
		NextAttemptAt: pgtype.Timestamptz{Time: s.now().Add(delay), Valid: true},
		ID:            job.ID,
	})
	if err != nil {
		s.logger.Error("marking payment job for retry", "id", job.ID, "error", err)
	}
}

// captureGiveup
func (s *Service) captureGiveUp(ctx context.Context, job db.ClaimDuePaymentJobsRow, message string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	q := s.queries.WithTx(tx)

	//lockpayment
	lockedPayment, err := q.LockPayment(ctx, job.PaymentID)
	if err != nil {
		return err
	}

	err = q.MarkPaymentJobFailed(ctx, db.MarkPaymentJobFailedParams{
		ID:        job.ID,
		LastError: &message,
	})

	if err != nil {
		return err
	}

	_, err = q.MarkPaymentCaptureFailed(ctx, db.MarkPaymentCaptureFailedParams{
		ID:          lockedPayment.ID,
		FailureCode: nil,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil { // save everything at once
			return err
		}
		return nil
	}

	if err != nil {
		return err
	}
	order, err := q.GetOrderByID(ctx, job.OrderID)
	if err != nil {
		return err
	}
	note := "Couldn't take " + FormatPence(*job.Amount) + " from the card. Don't hand the order over."
	if err := q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID:    order.ID,
		FromStatus: &order.Status,
		ToStatus:   order.Status,
		Kind:       "payment",
		Note:       &note,
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil { // save everything at once
		return err
	}

	return nil
}

// isPermanentJobError: errors that will fail the same way on every retry.
func isPermanentJobError(err error) bool {
	return errors.Is(err, ErrNotCapturable) ||
		errors.Is(err, ErrAlreadyCaptured) ||
		errors.Is(err, errNoPaymentRef) ||
		errors.Is(err, errInvalidAmount) ||
		errors.Is(err, ErrNotRefundable) ||
		errors.Is(err, ErrRefundTooLarge)
}

// giveUp decides whether a failed job stops for good. attempts already
// counts the try that just failed (the claim adds 1).
func giveUp(attempts int32, err error) bool {
	return isPermanentJobError(err) || attempts > int32(len(retryDelays))
}
