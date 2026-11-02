package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/db"
)

// CollectionDay is when today's collection orders close: the shop's last
// closing time today minus its "last orders" lead time.
type CollectionDay struct {
	Date        string    // YYYY-MM-DD, branch time
	ClosesAt    time.Time // the day's last closing time
	LastOrderAt time.Time
	Accepting   bool // now is before LastOrderAt
}

// collectionDay works out today's collection window from the weekly hours
// and any date override. nil = the shop is closed today. An override for the
// date replaces the weekly hours entirely, as in isOpenOn.
func collectionDay(now time.Time, loc *time.Location, hours []db.BranchOpeningHour, exceptions []db.BranchHoursException, lastOrderMinutes int32) *CollectionDay {
	local := now.In(loc)
	y, m, d := local.Date()

	var closesAt time.Time
	found := false
	setClose := func(microseconds int64) {
		t := time.Date(y, m, d, 0, 0, 0, 0, loc).Add(time.Duration(microseconds) * time.Microsecond)
		if !found || t.After(closesAt) {
			closesAt, found = t, true
		}
	}

	var todays []db.BranchHoursException
	for _, e := range exceptions {
		ey, em, ed := e.Date.Time.Date() // DATE arrives as midnight UTC; read it as-is
		if ey == y && em == m && ed == d {
			todays = append(todays, e)
		}
	}
	if len(todays) > 0 {
		for _, e := range todays {
			if e.Closed {
				return nil
			}
			if e.CloseTime.Valid {
				setClose(e.CloseTime.Microseconds)
			}
		}
	} else {
		weekday := int16(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		for _, h := range hours {
			if h.DayOfWeek == weekday {
				setClose(h.CloseTime.Microseconds)
			}
		}
	}
	if !found {
		return nil
	}

	lastOrder := closesAt.Add(-time.Duration(lastOrderMinutes) * time.Minute)
	return &CollectionDay{
		Date:        local.Format(time.DateOnly),
		ClosesAt:    closesAt,
		LastOrderAt: lastOrder,
		Accepting:   now.Before(lastOrder),
	}
}

// CollectionToday loads the branch's hours and works out today's collection
// window. nil = closed today (or no store details yet).
func (s *Service) CollectionToday(ctx context.Context, branchID uuid.UUID) (*CollectionDay, error) {
	return CollectionTodayFor(ctx, s.queries, branchID, s.now())
}

// CollectionTodayFor is CollectionToday for callers with their own queries
// (e.g. inside a transaction) and clock.
func CollectionTodayFor(ctx context.Context, q *db.Queries, branchID uuid.UUID, now time.Time) (*CollectionDay, error) {
	info, err := q.GetBranchInfo(ctx, branchID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(info.Timezone)
	if err != nil {
		loc = time.UTC
	}
	hours, err := q.ListBranchOpeningHours(ctx, branchID)
	if err != nil {
		return nil, err
	}
	today := toPgDate(now.In(loc))
	exceptions, err := q.ListBranchHoursExceptions(ctx, db.ListBranchHoursExceptionsParams{
		BranchID: branchID, FromDate: today, ToDate: today,
	})
	if err != nil {
		return nil, err
	}
	return collectionDay(now, loc, hours, exceptions, info.LastOrderMinutes), nil
}

// CollectBy is when a collection order that's ready now must be picked up:
// window after it's ready, but no later than closing time - unless it was
// only made ready after closing, when the full window applies.
func CollectBy(readyAt time.Time, window time.Duration, day *CollectionDay) time.Time {
	by := readyAt.Add(window)
	if day != nil && day.ClosesAt.After(readyAt) && day.ClosesAt.Before(by) {
		return day.ClosesAt
	}
	return by
}

// NewCollectionCode is a random 6-digit code for the customer to show at
// the counter.
func NewCollectionCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
