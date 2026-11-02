package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/portal"
)

// Platform admin: onboard vendors and their users. Vendors can't sign
// themselves up - the platform decides who trades under the brand.

type adminVendorResponse struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

func toAdminVendorResponse(v db.Vendor) adminVendorResponse {
	return adminVendorResponse{ID: v.ID.String(), Slug: v.Slug, Name: v.Name, IsActive: v.IsActive}
}

type adminVendorListItem struct {
	adminVendorResponse
	BranchCount    int32 `json:"branch_count"`
	UserCount      int32 `json:"user_count"`
	PendingInvites int32 `json:"pending_invites"`
}

type adminVendorBranch struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	Host                   string     `json:"host"`
	IsActive               bool       `json:"is_active"`
	StorefrontAPIKeyPrefix *string    `json:"storefront_api_key_prefix"`
	HasStoreDetails        bool       `json:"has_store_details"`
	LastSyncAt             *time.Time `json:"last_sync_at"`
	LastSyncStatus         *string    `json:"last_sync_status"`
	LastSyncError          *string    `json:"last_sync_error"`
}

type adminVendorUser struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Status      string     `json:"status"` // active | invited | invite_expired | locked | disabled
	LastLoginAt *time.Time `json:"last_login_at"`
	InviteUntil *time.Time `json:"invite_expires_at"`
}

type adminVendorDetail struct {
	adminVendorResponse
	Branches []adminVendorBranch `json:"branches"`
	Users    []adminVendorUser   `json:"users"`
}

type inviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) handleListVendors(w http.ResponseWriter, r *http.Request) {
	var search *string
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		search = &q
	}
	rows, err := s.queries.ListVendorsWithCounts(r.Context(), search)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]adminVendorListItem, 0, len(rows))
	for _, v := range rows {
		out = append(out, adminVendorListItem{
			adminVendorResponse: adminVendorResponse{ID: v.ID.String(), Slug: v.Slug, Name: v.Name, IsActive: v.IsActive},
			BranchCount:         v.BranchCount,
			UserCount:           v.UserCount,
			PendingInvites:      v.PendingInvites,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateVendor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := s.portal.CreateVendor(r.Context(), req.Slug, req.Name)
	if errors.Is(err, portal.ErrSlugTaken) {
		writeIssues(w, err.Error(), []portal.Issue{{Field: "slug", Message: "That slug is already used by another vendor"}})
		return
	}
	if s.writePortalError(w, r, err) {
		return
	}
	s.audit(r, "vendor.created", "vendor", v.ID.String(), fmt.Sprintf("Created vendor %s", v.Name))
	writeJSON(w, http.StatusCreated, toAdminVendorResponse(v))
}

// vendorFromRequest loads {vendorID}, writing a 404 if it doesn't exist.
func (s *Server) vendorFromRequest(w http.ResponseWriter, r *http.Request) (db.Vendor, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "vendorID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "vendor not found")
		return db.Vendor{}, false
	}
	v, err := s.queries.GetVendor(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "vendor not found")
		return db.Vendor{}, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return db.Vendor{}, false
	}
	return v, true
}

