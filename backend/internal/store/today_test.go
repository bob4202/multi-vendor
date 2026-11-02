package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

func clockAt(h, m int) pgtype.Time {
	return pgtype.Time{Microseconds: int64(h*60+m) * 60_000_000, Valid: true}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"07700 900123":       "+447700900123",
		"07700-900-123":      "+447700900123",
		"+44 7700 900123":    "+447700900123",
		"+44 (0)7700 900123": "+447700900123",
		"0044 7700 900123":   "+447700900123",
		"447700900123":       "+447700900123",
		"020 7946 0018":      "+442079460018",
		"+33 6 12 34 56 78":  "+33612345678",
	}
	for in, want := range cases {
		got, ok := NormalizePhone(in)
		if !ok || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "12345", "0770090012", "07700 900123 ext 4", "+44 7700 9001234", "phone"} {
		if got, ok := NormalizePhone(bad); ok {
			t.Errorf("NormalizePhone(%q) = %q, want rejected", bad, got)
		}
	}
}

func TestCollectionDay(t *testing.T) {
	london, _ := time.LoadLocation("Europe/London")
	// Wednesday 14 Oct 2026, split hours 09:00-13:00 and 14:00-18:00.
	hours := []db.BranchOpeningHour{
		{DayOfWeek: 3, OpenTime: clockAt(9, 0), CloseTime: clockAt(13, 0)},
		{DayOfWeek: 3, OpenTime: clockAt(14, 0), CloseTime: clockAt(18, 0)},
	}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 14, h, m, 0, 0, london) }

	day := collectionDay(at(10, 0), london, hours, nil, 90)
	if day == nil || day.Date != "2026-10-14" {
		t.Fatalf("expected a collection day, got %+v", day)
	}
	if !day.ClosesAt.Equal(at(18, 0)) || !day.LastOrderAt.Equal(at(16, 30)) || !day.Accepting {
		t.Fatalf("unexpected window %+v", day)
	}
	if late := collectionDay(at(16, 30), london, hours, nil, 90); late.Accepting {
		t.Fatal("last orders at 16:30 should stop accepting at 16:30")
	}

	// A closure for today wins over the weekly hours.
	closed := []db.BranchHoursException{{Date: pgtype.Date{Time: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Valid: true}, Closed: true}}
	if collectionDay(at(10, 0), london, hours, closed, 90) != nil {
		t.Fatal("closed today should mean no collection")
	}
	// Special hours replace them.
	special := []db.BranchHoursException{{
		Date:      pgtype.Date{Time: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), Valid: true},
		OpenTime:  clockAt(10, 0),
		CloseTime: clockAt(15, 0),
	}}
	if d := collectionDay(at(10, 0), london, hours, special, 60); !d.LastOrderAt.Equal(at(14, 0)) {
		t.Fatalf("special hours: last order = %v, want 14:00", d.LastOrderAt)
	}
	// Thursday has no hours.
	if collectionDay(at(10, 0).AddDate(0, 0, 1), london, hours, nil, 90) != nil {
		t.Fatal("a day without hours should mean no collection")
	}
}

func TestCollectBy(t *testing.T) {
	london, _ := time.LoadLocation("Europe/London")
	at := func(h, m int) time.Time { return time.Date(2026, 10, 14, h, m, 0, 0, london) }
	day := &CollectionDay{ClosesAt: at(18, 0)}

	if got := CollectBy(at(12, 0), time.Hour, day); !got.Equal(at(13, 0)) {
		t.Errorf("ready at noon: collect by %v, want 13:00", got)
	}
	if got := CollectBy(at(17, 30), time.Hour, day); !got.Equal(at(18, 0)) {
		t.Errorf("ready 17:30: collect by %v, want closing 18:00", got)
	}
	if got := CollectBy(at(18, 30), time.Hour, day); !got.Equal(at(19, 30)) {
		t.Errorf("ready after closing: collect by %v, want the full hour", got)
	}
	if got := CollectBy(at(12, 0), time.Hour, nil); !got.Equal(at(13, 0)) {
		t.Errorf("no hours known: collect by %v, want 13:00", got)
	}
}
