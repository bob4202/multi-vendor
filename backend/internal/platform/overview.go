package platform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"ecommerce/backend/internal/db"
)

// Overview is the admin home page: headline counts, what needs someone to
// act, and what admins did recently. It deliberately covers business
// health (a branch that can't go live, an owner who never logged in) - for
// errors and logs, use a logging platform on the API's JSON output.
type Overview struct {
	Counts         db.GetPlatformCountsRow `json:"counts"`
	Attention      []AttentionItem         `json:"attention"`
	RecentActivity []ActivityItem          `json:"recent_activity"`
}

// Attention kinds, in rough order of urgency.
const (
	KindSyncFailed         = "sync_failed"
	KindAccountLocked      = "account_locked"
	KindVendorWithoutOwner = "vendor_without_owner"
	KindInviteExpired      = "invite_expired"
	KindInvitePending      = "invite_pending"
	KindNoStoreDetails     = "no_store_details"
	KindNoStorefrontKey    = "no_storefront_key"
	KindNeverSynced        = "never_synced"
)

var kindRank = map[string]int{
	KindSyncFailed: 0, KindAccountLocked: 1, KindVendorWithoutOwner: 2, KindInviteExpired: 3,
	KindInvitePending: 4, KindNoStoreDetails: 5, KindNoStorefrontKey: 6, KindNeverSynced: 7,
}

type AttentionItem struct {
	Kind       string     `json:"kind"`
	VendorID   string     `json:"vendor_id"`
	VendorName string     `json:"vendor_name"`
	BranchID   *string    `json:"branch_id"`
	BranchName *string    `json:"branch_name"`
	Detail     string     `json:"detail"`
	Since      *time.Time `json:"since"`
}

type ActivityItem struct {
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Summary    string    `json:"summary"`
	AdminName  *string   `json:"admin_name"` // nil = via ADMIN_API_KEY
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	q := s.queries
	counts, err := q.GetPlatformCounts(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("counts: %w", err)
	}

	items := []AttentionItem{}
	add := func(kind, vendorID, vendorName string, branchID, branchName *string, detail string, since *time.Time) {
		items = append(items, AttentionItem{kind, vendorID, vendorName, branchID, branchName, detail, since})
	}
	str := func(s string) *string { return &s }

	failed, err := q.ListFailedSyncs(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("failed syncs: %w", err)
	}
	for _, f := range failed {
		detail := fmt.Sprintf("%s sync failed", f.Resource)
		if f.LastRunError != nil {
			detail += ": " + *f.LastRunError
		}
		add(KindSyncFailed, f.VendorID.String(), f.VendorName, str(f.BranchID.String()), &f.BranchName, detail, timePtr(f.LastRunAt.Time, f.LastRunAt.Valid))
	}

	locked, err := q.ListLockedVendorUsers(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("locked users: %w", err)
	}
	for _, l := range locked {
		add(KindAccountLocked, l.VendorID.String(), l.VendorName, nil, nil,
			fmt.Sprintf("%s (%s) is locked out after repeated wrong passwords", l.UserName, l.Email), nil)
	}

	noOwner, err := q.ListVendorsWithoutOwner(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("vendors without owner: %w", err)
	}
	for _, v := range noOwner {
		add(KindVendorWithoutOwner, v.ID.String(), v.Name, nil, nil, "No owner can log in yet", timePtr(v.CreatedAt.Time, true))
	}

	invites, err := q.ListPendingVendorInvites(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("pending invites: %w", err)
	}
	now := s.now()
	for _, i := range invites {
		if !i.InviteExpiresAt.Valid || !i.InviteExpiresAt.Time.After(now) {
			add(KindInviteExpired, i.VendorID.String(), i.VendorName, nil, nil,
				fmt.Sprintf("Invite for %s (%s) expired - send a new link", i.UserName, i.Email), nil)
			continue
		}
		add(KindInvitePending, i.VendorID.String(), i.VendorName, nil, nil,
			fmt.Sprintf("%s (%s) hasn't accepted their invite", i.UserName, i.Email), timePtr(i.InviteExpiresAt.Time, true))
	}

	setup, err := q.ListBranchesNeedingSetup(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("branches needing setup: %w", err)
	}
	for _, b := range setup {
		id, name := str(b.BranchID.String()), b.BranchName
		since := timePtr(b.CreatedAt.Time, true)
		if b.NoStoreDetails {
			add(KindNoStoreDetails, b.VendorID.String(), b.VendorName, id, &name, "No store details (phone, hours, delivery) yet", since)
		}
		if b.NoStorefrontKey {
			add(KindNoStorefrontKey, b.VendorID.String(), b.VendorName, id, &name, "No storefront key issued, so the shop can't go live", since)
		}
		if b.NeverSynced {
			add(KindNeverSynced, b.VendorID.String(), b.VendorName, id, &name, "Products have never been synced from the inventory system", since)
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return kindRank[items[i].Kind] < kindRank[items[j].Kind] })

	events, err := q.ListRecentAdminAuditEvents(ctx, 20)
	if err != nil {
		return Overview{}, fmt.Errorf("recent activity: %w", err)
	}
	activity := make([]ActivityItem, 0, len(events))
	for _, e := range events {
		activity = append(activity, ActivityItem{e.Action, e.TargetType, e.TargetID, e.Summary, e.AdminName, e.CreatedAt.Time})
	}

	return Overview{Counts: counts, Attention: items, RecentActivity: activity}, nil
}

func timePtr(t time.Time, valid bool) *time.Time {
	if !valid {
		return nil
	}
	return &t
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
