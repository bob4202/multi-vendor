package payment

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
)

// Service is the money side of an order: starting checkout on the
// provider's hosted page, applying webhooks, and capturing, voiding and
// refunding. Each payment is made with the vendor's own provider account,
// so a Provider is built per call from the registry and their keys.
//
// The one rule: never call a provider inside a DB transaction. A slow or
// failing provider must not hold row locks, and a rollback can't undo a
// charge. Write our row first, call the provider, then update the row.
type Service struct {
	pool      *pgxpool.Pool
	queries   *db.Queries
	providers Registry
	secret    *crypto.SecretBox
	logger    *slog.Logger
	now       func() time.Time // swapped in tests

	// How long the shop has to accept an order once it's paid
	// (ORDER_ACCEPT_TIMEOUT_MINUTES): the countdown starts at payment.
	acceptTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, providers Registry, secret *crypto.SecretBox, acceptTimeout time.Duration, logger *slog.Logger) *Service {
	return &Service{
		pool:          pool,
		queries:       db.New(pool),
		providers:     providers,
		secret:        secret,
		logger:        logger,
		now:           time.Now,
		acceptTimeout: acceptTimeout,
	}
}

// HasProvider reports whether this server offers the named provider
// (PAYMENT_PROVIDERS).
func (s *Service) HasProvider(name string) bool {
	return s.providers.Has(name)
}

// CheckoutEnabled reports whether a branch can take card payments: its
// vendor has an active account with a provider this server offers. Card is
// the only way to pay, so false means checkout is closed.
func (s *Service) CheckoutEnabled(ctx context.Context, branchID uuid.UUID) (bool, error) {
	account, err := s.queries.GetBranchCheckoutAccount(ctx, branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.providers.Has(account.Provider), nil
}
