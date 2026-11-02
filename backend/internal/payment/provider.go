package payment

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Provider interface {
	Name() string // stripe, dojo and etc....

	CreateSession(ctx context.Context, in SessionInput) (Session, error)
	Capture(ctx context.Context, ref string, amount int64, idemKey string) error

	Void(ctx context.Context, ref string, idemKey string) error
	Refund(ctx context.Context, ref string, amount int64, idemKey string) (refundRef string, err error)

	ParseWebhook(r *http.Request, body []byte) (ev Event, ok bool, err error)
}

type SessionInput struct {
	PaymentID  uuid.UUID
	Amount     int64  // amount in pence
	Currency   string // gbp
	Reference  string // shown on the card statement
	Email      string
	SuccessURL string
	CancelURL  string
	ExpiresAt  time.Time
}

type Session struct {
	ProviderRef string //Payment Provider Reference
	RedirectURL string
}

type EventKind string

const (
	EventAuthorised EventKind = "authorised"
	EventFailed     EventKind = "failed"
	EventExpired    EventKind = "expired"
	EventCaptured   EventKind = "captured"
	EventVoided     EventKind = "voided"
	EventRefunded   EventKind = "refunded"
)

type PaymentKind string

// This must match EXACTLY with the db const and you can check it in the 0012_payments.sql Line 84:
const (
	StatusCreated           PaymentKind = "created"
	StatusAuthorised        PaymentKind = "authorised"
	StatusFailed            PaymentKind = "failed"
	StatusExpired           PaymentKind = "expired"
	StatusCaptured          PaymentKind = "captured"
	StatusVoided            PaymentKind = "voided"
	StatusPartiallyRefunded PaymentKind = "partially_refunded"
	StatusRefunded          PaymentKind = "refunded"
	StatusCaptureFailed     PaymentKind = "capture_failed"
)

var (
	ErrNotCapturable   = errors.New("payment: not capturable")
	ErrRefundTooLarge  = errors.New("payment: refund too large")
	ErrAlreadyCaptured = errors.New("payment: already captured")
	ErrNotRefundable   = errors.New("payment: not refundable")
)

type Event struct {
	ID          string // provider's event id, for dedupe
	Kind        EventKind
	PaymentID   uuid.UUID // ours, sent to the provider as metadata
	ProviderRef string    // which session/intent
	PaymentRef  string    // the charge reference Capture/Void/Refund need
	Amount      int64
	Raw         []byte
}

type Credentials struct {
	AccountID     uuid.UUID // vendor_payment_accounts.id
	SecretKey     string
	WebhookSecret string
	Live          bool
}

type Factory func(c Credentials) (Provider, error)
type Registry map[string]Factory

type AccountInfo struct {
	DisplayName string // business name from the provider
	Live        bool   // live key or test key
}

type Connector interface {
	VerifyCredentials(ctx context.Context) (AccountInfo, error)
	RegisterWebhook(ctx context.Context, url string) (webhookID, secret string, err error)
	DeleteWebhook(ctx context.Context, webhookID string) error
}
