package portal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
)

// Invites let a vendor user choose their own password: the platform admin
// gets a one-time link to pass on (WhatsApp, email...), so nobody else ever
// knows the password and no email server is needed.

const (
	InviteTokenPrefix = "vi_"
	InviteLifetime    = 7 * 24 * time.Hour
)

var (
	ErrInviteNotFound = errors.New("invite not found")
	// ErrInviteExpired covers used, expired and superseded links, and
	// accounts disabled since the invite was sent.
	ErrInviteExpired   = errors.New("this invite link has expired or was already used")
	ErrAlreadyHasLogin = errors.New("this user has already set a password")
	ErrUserNotFound    = errors.New("user not found")
)

// Invite is a freshly issued link token - the plaintext exists only here.
type Invite struct {
	Token     string
	ExpiresAt time.Time
}

// InviteInfo is what the invite page shows before the password is set.
type InviteInfo struct {
	Email      string
	Name       string
	VendorName string
	ExpiresAt  time.Time
}

// issueInvite expires the user's older unused invites and creates a new
// one, on q (so callers can include it in their transaction).
func (s *Service) issueInvite(ctx context.Context, q *db.Queries, userID uuid.UUID, invitedBy *uuid.UUID) (Invite, error) {
	if err := q.ExpireUnusedVendorInvites(ctx, userID); err != nil {
		return Invite{}, fmt.Errorf("expire old invites: %w", err)
	}
	token, err := crypto.GenerateToken(InviteTokenPrefix)
	if err != nil {
		return Invite{}, err
	}
	var by pgtype.UUID
	if invitedBy != nil {
		by = pgtype.UUID{Bytes: *invitedBy, Valid: true}
	}
	inv, err := q.CreateVendorInvite(ctx, db.CreateVendorInviteParams{
		UserID:           userID,
		TokenHash:        crypto.HashAPIKey(token),
		CreatedByAdminID: by,
		ExpiresAt:        timestamptz(s.now().Add(InviteLifetime)),
	})
	if err != nil {
		return Invite{}, fmt.Errorf("create invite: %w", err)
	}
	return Invite{Token: token, ExpiresAt: inv.ExpiresAt.Time}, nil
}

// ReissueInvite sends a fresh link to a user of vendorID who hasn't set a
// password yet; any earlier link stops working.
func (s *Service) ReissueInvite(ctx context.Context, vendorID, userID uuid.UUID, invitedBy *uuid.UUID) (Invite, error) {
	user, err := s.queries.GetVendorUserForVendor(ctx, db.GetVendorUserForVendorParams{ID: userID, VendorID: vendorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrUserNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	if user.PasswordHash != nil {
		return Invite{}, ErrAlreadyHasLogin
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer tx.Rollback(ctx)
	inv, err := s.issueInvite(ctx, s.queries.WithTx(tx), userID, invitedBy)
	if err != nil {
		return Invite{}, err
	}
	return inv, tx.Commit(ctx)
}

func (s *Service) loadInvite(ctx context.Context, q *db.Queries, token string) (db.GetVendorInviteByTokenHashRow, error) {
	row, err := q.GetVendorInviteByTokenHash(ctx, crypto.HashAPIKey(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrInviteNotFound
	}
	if err != nil {
		return row, err
	}
	i := row.VendorInvite
	if i.UsedAt.Valid || !i.ExpiresAt.Time.After(s.now()) || !row.VendorUser.IsActive || !row.Vendor.IsActive {
		return row, ErrInviteExpired
	}
	return row, nil
}

func (s *Service) GetInvite(ctx context.Context, token string) (InviteInfo, error) {
	row, err := s.loadInvite(ctx, s.queries, token)
	if err != nil {
		return InviteInfo{}, err
	}
	return InviteInfo{
		Email:      row.VendorUser.Email,
		Name:       row.VendorUser.Name,
		VendorName: row.Vendor.Name,
		ExpiresAt:  row.VendorInvite.ExpiresAt.Time,
	}, nil
}

// AcceptInvite sets the user's password and logs them straight in.
func (s *Service) AcceptInvite(ctx context.Context, token, password, userAgent string) (LoginResult, error) {
	if err := auth.ValidatePassword(password); err != nil {
		return LoginResult{}, &ValidationError{Issues: []Issue{{"password", err.Error()}}}
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return LoginResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)

	row, err := s.loadInvite(ctx, q, token)
	if err != nil {
		return LoginResult{}, err
	}
	// Guarded update: if a concurrent accept got there first, 0 rows.
	n, err := q.MarkVendorInviteUsed(ctx, row.VendorInvite.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if n == 0 {
		return LoginResult{}, ErrInviteExpired
	}
	if err := q.UpdateVendorUserPasswordHash(ctx, db.UpdateVendorUserPasswordHashParams{ID: row.VendorUser.ID, PasswordHash: &hash}); err != nil {
		return LoginResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LoginResult{}, err
	}

	session, err := s.auth.IssueSession(ctx, row.VendorUser.ID, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return s.loginResult(ctx, row.VendorUser.ID, session)
}
