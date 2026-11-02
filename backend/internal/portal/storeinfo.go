package portal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

// StoreInfo is a branch's editable storefront details. Field names and
// nesting match GET /v1/store/branch so both sides share one mental model.
// Money is in pence.
type StoreInfo struct {
	Phone        string        `json:"phone"`
	Email        *string       `json:"email"`
	Address      Address       `json:"address"`
	Fulfilment   Fulfilment    `json:"fulfilment"`
	OpeningHours []OpeningHour `json:"opening_hours"`
}

type Address struct {
	Line1    *string `json:"line1"`
	Line2    *string `json:"line2"`
	City     *string `json:"city"`
	Postcode *string `json:"postcode"`
}

type Fulfilment struct {
	Collection struct {
		Enabled bool `json:"enabled"`
		// Collection orders close this many minutes before the day's
		// last closing time.
		LastOrderMinutes int32 `json:"last_order_minutes"`
	} `json:"collection"`
	Delivery Delivery `json:"delivery"`
}

type Delivery struct {
	Enabled          bool     `json:"enabled"`
	Fee              int64    `json:"fee"`
	FreeOver         *int64   `json:"free_over"` // nil = no free-delivery threshold
	MinOrder         int64    `json:"min_order"`
	PostcodePrefixes []string `json:"postcode_prefixes"`
}

// OpeningHour is one open period; a day can have several (split hours).
type OpeningHour struct {
	Day   int16  `json:"day"`   // ISO: 1 = Monday ... 7 = Sunday
	Open  string `json:"open"`  // "HH:MM"
	Close string `json:"close"` // "HH:MM"
}

type Closure struct {
	ID     int32   `json:"id"`
	Date   string  `json:"date"` // YYYY-MM-DD
	Closed bool    `json:"closed"`
	Open   *string `json:"open"`
	Close  *string `json:"close"`
	Note   *string `json:"note"`
}

// Issue is one field-level validation problem.
type Issue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d validation issue(s)", len(e.Issues))
}

var ErrClosureExists = errors.New("there is already an entry for that date")

const defaultTimezone = "Europe/London"

// DefaultLastOrderMinutes matches the column default in 0011_customer_side.sql.
const DefaultLastOrderMinutes = 90

// GetStoreInfo returns the branch's details, or an empty StoreInfo if none
// have been saved yet (so the portal can show a blank form, not an error).
func (s *Service) GetStoreInfo(ctx context.Context, branchID uuid.UUID) (StoreInfo, error) {
	info, err := s.queries.GetBranchInfo(ctx, branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		empty := StoreInfo{OpeningHours: []OpeningHour{}, Fulfilment: Fulfilment{Delivery: Delivery{PostcodePrefixes: []string{}}}}
		empty.Fulfilment.Collection.LastOrderMinutes = DefaultLastOrderMinutes
		return empty, nil
	}
	if err != nil {
		return StoreInfo{}, fmt.Errorf("load branch info: %w", err)
	}
	hours, err := s.queries.ListBranchOpeningHours(ctx, branchID)
	if err != nil {
		return StoreInfo{}, fmt.Errorf("load opening hours: %w", err)
	}

	out := StoreInfo{
		Phone: info.Phone,
		Email: info.Email,
		Address: Address{
			Line1: info.AddressLine1, Line2: info.AddressLine2,
			City: info.City, Postcode: info.PostalCode,
		},
		OpeningHours: make([]OpeningHour, 0, len(hours)),
	}
	out.Fulfilment.Collection.Enabled = info.Collection
	out.Fulfilment.Collection.LastOrderMinutes = info.LastOrderMinutes
	out.Fulfilment.Delivery = Delivery{
		Enabled:          info.Delivery,
		Fee:              info.DeliveryFee,
		FreeOver:         info.DeliveryFreeOver,
		MinOrder:         info.DeliveryMinOrder,
		PostcodePrefixes: append([]string{}, info.DeliveryPostcodePrefixes...),
	}
	for _, h := range hours {
		out.OpeningHours = append(out.OpeningHours, OpeningHour{
			Day: h.DayOfWeek, Open: formatClock(h.OpenTime), Close: formatClock(h.CloseTime),
		})
	}
	return out, nil
}

