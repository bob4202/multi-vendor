package stripe

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	stripeapi "github.com/stripe/stripe-go/v87"

	"ecommerce/backend/internal/payment"
)

// fakeStripeErr builds the error stripe-go returns, with the PaymentIntent
// status Stripe sends along ("" = none).
func fakeStripeErr(code stripeapi.ErrorCode, status stripeapi.PaymentIntentStatus) error {
	se := &stripeapi.Error{Code: code, Msg: "message from Stripe"}
	if status != "" {
		se.PaymentIntent = &stripeapi.PaymentIntent{Status: status}
	}
	return se
}

// No network: which of our errors each Stripe error becomes, per call.
// want nil = not one of ours, so the worker retries it.
func TestStripeErrorMapping(t *testing.T) {
	const (
		succeeded = stripeapi.PaymentIntentStatusSucceeded
		canceled  = stripeapi.PaymentIntentStatusCanceled
	)
	cases := []struct {
		name string
		op   string
		err  error
		want error
	}{
		{"capture: already captured", opCapture, fakeStripeErr(stripeapi.ErrorCodeChargeAlreadyCaptured, ""), payment.ErrAlreadyCaptured},
		{"capture: hold expired", opCapture, fakeStripeErr(stripeapi.ErrorCodeChargeExpiredForCapture, ""), payment.ErrNotCapturable},
		{"capture: more than the hold", opCapture, fakeStripeErr(stripeapi.ErrorCodeAmountTooLarge, ""), payment.ErrNotCapturable},
		{"capture: no such intent", opCapture, fakeStripeErr(stripeapi.ErrorCodeResourceMissing, ""), payment.ErrNotCapturable},
		{"capture: wrong state, captured", opCapture, fakeStripeErr(stripeapi.ErrorCodePaymentIntentUnexpectedState, succeeded), payment.ErrAlreadyCaptured},
		{"capture: wrong state, cancelled", opCapture, fakeStripeErr(stripeapi.ErrorCodePaymentIntentUnexpectedState, canceled), payment.ErrNotCapturable},
		{"void: wrong state, captured", opVoid, fakeStripeErr(stripeapi.ErrorCodePaymentIntentUnexpectedState, succeeded), payment.ErrAlreadyCaptured},
		{"void: no such intent", opVoid, fakeStripeErr(stripeapi.ErrorCodeResourceMissing, ""), payment.ErrNotCapturable},
		{"refund: too large", opRefund, fakeStripeErr(stripeapi.ErrorCodeAmountTooLarge, ""), payment.ErrRefundTooLarge},
		{"refund: already refunded", opRefund, fakeStripeErr(stripeapi.ErrorCodeChargeAlreadyRefunded, ""), payment.ErrNotRefundable},
		{"refund: disputed", opRefund, fakeStripeErr(stripeapi.ErrorCodeChargeDisputed, ""), payment.ErrNotRefundable},
		{"refund: nothing captured", opRefund, fakeStripeErr(stripeapi.ErrorCodePaymentIntentUnexpectedState, ""), payment.ErrNotRefundable},
		{"refund: too large (no code, amount param)", opRefund, &stripeapi.Error{Type: stripeapi.ErrorTypeInvalidRequest, Param: "amount"}, payment.ErrRefundTooLarge},
		{"refund: uncaptured (no code)", opRefund, &stripeapi.Error{Type: stripeapi.ErrorTypeInvalidRequest}, payment.ErrNotRefundable},
		{"refund: no code, not an invalid request", opRefund, &stripeapi.Error{Type: stripeapi.ErrorTypeAPI}, nil},
		{"refund: code is per call (already captured means nothing here)", opRefund, fakeStripeErr(stripeapi.ErrorCodeChargeAlreadyCaptured, ""), nil},
		{"unknown Stripe code is retried", opCapture, fakeStripeErr(stripeapi.ErrorCodeRateLimit, ""), nil},
		{"not a Stripe error (network) is retried", opCapture, errors.New("connection reset"), nil},
	}
	ours := []error{payment.ErrNotCapturable, payment.ErrAlreadyCaptured, payment.ErrNotRefundable, payment.ErrRefundTooLarge}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := stripeError(c.op, c.err)
			if got == nil {
				t.Fatal("stripeError returned nil")
			}
			if c.want != nil {
				if !errors.Is(got, c.want) {
					t.Errorf("got %v, want it to be %v", got, c.want)
				}
				return
			}
			for _, e := range ours {
				if errors.Is(got, e) {
					t.Errorf("got %v, want a plain error the worker retries", got)
				}
			}
		})
	}
}

