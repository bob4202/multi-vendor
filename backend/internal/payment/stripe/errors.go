package stripe

import (
	"errors"
	"fmt"

	stripeapi "github.com/stripe/stripe-go/v87"

	"ecommerce/backend/internal/payment"
)

// The calls whose errors we map. The same Stripe code can mean different
// things for each, e.g. amount_too_large on a capture vs a refund.
const (
	opCapture = "capture"
	opVoid    = "void"
	opRefund  = "refund"
)

// stripeError turns an error from a Stripe call into what the jobs worker
// understands. Errors that can never succeed become payment's errors, so the
// worker gives up straight away; everything else (network, Stripe down, rate
// limits) stays a plain error, so the worker retries it.
//
// Stripe's own Error() text is the whole JSON response, PaymentIntent and
// client secret included, so only its code and message go in the text.
func stripeError(op string, err error) error {
	var se *stripeapi.Error
	if !errors.As(err, &se) {
		return fmt.Errorf("stripe: %s: %w", op, err)
	}
	if mapped := mapStripeError(op, se); mapped != nil {
		return fmt.Errorf("stripe: %s: %s: %w", op, se.Code, mapped)
	}
	return fmt.Errorf("stripe: %s: %s: %s", op, se.Code, se.Msg)
}

// mapStripeError picks our error for one Stripe error, or nil when it isn't
// one we know can never succeed.
func mapStripeError(op string, se *stripeapi.Error) error {
	switch op {
	case opCapture:
		switch se.Code {
		case stripeapi.ErrorCodeChargeAlreadyCaptured:
			return payment.ErrAlreadyCaptured
		case stripeapi.ErrorCodeChargeExpiredForCapture, // hold too old
			stripeapi.ErrorCodeAmountTooLarge,  // more than the hold
			stripeapi.ErrorCodeResourceMissing: // no such PaymentIntent on this account
			return payment.ErrNotCapturable
		case stripeapi.ErrorCodePaymentIntentUnexpectedState:
			if intentStatus(se) == stripeapi.PaymentIntentStatusSucceeded {
				return payment.ErrAlreadyCaptured
			}
			return payment.ErrNotCapturable // cancelled, or never held
		}

	case opVoid:
		switch se.Code {
		case stripeapi.ErrorCodeResourceMissing:
			return payment.ErrNotCapturable
		case stripeapi.ErrorCodePaymentIntentUnexpectedState:
			// Already cancelled is handled in Void (it counts as done).
			// Captured: the money is taken, there's no hold left to release.
			if intentStatus(se) == stripeapi.PaymentIntentStatusSucceeded {
				return payment.ErrAlreadyCaptured
			}
			return payment.ErrNotCapturable
		}

	case opRefund:
		switch se.Code {
		case stripeapi.ErrorCodeAmountTooLarge:
			return payment.ErrRefundTooLarge
		case stripeapi.ErrorCodeChargeAlreadyRefunded,
			stripeapi.ErrorCodeChargeDisputed, // the bank handles it now
			stripeapi.ErrorCodeRefundDisputedPayment,
			stripeapi.ErrorCodePaymentIntentUnexpectedState, // nothing captured
			stripeapi.ErrorCodeResourceMissing:
			return payment.ErrNotRefundable
		case "":
			// Stripe sends these two with no code (checked against test mode):
			// more than is left to refund names the amount param; refunding
			// a hold that was never captured is a bare invalid request.
			if se.Type == stripeapi.ErrorTypeInvalidRequest {
				if se.Param == "amount" {
					return payment.ErrRefundTooLarge
				}
				return payment.ErrNotRefundable
			}
		}
	}
	return nil
}

// alreadyCancelled reports whether a Stripe error says the PaymentIntent is
// cancelled already: for a void, the job is done (e.g. Stripe dropped an
// expired hold before our void ran).
func alreadyCancelled(err error) bool {
	var se *stripeapi.Error
	return errors.As(err, &se) &&
		se.Code == stripeapi.ErrorCodePaymentIntentUnexpectedState &&
		intentStatus(se) == stripeapi.PaymentIntentStatusCanceled
}

// intentStatus is the PaymentIntent's status as Stripe sent it with the
// error, or "" when the error carries none.
func intentStatus(se *stripeapi.Error) stripeapi.PaymentIntentStatus {
	if se.PaymentIntent == nil {
		return ""
	}
	return se.PaymentIntent.Status
}
