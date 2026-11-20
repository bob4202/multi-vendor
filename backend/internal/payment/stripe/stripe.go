package stripe

import (
	"context"
	"errors"
	"net/http"

	stripeapi "github.com/stripe/stripe-go/v87"

	"ecommerce/backend/internal/payment"
)

var _ payment.Provider = (*Stripe)(nil)

var errNotBuilt = errors.New("stripe not implemented yet")

type Stripe struct {
	client        *stripeapi.Client
	webhookSecret string
}

func New(c payment.Credentials) (payment.Provider, error) {
	if c.SecretKey == "" {
		return nil, errors.New("stripe api key not configured")
	}
	return &Stripe{
		client:        stripeapi.NewClient(c.SecretKey),
		webhookSecret: c.WebhookSecret,
	}, nil
}

func (s *Stripe) Name() string { return "stripe" }

func (s *Stripe) CreateSession(ctx context.Context, in payment.SessionInput) (payment.Session, error) {
	return payment.Session{}, errNotBuilt
}

func (s *Stripe) Capture(ctx context.Context, ref string, amount int64, idemKey string) error {
	return errNotBuilt
}

func (s *Stripe) Void(ctx context.Context, ref string, idemKey string) error {
	return errNotBuilt
}

func (s *Stripe) Refund(ctx context.Context, ref string, amount int64, idemKey string) (string, error) {
	return "", errNotBuilt
}

func (s *Stripe) ParseWebhook(r *http.Request, body []byte) (payment.Event, bool, error) {
	return payment.Event{}, false, errNotBuilt
}
