package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ecommerce/backend/internal/db"
)

var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrNotAwaitingPayment = errors.New("order is not awaiting payment")
	ErrPaymentExpired     = errors.New("payment window has closed")
	ErrProviderNotSetup   = errors.New("shop has no active payment account")
	// Another request for the same order is creating a session right now
	// (two tabs, double click). Trying again a moment later returns it.
	ErrCheckoutInProgress = errors.New("checkout already in progress")
)

// Must match the order status in the DB (orders.status).
const awaitingPayment = "awaiting_payment"

type CheckoutParams struct {
	BranchID   uuid.UUID
	OrderID    uuid.UUID
	SuccessURL string
	CancelURL  string
}

type CheckoutReturn struct {
	RedirectURL string
	ExpiresAt   time.Time
}

// StartCheckout returns the provider's hosted payment page for an order
// that is awaiting payment. Calling it again while a session is open
// returns the same page, so a double click or refresh can't start a
// second hold on the customer's card.
func (s *Service) StartCheckout(ctx context.Context, cp CheckoutParams) (CheckoutReturn, error) {
	order, err := s.queries.GetOrderByBranchAndID(ctx, db.GetOrderByBranchAndIDParams{
		BranchID: cp.BranchID,
		ID:       cp.OrderID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckoutReturn{}, ErrOrderNotFound
	}
	if err != nil {
		return CheckoutReturn{}, err
	}

	if order.Status != awaitingPayment {
		return CheckoutReturn{}, ErrNotAwaitingPayment
	}

	// A NULL deadline can't happen for awaiting_payment (DB check
	// constraint); if it ever does, treat the window as closed.
	if !order.PaymentDeadline.Valid || !s.now().Before(order.PaymentDeadline.Time) {
		return CheckoutReturn{}, ErrPaymentExpired
	}

	// Reuse the open session if there is one.
	session, err := s.queries.GetOpenPaymentSession(ctx, order.ID)
	if err == nil {
		return checkoutReturn(session)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CheckoutReturn{}, err
	}

	// None usable: close any stale one so CreatePayment isn't blocked by
	// payments_one_open_session_idx.
	if _, err := s.queries.ExpireStalePaymentSessions(ctx, order.ID); err != nil {
		return CheckoutReturn{}, err
	}

	return s.newSession(ctx, order, cp)
}

// newSession writes our payments row, asks the provider for a hosted page,
// then saves the page on the row. The provider call is outside any
// transaction (see Service).
func (s *Service) newSession(ctx context.Context, order db.Order, cp CheckoutParams) (CheckoutReturn, error) {
	// The vendor may have disconnected their account since the order was
	// placed: no active row means no payments.
	account, err := s.queries.GetBranchCheckoutAccount(ctx, order.BranchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckoutReturn{}, ErrProviderNotSetup
	}
	if err != nil {
		return CheckoutReturn{}, err
	}

	// The account's provider must be enabled on this server (PAYMENT_PROVIDERS).
	if !s.providers.Has(account.Provider) {
		return CheckoutReturn{}, ErrProviderNotSetup
	}

	secretKey, err := s.secret.Decrypt(account.SecretKeyEnc)
	if err != nil {
		return CheckoutReturn{}, fmt.Errorf("decrypt secret key for account %s: %w", account.ID, err)
	}
	webhookSecret, err := s.secret.Decrypt(account.WebhookSecretEnc)
	if err != nil {
		return CheckoutReturn{}, fmt.Errorf("decrypt webhook secret for account %s: %w", account.ID, err)
	}

	provider, err := s.providers.Build(account.Provider, Credentials{
		AccountID:     account.ID,
		SecretKey:     secretKey,
		WebhookSecret: webhookSecret,
		Live:          account.Live,
	})
	if err != nil {
		return CheckoutReturn{}, err
	}

	// The session ends with the order's payment window. A provider with a
	// longer minimum (Stripe: 30 minutes) clamps it in its adapter; money
	// held after the order is dropped is voided by the webhook.
	payment, err := s.queries.CreatePayment(ctx, db.CreatePaymentParams{
		Provider:         account.Provider,
		PaymentAccountID: account.ID,
		ExpiresAt:        order.PaymentDeadline,
		OrderID:          order.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The order left awaiting_payment since we read it (e.g. cancelled).
		return CheckoutReturn{}, ErrNotAwaitingPayment
	}
	if isUniqueViolation(err, "payments_one_open_session_idx") {
		// A concurrent request created the row first: hand back its session.
		session, err := s.queries.GetOpenPaymentSession(ctx, order.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			// Its provider call hasn't finished yet.
			return CheckoutReturn{}, ErrCheckoutInProgress
		}
		if err != nil {
			return CheckoutReturn{}, err
		}
		return checkoutReturn(session)
	}
	if err != nil {
		return CheckoutReturn{}, err
	}

	created, err := provider.CreateSession(ctx, SessionInput{
		PaymentID:  payment.ID,
		Amount:     payment.AmountAuthorised,
		Currency:   payment.Currency,
		Reference:  order.Reference,
		Email:      order.CustomerEmail,
		SuccessURL: cp.SuccessURL,
		CancelURL:  cp.CancelURL,
		ExpiresAt:  payment.ExpiresAt.Time,
	})
	if err != nil {
		// The row stays 'created' without a redirect URL;
		// ExpireStalePaymentSessions closes it on the next attempt.
		return CheckoutReturn{}, fmt.Errorf("create %s session for payment %s: %w", account.Provider, payment.ID, err)
	}

	payment, err = s.queries.SetPaymentSession(ctx, db.SetPaymentSessionParams{
		ProviderRef: &created.ProviderRef,
		RedirectUrl: &created.RedirectURL,
		ID:          payment.ID,
	})
	if err != nil {
		return CheckoutReturn{}, err
	}

	return checkoutReturn(payment)
}

func checkoutReturn(p db.Payment) (CheckoutReturn, error) {
	if p.RedirectUrl == nil {
		return CheckoutReturn{}, fmt.Errorf("payment %s has no redirect url", p.ID)
	}
	return CheckoutReturn{RedirectURL: *p.RedirectUrl, ExpiresAt: p.ExpiresAt.Time}, nil
}

// isUniqueViolation reports whether err is a unique violation on the named
// index, e.g. payments_one_open_session_idx (one 'created' payment per order).
func isUniqueViolation(err error, index string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == index
}