// SaveStoreInfo validates and stores the branch's details, replacing its
// weekly schedule wholesale, in one transaction.
func (s *Service) SaveStoreInfo(ctx context.Context, branchID uuid.UUID, in StoreInfo) (StoreInfo, error) {
	in = normalizeStoreInfo(in)
	if issues := validateStoreInfo(in); len(issues) > 0 {
		return StoreInfo{}, &ValidationError{Issues: issues}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return StoreInfo{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	// Country and timezone aren't editable from the portal yet; keep
	// whatever is stored, or the defaults for a first save.
	country, timezone := "GB", defaultTimezone
	if existing, err := q.GetBranchInfo(ctx, branchID); err == nil {
		country, timezone = existing.Country, existing.Timezone
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return StoreInfo{}, fmt.Errorf("load branch info: %w", err)
	}

	d := in.Fulfilment.Delivery
	if _, err := q.UpsertBranchInfo(ctx, db.UpsertBranchInfoParams{
		BranchID:                 branchID,
		Phone:                    in.Phone,
		Email:                    in.Email,
		AddressLine1:             in.Address.Line1,
		AddressLine2:             in.Address.Line2,
		City:                     in.Address.City,
		PostalCode:               in.Address.Postcode,
		Country:                  country,
		Timezone:                 timezone,
		Collection:               in.Fulfilment.Collection.Enabled,
		Delivery:                 d.Enabled,
		DeliveryFee:              d.Fee,
		DeliveryFreeOver:         d.FreeOver,
		DeliveryMinOrder:         d.MinOrder,
		DeliveryPostcodePrefixes: d.PostcodePrefixes,
		LastOrderMinutes:         in.Fulfilment.Collection.LastOrderMinutes,
	}); err != nil {
		return StoreInfo{}, fmt.Errorf("save branch info: %w", err)
	}

	if err := q.DeleteBranchOpeningHours(ctx, branchID); err != nil {
		return StoreInfo{}, fmt.Errorf("clear opening hours: %w", err)
	}
	for _, h := range in.OpeningHours {
		open, _ := parseClock(h.Open)
		closeAt, _ := parseClock(h.Close)
		if _, err := q.CreateBranchOpeningHour(ctx, db.CreateBranchOpeningHourParams{
			BranchID: branchID, DayOfWeek: h.Day, OpenTime: open, CloseTime: closeAt,
		}); err != nil {
			return StoreInfo{}, fmt.Errorf("save opening hours: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return StoreInfo{}, err
	}
	return s.GetStoreInfo(ctx, branchID)
}

// ListClosures returns date overrides from today (branch time) onwards.
func (s *Service) ListClosures(ctx context.Context, branchID uuid.UUID) ([]Closure, error) {
	today := s.branchToday(ctx, branchID)
	rows, err := s.queries.ListBranchHoursExceptions(ctx, db.ListBranchHoursExceptionsParams{
		BranchID: branchID,
		FromDate: pgDate(today),
		ToDate:   pgDate(today.AddDate(1, 0, 0)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Closure, 0, len(rows))
	for _, r := range rows {
		out = append(out, toClosure(r))
	}
	return out, nil
}

func (s *Service) AddClosure(ctx context.Context, branchID uuid.UUID, in Closure) (Closure, error) {
	var issues []Issue
	date, err := time.Parse(time.DateOnly, in.Date)
	if err != nil {
		issues = append(issues, Issue{"date", "Date must be YYYY-MM-DD"})
	} else if date.Before(s.branchToday(ctx, branchID)) {
		issues = append(issues, Issue{"date", "Date can't be in the past"})
	}

	var open, closeAt pgtype.Time
	if !in.Closed {
		var okOpen, okClose bool
		if in.Open != nil {
			open, okOpen = parseClock(*in.Open)
		}
		if in.Close != nil {
			closeAt, okClose = parseClock(*in.Close)
		}
		if !okOpen || !okClose {
			issues = append(issues, Issue{"open", "Special hours need an opening and closing time (HH:MM)"})
		} else if closeAt.Microseconds <= open.Microseconds {
			issues = append(issues, Issue{"close", "Closing time must be after opening time"})
		}
	}
	note := trimmedOrNil(in.Note)
	if note != nil && len(*note) > 200 {
		issues = append(issues, Issue{"note", "Note must be 200 characters or fewer"})
	}
	if len(issues) > 0 {
		return Closure{}, &ValidationError{Issues: issues}
	}

	row, err := s.queries.CreateBranchHoursException(ctx, db.CreateBranchHoursExceptionParams{
		BranchID: branchID, Date: pgDate(date), Closed: in.Closed,
		OpenTime: open, CloseTime: closeAt, Note: note,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Closure{}, ErrClosureExists
		}
		if isForeignKeyViolation(err) { // no branch_info row yet
			return Closure{}, &ValidationError{Issues: []Issue{{"date", "Save the store details before adding closures"}}}
		}
		return Closure{}, err
	}
	return toClosure(row), nil
}

func (s *Service) DeleteClosure(ctx context.Context, branchID uuid.UUID, id int32) error {
	return s.queries.DeleteBranchHoursException(ctx, db.DeleteBranchHoursExceptionParams{ID: id, BranchID: branchID})
}

// branchToday is today's date in the branch's timezone, as midnight UTC (the
// form DATE columns use).
func (s *Service) branchToday(ctx context.Context, branchID uuid.UUID) time.Time {
	tz := defaultTimezone
	if info, err := s.queries.GetBranchInfo(ctx, branchID); err == nil {
		tz = info.Timezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	y, m, d := s.now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

var postcodePrefixRe = regexp.MustCompile(`^[A-Z]{1,2}[0-9][0-9A-Z]?$`)

func normalizeStoreInfo(in StoreInfo) StoreInfo {
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = trimmedOrNil(in.Email)
	in.Address.Line1 = trimmedOrNil(in.Address.Line1)
	in.Address.Line2 = trimmedOrNil(in.Address.Line2)
	in.Address.City = trimmedOrNil(in.Address.City)
	if pc := trimmedOrNil(in.Address.Postcode); pc != nil {
		upper := strings.ToUpper(*pc)
		in.Address.Postcode = &upper
	} else {
		in.Address.Postcode = nil
	}

	// Outward codes: uppercase, no spaces, deduplicated, sorted.
	seen := map[string]bool{}
	prefixes := []string{}
	for _, p := range in.Fulfilment.Delivery.PostcodePrefixes {
		p = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(p), " ", ""))
		if p != "" && !seen[p] {
			seen[p] = true
			prefixes = append(prefixes, p)
		}
	}
	sort.Strings(prefixes)
	in.Fulfilment.Delivery.PostcodePrefixes = prefixes

	if in.OpeningHours == nil {
		in.OpeningHours = []OpeningHour{}
	}
	return in
}

// sortedHourIndexes returns indexes into hours ordered by day, then opening
// time - so overlapping periods end up adjacent - without reordering hours
// itself, keeping issue fields pointing at the rows the client sent.
func sortedHourIndexes(hours []OpeningHour) []int {
	idx := make([]int, len(hours))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := hours[idx[a]], hours[idx[b]]
		if x.Day != y.Day {
			return x.Day < y.Day
		}
		return x.Open < y.Open
	})
	return idx
}

func validateStoreInfo(in StoreInfo) []Issue {
	var issues []Issue
	if in.Phone == "" {
		issues = append(issues, Issue{"phone", "Phone number is required"})
	}
	if in.Email != nil && !strings.Contains(*in.Email, "@") {
		issues = append(issues, Issue{"email", "Enter a valid email address"})
	}

	if m := in.Fulfilment.Collection.LastOrderMinutes; m < 0 || m > 480 {
		issues = append(issues, Issue{"fulfilment.collection.last_order_minutes", "Last orders must be between 0 and 480 minutes before closing"})
	}

	d := in.Fulfilment.Delivery
	if d.Fee < 0 {
		issues = append(issues, Issue{"fulfilment.delivery.fee", "Delivery fee can't be negative"})
	}
	if d.MinOrder < 0 {
		issues = append(issues, Issue{"fulfilment.delivery.min_order", "Minimum order can't be negative"})
	}
	if d.FreeOver != nil && *d.FreeOver < 0 {
		issues = append(issues, Issue{"fulfilment.delivery.free_over", "Free delivery threshold can't be negative"})
	}
	for _, p := range d.PostcodePrefixes {
		if !postcodePrefixRe.MatchString(p) {
			issues = append(issues, Issue{"fulfilment.delivery.postcode_prefixes", fmt.Sprintf("%q isn't a valid postcode area (e.g. LS1, E15)", p)})
		}
	}
	if d.Enabled && len(d.PostcodePrefixes) == 0 {
		issues = append(issues, Issue{"fulfilment.delivery.postcode_prefixes", "Add at least one postcode area to offer delivery"})
	}

	// Issue fields use the index the client sent (opening_hours.<i>).
	valid := true
	for i, h := range in.OpeningHours {
		field := fmt.Sprintf("opening_hours.%d", i)
		open, okOpen := parseClock(h.Open)
		closeAt, okClose := parseClock(h.Close)
		switch {
		case h.Day < 1 || h.Day > 7:
			issues = append(issues, Issue{field, "Day must be 1 (Monday) to 7 (Sunday)"})
		case !okOpen || !okClose:
			issues = append(issues, Issue{field, "Times must be HH:MM"})
		case closeAt.Microseconds <= open.Microseconds:
			issues = append(issues, Issue{field, "Closing time must be after opening time"})
		default:
			continue
		}
		valid = false
	}
	// Overlaps only make sense to check once every period is well-formed.
	// "HH:MM" strings compare correctly as text.
	if valid {
		idx := sortedHourIndexes(in.OpeningHours)
		for k := 1; k < len(idx); k++ {
			prev, cur := in.OpeningHours[idx[k-1]], in.OpeningHours[idx[k]]
			if prev.Day == cur.Day && cur.Open < prev.Close {
				issues = append(issues, Issue{fmt.Sprintf("opening_hours.%d", idx[k]), "Opening periods on the same day can't overlap"})
			}
		}
	}
	return issues
}

var clockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)

func parseClock(s string) (pgtype.Time, bool) {
	m := clockRe.FindStringSubmatch(s)
	if m == nil {
		return pgtype.Time{}, false
	}
	var h, min int64
	fmt.Sscanf(m[1], "%d", &h)
	fmt.Sscanf(m[2], "%d", &min)
	return pgtype.Time{Microseconds: (h*60 + min) * 60 * 1_000_000, Valid: true}, true
}

func formatClock(t pgtype.Time) string {
	mins := t.Microseconds / 60_000_000
	return fmt.Sprintf("%02d:%02d", mins/60, mins%60)
}

func toClosure(r db.BranchHoursException) Closure {
	c := Closure{ID: r.ID, Date: r.Date.Time.Format(time.DateOnly), Closed: r.Closed, Note: r.Note}
	if r.OpenTime.Valid {
		v := formatClock(r.OpenTime)
		c.Open = &v
	}
	if r.CloseTime.Valid {
		v := formatClock(r.CloseTime)
		c.Close = &v
	}
	return c
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}
