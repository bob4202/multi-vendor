// Package platform is the business logic for the people who run the
// marketplace: platform admin login, the audit trail of their actions, and
// the overview of what needs attention across all vendors.
package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
)

// AdminSessionTokenPrefix marks platform admin session tokens.
const AdminSessionTokenPrefix = "as_"

var ErrEmailTaken = errors.New("an admin with that email already exists")

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	auth    *auth.Authenticator
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service {
	q := db.New(pool)
	return &Service{pool: pool, queries: q, auth: auth.New(adminAuthStore{q}, AdminSessionTokenPrefix), now: time.Now}
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	Admin     db.PlatformAdmin
}

func (s *Service) Login(ctx context.Context, email, password, userAgent string) (LoginResult, error) {
	_, session, err := s.auth.Login(ctx, email, password, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	admin, err := s.queries.GetPlatformAdminByEmail(ctx, auth.NormalizeEmail(email))
	if err != nil {
		return LoginResult{}, fmt.Errorf("load admin: %w", err)
	}
	return LoginResult{Token: session.Token, ExpiresAt: session.ExpiresAt, Admin: admin}, nil
}

// Authenticate resolves an admin session token. Tokens of any other kind
// (vendor sessions, storefront keys) fail on their prefix.
func (s *Service) Authenticate(ctx context.Context, token string) (db.PlatformAdmin, error) {
	hash, err := s.auth.TokenHash(token)
	if err != nil {
		return db.PlatformAdmin{}, err
	}
	row, err := s.queries.GetActivePlatformAdminSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PlatformAdmin{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return db.PlatformAdmin{}, fmt.Errorf("load session: %w", err)
	}
	if err := s.auth.Touch(ctx, row.PlatformAdminSession.ID, row.PlatformAdminSession.LastSeenAt.Time); err != nil {
		return db.PlatformAdmin{}, err
	}
	return row.PlatformAdmin, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.auth.Logout(ctx, token)
}

// CreateAdmin is used by the cmd/admin bootstrap command.
func (s *Service) CreateAdmin(ctx context.Context, email, name, password string) (db.PlatformAdmin, error) {
	if err := auth.ValidatePassword(password); err != nil {
		return db.PlatformAdmin{}, err
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return db.PlatformAdmin{}, err
	}
	admin, err := s.queries.CreatePlatformAdmin(ctx, db.CreatePlatformAdminParams{
		Email: auth.NormalizeEmail(email), Name: name, PasswordHash: hash,
	})
	if err != nil && isUniqueViolation(err) {
		return db.PlatformAdmin{}, ErrEmailTaken
	}
	return admin, err
}

// Actor is who performed an admin action: a signed-in admin, or nil for
// the shared ADMIN_API_KEY (scripts).
type Actor struct {
	AdminID *uuid.UUID
}

// Audit records an admin action. It never fails the request it describes:
// errors are returned for logging only.
func (s *Service) Audit(ctx context.Context, actor Actor, action, targetType, targetID, summary string) error {
	var adminID pgtype.UUID
	if actor.AdminID != nil {
		adminID = pgtype.UUID{Bytes: *actor.AdminID, Valid: true}
	}
	return s.queries.InsertAdminAuditEvent(ctx, db.InsertAdminAuditEventParams{
		AdminID: adminID, Action: action, TargetType: targetType, TargetID: targetID, Summary: summary,
	})
}

// adminAuthStore adapts the platform_admins tables to auth.Store.
type adminAuthStore struct{ q *db.Queries }

func (st adminAuthStore) FindAccount(ctx context.Context, email string) (auth.Account, error) {
	a, err := st.q.GetPlatformAdminByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Account{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Account{}, err
	}
	hash := a.PasswordHash
	acct := auth.Account{ID: a.ID, PasswordHash: &hash, Active: a.IsActive}
	if a.LockedUntil.Valid {
		acct.LockedUntil = &a.LockedUntil.Time
	}
	return acct, nil
}

func (st adminAuthStore) RecordFailure(ctx context.Context, id uuid.UUID, maxAttempts int32, lockUntil time.Time) error {
	return st.q.RecordPlatformAdminLoginFailure(ctx, db.RecordPlatformAdminLoginFailureParams{
		ID: id, MaxAttempts: maxAttempts, LockUntil: timestamptz(lockUntil),
	})
}

func (st adminAuthStore) RecordSuccess(ctx context.Context, id uuid.UUID) error {
	return st.q.RecordPlatformAdminLoginSuccess(ctx, id)
}

func (st adminAuthStore) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return st.q.UpdatePlatformAdminPasswordHash(ctx, db.UpdatePlatformAdminPasswordHashParams{ID: id, PasswordHash: hash})
}

func (st adminAuthStore) CreateSession(ctx context.Context, s auth.NewSession) error {
	_, err := st.q.CreatePlatformAdminSession(ctx, db.CreatePlatformAdminSessionParams{
		AdminID:           s.AccountID,
		TokenHash:         s.TokenHash,
		UserAgent:         s.UserAgent,
		ExpiresAt:         timestamptz(s.ExpiresAt),
		AbsoluteExpiresAt: timestamptz(s.AbsoluteExpiresAt),
	})
	return err
}

func (st adminAuthStore) TouchSession(ctx context.Context, sessionID uuid.UUID, expiresAt time.Time) error {
	return st.q.TouchPlatformAdminSession(ctx, db.TouchPlatformAdminSessionParams{ID: sessionID, ExpiresAt: timestamptz(expiresAt)})
}

func (st adminAuthStore) RevokeSession(ctx context.Context, tokenHash []byte) error {
	return st.q.RevokePlatformAdminSession(ctx, tokenHash)
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
