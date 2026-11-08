package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ecommerce/backend/internal/db"
)

var (
	// The URL's account doesn't exist, belongs to another provider, or has
	// no webhook: answer 404 so nobody can probe for accounts.
	ErrAccountNotFound = errors.New("payment account not found")
	// The provider adapter rejected the signature or the body: answer 400.
	ErrBadWebhook = errors.New("webhook rejected")
)

// HandleWebhook checks one webhook from a vendor's provider account and
// applies it. Duplicates, events we don't use and events for payments we
// don't know return nil, so the provider stops retrying them. Any other
// error means our database failed: answer 500 and let the provider retry.
func (s *Service) HandleWebhook(ctx context.Context, providerName string, accountID uuid.UUID, r *http.Request, body []byte) error {
	account, err := s.queries.GetVendorPaymentAccountCredentials(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAccountNotFound
	}
	if err != nil {
		return err
	}
	// Any status: a disabled account still settles the payments it took.
	if account.Provider != providerName || !s.providers.Has(account.Provider) || account.WebhookSecretEnc == nil {
		return ErrAccountNotFound
	}

	secretKey, err := s.secret.Decrypt(account.SecretKeyEnc)
	if err != nil {
		return fmt.Errorf("decrypt secret key for account %s: %w", account.ID, err)
	}
	webhookSecret, err := s.secret.Decrypt(account.WebhookSecretEnc)
	if err != nil {
		return fmt.Errorf("decrypt webhook secret for account %s: %w", account.ID, err)
	}
	provider, err := s.providers.Build(account.Provider, Credentials{
		AccountID:     account.ID,
		SecretKey:     secretKey,
		WebhookSecret: webhookSecret,
		Live:          account.Live,
	})
	if err != nil {
		return err
	}

	ev, ok, err := provider.ParseWebhook(r, body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadWebhook, err)
	}
	if !ok {
		return nil // an event we don't use
	}
	return s.applyEvent(ctx, account.ID, account.Provider, ev)
}

// applyEvent records the event and applies it in one transaction, with the
// payment locked. If anything fails, both roll back, so the provider's
// retry is processed instead of skipped as a duplicate.
func (s *Service) applyEvent(ctx context.Context, accountID uuid.UUID, providerName string, ev Event) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	p, err := q.LockPayment(ctx, ev.PaymentID)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Only the account a payment was made with can change it.
	if found && p.PaymentAccountID != accountID {
		s.logger.Warn("payment webhook from another account ignored",
			"payment_id", p.ID, "account_id", accountID, "event_id", ev.ID)
		found = false
	}

	payload := ev.Raw
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	inserted, err := q.InsertPaymentEvent(ctx, db.InsertPaymentEventParams{
		Provider:  providerName,
		EventID:   ev.ID,
		PaymentID: pgtype.UUID{Bytes: p.ID, Valid: found},
		Kind:      string(ev.Kind),
		Payload:   payload,
	})
	if err != nil {
		return err
	}
	if inserted == 0 {
		return nil // already handled
	}
	if !found {
		s.logger.Warn("payment webhook for unknown payment", "payment_id", ev.PaymentID, "event_id", ev.ID, "kind", ev.Kind)
		return tx.Commit(ctx)
	}

	switch ev.Kind {
	case EventAuthorised:
		err = s.applyAuthorised(ctx, tx, p, ev)
	case EventFailed:
		_, err = q.MarkPaymentFailed(ctx, db.MarkPaymentFailedParams{ID: p.ID})
	case EventExpired:
		_, err = q.MarkPaymentExpired(ctx, p.ID)
	case EventCaptured:
		amount := ev.Amount
		if amount <= 0 {
			amount = p.AmountAuthorised
		}
		_, err = q.MarkPaymentCaptured(ctx, db.MarkPaymentCapturedParams{ID: p.ID, Amount: amount})
	case EventVoided:
		_, err = q.MarkPaymentVoided(ctx, p.ID)
	case EventRefunded:
		_, err = q.SetPaymentRefundedTotal(ctx, db.SetPaymentRefundedTotalParams{ID: p.ID, RefundedTotal: ev.Amount})
	}
	// No row back = the event is stale or out of order for this payment's
	// status (see payments.sql): it's recorded, nothing else changes.
	if errors.Is(err, pgx.ErrNoRows) {
		s.logger.Info("payment webhook didn't apply", "payment_id", p.ID, "status", p.Status, "kind", ev.Kind)
		err = nil
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// applyAuthorised records the hold and sends the order to the shop with a
// fresh accept countdown. Money held for an order that can't take it (paid
// twice, or paid after it was dropped) is queued to be released.
func (s *Service) applyAuthorised(ctx context.Context, tx pgx.Tx, p db.Payment, ev Event) error {
	q := s.queries.WithTx(tx)
	amount := ev.Amount
	if amount <= 0 {
		amount = p.AmountAuthorised
	}
	var paymentRef *string
	if ev.PaymentRef != "" {
		paymentRef = &ev.PaymentRef
	}

	// A savepoint, because a unique violation would otherwise abort the
	// whole transaction, event row included.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = s.queries.WithTx(sp).MarkPaymentAuthorised(ctx, db.MarkPaymentAuthorisedParams{
		ID: p.ID, PaymentRef: paymentRef, Amount: amount,
	})
	switch {
	case isUniqueViolation(err, "payments_one_live_idx"):
		// Another attempt already paid for this order.
		_ = sp.Rollback(ctx)
		s.logger.Warn("second payment held for an order - voiding it", "payment_id", p.ID, "order_id", p.OrderID)
		return s.queueVoid(ctx, q, p.ID, paymentRef)
	case err != nil:
		_ = sp.Rollback(ctx)
		return err
	}
	if err := sp.Commit(ctx); err != nil {
		return err
	}

	order, err := q.MarkOrderPaid(ctx, db.MarkOrderPaidParams{
		ID: p.OrderID, AcceptMinutes: int32(s.acceptTimeout / time.Minute),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Paid after the order was dropped (or cancelled): give it back.
		s.logger.Warn("payment held for an order that's no longer awaiting payment - voiding it",
			"payment_id", p.ID, "order_id", p.OrderID)
		return s.queueVoid(ctx, q, p.ID, nil)
	}
	if err != nil {
		return err
	}

	from, note := "awaiting_payment", "Card held "+FormatPence(amount)
	if err := q.InsertOrderStatusEvent(ctx, db.InsertOrderStatusEventParams{
		OrderID: order.ID, FromStatus: &from, ToStatus: "pending", Note: &note, Kind: "status",
	}); err != nil {
		return err
	}
	s.logger.Info("order paid", "order_id", order.ID, "reference", order.Reference, "payment_id", p.ID)
	return nil
}

// queueVoid asks the jobs worker to release a hold. paymentRef is saved
// first when the payment row doesn't have it yet, since Void needs it.
func (s *Service) queueVoid(ctx context.Context, q *db.Queries, paymentID uuid.UUID, paymentRef *string) error {
	if paymentRef != nil {
		if _, err := q.SetPaymentRef(ctx, db.SetPaymentRefParams{ID: paymentID, PaymentRef: paymentRef}); err != nil {
			return err
		}
	}
	_, err := q.EnqueuePaymentJob(ctx, db.EnqueuePaymentJobParams{PaymentID: paymentID, Kind: "void"})
	return err
}

// FormatPence turns pence into "£5.00" for history notes.
func FormatPence(p int64) string {
	return fmt.Sprintf("£%d.%02d", p/100, p%100)
}
