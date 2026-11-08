package portal

import (
	"slices"
	"testing"
)

func TestNextStatuses(t *testing.T) {
	cases := []struct {
		from, fulfilment string
		want             []string
	}{
		{StatusPending, "collection", []string{StatusAccepted, StatusRejected, StatusCancelled}},
		{StatusAccepted, "delivery", []string{StatusPicking, StatusCancelled}},
		{StatusPicking, "collection", []string{StatusReady, StatusCancelled}},
		{StatusReady, "collection", []string{StatusCompleted, StatusNoShow}},
		{StatusReady, "delivery", []string{StatusOutForDelivery}},
		{StatusOutForDelivery, "delivery", []string{StatusCompleted}},
	}
	for _, c := range cases {
		if got := NextStatuses(c.from, c.fulfilment); !slices.Equal(got, c.want) {
			t.Errorf("NextStatuses(%s, %s) = %v, want %v", c.from, c.fulfilment, got, c.want)
		}
	}
}

func TestTerminalStatusesHaveNoNextStatus(t *testing.T) {
	for _, s := range DoneStatuses {
		for _, f := range []string{"collection", "delivery"} {
			if got := NextStatuses(s, f); len(got) != 0 {
				t.Errorf("%s (%s) should be terminal, got %v", s, f, got)
			}
		}
	}
}

func TestCanTransitionRejectsSkipsAndWrongFulfilment(t *testing.T) {
	if canTransition(StatusPending, StatusCompleted, "collection") {
		t.Error("pending -> completed must not skip the lifecycle")
	}
	if canTransition(StatusReady, StatusNoShow, "delivery") {
		t.Error("no_show is collection-only")
	}
	if canTransition(StatusReady, StatusOutForDelivery, "collection") {
		t.Error("out_for_delivery is delivery-only")
	}
	if canTransition(StatusCompleted, StatusCancelled, "collection") {
		t.Error("completed is terminal")
	}
	for _, f := range []string{"collection", "delivery"} {
		if canTransition(StatusReady, StatusCancelled, f) {
			t.Errorf("ready (%s) can't be cancelled: the money is taken at ready", f)
		}
	}
}

func TestEveryStatusIsActiveOrDone(t *testing.T) {
	all := append(slices.Clone(ActiveStatuses), DoneStatuses...)
	for from, ts := range transitions {
		if !slices.Contains(all, from) {
			t.Errorf("%s has transitions but is neither active nor done", from)
		}
		for _, tr := range ts {
			if !slices.Contains(all, tr.to) {
				t.Errorf("transition target %s is neither active nor done", tr.to)
			}
		}
	}
}

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role   Role
		action Action
		want   bool
	}{
		{RoleOwner, EditStoreInfo, true},
		{RoleOwner, TriggerSync, true},
		{RoleManager, EditStoreInfo, true},
		{RoleManager, TriggerSync, true},
		{RoleManager, UpdateOrderStatus, true},
		{RoleManager, RefundOrders, true},
		{RoleOwner, RefundOrders, true},
		{RoleStaff, RefundOrders, false},
		{RoleStaff, UpdateOrderStatus, true},
		{RoleStaff, ViewStoreInfo, true},
		{RoleStaff, EditStoreInfo, false},
		{RoleStaff, TriggerSync, false},
		{Role("intruder"), ViewOrders, false},
	}
	for _, c := range cases {
		if got := c.role.Can(c.action); got != c.want {
			t.Errorf("%s.Can(%d) = %v, want %v", c.role, c.action, got, c.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Owner@Group.Example "); got != "owner@group.example" {
		t.Errorf("got %q", got)
	}
}

func TestValidateStoreInfoReportsClientIndexes(t *testing.T) {
	in := normalizeStoreInfo(StoreInfo{
		Phone: "020 7946 0101",
		OpeningHours: []OpeningHour{
			{Day: 2, Open: "09:00", Close: "17:00"},
			{Day: 1, Open: "09:00", Close: "12:00"},
			{Day: 2, Open: "16:00", Close: "18:00"}, // overlaps index 0
		},
	})
	issues := validateStoreInfo(in)
	if len(issues) != 1 || issues[0].Field != "opening_hours.2" {
		t.Fatalf("expected one overlap issue on opening_hours.2, got %+v", issues)
	}
}

func TestValidateStoreInfoDelivery(t *testing.T) {
	in := StoreInfo{Phone: "1"}
	in.Fulfilment.Delivery = Delivery{Enabled: true, Fee: -1, PostcodePrefixes: []string{" ls1 ", "LS1", "bad code"}}
	in = normalizeStoreInfo(in)
	if got := in.Fulfilment.Delivery.PostcodePrefixes; !slices.Equal(got, []string{"BADCODE", "LS1"}) {
		t.Errorf("prefixes not normalised: %v", got)
	}
	fields := []string{}
	for _, is := range validateStoreInfo(in) {
		fields = append(fields, is.Field)
	}
	want := []string{"fulfilment.delivery.fee", "fulfilment.delivery.postcode_prefixes"}
	if !slices.Equal(fields, want) {
		t.Errorf("fields = %v, want %v", fields, want)
	}
}

func TestClockRoundTrip(t *testing.T) {
	for _, s := range []string{"00:00", "09:05", "23:59"} {
		v, ok := parseClock(s)
		if !ok || formatClock(v) != s {
			t.Errorf("round trip %q failed", s)
		}
	}
	for _, s := range []string{"24:00", "9:00", "12:60", ""} {
		if _, ok := parseClock(s); ok {
			t.Errorf("%q should be rejected", s)
		}
	}
}
