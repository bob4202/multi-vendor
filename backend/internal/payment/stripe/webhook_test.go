package stripe

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v87/webhook"

	"ecommerce/backend/internal/payment"
)

const testWebhookSecret = "whsec_test_secret"

// stripeEvent builds a webhook body like Stripe's, wrapping one object.
func stripeEvent(t *testing.T, id, typ string, object map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id":          id,
		"object":      "event",
		"api_version": "2020-08-27", // an older account version must still parse
		"type":        typ,
		"data":        map[string]any{"object": object},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// parse signs body with secret as Stripe would at time at, and hands it to
// ParseWebhook on a provider whose webhook secret is testWebhookSecret.
func parse(t *testing.T, body []byte, secret string, at time.Time) (payment.Event, bool, error) {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: body, Secret: secret, Timestamp: at,
	})
	r := httptest.NewRequest("POST", "/v1/webhooks/stripe/acct", bytes.NewReader(body))
	r.Header.Set("Stripe-Signature", signed.Header)
	s := &Stripe{webhookSecret: testWebhookSecret}
	return s.ParseWebhook(r, body)
}

func TestParseWebhook(t *testing.T) {
	pid := uuid.New()
	meta := map[string]any{"payment_id": pid.String()}

	intent := func(status string, extra map[string]any) map[string]any {
		o := map[string]any{"id": "pi_1", "object": "payment_intent", "status": status, "metadata": meta}
		for k, v := range extra {
			o[k] = v
		}
		return o
	}

	cases := []struct {
		name   string
		typ    string
		object map[string]any
		want   payment.Event // ID and Raw are checked separately
		wantOK bool
	}{
		{
			name:   "hold in place → authorised",
			typ:    "payment_intent.amount_capturable_updated",
			object: intent("requires_capture", map[string]any{"amount": 1000, "amount_capturable": 1000}),
			want:   payment.Event{Kind: payment.EventAuthorised, PaymentID: pid, ProviderRef: "pi_1", PaymentRef: "pi_1", Amount: 1000},
			wantOK: true,
		},
		{
			name:   "capturable update without a hold → ignored",
			typ:    "payment_intent.amount_capturable_updated",
			object: intent("succeeded", map[string]any{"amount_capturable": 0}),
		},
		{
			name:   "card declined → failed",
			typ:    "payment_intent.payment_failed",
			object: intent("requires_payment_method", nil),
			want:   payment.Event{Kind: payment.EventFailed, PaymentID: pid, ProviderRef: "pi_1", PaymentRef: "pi_1"},
			wantOK: true,
		},
		{
			name:   "captured (less than the hold) → captured with the amount taken",
			typ:    "payment_intent.succeeded",
			object: intent("succeeded", map[string]any{"amount": 1000, "amount_received": 950}),
			want:   payment.Event{Kind: payment.EventCaptured, PaymentID: pid, ProviderRef: "pi_1", PaymentRef: "pi_1", Amount: 950},
			wantOK: true,
		},
		{
			name:   "cancelled → voided",
			typ:    "payment_intent.canceled",
			object: intent("canceled", nil),
			want:   payment.Event{Kind: payment.EventVoided, PaymentID: pid, ProviderRef: "pi_1", PaymentRef: "pi_1"},
			wantOK: true,
		},
		{
			name:   "session expired → expired",
			typ:    "checkout.session.expired",
			object: map[string]any{"id": "cs_1", "object": "checkout.session", "metadata": meta, "payment_intent": nil},
			want:   payment.Event{Kind: payment.EventExpired, PaymentID: pid, ProviderRef: "cs_1"},
			wantOK: true,
		},
		{
			name: "refund → refunded with the running total",
			typ:  "charge.refunded",
			object: map[string]any{
				"id": "ch_1", "object": "charge", "metadata": meta, "payment_intent": "pi_1",
				"amount": 1000, "amount_refunded": 700, // e.g. 300 earlier + 400 now
			},
			want:   payment.Event{Kind: payment.EventRefunded, PaymentID: pid, ProviderRef: "ch_1", PaymentRef: "pi_1", Amount: 700},
			wantOK: true,
		},
		{
			name:   "session completed → ignored (the hold comes from the intent)",
			typ:    "checkout.session.completed",
			object: map[string]any{"id": "cs_1", "object": "checkout.session", "metadata": meta, "payment_intent": "pi_1"},
		},
		{
			name:   "event type we don't use → ignored",
			typ:    "customer.created",
			object: map[string]any{"id": "cus_1", "object": "customer"},
		},
		{
			name:   "no payment_id: a payment made elsewhere on the shop's account → ignored",
			typ:    "payment_intent.succeeded",
			object: map[string]any{"id": "pi_2", "object": "payment_intent", "status": "succeeded", "amount_received": 500},
		},
		{
			name:   "payment_id that isn't a uuid → ignored",
			typ:    "payment_intent.canceled",
			object: map[string]any{"id": "pi_3", "object": "payment_intent", "status": "canceled", "metadata": map[string]any{"payment_id": "nope"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := stripeEvent(t, "evt_1", c.typ, c.object)
			ev, ok, err := parse(t, body, testWebhookSecret, time.Now())
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if ev.ID != "evt_1" {
				t.Errorf("ID = %q, want evt_1", ev.ID)
			}
			if !bytes.Equal(ev.Raw, body) {
				t.Error("Raw isn't the body as received")
			}
			ev.ID, ev.Raw = "", nil
			if !reflect.DeepEqual(ev, c.want) {
				t.Errorf("event = %+v\n          want %+v", ev, c.want)
			}
		})
	}
}

func TestParseWebhookRejects(t *testing.T) {
	body := stripeEvent(t, "evt_1", "payment_intent.canceled", map[string]any{
		"id": "pi_1", "object": "payment_intent", "metadata": map[string]any{"payment_id": uuid.NewString()},
	})

	t.Run("signed with another secret", func(t *testing.T) {
		if _, _, err := parse(t, body, "whsec_someone_else", time.Now()); err == nil {
			t.Error("want an error")
		}
	})

	t.Run("signature too old (a replayed request)", func(t *testing.T) {
		if _, _, err := parse(t, body, testWebhookSecret, time.Now().Add(-time.Hour)); err == nil {
			t.Error("want an error")
		}
	})

	t.Run("body changed after signing", func(t *testing.T) {
		signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: testWebhookSecret})
		tampered := bytes.Replace(body, []byte("payment_intent.canceled"), []byte("payment_intent.succeeded"), 1)
		r := httptest.NewRequest("POST", "/", bytes.NewReader(tampered))
		r.Header.Set("Stripe-Signature", signed.Header)
		s := &Stripe{webhookSecret: testWebhookSecret}
		if _, _, err := s.ParseWebhook(r, tampered); err == nil {
			t.Error("want an error")
		}
	})

	t.Run("no signature header", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
		s := &Stripe{webhookSecret: testWebhookSecret}
		if _, _, err := s.ParseWebhook(r, body); err == nil {
			t.Error("want an error")
		}
	})
}
