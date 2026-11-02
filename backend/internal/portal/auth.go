package portal

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

// Vendor user login and sessions. The policy (lockout, expiry, timing) lives
// in internal/auth; this file only connects it to the vendor tables.

var (
	ErrInvalidCredentials = auth.ErrInvalidCredentials
	ErrAccountLocked      = auth.ErrAccountLocked
	ErrUnauthenticated    = auth.ErrUnauthenticated
)

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	auth    *auth.Authenticator
	now     func() time.Time
	// collectionWindow is how long a ready collection order waits.
	collectionWindow time.Duration
}

func NewService(pool *pgxpool.Pool) *Service {
	q := db.New(pool)
	return &Service{
		pool:    pool,
		queries: q,
		auth:    auth.New(vendorAuthStore{q}, crypto.SessionTokenPrefix),
		now:     time.Now,

		collectionWindow: time.Hour,
	}
}

// WithCollectionWindow sets how long a ready collection order waits to be
// collected (COLLECTION_WINDOW_MINUTES).
func (s *Service) WithCollectionWindow(d time.Duration) *Service {
	s.collectionWindow = d
	return s
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      db.VendorUser
	Vendor    db.Vendor
}

// NormalizeEmail is how emails are stored and looked up.
func NormalizeEmail(email string) string {
	return auth.NormalizeEmail(email)
}

func (s *Service) Login(ctx context.Context, email, password, userAgent string) (LoginResult, error) {
	userID, session, err := s.auth.Login(ctx, email, password, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return s.loginResult(ctx, userID, session)
}

func (s *Service) loginResult(ctx context.Context, userID uuid.UUID, session auth.Session) (LoginResult, error) {
	user, err := s.queries.GetVendorUser(ctx, userID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load user: %w", err)
	}
	vendor, err := s.queries.GetVendor(ctx, user.VendorID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("load vendor: %w", err)
	}
	return LoginResult{Token: session.Token, ExpiresAt: session.ExpiresAt, User: user, Vendor: vendor}, nil
}

// Authenticate resolves a session token to the user behind it.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	hash, err := s.auth.TokenHash(token)
	if err != nil {
		return Principal{}, err
	}
	row, err := s.queries.GetActiveVendorSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load session: %w", err)
	}
	if err := s.auth.Touch(ctx, row.VendorSession.ID, row.VendorSession.LastSeenAt.Time); err != nil {
		return Principal{}, err
	}
	return Principal{SessionID: row.VendorSession.ID, User: row.VendorUser, Vendor: row.Vendor}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.auth.Logout(ctx, token)
}

// Branches lists the branches the user may manage.
func (s *Service) Branches(ctx context.Context, userID uuid.UUID) ([]db.ListVendorUserBranchesRow, error) {
	return s.queries.ListVendorUserBranches(ctx, userID)
}

// vendorAuthStore adapts the vendor tables to auth.Store.
type vendorAuthStore struct{ q *db.Queries }

func (st vendorAuthStore) FindAccount(ctx context.Context, email string) (auth.Account, error) {
	user, err := st.q.GetVendorUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Account{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Account{}, err
	}
	vendor, err := st.q.GetVendor(ctx, user.VendorID)
	if err != nil {
		return auth.Account{}, err
	}
	return auth.Account{
		ID:           user.ID,
		PasswordHash: user.PasswordHash,
		Active:       user.IsActive && vendor.IsActive,
		LockedUntil:  timePtr(user.LockedUntil),
	}, nil
}

func (st vendorAuthStore) RecordFailure(ctx context.Context, id uuid.UUID, maxAttempts int32, lockUntil time.Time) error {
	return st.q.RecordVendorLoginFailure(ctx, db.RecordVendorLoginFailureParams{
		ID: id, MaxAttempts: maxAttempts, LockUntil: timestamptz(lockUntil),
	})
}

func (st vendorAuthStore) RecordSuccess(ctx context.Context, id uuid.UUID) error {
	return st.q.RecordVendorLoginSuccess(ctx, id)
}

func (st vendorAuthStore) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return st.q.UpdateVendorUserPasswordHash(ctx, db.UpdateVendorUserPasswordHashParams{ID: id, PasswordHash: &hash})
}

func (st vendorAuthStore) CreateSession(ctx context.Context, s auth.NewSession) error {
	_, err := st.q.CreateVendorSession(ctx, db.CreateVendorSessionParams{
		UserID:            s.AccountID,
		TokenHash:         s.TokenHash,
		UserAgent:         s.UserAgent,
		ExpiresAt:         timestamptz(s.ExpiresAt),
		AbsoluteExpiresAt: timestamptz(s.AbsoluteExpiresAt),
	})
	return err
}

func (st vendorAuthStore) TouchSession(ctx context.Context, sessionID uuid.UUID, expiresAt time.Time) error {
	return st.q.TouchVendorSession(ctx, db.TouchVendorSessionParams{ID: sessionID, ExpiresAt: timestamptz(expiresAt)})
}

func (st vendorAuthStore) RevokeSession(ctx context.Context, tokenHash []byte) error {
	return st.q.RevokeVendorSession(ctx, tokenHash)
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
