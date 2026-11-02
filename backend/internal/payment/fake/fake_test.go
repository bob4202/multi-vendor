package fake_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"ecommerce/backend/internal/payment"
	"ecommerce/backend/internal/payment/fake"
)

// newFake resets the shared memory and builds a provider, like the service would.
func newFake(t *testing.T) payment.Provider {
	t.Helper()
	fake.Reset()
	p, err := fake.New(payment.Credentials{WebhookSecret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// newSession starts a payment for the given amount and returns its ref.
func newSession(t *testing.T, p payment.Provider, amount int64) string {
	t.Helper()
	s, err := p.CreateSession(context.Background(), payment.SessionInput{
		PaymentID: uuid.New(),
		Amount:    amount,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s.ProviderRef
}

func TestCreateSessionSamePaymentSameRef(t *testing.T) {
	p := newFake(t)
	in := payment.SessionInput{PaymentID: uuid.New(), Amount: 2450}

	a, err := p.CreateSession(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.CreateSession(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.ProviderRef != b.ProviderRef {
		t.Fatalf("same payment id should give the same ref: %q vs %q", a.ProviderRef, b.ProviderRef)
	}
	if a.RedirectURL == "" {
		t.Fatal("redirect url should not be empty")
	}
}

func TestCaptureLessThanHold(t *testing.T) {
	p := newFake(t)
	ctx := context.Background()
	ref := newSession(t, p, 2450)

	if err := p.Capture(ctx, ref, 2210, "job-1"); err != nil {
		t.Fatal(err)
	}
	// Same key again: a safe retry, not a second capture.
	if err := p.Capture(ctx, ref, 2210, "job-1"); err != nil {
		t.Fatalf("same key should be a safe retry, got %v", err)
	}
	// A different key is a genuinely new capture.
	if err := p.Capture(ctx, ref, 100, "job-2"); !errors.Is(err, payment.ErrAlreadyCaptured) {
		t.Fatalf("want ErrAlreadyCaptured, got %v", err)
	}
}

func TestCaptureMoreThanHold(t *testing.T) {
	p := newFake(t)
	ref := newSession(t, p, 1000)

	err := p.Capture(context.Background(), ref, 1001, "job-1")
	if !errors.Is(err, payment.ErrNotCapturable) {
		t.Fatalf("want ErrNotCapturable, got %v", err)
	}
}

func TestVoid(t *testing.T) {
	p := newFake(t)
	ctx := context.Background()

	// Void, then capture: no longer possible.
	ref := newSession(t, p, 1000)
	if err := p.Void(ctx, ref, "void-1"); err != nil {
		t.Fatal(err)
	}
	if err := p.Capture(ctx, ref, 1000, "cap-1"); !errors.Is(err, payment.ErrNotCapturable) {
		t.Fatalf("want ErrNotCapturable after void, got %v", err)
	}

	// Capture, then void: too late, you must refund instead.
	ref2 := newSession(t, p, 1000)
	if err := p.Capture(ctx, ref2, 1000, "cap-2"); err != nil {
		t.Fatal(err)
	}
	if err := p.Void(ctx, ref2, "void-2"); !errors.Is(err, payment.ErrAlreadyCaptured) {
		t.Fatalf("want ErrAlreadyCaptured, got %v", err)
	}
}

func TestRefund(t *testing.T) {
	p := newFake(t)
	ctx := context.Background()
	ref := newSession(t, p, 2450)

	// Nothing captured yet, so nothing to refund.
	if _, err := p.Refund(ctx, ref, 100, "ref-0"); !errors.Is(err, payment.ErrNotRefundable) {
		t.Fatalf("want ErrNotRefundable, got %v", err)
	}

	if err := p.Capture(ctx, ref, 2210, "cap-1"); err != nil {
		t.Fatal(err)
	}

	first, err := p.Refund(ctx, ref, 500, "ref-1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.Refund(ctx, ref, 500, "ref-1")
	if err != nil || again != first {
		t.Fatalf("same key should return the same refund ref: %q vs %q (err %v)", first, again, err)
	}

	// 2210 captured - 500 refunded = 1710 left.
	if _, err := p.Refund(ctx, ref, 1711, "ref-2"); !errors.Is(err, payment.ErrRefundTooLarge) {
		t.Fatalf("want ErrRefundTooLarge, got %v", err)
	}
	if _, err := p.Refund(ctx, ref, 1710, "ref-3"); err != nil {
		t.Fatalf("refunding the exact remainder should work, got %v", err)
	}
}

func TestZeroAmountsRejected(t *testing.T) {
	p := newFake(t)
	ctx := context.Background()
	ref := newSession(t, p, 1000)

	if err := p.Capture(ctx, ref, 0, "cap-0"); err == nil {
		t.Fatal("capturing 0 should fail")
	}
	if err := p.Capture(ctx, ref, 1000, "cap-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Refund(ctx, ref, 0, "ref-0"); err == nil {
		t.Fatal("refunding 0 should fail")
	}
}

func TestFailNextThenRetry(t *testing.T) {
	p := newFake(t)
	ctx := context.Background()
	ref := newSession(t, p, 1000)

	boom := errors.New("provider down")
	fake.FailNext(boom)
	if err := p.Capture(ctx, ref, 1000, "job-1"); !errors.Is(err, boom) {
		t.Fatalf("want injected error, got %v", err)
	}
	// The failure clears itself, so the retry with the same key works.
	if err := p.Capture(ctx, ref, 1000, "job-1"); err != nil {
		t.Fatalf("retry after failure should work, got %v", err)
	}
}

func TestWebhook(t *testing.T) {
	p := newFake(t)
	pid := uuid.New()

	post := func(body, secret string) (payment.Event, bool, error) {
		req := httptest.NewRequest("POST", "/v1/webhooks/fake/x", strings.NewReader(body))
		if secret != "" {
			req.Header.Set("X-Fake-Secret", secret)
		}
		return p.ParseWebhook(req, []byte(body))
	}

	good := `{"id":"evt_1","kind":"authorised","payment_id":"` + pid.String() + `","amount":2450}`
	ev, ok, err := post(good, "s3cret")
	if err != nil || !ok {
		t.Fatalf("good webhook: ok=%v err=%v", ok, err)
	}
	if ev.Kind != payment.EventAuthorised || ev.PaymentID != pid || ev.Amount != 2450 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.ProviderRef != "fake_"+pid.String() {
		t.Fatalf("provider ref should default to the CreateSession ref, got %q", ev.ProviderRef)
	}

	// Missing secret: rejected.
	if _, _, err := post(good, ""); err == nil {
		t.Fatal("missing secret should be rejected")
	}

	// A kind we don't know: ignored, not an error.
	odd := `{"id":"evt_2","kind":"banana","payment_id":"` + pid.String() + `"}`
	if _, ok, err := post(odd, "s3cret"); ok || err != nil {
		t.Fatalf("unknown kind should be ignored: ok=%v err=%v", ok, err)
	}
}

func TestConnector(t *testing.T) {
	p := newFake(t)
	c, ok := p.(payment.Connector)
	if !ok {
		t.Fatal("fake provider should also be a Connector")
	}
	info, err := c.VerifyCredentials(context.Background())
	if err != nil || info.DisplayName == "" {
		t.Fatalf("verify: %+v %v", info, err)
	}
}