// The error text names the call and Stripe's code, never the whole JSON
// response (which carries the PaymentIntent and its client secret).
func TestStripeErrorText(t *testing.T) {
	se := &stripeapi.Error{
		Code:          stripeapi.ErrorCodeRateLimit,
		Msg:           "slow down",
		PaymentIntent: &stripeapi.PaymentIntent{ClientSecret: "pi_x_secret_y"},
	}
	got := stripeError(opRefund, se).Error()
	if want := "stripe: refund: rate_limit: slow down"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestAlreadyCancelled(t *testing.T) {
	unexpected := stripeapi.ErrorCodePaymentIntentUnexpectedState
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"cancelled", fakeStripeErr(unexpected, stripeapi.PaymentIntentStatusCanceled), true},
		{"wrapped", fmt.Errorf("x: %w", fakeStripeErr(unexpected, stripeapi.PaymentIntentStatusCanceled)), true},
		{"captured", fakeStripeErr(unexpected, stripeapi.PaymentIntentStatusSucceeded), false},
		{"no intent sent", fakeStripeErr(unexpected, ""), false},
		{"other code", fakeStripeErr(stripeapi.ErrorCodeResourceMissing, stripeapi.PaymentIntentStatusCanceled), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := alreadyCancelled(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// With a test key: the codes Stripe really sends map the way the table says.
// Each call uses a new idempotency key, like a new job, not a retry.
func TestStripeErrorsAgainstStripe(t *testing.T) {
	p := testProvider(t)
	ctx := context.Background()
	key := uuid.NewString

	t.Run("capture a cancelled hold", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Void(ctx, ref, key()); err != nil {
			t.Fatalf("Void: %v", err)
		}
		if err := p.Capture(ctx, ref, 1000, key()); !errors.Is(err, payment.ErrNotCapturable) {
			t.Errorf("Capture = %v, want ErrNotCapturable", err)
		}
	})

	t.Run("capture twice", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Capture(ctx, ref, 1000, key()); err != nil {
			t.Fatalf("Capture: %v", err)
		}
		if err := p.Capture(ctx, ref, 1000, key()); !errors.Is(err, payment.ErrAlreadyCaptured) {
			t.Errorf("second Capture = %v, want ErrAlreadyCaptured", err)
		}
	})

	t.Run("capture more than the hold", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Capture(ctx, ref, 1500, key()); !errors.Is(err, payment.ErrNotCapturable) {
			t.Errorf("Capture = %v, want ErrNotCapturable", err)
		}
	})

	t.Run("void twice is done, not an error", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Void(ctx, ref, key()); err != nil {
			t.Fatalf("Void: %v", err)
		}
		if err := p.Void(ctx, ref, key()); err != nil {
			t.Errorf("second Void = %v, want nil", err)
		}
	})

	t.Run("void after capture", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Capture(ctx, ref, 1000, key()); err != nil {
			t.Fatalf("Capture: %v", err)
		}
		if err := p.Void(ctx, ref, key()); !errors.Is(err, payment.ErrAlreadyCaptured) {
			t.Errorf("Void = %v, want ErrAlreadyCaptured", err)
		}
	})

	t.Run("refund more than captured", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if err := p.Capture(ctx, ref, 1000, key()); err != nil {
			t.Fatalf("Capture: %v", err)
		}
		if _, err := p.Refund(ctx, ref, 1500, key()); !errors.Is(err, payment.ErrRefundTooLarge) {
			t.Errorf("Refund = %v, want ErrRefundTooLarge", err)
		}
	})

	t.Run("refund a hold that was never captured", func(t *testing.T) {
		ref := heldPaymentIntent(t, 1000)
		if _, err := p.Refund(ctx, ref, 500, key()); !errors.Is(err, payment.ErrNotRefundable) {
			t.Errorf("Refund = %v, want ErrNotRefundable", err)
		}
	})

	t.Run("no such PaymentIntent", func(t *testing.T) {
		if err := p.Capture(ctx, "pi_does_not_exist", 1000, key()); !errors.Is(err, payment.ErrNotCapturable) {
			t.Errorf("Capture = %v, want ErrNotCapturable", err)
		}
	})
}
