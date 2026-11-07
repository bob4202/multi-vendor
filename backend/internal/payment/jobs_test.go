package payment

import (
	"errors"
	"fmt"
	"testing"
)

func TestGiveUp(t *testing.T) {
	network := errors.New("connection reset")
	tests := []struct {
		name     string
		attempts int32
		err      error
		want     bool
	}{
		{"first network failure retries", 1, network, false},
		{"last retry still allowed", 3, network, false},
		{"out of retries", 4, network, true},
		{"provider doesn't know the payment", 1, fmt.Errorf("fake void x: %w", ErrNotCapturable), true},
		{"money already taken", 1, fmt.Errorf("void: %w", ErrAlreadyCaptured), true},
		{"no provider reference", 1, errNoPaymentRef, true},
		{"capture job with no valid amount", 1, errInvalidAmount, true},
		{"refund too large", 1, fmt.Errorf("fake refund x: %w", ErrRefundTooLarge), true},
		{"not refundable", 1, fmt.Errorf("fake refund x: %w", ErrNotRefundable), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := giveUp(tt.attempts, tt.err); got != tt.want {
				t.Errorf("giveUp(%d, %v) = %v, want %v", tt.attempts, tt.err, got, tt.want)
			}
		})
	}
}

// Every retry before giving up has a delay to wait.
func TestRetryDelaysCoverAllRetries(t *testing.T) {
	network := errors.New("timeout")
	for attempts := int32(1); !giveUp(attempts, network); attempts++ {
		if int(attempts-1) >= len(retryDelays) {
			t.Fatalf("attempt %d retries but has no delay", attempts)
		}
	}
}
