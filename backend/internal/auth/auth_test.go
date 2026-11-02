package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"ecommerce/backend/internal/crypto"
)

// memStore is an in-memory Store with one account per email.
type memStore struct {
	accounts map[string]*Account
	failures map[uuid.UUID]int32
	sessions []NewSession
	touched  int
	revoked  [][]byte
}

func newMemStore() *memStore {
	return &memStore{accounts: map[string]*Account{}, failures: map[uuid.UUID]int32{}}
}

func (m *memStore) add(t *testing.T, email, password string, active bool) *Account {
	t.Helper()
	acct := &Account{ID: uuid.New(), Active: active}
	if password != "" {
		hash, err := crypto.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		acct.PasswordHash = &hash
	}
	m.accounts[email] = acct
	return acct
}

func (m *memStore) byID(id uuid.UUID) *Account {
	for _, a := range m.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

func (m *memStore) FindAccount(_ context.Context, email string) (Account, error) {
	a, ok := m.accounts[email]
	if !ok {
		return Account{}, ErrNotFound
	}
	return *a, nil
}

// Mirrors RecordVendorLoginFailure: lock and reset the count on the last try.
func (m *memStore) RecordFailure(_ context.Context, id uuid.UUID, max int32, lockUntil time.Time) error {
	m.failures[id]++
	if m.failures[id] >= max {
		m.failures[id] = 0
		m.byID(id).LockedUntil = &lockUntil
	}
	return nil
}

func (m *memStore) RecordSuccess(_ context.Context, id uuid.UUID) error {
	m.failures[id] = 0
	m.byID(id).LockedUntil = nil
	return nil
}

func (m *memStore) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	m.byID(id).PasswordHash = &hash
	return nil
}

func (m *memStore) CreateSession(_ context.Context, s NewSession) error {
	m.sessions = append(m.sessions, s)
	return nil
}

func (m *memStore) TouchSession(context.Context, uuid.UUID, time.Time) error {
	m.touched++
	return nil
}

func (m *memStore) RevokeSession(_ context.Context, h []byte) error {
	m.revoked = append(m.revoked, h)
	return nil
}

const pw = "correct horse battery"

func TestLoginSuccessStoresOnlyTokenHash(t *testing.T) {
	store := newMemStore()
	acct := store.add(t, "a@x.example", pw, true)
	a := New(store, "t_")

	id, session, err := a.Login(context.Background(), "  A@X.example ", pw, "agent")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if id != acct.ID {
		t.Errorf("wrong account id")
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(store.sessions))
	}
	s := store.sessions[0]
	if !bytes.Equal(s.TokenHash, crypto.HashAPIKey(session.Token)) || bytes.Contains(s.TokenHash, []byte(session.Token)) {
		t.Error("session must be stored by hash only")
	}
	if !s.ExpiresAt.Before(s.AbsoluteExpiresAt) {
		t.Error("idle expiry should come before absolute expiry")
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	store := newMemStore()
	store.add(t, "active@x.example", pw, true)
	store.add(t, "disabled@x.example", pw, false)
	store.add(t, "invited@x.example", "", true) // no password yet
	a := New(store, "t_")
	ctx := context.Background()

	cases := map[string][2]string{
		"unknown email":   {"nobody@x.example", pw},
		"wrong password":  {"active@x.example", "wrong password!"},
		"disabled":        {"disabled@x.example", pw},
		"no password yet": {"invited@x.example", pw},
	}
	for name, c := range cases {
		if _, _, err := a.Login(ctx, c[0], c[1], ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: got %v, want ErrInvalidCredentials", name, err)
		}
	}
	if len(store.sessions) != 0 {
		t.Error("no session may be created on a failed login")
	}
}

func TestLockoutAfterMaxAttempts(t *testing.T) {
	store := newMemStore()
	store.add(t, "a@x.example", pw, true)
	a := New(store, "t_")
	ctx := context.Background()

	for i := 0; i < MaxLoginAttempts; i++ {
		if _, _, err := a.Login(ctx, "a@x.example", "wrong password!", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if _, _, err := a.Login(ctx, "a@x.example", pw, ""); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("correct password while locked: got %v, want ErrAccountLocked", err)
	}

	// Once the lock has passed, the right password works again.
	a.now = func() time.Time { return time.Now().Add(LockoutDuration + time.Minute) }
	if _, _, err := a.Login(ctx, "a@x.example", pw, ""); err != nil {
		t.Fatalf("after lock expiry: %v", err)
	}
}

func TestTokenHashRejectsOtherPrefixes(t *testing.T) {
	a := New(newMemStore(), "vs_")
	if _, err := a.TokenHash("as_abc"); !errors.Is(err, ErrUnauthenticated) {
		t.Error("a token with another prefix must be rejected")
	}
	if _, err := a.TokenHash("vs_abc"); err != nil {
		t.Errorf("own prefix: %v", err)
	}
}

func TestTouchIsThrottled(t *testing.T) {
	store := newMemStore()
	a := New(store, "t_")
	ctx := context.Background()
	_ = a.Touch(ctx, uuid.New(), time.Now().Add(-time.Minute))
	if store.touched != 0 {
		t.Error("a session seen a minute ago shouldn't be written to")
	}
	_ = a.Touch(ctx, uuid.New(), time.Now().Add(-2*time.Hour))
	if store.touched != 1 {
		t.Error("a session idle for 2h should slide its expiry")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Error("short password accepted")
	}
	if err := ValidatePassword("twelve chars"); err != nil {
		t.Errorf("12 characters rejected: %v", err)
	}
}
