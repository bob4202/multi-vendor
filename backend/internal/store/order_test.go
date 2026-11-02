package store

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

// validInput is an order with nothing wrong with it. Each test case copies
// it and breaks exactly one thing, so a failure points at one check.
func validInput() PlaceOrderInput {
	return PlaceOrderInput{
		BranchID:       uuid.New(),
		IdempotencyKey: "key-1",
		Items: []ItemInput{
			{ProductID: uuid.New(), Quantity: 2},
		},
		FulfillmentType: "collection",
		FulfillmentDate: "2026-10-12",
		CustomerName:    "Amina Khan",
		CustomerEmail:   "amina@example.com",
		CustomerPhone:   "07700 900123",
		TermsVersion:    "2026-10-14",
	}
}

func TestCheckInput(t *testing.T) {
	tests := []struct {
		name string
		// change breaks one thing in a valid order.
		change func(in *PlaceOrderInput)
		// wantFields is the Field of every problem expected, in order.
		// Empty means the order should pass.
		wantFields []string
	}{
		// Good orders - no problems.
		{"valid collection", func(in *PlaceOrderInput) {}, nil},
		{"valid delivery", func(in *PlaceOrderInput) {
			in.FulfillmentType = "delivery"
			in.AddressLine1 = "12 Broadway"
			in.City = "London"
			in.Postcode = "E15 1NG"
		}, nil},
		{"quantity 1 is allowed", func(in *PlaceOrderInput) { in.Items[0].Quantity = 1 }, nil},
		{"quantity 99 is allowed", func(in *PlaceOrderInput) { in.Items[0].Quantity = 99 }, nil},
		{"collection doesn't need a postcode", func(in *PlaceOrderInput) { in.Postcode = "" }, nil},

		// Basket.
		{"empty basket", func(in *PlaceOrderInput) { in.Items = nil }, []string{"items"}},
		{"quantity 0", func(in *PlaceOrderInput) { in.Items[0].Quantity = 0 }, []string{"quantity"}},
		{"quantity 100", func(in *PlaceOrderInput) { in.Items[0].Quantity = 100 }, []string{"quantity"}},
		{"negative quantity", func(in *PlaceOrderInput) { in.Items[0].Quantity = -1 }, []string{"quantity"}},

		// Fulfilment type.
		{"empty fulfilment type", func(in *PlaceOrderInput) { in.FulfillmentType = "" }, []string{"fulfillment_type"}},
		{"unknown fulfilment type", func(in *PlaceOrderInput) { in.FulfillmentType = "pickup" }, []string{"fulfillment_type"}},

		// Terms and phone.
		{"terms not accepted", func(in *PlaceOrderInput) { in.TermsVersion = "" }, []string{"terms"}},
		{"phone is not a number", func(in *PlaceOrderInput) { in.CustomerPhone = "call me" }, []string{"customer_phone"}},
		{"email without a domain", func(in *PlaceOrderInput) { in.CustomerEmail = "amina@" }, []string{"customer_email"}},

		// Customer details.
		{"missing name", func(in *PlaceOrderInput) { in.CustomerName = "" }, []string{"customer_name"}},
		{"name is only spaces", func(in *PlaceOrderInput) { in.CustomerName = "   " }, []string{"customer_name"}},
		{"missing email", func(in *PlaceOrderInput) { in.CustomerEmail = "" }, []string{"customer_email"}},
		{"missing phone", func(in *PlaceOrderInput) { in.CustomerPhone = "" }, []string{"customer_phone"}},

		// Postcode - only required for delivery.
		{"delivery without postcode", func(in *PlaceOrderInput) {
			in.FulfillmentType = "delivery"
			in.AddressLine1 = "12 Broadway"
			in.City = "London"
			in.Postcode = ""
		}, []string{"postcode"}},
		{"delivery with spaces-only postcode", func(in *PlaceOrderInput) {
			in.FulfillmentType = "delivery"
			in.AddressLine1 = "12 Broadway"
			in.City = "London"
			in.Postcode = "  "
		}, []string{"postcode"}},
		{"delivery without address or city", func(in *PlaceOrderInput) {
			in.FulfillmentType = "delivery"
			in.Postcode = "E15 1NG"
		}, []string{"address_line_1", "address_city"}},

		// Fulfilment date - must be a real calendar date written as YYYY-MM-DD.
		// (Whether it's in the past or too far ahead is checked later, with
		// the branch's timezone - not here.)
		{"missing date", func(in *PlaceOrderInput) { in.FulfillmentDate = "" }, []string{"fulfillment_date"}},
		{"date is not a date", func(in *PlaceOrderInput) { in.FulfillmentDate = "banana" }, []string{"fulfillment_date"}},
		{"date in UK format", func(in *PlaceOrderInput) { in.FulfillmentDate = "12/10/2026" }, []string{"fulfillment_date"}},

		// Several problems at once - all of them come back, not just the first.
		{"no name and no phone", func(in *PlaceOrderInput) {
			in.CustomerName = ""
			in.CustomerPhone = ""
		}, []string{"customer_name", "customer_phone"}},
		{"everything wrong", func(in *PlaceOrderInput) {
			in.Items = nil
			in.FulfillmentType = "delivery"
			in.CustomerName = ""
			in.CustomerEmail = ""
			in.CustomerPhone = ""
			in.Postcode = ""
		}, []string{"items", "customer_name", "customer_email", "customer_phone", "address_line_1", "address_city", "postcode"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.change(&in)

			var gotFields []string
			for _, p := range checkInput(in) {
				gotFields = append(gotFields, p.Field)
			}

			if !slices.Equal(gotFields, tt.wantFields) {
				t.Errorf("problem fields = %v, want %v", gotFields, tt.wantFields)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name string
		date string
		want bool
	}{
		// Valid: YYYY-MM-DD, a day that exists.
		{"normal date", "2026-10-12", true},
		{"first of the year", "2027-01-01", true},
		{"leap day in a leap year", "2028-02-29", true},

		// Not a date at all.
		{"empty", "", false},
		{"text", "banana", false},
		{"only spaces", "   ", false},

		// Right shape, but the day doesn't exist.
		{"month 13", "2026-13-01", false},
		{"day 32", "2026-10-32", false},
		{"30 February", "2026-02-30", false},
		{"leap day in a normal year", "2026-02-29", false},

		// A real date, but written the wrong way.
		{"UK format", "12/10/2026", false},
		{"no leading zeros", "2026-1-5", false},
		{"with a time", "2026-10-12T10:00:00Z", false},
		{"space in front", " 2026-10-12", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateDate(tt.date); got != tt.want {
				t.Errorf("validateDate(%q) = %t, want %t", tt.date, got, tt.want)
			}
		})
	}
}

func TestMergeDuplicateItems(t *testing.T) {
	chicken, lamb, beef := uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name  string
		items []ItemInput
		want  []ItemInput
	}{
		{"empty basket", nil, []ItemInput{}},
		{"no duplicates - unchanged",
			[]ItemInput{{chicken, 2}, {lamb, 1}},
			[]ItemInput{{chicken, 2}, {lamb, 1}}},
		{"same product twice",
			[]ItemInput{{chicken, 2}, {lamb, 1}, {chicken, 3}},
			[]ItemInput{{chicken, 5}, {lamb, 1}}},
		{"same product three times",
			[]ItemInput{{beef, 1}, {beef, 1}, {beef, 1}},
			[]ItemInput{{beef, 3}}},
		{"keeps first-seen order",
			[]ItemInput{{lamb, 1}, {chicken, 1}, {beef, 1}, {lamb, 1}},
			[]ItemInput{{lamb, 2}, {chicken, 1}, {beef, 1}}},
		// Merging can push a product over 99 - that's on purpose: checkInput
		// runs afterwards and rejects it.
		{"combined total can exceed 99",
			[]ItemInput{{chicken, 60}, {chicken, 50}},
			[]ItemInput{{chicken, 110}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeDuplicateItems(tt.items)
			if !slices.Equal(got, tt.want) {
				t.Errorf("mergeDuplicateItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The order check above must hold every time, not just by luck - Go
// shuffles map order, so run it many times.
func TestMergeDuplicateItemsOrderIsStable(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	items := []ItemInput{{a, 1}, {b, 1}, {c, 1}, {d, 1}, {a, 1}}
	want := []ItemInput{{a, 2}, {b, 1}, {c, 1}, {d, 1}}

	for range 50 {
		if got := mergeDuplicateItems(items); !slices.Equal(got, want) {
			t.Fatalf("order changed between runs: got %v, want %v", got, want)
		}
	}
}
