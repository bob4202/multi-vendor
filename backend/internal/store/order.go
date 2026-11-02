package store

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type ItemInput struct {
	ProductID uuid.UUID
	Quantity  int
}

type PlaceOrderInput struct {
	BranchID       uuid.UUID
	IdempotencyKey string

	Items           []ItemInput
	FulfillmentType string // "collection" or "delivery"
	FulfillmentDate string

	CustomerName  string
	CustomerEmail string
	CustomerPhone string

	// Delivery address - only needed if it's a delivery.
	AddressLine1 string
	AddressLine2 string
	City         string
	Postcode     string

	Notes string

	// ExpectedTotal is the total (pence) the customer was shown at checkout.
	// Optional; if set and it doesn't match what the server works out, the
	// order is rejected with "price_changed" instead of charging an amount
	// the customer never saw.
	ExpectedTotal *int64

	// TermsVersion is the version of the terms the customer ticked at
	// checkout (substitutions, collection window, no-show policy). Required.
	TermsVersion string
	// AllowSubstitutions: may the shop offer a similar product when one is
	// out of stock? If not, unavailable items are simply removed.
	AllowSubstitutions bool
	// CustomerAccountID is set when a signed-in customer places the order.
	CustomerAccountID *uuid.UUID
}

// Problem is one thing wrong with an order, shown to the customer (422).
type Problem struct {
	Code      string     // machine-readable, e.g. "required", "unavailable"
	Field     string     // which input it's about, e.g. "customer_name" (may be empty)
	ProductID *uuid.UUID // which basket line it's about (item problems only)
	Message   string     // what to tell the customer
}

func checkInput(in PlaceOrderInput) []Problem {
	var problems []Problem

	//item related checks
	if len(in.Items) == 0 {
		problems = append(problems, Problem{Code: "required", Field: "items", Message: "Your basket is empty."})
	}
	for _, item := range in.Items {
		if item.Quantity < 1 || item.Quantity > 99 {
			problems = append(problems, Problem{Code: "invalid_quantity", Field: "quantity", ProductID: &item.ProductID, Message: "Quantity must be between 1 and 99."})
		}
	}

	// Customer Information Check
	if strings.TrimSpace(in.CustomerName) == "" {
		problems = append(problems, Problem{Code: "required", Field: "customer_name", Message: "Please enter your name."})
	}
	if strings.TrimSpace(in.CustomerEmail) == "" {
		problems = append(problems, Problem{Code: "required", Field: "customer_email", Message: "Please enter your email."})
	}

	if strings.TrimSpace(in.CustomerPhone) == "" {
		problems = append(problems, Problem{Code: "required", Field: "customer_phone", Message: "Please enter your phone number."})
	} else if _, ok := NormalizePhone(in.CustomerPhone); !ok {
		problems = append(problems, Problem{Code: "invalid", Field: "customer_phone", Message: "Please enter a valid UK phone number, e.g. 07700 900123."})
	}
	if email := strings.TrimSpace(in.CustomerEmail); email != "" && !looksLikeEmail(email) {
		problems = append(problems, Problem{Code: "invalid", Field: "customer_email", Message: "Please enter a valid email address."})
	}

	if strings.TrimSpace(in.TermsVersion) == "" {
		problems = append(problems, Problem{Code: "required", Field: "terms", Message: "Please accept the terms to place your order."})
	}

	//fulfillment type check
	if in.FulfillmentType != "collection" && in.FulfillmentType != "delivery" {
		problems = append(problems, Problem{Code: "invalid", Field: "fulfillment_type", Message: "Choose collection or delivery."})
	}

	if in.FulfillmentType == "delivery" {
		if strings.TrimSpace(in.AddressLine1) == "" {
			problems = append(problems, Problem{Code: "required", Field: "address_line_1", Message: "Please enter your address."})
		}
		if strings.TrimSpace(in.City) == "" {
			problems = append(problems, Problem{Code: "required", Field: "address_city", Message: "Please enter your town or city."})
		}
		if strings.TrimSpace(in.Postcode) == "" {
			problems = append(problems, Problem{Code: "required", Field: "postcode", Message: "Please enter your postcode."})
		}
	}

	if strings.TrimSpace(in.FulfillmentDate) == "" || !validateDate(in.FulfillmentDate) {
		problems = append(problems, Problem{Code: "invalid", Field: "fulfillment_date", Message: "Choose a valid date."})
	}

	return problems
}

// mergeDuplicateItems combines basket lines for the same product into one
// line, adding their quantities: chicken x2 + chicken x3 -> chicken x5.
// The database allows only one line per product per order, so this must run
// before saving.
//
// Call it BEFORE checkInput, so the 1-99 quantity check sees the combined
// amount (chicken x60 + chicken x50 = 110 must be rejected, even though each
// line alone was fine).
//
// Lines keep the order each product first appeared in the basket, so the
// receipt matches what the customer added.
func mergeDuplicateItems(items []ItemInput) []ItemInput {
	totals := make(map[uuid.UUID]int) // product -> quantity so far
	var order []uuid.UUID             // products in first-seen order

	for _, item := range items {
		if _, seen := totals[item.ProductID]; !seen {
			order = append(order, item.ProductID)
		}
		totals[item.ProductID] += item.Quantity
	}

	merged := make([]ItemInput, 0, len(order))
	for _, id := range order {
		merged = append(merged, ItemInput{ProductID: id, Quantity: totals[id]})
	}
	return merged
}

func validateDate(dateStr string) bool {
	const layout = "2006-01-02"
	_, err := time.Parse(layout, dateStr)

	return err == nil
}

// NormalizePhone turns what people type into E.164: UK numbers in national
// form ("07700 900123", "0044 7700...", "44 7700...") become +44..., and an
// international number is kept if it starts with +. ok is false for
// anything that isn't a plausible phone number.
func NormalizePhone(raw string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
			// separators people type
		default:
			return "", false
		}
	}
	n := b.String()
	switch {
	case strings.HasPrefix(n, "+"):
	case strings.HasPrefix(n, "00"):
		n = "+" + n[2:]
	case strings.HasPrefix(n, "0"):
		n = "+44" + n[1:]
	case strings.HasPrefix(n, "44") && len(n) == 12:
		n = "+" + n
	default:
		return "", false
	}
	// "+44 (0)7700..." - the bracketed 0 isn't dialled from abroad.
	if strings.HasPrefix(n, "+440") {
		n = "+44" + n[4:]
	}
	if !ValidatePhoneNumber(n) {
		return "", false
	}
	// UK numbers are 10 digits after the country code.
	if strings.HasPrefix(n, "+44") && len(n) != 13 {
		return "", false
	}
	return n, true
}

// looksLikeEmail is a shape check, not full RFC validation: something@domain.tld.
func looksLikeEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	if at < 1 || at == len(s)-1 || strings.ContainsAny(s, " \t\n") {
		return false
	}
	return strings.Contains(s[at+1:], ".")
}
