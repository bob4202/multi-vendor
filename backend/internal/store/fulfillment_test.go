package store

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

// ptr returns a pointer to v - for the nullable free-delivery threshold
// (db.BranchInfo.DeliveryFreeOver is *int64: nil = no free-delivery offer).
func ptr(v int64) *int64 { return &v }

func TestCalculateDeliveryFee(t *testing.T) {
	tests := []struct {
		name     string
		subtotal int64
		freeOver *int64
		fee      int64
		want     int64
	}{
		// No free-delivery offer (NULL in the DB): always charged, however
		// big the basket - the case a plain int64 couldn't express.
		{"no threshold, small basket", 999, nil, 300, 300},
		{"no threshold, huge basket", 1_000_000, nil, 300, 300},
		{"no threshold, zero fee", 999, nil, 0, 0},

		// Fee charged: subtotal is below the free-delivery threshold.
		{"below threshold", 999, ptr(1000), 300, 300},
		{"zero subtotal", 0, ptr(1000), 300, 300},
		{"one below threshold", 999, ptr(1000), 1, 1},

		// Free: subtotal meets or exceeds the threshold ("free on orders of
		// £X or more" - reaching it exactly counts).
		{"exactly at threshold", 1000, ptr(1000), 300, 0},
		{"above threshold", 1500, ptr(1000), 300, 0},

		// Free: no fee configured, or a threshold of 0 (free for everyone).
		{"zero fee", 500, ptr(1000), 0, 0},
		{"zero threshold", 500, ptr(0), 300, 0},
		{"both zero", 500, ptr(0), 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateDeliveryFee(tt.subtotal, tt.freeOver, tt.fee); got != tt.want {
				t.Errorf("calculateDeliveryFee(%d, %s, %d) = %d, want %d",
					tt.subtotal, fmtPtr(tt.freeOver), tt.fee, got, tt.want)
			}
		})
	}
}

// fmtPtr prints a *int64 as its value or "nil", so a failing case reads
// "calculateDeliveryFee(999, nil, 300)" rather than a memory address.
func fmtPtr(p *int64) string {
	if p == nil {
		return "nil"
	}
	return strconv.FormatInt(*p, 10)
}

func TestDeliveryMinOrder(t *testing.T) {
	tests := []struct {
		name     string
		subtotal int64
		minOrder int64
		want     bool
	}{
		// minOrder meets: subtotal is higher or equal to the minOrder
		{"subtotal higher", 110, 100, true},
		{"subtotal equal", 100, 100, true},

		//minOrder doesn't meet:
		{"min order lower", 40, 100, false},
		{"no min order", 0, 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := minOrderCheck(tt.subtotal, tt.minOrder); got != tt.want {
				t.Errorf("minOrderCheck(%d, %d) = %t, want %t", tt.subtotal, tt.minOrder, got, tt.want)
			}
		})
	}
}

func TestCheckDeliveryArea(t *testing.T) {
	areas := DeliveryArea{"E1", "SE15", "W1A", "EC1A", "GIR"}

	tests := []struct {
		name     string
		postcode string
		areas    DeliveryArea
		wantErr  error
	}{
		{"empty", "", areas, ErrPostcodeTooShort},
		{"single letter", "E", areas, ErrPostcodeTooShort},
		{"outward only", "E1", areas, ErrPostcodeTooShort},
		{"digits only", "12345", areas, ErrPostcodeInvalid},
		{"bad format", "E1234", areas, ErrPostcodeInvalid},
		{"invalid second letter J", "AJ1 1AA", areas, ErrPostcodeInvalid},

		{"valid with space", "E1 6AN", areas, nil},
		{"valid no space", "E16AN", areas, nil},
		{"valid lowercase", "se15 4ab", areas, nil},
		{"valid A9A outward", "W1A 1AA", areas, nil},
		{"valid AA9A outward", "EC1A 1BB", areas, nil},
		{"GIR special case", "GIR 0AA", areas, nil},

		{"out of delivery area", "E2 6AA", DeliveryArea{"E1"}, ErrPostcodeOutOfDeliveryArea},
		{"nil area list", "E1 6AN", nil, ErrPostcodeOutOfDeliveryArea},
		{"empty area list", "E1 6AN", DeliveryArea{}, ErrPostcodeOutOfDeliveryArea},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDeliveryArea(tt.postcode, tt.areas)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("checkDeliveryArea(%q, %v) = %v, want %v", tt.postcode, tt.areas, err, tt.wantErr)
			}
		})
	}
}
func TestValidatePostCode(t *testing.T) {
	tests := []struct {
		name     string
		postcode string
		wantErr  error
	}{
		{"empty", "", ErrPostcodeTooShort},
		{"single letter", "E", ErrPostcodeTooShort},
		{"outward only", "E1", ErrPostcodeTooShort},
		{"digits only", "12345", ErrPostcodeInvalid},
		{"bad format", "E1234", ErrPostcodeInvalid},
		{"valid with space", "E1 6AN", nil},
		{"valid no space", "E16AN", nil},
		{"valid lowercase", "se15 4ab", nil},
		{"valid A9A outward", "W1A 1AA", nil},
		{"valid AA9A outward", "EC1A 1BB", nil},
		{"GIR special case", "GIR 0AA", nil},
		{"invalid second letter J", "AJ1 1AA", ErrPostcodeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePostCode(tt.postcode)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("validatePostCode(%q) = %v, want %v", tt.postcode, err, tt.wantErr)
			}
		})
	}
}

