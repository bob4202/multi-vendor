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
// nothing to void and retrying can't help.
var errNoPaymentRef = errors.New("payment has no provider reference")

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
		if job.Kind != "void" {
			s.logger.Info("Not Supported yet", "kind", job.Kind)
			continue
		}
		if voidErr := s.voidJob(ctx, job); voidErr != nil {
			s.failOrRetry(ctx, job, voidErr)
		}
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
		s.logger.Error("payment job gave up",
			"id", job.ID, "kind", job.Kind, "payment_id", job.PaymentID,
			"attempts", job.Attempts, "permanent", permanent, "error", jobErr)
		err := s.queries.MarkPaymentJobFailed(ctx, db.MarkPaymentJobFailedParams{LastError: &message, ID: job.ID})
		if err != nil {
			s.logger.Error("marking payment job failed", "id", job.ID, "error", err)
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

// isPermanentJobError: errors that will fail the same way on every retry.
func isPermanentJobError(err error) bool {
	return errors.Is(err, ErrNotCapturable) ||
		errors.Is(err, ErrAlreadyCaptured) ||
		errors.Is(err, errNoPaymentRef)
}

// giveUp decides whether a failed job stops for good. attempts already
// counts the try that just failed (the claim adds 1).
func giveUp(attempts int32, err error) bool {
	return isPermanentJobError(err) || attempts > int32(len(retryDelays))
}
