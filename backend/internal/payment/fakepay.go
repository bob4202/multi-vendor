package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ecommerce/backend/internal/db"
)

// The fake provider's hosted page (/fake-pay/{ref}), for local development.
// It plays the provider's side: Pay or Decline sends a signed webhook
// through HandleWebhook, the same path Stripe's webhooks take.

const fakeProvider = "fake"

var ErrFakePaymentNotFound = errors.New("fake payment not found")

// FakeCheckout is what the fake page shows.
type FakeCheckout struct {
	Reference string // the order's
	Amount    int64
	Status    string // the payment's
}

func (s *Service) fakePayment(ctx context.Context, providerRef string) (db.Payment, error) {
	if !s.providers.Has(fakeProvider) {
		return db.Payment{}, ErrFakePaymentNotFound
	}
	// Outside a transaction the lock is released at once: this is a read.
	p, err := s.queries.LockPaymentByProviderRef(ctx, db.LockPaymentByProviderRefParams{
		Provider: fakeProvider, ProviderRef: &providerRef,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Payment{}, ErrFakePaymentNotFound
	}
	return p, err
}

// FakeCheckoutInfo loads the payment behind a fake page.
func (s *Service) FakeCheckoutInfo(ctx context.Context, providerRef string) (FakeCheckout, error) {
	p, err := s.fakePayment(ctx, providerRef)
	if err != nil {
		return FakeCheckout{}, err
	}
	order, err := s.queries.GetOrderByID(ctx, p.OrderID)
	if err != nil {
		return FakeCheckout{}, err
	}
	return FakeCheckout{Reference: order.Reference, Amount: p.AmountAuthorised, Status: p.Status}, nil
}

// SimulateFakeEvent sends the webhook the fake provider would send for this
// payment: EventAuthorised for Pay, EventFailed for Decline. The body is
// the fake's wire format (see fake.ParseWebhook).
func (s *Service) SimulateFakeEvent(ctx context.Context, providerRef string, kind EventKind) error {
	p, err := s.fakePayment(ctx, providerRef)
	if err != nil {
		return err
	}
	account, err := s.queries.GetVendorPaymentAccountCredentials(ctx, p.PaymentAccountID)
	if err != nil {
		return err
	}
	webhookSecret, err := s.secret.Decrypt(account.WebhookSecretEnc)
	if err != nil {
		return fmt.Errorf("decrypt webhook secret for account %s: %w", account.ID, err)
	}

	body, err := json.Marshal(map[string]any{
		"id":           "evt_" + uuid.NewString(),
		"kind":         kind,
		"payment_id":   p.ID,
		"provider_ref": providerRef,
		"amount":       p.AmountAuthorised,
	})
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/fake-webhook", bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("X-Fake-Secret", webhookSecret)
	return s.HandleWebhook(ctx, fakeProvider, account.ID, r, body)
}