// Fixtures for the date rules. 5 Oct 2026 is a Monday.
var london = mustLoadLocation("Europe/London")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// pgDate builds a DATE value the way pgx returns one: midnight UTC.
func pgDate(year int, month time.Month, day int) pgtype.Date {
	return pgtype.Date{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Valid: true}
}

func TestIsOpenOn(t *testing.T) {
	// Open Monday-Saturday, closed Sundays (no row for day 7).
	weekly := []db.BranchOpeningHour{
		{DayOfWeek: 1}, {DayOfWeek: 2}, {DayOfWeek: 3},
		{DayOfWeek: 4}, {DayOfWeek: 5}, {DayOfWeek: 6},
	}
	christmasClosed := []db.BranchHoursException{
		{Date: pgDate(2026, 12, 25), Closed: true},
	}
	sundaySpecialHours := []db.BranchHoursException{
		{Date: pgDate(2026, 10, 11), Closed: false},
	}

	tests := []struct {
		name       string
		date       time.Time
		exceptions []db.BranchHoursException
		want       bool
	}{
		{"normal Monday", time.Date(2026, 10, 5, 10, 0, 0, 0, london), nil, true},
		{"Saturday", time.Date(2026, 10, 10, 10, 0, 0, 0, london), nil, true},
		{"Sunday with no weekly rows", time.Date(2026, 10, 11, 10, 0, 0, 0, london), nil, false},
		{"Christmas closed (a Friday)", time.Date(2026, 12, 25, 10, 0, 0, 0, london), christmasClosed, false},
		{"day after Christmas unaffected", time.Date(2026, 12, 26, 10, 0, 0, 0, london), christmasClosed, true},
		{"special hours on a normally-closed Sunday", time.Date(2026, 10, 11, 10, 0, 0, 0, london), sundaySpecialHours, true},
		// Late evening in BST: the exception's midnight-UTC date must still
		// match the local calendar day.
		{"closed exception matched late evening", time.Date(2026, 12, 25, 23, 30, 0, 0, london), christmasClosed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOpenOn(tt.date, weekly, tt.exceptions); got != tt.want {
				t.Errorf("isOpenOn(%s) = %t, want %t", tt.date.Format("Mon 2006-01-02 15:04"), got, tt.want)
			}
		})
	}
}

func TestIsDateAllowed(t *testing.T) {
	// "Now" is Monday 5 Oct 2026, 10:00 in London.
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, london)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, london) }

	tests := []struct {
		name string
		date time.Time
		now  time.Time
		want bool
	}{
		{"yesterday", day(4), now, false},
		{"today", day(5), now, true},
		{"tomorrow", day(6), now, true},
		{"day 14", day(19), now, true},
		{"day 15", day(20), now, false},
		// 23:30 UTC on 5 Oct is already 00:30 on 6 Oct in London (BST), so
		// "today" for the branch is the 6th - ordering for the 5th must fail.
		{"server clock still on previous day", day(5), time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC), false},
		{"server clock previous day, branch today", day(6), time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDateAllowed(tt.date, tt.now); got != tt.want {
				t.Errorf("isDateAllowed(%s, now=%s) = %t, want %t",
					tt.date.Format("2006-01-02"), tt.now.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}
