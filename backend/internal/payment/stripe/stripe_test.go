package stripe

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	stripeapi "github.com/stripe/stripe-go/v87"

	"ecommerce/backend/internal/payment"
)

// testKey returns STRIPE_TEST_SECRET_KEY, or skips the test when it isn't
// set, so CI and normal runs never call Stripe.
func testKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("STRIPE_TEST_SECRET_KEY")
	if key == "" {
		t.Skip("STRIPE_TEST_SECRET_KEY not set: skipping a test that calls Stripe")
	}
	if !strings.HasPrefix(key, "sk_test_") && !strings.HasPrefix(key, "rk_test_") {
		t.Fatal("STRIPE_TEST_SECRET_KEY must be a test-mode key (sk_test_ or rk_test_)")
	}
	return key
}

// testProvider builds a Stripe provider from the test key.
func testProvider(t *testing.T) payment.Provider {
	t.Helper()
	p, err := New(payment.Credentials{SecretKey: testKey(t)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewRefusesEmptyKey(t *testing.T) {
	if _, err := New(payment.Credentials{}); err == nil {
		t.Fatal("New with no secret key: want an error, got nil")
	}
}

func TestCreateSession(t *testing.T) {
	p := testProvider(t)

	session, err := p.CreateSession(context.Background(), payment.SessionInput{
		PaymentID:  uuid.New(),
		Amount:     1000, // £10.00
		Currency:   "gbp",
		Reference:  "Order TEST-1",
		Email:      "test@example.com",
		SuccessURL: "http://localhost:3000/orders/test?paid=1",
		CancelURL:  "http://localhost:3000/checkout",
		// Our usual window, already slightly under Stripe's 30-minute minimum.
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if !strings.HasPrefix(session.ProviderRef, "cs_test_") {
		t.Errorf("ProviderRef = %q, want a cs_test_ session id", session.ProviderRef)
	}
	if !strings.HasPrefix(session.RedirectURL, "https://checkout.stripe.com") {
		t.Errorf("RedirectURL = %q, want a checkout.stripe.com URL", session.RedirectURL)
	}
	t.Logf("open this and pay with 4242 4242 4242 4242: %s", session.RedirectURL)
}

// heldPaymentIntent makes a real card hold straight through the API, with
// Stripe's test card, so Capture and Void can be tested without a browser.
// It plays the part of the customer paying on the Checkout page.
func heldPaymentIntent(t *testing.T, amount int64) string {
	t.Helper()
	client := stripeapi.NewClient(testKey(t))
	pi, err := client.V1PaymentIntents.Create(context.Background(), &stripeapi.PaymentIntentCreateParams{
		Amount:        stripeapi.Int64(amount),
		Currency:      stripeapi.String("gbp"),
		CaptureMethod: stripeapi.String(string(stripeapi.PaymentIntentCaptureMethodManual)),
		PaymentMethod: stripeapi.String("pm_card_visa"),
		Confirm:       stripeapi.Bool(true),
		AutomaticPaymentMethods: &stripeapi.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled:        stripeapi.Bool(true),
			AllowRedirects: stripeapi.String(string(stripeapi.PaymentIntentAutomaticPaymentMethodsAllowRedirectsNever)),
		},
	})
	if err != nil {
		t.Fatalf("create test hold: %v", err)
	}
	if pi.Status != stripeapi.PaymentIntentStatusRequiresCapture {
		t.Fatalf("test hold status = %s, want requires_capture", pi.Status)
	}
	return pi.ID
}

// paymentIntent reads a PaymentIntent back from Stripe, to check what our
// method did to it.
func paymentIntent(t *testing.T, id string) *stripeapi.PaymentIntent {
	t.Helper()
	client := stripeapi.NewClient(testKey(t))
	pi, err := client.V1PaymentIntents.Retrieve(context.Background(), id, nil)
	if err != nil {
		t.Fatalf("retrieve %s: %v", id, err)
	}
	return pi
}

// A capture can take less than the hold (substitutions); Stripe releases the
// rest.
func TestCapturePartial(t *testing.T) {
	p := testProvider(t)
	ref := heldPaymentIntent(t, 1000)

	if err := p.Capture(context.Background(), ref, 950, uuid.NewString()); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	pi := paymentIntent(t, ref)
	if pi.Status != stripeapi.PaymentIntentStatusSucceeded {
		t.Errorf("status = %s, want succeeded", pi.Status)
	}
	if pi.AmountReceived != 950 {
		t.Errorf("amount received = %d, want 950", pi.AmountReceived)
	}
}

// The worker retries a job with the same job id. With the idempotency key,
// Stripe answers the retry like the first call instead of refusing it.
func TestCaptureRetryWithSameKey(t *testing.T) {
	p := testProvider(t)
	ref := heldPaymentIntent(t, 1000)
	key := uuid.NewString()

	if err := p.Capture(context.Background(), ref, 1000, key); err != nil {
		t.Fatalf("first Capture: %v", err)
	}
	if err := p.Capture(context.Background(), ref, 1000, key); err != nil {
		t.Fatalf("retried Capture with the same key: %v", err)
	}
	if got := paymentIntent(t, ref).AmountReceived; got != 1000 {
		t.Errorf("amount received = %d, want 1000 (taken once)", got)
	}
}

// Void releases the whole hold.
func TestVoid(t *testing.T) {
	p := testProvider(t)
	ref := heldPaymentIntent(t, 1000)

	if err := p.Void(context.Background(), ref, uuid.NewString()); err != nil {
		t.Fatalf("Void: %v", err)
	}

	pi := paymentIntent(t, ref)
	if pi.Status != stripeapi.PaymentIntentStatusCanceled {
		t.Errorf("status = %s, want canceled", pi.Status)
	}
	if pi.AmountReceived != 0 {
		t.Errorf("amount received = %d, want 0", pi.AmountReceived)
	}
}

// The worker retries a void with the same job id. A second cancel of an
// already-cancelled PaymentIntent is refused by Stripe, unless the
// idempotency key makes it a replay of the first.
func TestVoidRetryWithSameKey(t *testing.T) {
	p := testProvider(t)
	ref := heldPaymentIntent(t, 1000)
	key := uuid.NewString()

	if err := p.Void(context.Background(), ref, key); err != nil {
		t.Fatalf("first Void: %v", err)
	}
	if err := p.Void(context.Background(), ref, key); err != nil {
		t.Fatalf("retried Void with the same key: %v", err)
	}
}

// A refund can be part of what was captured, and returns Stripe's refund id.
func TestRefundPartial(t *testing.T) {
	p := testProvider(t)
	ref := heldPaymentIntent(t, 1000)
	if err := p.Capture(context.Background(), ref, 1000, uuid.NewString()); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	refundRef, err := p.Refund(context.Background(), ref, 300, uuid.NewString())
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if !strings.HasPrefix(refundRef, "re_") {
		t.Errorf("refund ref = %q, want a re_ refund id", refundRef)
	}
}
