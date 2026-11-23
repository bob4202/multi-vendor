package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
	// Stripe refuses expires_at under 30 minutes from now; 31 leaves a margin.
	// Send our deadline, unless it's earlier than Stripe allows.
	earliest := time.Now().Add(31 * time.Minute)
	expiresAt := in.ExpiresAt
	if expiresAt.Before(earliest) {
		expiresAt = earliest
	}

	param := &stripeapi.CheckoutSessionCreateParams{
		SuccessURL:    stripeapi.String(in.SuccessURL),
		CancelURL:     stripeapi.String(in.CancelURL),
		CustomerEmail: stripeapi.String(in.Email),

		PaymentIntentData: &stripeapi.CheckoutSessionCreatePaymentIntentDataParams{
			CaptureMethod: stripeapi.String(stripeapi.PaymentIntentCaptureMethodManual),
			Metadata: map[string]string{
				"payment_id": in.PaymentID.String(),
			},
		},
		Mode: stripeapi.String(stripeapi.CheckoutSessionModePayment),
		LineItems: []*stripeapi.CheckoutSessionCreateLineItemParams{
			{
				PriceData: &stripeapi.CheckoutSessionCreateLineItemPriceDataParams{
					Currency:   stripeapi.String(in.Currency),
					UnitAmount: stripeapi.Int64(in.Amount),
					ProductData: &stripeapi.CheckoutSessionCreateLineItemPriceDataProductDataParams{
						Name: stripeapi.String(in.Reference),
					},
				},
				Quantity: stripeapi.Int64(1),
			},
		},
		ExpiresAt: stripeapi.Int64(expiresAt.Unix()),
	}
	param.SetIdempotencyKey(in.PaymentID.String())
	param.AddMetadata("payment_id", in.PaymentID.String())

	result, err := s.client.V1CheckoutSessions.Create(ctx, param)
	if err != nil {
		return payment.Session{}, fmt.Errorf("stripe: create checkout session: %w", err)
	}

	return payment.Session{
		ProviderRef: result.ID,
		RedirectURL: result.URL,
	}, nil
}

func (s *Stripe) Capture(ctx context.Context, ref string, amount int64, idemKey string) error {
	param := &stripeapi.PaymentIntentCaptureParams{
		AmountToCapture: stripeapi.Int64(amount),
	}
	param.SetIdempotencyKey(idemKey)
	_, err := s.client.V1PaymentIntents.Capture(ctx, ref, param)
	if err != nil {
		return fmt.Errorf("stripe: capture err : %w", err)
	}
	return nil
}

func (s *Stripe) Void(ctx context.Context, ref string, idemKey string) error {
	param := &stripeapi.PaymentIntentCancelParams{}
	param.SetIdempotencyKey(idemKey)
	_, err := s.client.V1PaymentIntents.Cancel(ctx, ref, param)
	if err != nil {
		return fmt.Errorf("stripe: void err: %w", err)
	}
	return nil
}

func (s *Stripe) Refund(ctx context.Context, ref string, amount int64, idemKey string) (string, error) {
	param := &stripeapi.RefundCreateParams{
		PaymentIntent: stripeapi.String(ref),
		Amount:        stripeapi.Int64(amount),
	}
	param.SetIdempotencyKey(idemKey)
	result, err := s.client.V1Refunds.Create(ctx, param)
	if err != nil {
		return "", fmt.Errorf("stripe: refund err: %w", err)
	}
	return result.ID, nil
}

func (s *Stripe) ParseWebhook(r *http.Request, body []byte) (payment.Event, bool, error) {
	return payment.Event{}, false, errNotBuilt
}
