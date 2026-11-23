package stripe

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	stripeapi "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"

	"ecommerce/backend/internal/payment"
)

// ParseWebhook checks one webhook from this account's Stripe and turns it
// into our Event (payments-guide.md, "Webhook events to map").
//
//   - err: the signature or body is bad (HandleWebhook answers 400)
//   - ok = false: an event we don't use, or a payment that isn't ours
//     (HandleWebhook answers 200, so Stripe stops sending it)
func (s *Stripe) ParseWebhook(r *http.Request, body []byte) (payment.Event, bool, error) {
	// The signature is checked on the raw body before anything is decoded.
	// The account's webhook may run on an older Stripe API version than this
	// library; the fields we read are the same in all of them.
	e, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), s.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return payment.Event{}, false, fmt.Errorf("stripe webhook: %w", err)
	}
	if e.Data == nil {
		return payment.Event{}, false, fmt.Errorf("stripe webhook %s: no data object", e.ID)
	}

	ev := payment.Event{ID: e.ID, Raw: body}
	var metadata map[string]string

	switch e.Type {
	case stripeapi.EventTypePaymentIntentAmountCapturableUpdated,
		stripeapi.EventTypePaymentIntentPaymentFailed,
		stripeapi.EventTypePaymentIntentSucceeded,
		stripeapi.EventTypePaymentIntentCanceled:
		var pi stripeapi.PaymentIntent
		if err := json.Unmarshal(e.Data.Raw, &pi); err != nil {
			return payment.Event{}, false, fmt.Errorf("stripe webhook %s: decode payment intent: %w", e.ID, err)
		}
		metadata = pi.Metadata
		ev.ProviderRef = pi.ID
		ev.PaymentRef = pi.ID

		switch e.Type {
		case stripeapi.EventTypePaymentIntentAmountCapturableUpdated:
			// Only a real hold counts: the card is held and there's money
			// to capture.
			if pi.Status != stripeapi.PaymentIntentStatusRequiresCapture || pi.AmountCapturable <= 0 {
				return payment.Event{}, false, nil
			}
			ev.Kind = payment.EventAuthorised
			ev.Amount = pi.AmountCapturable
		case stripeapi.EventTypePaymentIntentPaymentFailed:
			ev.Kind = payment.EventFailed
		case stripeapi.EventTypePaymentIntentSucceeded:
			ev.Kind = payment.EventCaptured
			ev.Amount = pi.AmountReceived
		case stripeapi.EventTypePaymentIntentCanceled:
			ev.Kind = payment.EventVoided
		}

	case stripeapi.EventTypeCheckoutSessionExpired:
		var cs stripeapi.CheckoutSession
		if err := json.Unmarshal(e.Data.Raw, &cs); err != nil {
			return payment.Event{}, false, fmt.Errorf("stripe webhook %s: decode checkout session: %w", e.ID, err)
		}
		metadata = cs.Metadata
		ev.Kind = payment.EventExpired
		ev.ProviderRef = cs.ID
		if cs.PaymentIntent != nil {
			ev.PaymentRef = cs.PaymentIntent.ID
		}

	case stripeapi.EventTypeChargeRefunded:
		var ch stripeapi.Charge
		if err := json.Unmarshal(e.Data.Raw, &ch); err != nil {
			return payment.Event{}, false, fmt.Errorf("stripe webhook %s: decode charge: %w", e.ID, err)
		}
		// A charge made by a PaymentIntent carries a copy of its metadata
		// (checked in test mode), so payment_id is here too.
		metadata = ch.Metadata
		ev.Kind = payment.EventRefunded
		ev.ProviderRef = ch.ID
		if ch.PaymentIntent != nil {
			ev.PaymentRef = ch.PaymentIntent.ID
		}
		// The running total refunded on this charge, not just this refund
		// (webhook.go stores it with SetPaymentRefundedTotal).
		ev.Amount = ch.AmountRefunded

	default:
		return payment.Event{}, false, nil
	}

	// No payment_id of ours: a payment made some other way on the shop's
	// own Stripe account (their till, another website). Not ours to handle,
	// and an error would only make Stripe resend it for days.
	id, err := uuid.Parse(metadata["payment_id"])
	if err != nil {
		return payment.Event{}, false, nil
	}
	ev.PaymentID = id
	return ev, true, nil
}
