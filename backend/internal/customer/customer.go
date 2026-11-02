// Package customer is storefront customer accounts: sign-in with a one-time
// code sent by email (no passwords), sessions, profile and order history.
//
// An account belongs to a vendor (one per email per vendor), so each
// business only ever sees its own customers. Sessions reuse the shared
// policy in internal/auth.
package customer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/auth"
	"ecommerce/backend/internal/crypto"
	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/mail"
	"ecommerce/backend/internal/store"
)

// SessionTokenPrefix marks customer session tokens.
const SessionTokenPrefix = "cs_"

const (
	codeTTL         = 10 * time.Minute
	codeMaxAttempts = 5
	resendInterval  = time.Minute
	maxCodesPerHour = 5
)

var (
	ErrInvalidEmail    = errors.New("enter a valid email address")
	ErrInvalidCode     = errors.New("that code is wrong or has expired")
	ErrTooManyAttempts = errors.New("too many wrong codes - ask for a new one")
	ErrUnauthenticated = auth.ErrUnauthenticated
)

// TooSoonError: a code was sent moments ago, or too many this hour.
type TooSoonError struct {
	RetryAfter time.Duration
}

func (e *TooSoonError) Error() string {
	return fmt.Sprintf("please wait %d seconds before asking for another code", int(e.RetryAfter.Seconds()))
}

// Issue is one field-level problem with a profile update.
type Issue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string { return fmt.Sprintf("%d validation issue(s)", len(e.Issues)) }

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	auth    *auth.Authenticator
	mail    mail.Sender
	logger  *slog.Logger
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, sender mail.Sender, logger *slog.Logger) *Service {
	q := db.New(pool)
	return &Service{
		pool:    pool,
		queries: q,
		auth:    auth.New(customerAuthStore{q}, SessionTokenPrefix),
		mail:    sender,
		logger:  logger,
		now:     time.Now,
	}
}

// SendCode emails a fresh sign-in code. It answers the same way whether or
// not an account exists - one is created on first sign-in.
func (s *Service) SendCode(ctx context.Context, vendorID uuid.UUID, shopName, email string) error {
	email = auth.NormalizeEmail(email)
	if !looksLikeEmail(email) {
		return ErrInvalidEmail
	}

	now := s.now()
	latest, err := s.queries.LatestCustomerLoginCode(ctx, db.LatestCustomerLoginCodeParams{VendorID: vendorID, Email: email})
	if err == nil {
		if wait := latest.CreatedAt.Time.Add(resendInterval).Sub(now); wait > 0 {
			return &TooSoonError{RetryAfter: wait.Round(time.Second)}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	recent, err := s.queries.CountCustomerLoginCodesSince(ctx, db.CountCustomerLoginCodesSinceParams{
		VendorID: vendorID, Email: email, Since: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true},
	})
	if err != nil {
		return err
	}
	if recent >= maxCodesPerHour {
		return &TooSoonError{RetryAfter: 15 * time.Minute}
	}

	code, err := newCode()
	if err != nil {
		return err
	}
	if _, err := s.queries.CreateCustomerLoginCode(ctx, db.CreateCustomerLoginCodeParams{
		VendorID:  vendorID,
		Email:     email,
		CodeHash:  codeHash(vendorID, email, code),
		ExpiresAt: pgtype.Timestamptz{Time: now.Add(codeTTL), Valid: true},
	}); err != nil {
		return err
	}

	return s.mail.Send(ctx, mail.Message{
		To:      email,
		Subject: fmt.Sprintf("%s is your %s sign-in code", code, shopName),
		Text: fmt.Sprintf("Your sign-in code for %s is:\n\n    %s\n\nIt works for %d minutes. If you didn't ask for it, you can ignore this email.\n",
			shopName, code, int(codeTTL.Minutes())),
		HTML: fmt.Sprintf(`<p>Your sign-in code for %s is:</p><p style="font-size:28px;font-weight:bold;letter-spacing:6px">%s</p><p>It works for %d minutes. If you didn't ask for it, you can ignore this email.</p>`,
			htmlEscape(shopName), code, int(codeTTL.Minutes())),
	})
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	Account   db.CustomerAccount
}

// Verify checks a code and signs the customer in, creating their account on
// first sign-in and attaching the vendor's guest orders placed with this
// email.
func (s *Service) Verify(ctx context.Context, vendorID uuid.UUID, email, code, userAgent string) (LoginResult, error) {
	email = auth.NormalizeEmail(email)
	code = strings.TrimSpace(code)

	row, err := s.queries.LatestCustomerLoginCode(ctx, db.LatestCustomerLoginCodeParams{VendorID: vendorID, Email: email})
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidCode
	}
	if err != nil {
		return LoginResult{}, err
	}
	if row.ConsumedAt.Valid || !row.ExpiresAt.Time.After(s.now()) {
		return LoginResult{}, ErrInvalidCode
	}
	if row.Attempts >= codeMaxAttempts {
		return LoginResult{}, ErrTooManyAttempts
	}
	if subtle.ConstantTimeCompare(row.CodeHash, codeHash(vendorID, email, code)) != 1 {
		if err := s.queries.RecordCustomerLoginCodeAttempt(ctx, row.ID); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCode
	}
	used, err := s.queries.ConsumeCustomerLoginCode(ctx, row.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if used == 0 {
		return LoginResult{}, ErrInvalidCode // another request used it first
	}

	account, err := s.queries.UpsertCustomerAccount(ctx, db.UpsertCustomerAccountParams{VendorID: vendorID, Email: email})
	if err != nil {
		return LoginResult{}, fmt.Errorf("upsert account: %w", err)
	}
	if _, err := s.queries.LinkGuestOrdersToCustomer(ctx, db.LinkGuestOrdersToCustomerParams{
		CustomerID: pgtype.UUID{Bytes: account.ID, Valid: true}, VendorID: vendorID, Email: email,
	}); err != nil {
		s.logger.Error("link guest orders failed", "customer_id", account.ID, "error", err)
	}
	session, err := s.auth.IssueSession(ctx, account.ID, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: session.Token, ExpiresAt: session.ExpiresAt, Account: account}, nil
}

// Authenticate resolves a session token to its account.
func (s *Service) Authenticate(ctx context.Context, token string) (db.CustomerAccount, error) {
	hash, err := s.auth.TokenHash(token)
	if err != nil {
		return db.CustomerAccount{}, err
	}
	row, err := s.queries.GetActiveCustomerSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CustomerAccount{}, ErrUnauthenticated
	}
	if err != nil {
		return db.CustomerAccount{}, err
	}
	if err := s.auth.Touch(ctx, row.CustomerSession.ID, row.CustomerSession.LastSeenAt.Time); err != nil {
		return db.CustomerAccount{}, err
	}
	return row.CustomerAccount, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.auth.Logout(ctx, token)
}

