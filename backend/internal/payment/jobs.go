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
	RetrySecond    = 180
)

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
		err = s.voidJob(ctx, job)
		if err != nil {
			s.logger.Error("error voiding payment", "id", job.ID, "error", err)
			//if there's an error we'll mark that payment as the attempted and stuffs
			message := err.Error()
			retryDelays := [...]time.Duration{time.Minute, 3 * time.Minute, 5 * time.Minute}
			if job.Attempts >= int32(len(retryDelays)+1) {
				err = s.queries.MarkPaymentJobFailed(ctx, db.MarkPaymentJobFailedParams{
					LastError: &message,
					ID:        job.ID,
				})
				if err != nil {
					s.logger.Error("error marking payment job as failed", "id", job.ID, "error", err)
				}
			} else {
				delay := retryDelays[job.Attempts-1]
				err = s.queries.MarkPaymentJobRetry(ctx, db.MarkPaymentJobRetryParams{
					LastError: &message,
					NextAttemptAt: pgtype.Timestamptz{
						Time:  s.now().Add(delay),
						Valid: true,
					},
					ID: job.ID,
				})
				if err != nil {
					s.logger.Error("error marking payment job for retry", "id", job.ID, "error", err)
				}
			}
		}
	}
	return nil

}

func (s *Service) voidJob(ctx context.Context, job db.ClaimDuePaymentJobsRow) error {
	if job.PaymentRef == nil {
		return errors.New("payment reference nil")
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
