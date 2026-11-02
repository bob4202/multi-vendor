// Package auth is the login and session policy shared by every kind of
// account (vendor users, platform admins): argon2id password checks with
// constant-ish timing, lockout after repeated failures, and opaque session
// tokens stored only as hashes, with sliding and absolute expiry.
//
// Each account type plugs in its own storage through Store, so the policy
// lives in one place while the tables stay separate.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/crypto"
)

const (
	// A session dies after SessionIdleTimeout without use, and after
	// SessionMaxAge no matter what.
	SessionIdleTimeout = 7 * 24 * time.Hour
	SessionMaxAge      = 30 * 24 * time.Hour
	// Sliding the idle expiry costs a write, so do it at most this often.
	sessionTouchInterval = time.Hour

	MaxLoginAttempts  = 10
	LockoutDuration   = 15 * time.Minute
	MinPasswordLength = 12
)

var (
	// ErrInvalidCredentials covers unknown email, wrong password, no
	// password set yet and disabled accounts alike, so a login response
	// never reveals which emails have accounts.
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountLocked      = errors.New("too many failed attempts, try again later")
	ErrUnauthenticated    = errors.New("invalid or expired session")
	// ErrNotFound is what Store.FindAccount returns for an unknown email.
	ErrNotFound = errors.New("account not found")
)

// Account is what the policy needs to know about whoever is logging in.
type Account struct {
	ID           uuid.UUID
	PasswordHash *string    // nil = no password set yet (e.g. invited)
	Active       bool       // false if the account, or anything it belongs to, is disabled
	LockedUntil  *time.Time // nil = not locked
}

// Store is the storage one account type provides.
type Store interface {
	FindAccount(ctx context.Context, email string) (Account, error)
	RecordFailure(ctx context.Context, id uuid.UUID, maxAttempts int32, lockUntil time.Time) error
	RecordSuccess(ctx context.Context, id uuid.UUID) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	CreateSession(ctx context.Context, s NewSession) error
	TouchSession(ctx context.Context, sessionID uuid.UUID, expiresAt time.Time) error
	RevokeSession(ctx context.Context, tokenHash []byte) error
}

type NewSession struct {
	AccountID         uuid.UUID
	TokenHash         []byte
	UserAgent         *string
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
}

// Session is a freshly issued token. The plaintext is only ever held here
// and handed to the client once.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Authenticator struct {
	store       Store
	tokenPrefix string
	now         func() time.Time
}

// New builds an Authenticator. tokenPrefix marks this account type's
// session tokens (e.g. "vs_") so a token of another kind is rejected
// without a database lookup.
func New(store Store, tokenPrefix string) *Authenticator {
	return &Authenticator{store: store, tokenPrefix: tokenPrefix, now: time.Now}
}

// NormalizeEmail is how emails are stored and looked up.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Login checks the password and opens a session, returning the account id
// alongside it so the caller can load whatever it shows after login.
func (a *Authenticator) Login(ctx context.Context, email, password, userAgent string) (uuid.UUID, Session, error) {
	acct, err := a.store.FindAccount(ctx, NormalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		// Burn the same time a real check would, so response timing
		// doesn't reveal whether the email exists.
		_, _ = crypto.VerifyPassword(password, dummyPasswordHash())
		return uuid.Nil, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return uuid.Nil, Session{}, fmt.Errorf("load account: %w", err)
	}

	now := a.now()
	if acct.LockedUntil != nil && acct.LockedUntil.After(now) {
		return uuid.Nil, Session{}, ErrAccountLocked
	}
	if acct.PasswordHash == nil {
		_, _ = crypto.VerifyPassword(password, dummyPasswordHash())
		return uuid.Nil, Session{}, ErrInvalidCredentials
	}

	ok, err := crypto.VerifyPassword(password, *acct.PasswordHash)
	if err != nil {
		return uuid.Nil, Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		if err := a.store.RecordFailure(ctx, acct.ID, MaxLoginAttempts, now.Add(LockoutDuration)); err != nil {
			return uuid.Nil, Session{}, fmt.Errorf("record login failure: %w", err)
		}
		return uuid.Nil, Session{}, ErrInvalidCredentials
	}
	if !acct.Active {
		return uuid.Nil, Session{}, ErrInvalidCredentials
	}

	if err := a.store.RecordSuccess(ctx, acct.ID); err != nil {
		return uuid.Nil, Session{}, fmt.Errorf("record login success: %w", err)
	}
	if crypto.NeedsRehash(*acct.PasswordHash) {
		if hash, err := crypto.HashPassword(password); err == nil {
			_ = a.store.UpdatePasswordHash(ctx, acct.ID, hash)
		}
	}

	session, err := a.IssueSession(ctx, acct.ID, userAgent)
	return acct.ID, session, err
}

// IssueSession opens a session without a password check - for flows that
// have already proven who the user is (e.g. accepting an invite).
func (a *Authenticator) IssueSession(ctx context.Context, accountID uuid.UUID, userAgent string) (Session, error) {
	token, err := crypto.GenerateToken(a.tokenPrefix)
	if err != nil {
		return Session{}, err
	}
	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}
	now := a.now()
	expires := now.Add(SessionIdleTimeout)
	if err := a.store.CreateSession(ctx, NewSession{
		AccountID:         accountID,
		TokenHash:         crypto.HashAPIKey(token),
		UserAgent:         ua,
		ExpiresAt:         expires,
		AbsoluteExpiresAt: now.Add(SessionMaxAge),
	}); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return Session{Token: token, ExpiresAt: expires}, nil
}

// TokenHash validates a token's shape and returns the hash to look the
// session up by. Each account type loads its own session row (it joins
// different tables), then calls Touch.
func (a *Authenticator) TokenHash(token string) ([]byte, error) {
	if !strings.HasPrefix(token, a.tokenPrefix) {
		return nil, ErrUnauthenticated
	}
	return crypto.HashAPIKey(token), nil
}

// Touch slides a session's idle expiry forward if it hasn't been touched
// for a while.
func (a *Authenticator) Touch(ctx context.Context, sessionID uuid.UUID, lastSeen time.Time) error {
	now := a.now()
	if now.Sub(lastSeen) <= sessionTouchInterval {
		return nil
	}
	if err := a.store.TouchSession(ctx, sessionID, now.Add(SessionIdleTimeout)); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (a *Authenticator) Logout(ctx context.Context, token string) error {
	return a.store.RevokeSession(ctx, crypto.HashAPIKey(token))
}

// ValidatePassword is the shared rule for choosing a new password.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	return nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// dummyPasswordHash is a real argon2id hash of a random password, made once,
// used to equalise timing for logins that can't succeed.
func dummyPasswordHash() string {
	dummyHashOnce.Do(func() {
		token, _ := crypto.GenerateToken("")
		dummyHash, _ = crypto.HashPassword(token)
	})
	return dummyHash
}