func (s *Server) handleGetVendor(w http.ResponseWriter, r *http.Request) {
	v, ok := s.vendorFromRequest(w, r)
	if !ok {
		return
	}
	branches, err := s.queries.ListVendorBranchesForAdmin(r.Context(), v.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	users, err := s.queries.ListVendorUsersForAdmin(r.Context(), v.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	out := adminVendorDetail{
		adminVendorResponse: toAdminVendorResponse(v),
		Branches:            make([]adminVendorBranch, 0, len(branches)),
		Users:               make([]adminVendorUser, 0, len(users)),
	}
	for _, b := range branches {
		out.Branches = append(out.Branches, adminVendorBranch{
			ID: b.ID.String(), Name: b.Name, Host: b.Host, IsActive: b.IsActive,
			StorefrontAPIKeyPrefix: b.StorefrontApiKeyPrefix,
			HasStoreDetails:        b.HasStoreDetails,
			LastSyncAt:             optionalTime(b.LastSyncAt.Time, b.LastSyncAt.Valid),
			LastSyncStatus:         b.LastSyncStatus,
			LastSyncError:          b.LastSyncError,
		})
	}
	now := time.Now()
	for _, u := range users {
		out.Users = append(out.Users, adminVendorUser{
			ID: u.ID.String(), Name: u.Name, Email: u.Email, Role: u.Role,
			Status:      vendorUserStatus(u, now),
			LastLoginAt: optionalTime(u.LastLoginAt.Time, u.LastLoginAt.Valid),
			InviteUntil: optionalTime(u.InviteExpiresAt.Time, u.InviteExpiresAt.Valid && !u.HasPassword),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func vendorUserStatus(u db.ListVendorUsersForAdminRow, now time.Time) string {
	switch {
	case !u.IsActive:
		return "disabled"
	case !u.HasPassword && u.InviteExpiresAt.Valid && u.InviteExpiresAt.Time.After(now):
		return "invited"
	case !u.HasPassword:
		return "invite_expired"
	case u.LockedUntil.Valid && u.LockedUntil.Time.After(now):
		return "locked"
	default:
		return "active"
	}
}

// handleUpdateVendor renames or (de)activates a vendor. Deactivating logs
// all its users out at once: sessions only count while the vendor is active.
func (s *Server) handleUpdateVendor(w http.ResponseWriter, r *http.Request) {
	v, ok := s.vendorFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, active := v.Name, v.IsActive
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			writeIssues(w, "name is required", []portal.Issue{{Field: "name", Message: "Name is required"}})
			return
		}
	}
	if req.IsActive != nil {
		active = *req.IsActive
	}

	updated, err := s.queries.UpdateVendor(r.Context(), db.UpdateVendorParams{ID: v.ID, Name: name, IsActive: active})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	switch {
	case v.IsActive && !active:
		s.audit(r, "vendor.deactivated", "vendor", v.ID.String(), fmt.Sprintf("Deactivated vendor %s", name))
	case !v.IsActive && active:
		s.audit(r, "vendor.reactivated", "vendor", v.ID.String(), fmt.Sprintf("Reactivated vendor %s", name))
	}
	if name != v.Name {
		s.audit(r, "vendor.renamed", "vendor", v.ID.String(), fmt.Sprintf("Renamed vendor %s to %s", v.Name, name))
	}
	writeJSON(w, http.StatusOK, toAdminVendorResponse(updated))
}

// handleCreateVendorUser adds a user to a vendor. Without a password (the
// normal case from the admin UI) it returns an invite token the admin turns
// into a link for the user to set their own password.
func (s *Server) handleCreateVendorUser(w http.ResponseWriter, r *http.Request) {
	v, ok := s.vendorFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Email     string      `json:"email"`
		Name      string      `json:"name"`
		Role      string      `json:"role"`
		Password  string      `json:"password"`
		BranchIDs []uuid.UUID `json:"branch_ids"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, invite, err := s.portal.CreateUser(r.Context(), portal.CreateUserInput{
		VendorID:  v.ID,
		Email:     req.Email,
		Name:      req.Name,
		Role:      portal.Role(req.Role),
		Password:  req.Password,
		BranchIDs: req.BranchIDs,
		InvitedBy: adminActor(r).AdminID,
	})
	if errors.Is(err, portal.ErrEmailTaken) {
		writeIssues(w, err.Error(), []portal.Issue{{Field: "email", Message: "Someone already has an account with this email"}})
		return
	}
	if s.writePortalError(w, r, err) {
		return
	}

	s.audit(r, "user.created", "vendor_user", user.ID.String(),
		fmt.Sprintf("Added %s (%s) as %s of %s", user.Name, user.Email, user.Role, v.Name))

	resp := struct {
		User   vendorUserResponse `json:"user"`
		Invite *inviteResponse    `json:"invite"`
	}{User: toVendorUserResponse(user)}
	if invite != nil {
		resp.Invite = &inviteResponse{Token: invite.Token, ExpiresAt: invite.ExpiresAt}
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleReissueInvite(w http.ResponseWriter, r *http.Request) {
	v, ok := s.vendorFromRequest(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	invite, err := s.portal.ReissueInvite(r.Context(), v.ID, userID, adminActor(r).AdminID)
	switch {
	case errors.Is(err, portal.ErrUserNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, portal.ErrAlreadyHasLogin):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		s.internalError(w, r, err)
		return
	}
	s.audit(r, "user.invite_resent", "vendor_user", userID.String(), fmt.Sprintf("Sent a new invite link for a %s user", v.Name))
	writeJSON(w, http.StatusCreated, inviteResponse{Token: invite.Token, ExpiresAt: invite.ExpiresAt})
}

func (s *Server) handleAdminGetStoreInfo(w http.ResponseWriter, r *http.Request) {
	branchID, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return
	}
	info, err := s.portal.GetStoreInfo(r.Context(), branchID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleAdminSaveStoreInfo(w http.ResponseWriter, r *http.Request) {
	branch, ok := s.branchFromRequest(w, r)
	if !ok {
		return
	}
	var req portal.StoreInfo
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := s.portal.SaveStoreInfo(r.Context(), branch.ID, req)
	if s.writePortalError(w, r, err) {
		return
	}
	s.audit(r, "branch.store_details_updated", "branch", branch.ID.String(), fmt.Sprintf("Updated store details for %s", branch.Name))
	writeJSON(w, http.StatusOK, info)
}

// branchFromRequest loads {branchID}, writing a 404 if it doesn't exist.
func (s *Server) branchFromRequest(w http.ResponseWriter, r *http.Request) (db.Branch, bool) {
	id, err := branchIDFromRequest(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch not found")
		return db.Branch{}, false
	}
	b, err := s.queries.GetBranch(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "branch not found")
		return db.Branch{}, false
	}
	if err != nil {
		s.internalError(w, r, err)
		return db.Branch{}, false
	}
	return b, true
}

func optionalTime(t time.Time, valid bool) *time.Time {
	if !valid {
		return nil
	}
	return &t
}
