package portal

import (
	"slices"
	"testing"

	"ecommerce/backend/internal/db"
)

// A ready order can only be cancelled after the charge failed; every other
// status keeps its normal list.
func TestNextStatusesFor(t *testing.T) {
	tests := []struct {
		name               string
		status, fulfilment string
		awaitingCustomer   bool
		paymentStatus      string
		want               []string
	}{
		{"ready, charged", StatusReady, "delivery", false, "captured", []string{StatusOutForDelivery}},
		{"ready, no payment (old order)", StatusReady, "delivery", false, "", []string{StatusOutForDelivery}},
		{"picking, held", StatusPicking, "collection", false, "authorised", []string{StatusReady, StatusCancelled}},
		{"picking, waiting on a substitution", StatusPicking, "collection", true, "authorised", []string{StatusCancelled}},
		{"ready, charge failed", StatusReady, "delivery", false, "capture_failed", []string{StatusCancelled}},
		{"ready collection, charge failed", StatusReady, "collection", false, "capture_failed", []string{StatusNoShow, StatusCancelled}},
	}
	for _, tt := range tests {
		o := db.Order{Status: tt.status, FulfilmentType: tt.fulfilment}
		got := NextStatusesFor(o, tt.awaitingCustomer, tt.paymentStatus)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: NextStatusesFor = %v, want %v", tt.name, got, tt.want)
		}
	}
}

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
