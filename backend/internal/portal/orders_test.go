package portal

import "testing"

func TestBlocksHandover(t *testing.T) {
	tests := []struct {
		newStatus, paymentStatus string
		want                     bool
	}{
		// The money couldn't be taken: the shopping stays in the shop.
		{StatusCompleted, "capture_failed", true},
		{StatusOutForDelivery, "capture_failed", true},

		// A no-show closes the order without handing anything over.
		{StatusNoShow, "capture_failed", false},

		// Paid, or the capture is still on its way: hand over as normal.
		{StatusCompleted, "captured", false},
		{StatusCompleted, "authorised", false},
		{StatusOutForDelivery, "partially_refunded", false},
	}
	for _, tt := range tests {
		if got := blocksHandover(tt.newStatus, tt.paymentStatus); got != tt.want {
			t.Errorf("blocksHandover(%q, %q) = %v, want %v", tt.newStatus, tt.paymentStatus, got, tt.want)
		}
	}
}
