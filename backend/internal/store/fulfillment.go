package store

import (
	"ecommerce/backend/internal/db"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	ErrPostcodeTooShort          = errors.New("postcode too short")
	ErrPostcodeInvalid           = errors.New("invalid postcode")
	ErrPostcodeOutOfDeliveryArea = errors.New("postcode out of delivery area")
)

// isOpenOn reports whether the branch is open on date. An exception for that
// exact date wins over the weekly schedule; otherwise any weekly row for
// date's ISO weekday means open.
//
// date must already be in the branch's timezone (the caller converts with
// t.In(loc)) - its calendar day is what's checked.
func isOpenOn(date time.Time, hours []db.BranchOpeningHour, exceptions []db.BranchHoursException) bool {
	year, month, day := date.Date()

	for _, exception := range exceptions {
		// A DATE column arrives as midnight UTC. Read its calendar day as-is:
		// converting it to another zone could shift it to the previous day.
		excYear, excMonth, excDay := exception.Date.Time.Date()
		if excYear == year && excMonth == month && excDay == day {
			return !exception.Closed
		}
	}

	weekday := int16(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	for _, hour := range hours {
		if hour.DayOfWeek == weekday {
			return true
		}
	}

	return false
}

// isDateAllowed reports whether date is a valid order date: not before today
// and at most 14 days ahead, both measured in date's timezone - so date must
// already be in the branch's timezone. now is passed in rather than read from
// the clock so tests can control it.
func isDateAllowed(date, now time.Time) bool {
	loc := date.Location()

	today := truncateToDate(now.In(loc))
	requested := truncateToDate(date)

	if requested.Before(today) {
		return false
	}

	latest := today.AddDate(0, 0, 14)
	return !requested.After(latest)
}

// truncateToDate drops the time-of-day, keeping only year/month/day in t's
// own location, so two times on the same calendar day compare as equal.
func truncateToDate(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func calculateDeliveryFee(subtotal int64, deliveryFeeOver *int64, deliveryFee int64) int64 {
	if deliveryFeeOver == nil {
		return deliveryFee
	}

	if deliveryFee == 0 {
		return 0
	}

	if subtotal < *deliveryFeeOver {
		return deliveryFee
	}

	return 0
}

func minOrderCheck(subtotal, deliveryMinOrder int64) bool {
	if subtotal == 0 {
		return false
	}
	return subtotal >= deliveryMinOrder
}

type DeliveryArea []string

func checkDeliveryArea(postcode string, areas DeliveryArea) error {
	postcode = strings.TrimSpace(postcode)
	if err := validatePostCode(postcode); err != nil {
		return err
	}

	outward := outwardCode(postcode)
	if !slices.Contains(areas, outward) {
		return ErrPostcodeOutOfDeliveryArea
	}

	return nil
}

// outwardCode returns the outward part of a UK postcode, e.g. "se153NW" -> "SE15".
// The postcode must already have been validated: it panics on input shorter
// than 3 characters.
func outwardCode(postcode string) string {
	compact := strings.ReplaceAll(strings.ToUpper(postcode), " ", "")
	return compact[:len(compact)-3]
}

// Full UK postcode: outward + optional space + inward (digit + two letters).
var postcodeRe = regexp.MustCompile(
	`(?i)^(GIR ?0AA|[A-PR-UWYZ]([0-9]{1,2}|[A-HK-Y][0-9]([0-9ABEHMNPRV-Y])?|[0-9][A-HJKPS-UW]) ?[0-9][ABD-HJLNP-UW-Z]{2})$`,
)

func validatePostCode(postcode string) error {
	postcode = strings.TrimSpace(postcode)

	// Shortest valid postcode is 5 chars without a space, e.g. "M11AA".
	if len(postcode) < 5 {
		return ErrPostcodeTooShort
	}
	if !postcodeRe.MatchString(postcode) {
		return ErrPostcodeInvalid
	}
	return nil
}

var e164Regex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

func ValidatePhoneNumber(phone string) bool {
	return e164Regex.MatchString(phone)

}
