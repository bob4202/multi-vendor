package fake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"

	"ecommerce/backend/internal/payment"
)

// Compile-time checks: the compiler complains here if a method is missing.
var _ payment.Provider = (*Fake)(nil)
var _ payment.Connector = (*Fake)(nil)

// charge is what the fake remembers about ONE payment.
type charge struct {
	authorizedAmount int64 // the hold placed by CreateSession
	capturedAmount   int64 // amount captured so far (0 = not captured yet)
	refundedAmount   int64
	voided           bool
}

// The service builds a brand new Fake for every call, so the "provider's
// memory" has to live here, outside the struct. It plays the role of
// Stripe's servers.
var (
	mu       sync.Mutex
	charges  = map[string]*charge{} // key: provider ref
	done     = map[string]string{}  // key: idempotency key, value: what we answered
	failNext error                  // set by FailNext, used once
)

// FailNext makes the next CreateSession, Capture, Void or Refund call return
// err, then clears itself. Tests use it to check retries.
func FailNext(err error) {
	mu.Lock()
	defer mu.Unlock()
	failNext = err
}

// Reset forgets everything. Call it at the start of each test.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	charges = map[string]*charge{}
	done = map[string]string{}
	failNext = nil
}

// takeFailure returns the injected error once. The caller must hold mu.
func takeFailure() error {
	err := failNext
	failNext = nil
	return err
}

type Fake struct {
	secret string // webhook secret, taken from Credentials
}

// New is the factory you register in the registry.
func New(c payment.Credentials) (payment.Provider, error) {
	return &Fake{secret: c.WebhookSecret}, nil
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) CreateSession(ctx context.Context, in payment.SessionInput) (payment.Session, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := takeFailure(); err != nil {
		return payment.Session{}, err
	}

	// Same payment ID gives the same ref, so a retry never makes a second hold.
	ref := "fake_" + in.PaymentID.String()
	if _, ok := charges[ref]; !ok {
		charges[ref] = &charge{authorizedAmount: in.Amount}
	}
	// The Pay / Decline page (httpapi/fakepay.go) sends the customer back
	// to these, like the provider's hosted page would.
	back := url.Values{"success": {in.SuccessURL}, "cancel": {in.CancelURL}}
	return payment.Session{
		ProviderRef: ref,
		RedirectURL: "http://localhost:8080/fake-pay/" + ref + "?" + back.Encode(),
	}, nil
}

func (f *Fake) Capture(ctx context.Context, ref string, amount int64, idemKey string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := takeFailure(); err != nil {
		return err
	}
	if _, seen := done[idemKey]; seen {
		return nil // a retry of a call that already worked
	}
	if amount <= 0 {
		return fmt.Errorf("fake capture %s: amount must be positive", ref)
	}

	c, ok := charges[ref]
	switch {
	case !ok || c.voided:
		return fmt.Errorf("fake capture %s: %w", ref, payment.ErrNotCapturable)
	case c.capturedAmount > 0:
		return fmt.Errorf("fake capture %s: %w", ref, payment.ErrAlreadyCaptured)
	case amount > c.authorizedAmount:
		return fmt.Errorf("fake capture %s: amount %d above hold %d: %w",
			ref, amount, c.authorizedAmount, payment.ErrNotCapturable)
	}

	c.capturedAmount = amount
	done[idemKey] = "captured"
	return nil
}

func (f *Fake) Void(ctx context.Context, ref string, idemKey string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := takeFailure(); err != nil {
		return err
	}
	if _, seen := done[idemKey]; seen {
		return nil
	}

	c, ok := charges[ref]
	switch {
	case !ok:
		return fmt.Errorf("fake void %s: %w", ref, payment.ErrNotCapturable)
	case c.capturedAmount > 0:
		// Too late to void: the money is taken, so the service should refund.
		return fmt.Errorf("fake void %s: %w", ref, payment.ErrAlreadyCaptured)
	}

	c.voided = true
	done[idemKey] = "voided"
	return nil
}

func (f *Fake) Refund(ctx context.Context, ref string, amount int64, idemKey string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := takeFailure(); err != nil {
		return "", err
	}
	if r, seen := done[idemKey]; seen {
		return r, nil // same key gives the same refund ref
	}
	if amount <= 0 {
		return "", fmt.Errorf("fake refund %s: amount must be positive", ref)
	}

	c, ok := charges[ref]
	if !ok || c.capturedAmount == 0 {
		return "", fmt.Errorf("fake refund %s: %w", ref, payment.ErrNotRefundable)
	}
	if amount > c.capturedAmount-c.refundedAmount {
		return "", fmt.Errorf("fake refund %s: %w", ref, payment.ErrRefundTooLarge)
	}

	c.refundedAmount += amount
	refundRef := "fake_refund_" + idemKey
	done[idemKey] = refundRef
	return refundRef, nil
}

// webhookBody is the JSON the fake's own Pay / Fail page will post to your
// webhook route. Only id, kind and payment_id are required.
type webhookBody struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	PaymentID   string `json:"payment_id"`
	ProviderRef string `json:"provider_ref"`
	PaymentRef  string `json:"payment_ref"`
	Amount      int64  `json:"amount"`
}

func (f *Fake) ParseWebhook(r *http.Request, body []byte) (payment.Event, bool, error) {
	// Stand-in for a signature check: the header must carry the secret.
	if r.Header.Get("X-Fake-Secret") != f.secret {
		return payment.Event{}, false, errors.New("fake: bad signature")
	}

	var b webhookBody
	if err := json.Unmarshal(body, &b); err != nil {
		return payment.Event{}, false, fmt.Errorf("fake: bad body: %w", err)
	}
	pid, err := uuid.Parse(b.PaymentID)
	if err != nil || b.ID == "" {
		return payment.Event{}, false, errors.New("fake: missing id or payment_id")
	}

	switch payment.EventKind(b.Kind) {
	case payment.EventAuthorised, payment.EventFailed, payment.EventExpired,
		payment.EventCaptured, payment.EventVoided, payment.EventRefunded:
		// one of ours: carry on
	default:
		return payment.Event{}, false, nil // not an event we care about: ignore it
	}

	// Handy defaults, so a hand-written test webhook can leave the refs out.
	if b.ProviderRef == "" {
		b.ProviderRef = "fake_" + pid.String()
	}
	if b.PaymentRef == "" {
		b.PaymentRef = b.ProviderRef
	}

	return payment.Event{
		ID:          b.ID,
		Kind:        payment.EventKind(b.Kind),
		PaymentID:   pid,
		ProviderRef: b.ProviderRef,
		PaymentRef:  b.PaymentRef,
		Amount:      b.Amount,
		Raw:         body, // already valid JSON, so it fits payment_events.payload
	}, true, nil
}

// Connector: the vendor "connect" screen works locally with any key.
func (f *Fake) VerifyCredentials(ctx context.Context) (payment.AccountInfo, error) {
	return payment.AccountInfo{DisplayName: "Fake Test Shop", Live: false}, nil
}

func (f *Fake) RegisterWebhook(ctx context.Context, url string) (string, string, error) {
	return "fake_webhook", f.secret, nil
}

func (f *Fake) DeleteWebhook(ctx context.Context, webhookID string) error { return nil }
