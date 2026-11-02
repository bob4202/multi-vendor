package payment

import "testing"

// What state.go has to provide for these tests to compile:
//
//	type Outcome int
//	const (
//		Changed     Outcome = iota // save the new status
//		AlreadyDone                // duplicate or late event: we're at or past that step
//		Invalid                    // the event can't apply to this payment: log loudly
//	)
//
//	type Amounts struct {
//		Captured int64 // what we captured
//		Refunded int64 // total refunded, including this event
//	}
//
//	func NextStatus(from PaymentKind, ev EventKind, amt Amounts) (PaymentKind, Outcome)
//
// When the outcome isn't Changed, NextStatus returns `from` unchanged.

func TestNextStatus(t *testing.T) {
	none := Amounts{}

	tests := []struct {
		name    string
		from    PaymentKind
		ev      EventKind
		amt     Amounts
		want    PaymentKind
		outcome Outcome
	}{
		// The normal path.
		{"customer paid", StatusCreated, EventAuthorised, none, StatusAuthorised, Changed},
		{"card declined", StatusCreated, EventFailed, none, StatusFailed, Changed},
		{"left the payment page", StatusCreated, EventExpired, none, StatusExpired, Changed},
		{"shop marked ready", StatusAuthorised, EventCaptured, none, StatusCaptured, Changed},
		{"shop rejected", StatusAuthorised, EventVoided, none, StatusVoided, Changed},
		{"capture retry worked", StatusCaptureFailed, EventCaptured, none, StatusCaptured, Changed},

		// Refunds depend on the amounts.
		{"part refund", StatusCaptured, EventRefunded, Amounts{Captured: 2450, Refunded: 500}, StatusPartiallyRefunded, Changed},
		{"full refund", StatusCaptured, EventRefunded, Amounts{Captured: 2450, Refunded: 2450}, StatusRefunded, Changed},
		{"second part refund", StatusPartiallyRefunded, EventRefunded, Amounts{Captured: 2450, Refunded: 1000}, StatusPartiallyRefunded, Changed},
		{"rest refunded", StatusPartiallyRefunded, EventRefunded, Amounts{Captured: 2450, Refunded: 2450}, StatusRefunded, Changed},

		// Duplicates: providers retry webhooks, so this is normal.
		{"authorised twice", StatusAuthorised, EventAuthorised, none, StatusAuthorised, AlreadyDone},
		{"failed twice", StatusFailed, EventFailed, none, StatusFailed, AlreadyDone},
		{"captured twice", StatusCaptured, EventCaptured, none, StatusCaptured, AlreadyDone},
		{"voided twice", StatusVoided, EventVoided, none, StatusVoided, AlreadyDone},
		{"fully refunded twice", StatusRefunded, EventRefunded, Amounts{Captured: 2450, Refunded: 2450}, StatusRefunded, AlreadyDone},

		// Late events: we've already moved past that step. Never go backwards.
		{"failed after authorised", StatusAuthorised, EventFailed, none, StatusAuthorised, AlreadyDone},
		{"expired after authorised", StatusAuthorised, EventExpired, none, StatusAuthorised, AlreadyDone},
		{"authorised after captured", StatusCaptured, EventAuthorised, none, StatusCaptured, AlreadyDone},
		{"expired twice", StatusExpired, EventExpired, none, StatusExpired, AlreadyDone},
		{"authorised after voided", StatusVoided, EventAuthorised, none, StatusVoided, AlreadyDone},
		{"authorised after capture failed", StatusCaptureFailed, EventAuthorised, none, StatusCaptureFailed, AlreadyDone},
		{"authorised after part refund", StatusPartiallyRefunded, EventAuthorised, none, StatusPartiallyRefunded, AlreadyDone},
		{"authorised after refund", StatusRefunded, EventAuthorised, none, StatusRefunded, AlreadyDone},
		{"failed after captured", StatusCaptured, EventFailed, none, StatusCaptured, AlreadyDone},
		{"failed after voided", StatusVoided, EventFailed, none, StatusVoided, AlreadyDone},
		{"failed after refund", StatusRefunded, EventFailed, none, StatusRefunded, AlreadyDone},
		{"failed after expired", StatusExpired, EventFailed, none, StatusExpired, AlreadyDone},
		{"expired after captured", StatusCaptured, EventExpired, none, StatusCaptured, AlreadyDone},
		{"expired after voided", StatusVoided, EventExpired, none, StatusVoided, AlreadyDone},
		{"expired after failed", StatusFailed, EventExpired, none, StatusFailed, AlreadyDone},
		{"captured after part refund", StatusPartiallyRefunded, EventCaptured, none, StatusPartiallyRefunded, AlreadyDone},
		{"captured after refund", StatusRefunded, EventCaptured, none, StatusRefunded, AlreadyDone},
		// Two part refunds whose events arrive in reverse: the older, smaller total lands last.
		{"older part refund after full refund", StatusRefunded, EventRefunded, Amounts{Captured: 2450, Refunded: 1000}, StatusRefunded, AlreadyDone},

		// Impossible: something is wrong, so the caller should log it loudly.
		{"refund before payment", StatusCreated, EventRefunded, Amounts{Captured: 0, Refunded: 500}, StatusCreated, Invalid},
		{"capture before payment", StatusCreated, EventCaptured, none, StatusCreated, Invalid},
		{"capture after void", StatusVoided, EventCaptured, none, StatusVoided, Invalid},
		{"void after capture", StatusCaptured, EventVoided, none, StatusCaptured, Invalid},
		{"refund of an uncaptured hold", StatusAuthorised, EventRefunded, Amounts{Captured: 0, Refunded: 500}, StatusAuthorised, Invalid},
		{"refund more than captured", StatusCaptured, EventRefunded, Amounts{Captured: 2450, Refunded: 3000}, StatusCaptured, Invalid},
		{"refund of nothing", StatusCaptured, EventRefunded, Amounts{Captured: 2450, Refunded: 0}, StatusCaptured, Invalid},
		{"refund of nothing after part refund", StatusPartiallyRefunded, EventRefunded, Amounts{Captured: 2450, Refunded: 0}, StatusPartiallyRefunded, Invalid},
		{"void after part refund", StatusPartiallyRefunded, EventVoided, none, StatusPartiallyRefunded, Invalid},
		{"void before payment", StatusCreated, EventVoided, none, StatusCreated, Invalid},
		{"capture after failed", StatusFailed, EventCaptured, none, StatusFailed, Invalid},
		{"capture after expired", StatusExpired, EventCaptured, none, StatusExpired, Invalid},
		// Money is held on a row we already gave up on (the customer may have
		// started a new one). Must not be silently ignored.
		{"authorised after failed", StatusFailed, EventAuthorised, none, StatusFailed, Invalid},
		{"authorised after expired", StatusExpired, EventAuthorised, none, StatusExpired, Invalid},
		{"unknown event", StatusAuthorised, EventKind("disputed"), none, StatusAuthorised, Invalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, outcome := NextStatus(tt.from, tt.ev, tt.amt)
			if got != tt.want || outcome != tt.outcome {
				t.Errorf("NextStatus(%s, %s, %+v) = (%s, %d), want (%s, %d)",
					tt.from, tt.ev, tt.amt, got, outcome, tt.want, tt.outcome)
			}
		})
	}
}

// The database only accepts these exact strings (payments_status_check in
// 0012_payments.sql), so a typo in a constant fails here instead of on the
// first real payment.
func TestStatusesMatchDatabase(t *testing.T) {
	want := map[PaymentKind]string{
		StatusCreated:           "created",
		StatusAuthorised:        "authorised",
		StatusFailed:            "failed",
		StatusExpired:           "expired",
		StatusCaptured:          "captured",
		StatusVoided:            "voided",
		StatusPartiallyRefunded: "partially_refunded",
		StatusRefunded:          "refunded",
		StatusCaptureFailed:     "capture_failed",
	} // two constants with the same value won't compile: duplicate map key
	for s, literal := range want {
		if string(s) != literal {
			t.Errorf("status %q should be %q", s, literal)
		}
	}
}
