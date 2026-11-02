package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

// closuresWindowDays is how far ahead date overrides (holidays) are sent - it
// matches how far ahead an order can be placed.
const closuresWindowDays = 14

type branchInfoResponse struct {
	Name     string  `json:"name"`
	Phone    string  `json:"phone"`
	Email    *string `json:"email"`
	Timezone string  `json:"timezone"`
	Address  struct {
		Line1    *string `json:"line1"`
		Line2    *string `json:"line2"`
		City     *string `json:"city"`
		Postcode *string `json:"postcode"`
	} `json:"address"`
	Fulfilment struct {
		Collection struct {
			Enabled          bool  `json:"enabled"`
			LastOrderMinutes int32 `json:"last_order_minutes"`
		} `json:"collection"`
		Delivery struct {
			Enabled          bool     `json:"enabled"`
			Fee              int64    `json:"fee"`
			FreeOver         *int64   `json:"free_over"`
			MinOrder         int64    `json:"min_order"`
			PostcodePrefixes []string `json:"postcode_prefixes"`
		} `json:"delivery"`
	} `json:"fulfilment"`
	OpeningHours []openingHourResponse `json:"opening_hours"`
	Closures     []closureResponse     `json:"closures"`
	// Today's collection window; null = no collection today (closed, or
	// collection is off).
	CollectionToday *collectionTodayResponse `json:"collection_today"`
	// Card is the only way to pay: enabled = false means checkout is closed
	// (the vendor has no active payment account).
	Payments struct {
		Enabled bool `json:"enabled"`
	} `json:"payments"`
}

type collectionTodayResponse struct {
	Date        string    `json:"date"`
	LastOrderAt time.Time `json:"last_order_at"`
	ClosesAt    time.Time `json:"closes_at"`
	Accepting   bool      `json:"accepting"` // false once last orders have passed
}

type openingHourResponse struct {
	Day   int16  `json:"day"` // ISO: 1 = Monday ... 7 = Sunday
	Open  string `json:"open"`
	Close string `json:"close"`
}

type closureResponse struct {
	Date   string  `json:"date"`
	Closed bool    `json:"closed"`
	Open   *string `json:"open"`
	Close  *string `json:"close"`
	Note   *string `json:"note"`
}

// GET /v1/store/branch - everything the checkout needs about the shop:
// contact details, collection/delivery options and opening days.
func (s *Server) handleStorefrontGetBranch(w http.ResponseWriter, r *http.Request) {
	branch, ok := branchFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	ctx := r.Context()

	info, err := s.queries.GetBranchInfo(ctx, branch.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "this branch has no store info yet")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	hours, err := s.queries.ListBranchOpeningHours(ctx, branch.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	loc, err := time.LoadLocation(info.Timezone)
	if err != nil {
		loc = time.UTC
	}
	today := time.Now().In(loc)
	exceptions, err := s.queries.ListBranchHoursExceptions(ctx, db.ListBranchHoursExceptionsParams{
		BranchID: branch.ID,
		FromDate: dateOf(today),
		ToDate:   dateOf(today.AddDate(0, 0, closuresWindowDays)),
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	var resp branchInfoResponse
	resp.Name = branch.Name
	resp.Phone = info.Phone
	resp.Email = info.Email
	resp.Timezone = info.Timezone
	resp.Address.Line1 = info.AddressLine1
	resp.Address.Line2 = info.AddressLine2
	resp.Address.City = info.City
	resp.Address.Postcode = info.PostalCode
	resp.Fulfilment.Collection.Enabled = info.Collection
	resp.Fulfilment.Collection.LastOrderMinutes = info.LastOrderMinutes
	resp.Fulfilment.Delivery.Enabled = info.Delivery
	resp.Fulfilment.Delivery.Fee = info.DeliveryFee
	resp.Fulfilment.Delivery.FreeOver = info.DeliveryFreeOver
	resp.Fulfilment.Delivery.MinOrder = info.DeliveryMinOrder
	resp.Fulfilment.Delivery.PostcodePrefixes = append([]string{}, info.DeliveryPostcodePrefixes...)

	resp.OpeningHours = make([]openingHourResponse, 0, len(hours))
	for _, h := range hours {
		resp.OpeningHours = append(resp.OpeningHours, openingHourResponse{
			Day: h.DayOfWeek, Open: clock(h.OpenTime), Close: clock(h.CloseTime),
		})
	}
	resp.Closures = make([]closureResponse, 0, len(exceptions))
	for _, e := range exceptions {
		c := closureResponse{Date: e.Date.Time.Format("2006-01-02"), Closed: e.Closed, Note: e.Note}
		if e.OpenTime.Valid {
			open, closeAt := clock(e.OpenTime), clock(e.CloseTime)
			c.Open, c.Close = &open, &closeAt
		}
		resp.Closures = append(resp.Closures, c)
	}

	if info.Collection {
		day, err := s.orders.CollectionToday(ctx, branch.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if day != nil {
			resp.CollectionToday = &collectionTodayResponse{
				Date: day.Date, LastOrderAt: day.LastOrderAt, ClosesAt: day.ClosesAt, Accepting: day.Accepting,
			}
		}
	}

	if resp.Payments.Enabled, err = s.payments.CheckoutEnabled(ctx, branch.ID); err != nil {
		s.internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// clock formats a Postgres TIME (microseconds since midnight) as "HH:MM".
func clock(t pgtype.Time) string {
	minutes := t.Microseconds / int64(time.Minute/time.Microsecond)
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// dateOf turns a local time into the DATE for its calendar day.
func dateOf(t time.Time) pgtype.Date {
	y, m, d := t.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func chiURLParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}