// UpdateProfile sets the name and phone used to pre-fill checkout. Empty
// clears a field.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, name, phone *string) (db.CustomerAccount, error) {
	current, err := s.queries.GetCustomerAccount(ctx, id)
	if err != nil {
		return db.CustomerAccount{}, err
	}
	params := db.UpdateCustomerAccountParams{ID: id, Name: current.Name, Phone: current.Phone}
	var issues []Issue
	if name != nil {
		v := strings.TrimSpace(*name)
		switch {
		case v == "":
			params.Name = nil
		case len(v) > 100:
			issues = append(issues, Issue{"name", "Name must be 100 characters or fewer"})
		default:
			params.Name = &v
		}
	}
	if phone != nil {
		if strings.TrimSpace(*phone) == "" {
			params.Phone = nil
		} else if n, ok := store.NormalizePhone(*phone); ok {
			params.Phone = &n
		} else {
			issues = append(issues, Issue{"phone", "Enter a valid UK phone number, e.g. 07700 900123"})
		}
	}
	if len(issues) > 0 {
		return db.CustomerAccount{}, &ValidationError{Issues: issues}
	}
	return s.queries.UpdateCustomerAccount(ctx, params)
}

// OrdersPage is one page of a customer's orders, newest first.
type OrdersPage struct {
	Orders []db.ListCustomerOrdersRow
	Total  int64
}

func (s *Service) ListOrders(ctx context.Context, id uuid.UUID, page, pageSize int) (OrdersPage, error) {
	total, err := s.queries.CountCustomerOrders(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return OrdersPage{}, err
	}
	rows, err := s.queries.ListCustomerOrders(ctx, db.ListCustomerOrdersParams{
		CustomerID: pgtype.UUID{Bytes: id, Valid: true},
		PageLimit:  int32(pageSize),
		PageOffset: int32((page - 1) * pageSize),
	})
	if err != nil {
		return OrdersPage{}, err
	}
	return OrdersPage{Orders: rows, Total: total}, nil
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// codeHash binds a code to the vendor and email it was sent for.
func codeHash(vendorID uuid.UUID, email, code string) []byte {
	return crypto.HashAPIKey(vendorID.String() + "|" + email + "|" + code)
}

func looksLikeEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	return at > 0 && at < len(s)-1 && !strings.ContainsAny(s, " \t\n") &&
		strings.Contains(s[at+1:], ".") && len(s) <= 254
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// customerAuthStore adapts the customer tables to auth.Store. Customers
// never sign in with a password, so only the session methods do anything.
type customerAuthStore struct{ q *db.Queries }

func (customerAuthStore) FindAccount(context.Context, string) (auth.Account, error) {
	return auth.Account{}, auth.ErrNotFound
}
func (customerAuthStore) RecordFailure(context.Context, uuid.UUID, int32, time.Time) error {
	return nil
}
func (customerAuthStore) RecordSuccess(context.Context, uuid.UUID) error { return nil }
func (customerAuthStore) UpdatePasswordHash(context.Context, uuid.UUID, string) error {
	return nil
}

func (st customerAuthStore) CreateSession(ctx context.Context, s auth.NewSession) error {
	_, err := st.q.CreateCustomerSession(ctx, db.CreateCustomerSessionParams{
		CustomerID:        s.AccountID,
		TokenHash:         s.TokenHash,
		UserAgent:         s.UserAgent,
		ExpiresAt:         pgtype.Timestamptz{Time: s.ExpiresAt, Valid: true},
		AbsoluteExpiresAt: pgtype.Timestamptz{Time: s.AbsoluteExpiresAt, Valid: true},
	})
	return err
}

func (st customerAuthStore) TouchSession(ctx context.Context, sessionID uuid.UUID, expiresAt time.Time) error {
	return st.q.TouchCustomerSession(ctx, db.TouchCustomerSessionParams{
		ID: sessionID, ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

func (st customerAuthStore) RevokeSession(ctx context.Context, tokenHash []byte) error {
	return st.q.RevokeCustomerSession(ctx, tokenHash)
}
