package payment

type Outcome int

const (
	Changed Outcome = iota
	AlreadyDone
	Invalid
)

type Amounts struct {
	Captured int64
	Refunded int64
}

func NextStatus(from PaymentKind, ev EventKind, amount Amounts) (PaymentKind, Outcome) {

	switch ev {

	// Authorised Events
	case EventAuthorised:
		switch from {
		case StatusCreated:
			return StatusAuthorised, Changed
		case StatusFailed, StatusExpired:
			return from, Invalid
		default:
			return from, AlreadyDone
		}

	// Failure Events
	case EventFailed:
		switch from {
		case StatusCreated:
			return StatusFailed, Changed
		default:
			return from, AlreadyDone
		}

	// Expire Events
	case EventExpired:
		switch from {
		case StatusCreated:
			return StatusExpired, Changed
		default:
			return from, AlreadyDone

		}

	// Capture Events
	case EventCaptured:
		switch from {
		case StatusAuthorised, StatusCaptureFailed:
			return StatusCaptured, Changed
		case StatusCreated, StatusVoided, StatusFailed, StatusExpired:
			return from, Invalid
		default:
			return from, AlreadyDone
		}

	// Void Events
	case EventVoided:
		switch from {
		case StatusAuthorised:
			return StatusVoided, Changed
		case StatusVoided:
			return StatusVoided, AlreadyDone
		}

	// Refund Events
	case EventRefunded:
		switch from {
		case StatusCaptured, StatusPartiallyRefunded:
			if amount.Refunded <= 0 {
				return from, Invalid
			}
			if amount.Refunded < amount.Captured {
				return StatusPartiallyRefunded, Changed
			}
			if amount.Refunded == amount.Captured {
				return StatusRefunded, Changed
			}
			return from, Invalid
		case StatusRefunded:
			return StatusRefunded, AlreadyDone
		}
	}

	return from, Invalid
}
