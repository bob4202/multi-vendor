package stripe

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/payment"
)

// testProvider builds a Stripe provider from STRIPE_TEST_SECRET_KEY, or skips
// the test when it isn't set, so CI and normal runs never call Stripe.
func testProvider(t *testing.T) payment.Provider {
	t.Helper()
	key := os.Getenv("STRIPE_TEST_SECRET_KEY")
	if key == "" {
		t.Skip("STRIPE_TEST_SECRET_KEY not set: skipping a test that calls Stripe")
	}
	if !strings.HasPrefix(key, "sk_test_") && !strings.HasPrefix(key, "rk_test_") {
		t.Fatal("STRIPE_TEST_SECRET_KEY must be a test-mode key (sk_test_ or rk_test_)")
	}
	p, err := New(payment.Credentials{SecretKey: key})
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
