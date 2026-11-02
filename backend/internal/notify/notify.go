// Package notify sends customers their order updates by email and Web Push.
// Work comes from notification_outbox: rows are written in the same
// transaction as the change they announce (status changes by a trigger,
// substitutions by the order service), so nothing is lost, and rows are
// claimed with SKIP LOCKED, so several API instances never send one twice.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce/backend/internal/db"
	"ecommerce/backend/internal/mail"
	"ecommerce/backend/internal/push"
	"ecommerce/backend/internal/store"
)

const (
	maxAttempts  = 5
	pollInterval = 15 * time.Second
)

type Worker struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	mail    mail.Sender
	push    *push.Sender // nil = push off
	wake    <-chan struct{}
	logger  *slog.Logger
}

func NewWorker(pool *pgxpool.Pool, sender mail.Sender, pusher *push.Sender, wake <-chan struct{}, logger *slog.Logger) *Worker {
	return &Worker{pool: pool, queries: db.New(pool), mail: sender, push: pusher, wake: wake, logger: logger}
}

// Run sends due notifications until ctx ends: straight away when woken,
// and on a slow poll for retries.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		for {
			n, err := w.sendBatch(ctx)
			if err != nil && ctx.Err() == nil {
				w.logger.Error("send notifications failed", "error", err)
			}
			if n == 0 || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) sendBatch(ctx context.Context) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := w.queries.WithTx(tx)

	rows, err := q.ClaimDueNotifications(ctx)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		sendErr := w.deliver(ctx, row)
		switch {
		case sendErr == nil:
			err = q.MarkNotificationSent(ctx, db.MarkNotificationSentParams{ID: row.ID})
		case row.Attempts+1 >= maxAttempts:
			w.logger.Error("giving up on notification", "id", row.ID, "kind", row.Kind, "order_id", row.OrderID, "error", sendErr)
			msg := sendErr.Error()
			err = q.MarkNotificationSent(ctx, db.MarkNotificationSentParams{ID: row.ID, LastError: &msg})
		default:
			w.logger.Warn("notification failed, will retry", "id", row.ID, "kind", row.Kind, "error", sendErr)
			backoff := time.Duration(1<<row.Attempts) * time.Minute
			msg := sendErr.Error()
			err = q.MarkNotificationRetry(ctx, db.MarkNotificationRetryParams{
				ID: row.ID, LastError: &msg,
				NextAttemptAt: pgtype.Timestamptz{Time: time.Now().Add(backoff), Valid: true},
			})
		}
		if err != nil {
			return 0, err
		}
	}
	return len(rows), tx.Commit(ctx)
}

// deliver sends one notification. Only email failures are retried; push is
// best effort and only tried the first time, so a retry never repeats it.
func (w *Worker) deliver(ctx context.Context, row db.NotificationOutbox) error {
	orderRow, err := w.queries.GetOrderByID(ctx, row.OrderID)
	if err != nil {
		return fmt.Errorf("load order: %w", err)
	}
	order, err := store.LoadOrder(ctx, w.queries, orderRow)
	if err != nil {
		return fmt.Errorf("load order lines: %w", err)
	}
	branch, err := w.queries.GetOrderNotificationContext(ctx, order.BranchID)
	if err != nil {
		return fmt.Errorf("load branch: %w", err)
	}
	var detail map[string]any
	_ = json.Unmarshal(row.Detail, &detail)

	n, ok := compose(row.Kind, order, detail, branch)
	if !ok {
		return nil // nothing to tell the customer about this one
	}

	if row.Attempts == 0 && w.push != nil {
		w.sendPush(ctx, order, n.push)
	}
	if n.email == nil {
		return nil
	}
	n.email.To = order.CustomerEmail
	return w.mail.Send(ctx, *n.email)
}

func (w *Worker) sendPush(ctx context.Context, order store.Order, msg push.Message) {
	var customerID pgtype.UUID
	if order.CustomerAccountID != nil {
		customerID = pgtype.UUID{Bytes: *order.CustomerAccountID, Valid: true}
	}
	subs, err := w.queries.ListPushSubscriptionsForOrder(ctx, db.ListPushSubscriptionsForOrderParams{
		OrderID: order.ID, CustomerID: customerID,
	})
	if err != nil {
		w.logger.Error("load push subscriptions failed", "order_id", order.ID, "error", err)
		return
	}
	for _, sub := range subs {
		err := w.push.Send(ctx, push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, msg)
		switch {
		case errors.Is(err, push.ErrGone):
			_ = w.queries.DeletePushSubscriptionByID(ctx, sub.ID)
		case err != nil:
			w.logger.Warn("push failed", "order_id", order.ID, "subscription_id", sub.ID, "error", err)
		default:
			_ = w.queries.MarkPushSubscriptionSuccess(ctx, sub.ID)
		}
	}
}

type notification struct {
	email *mail.Message
	push  push.Message
}

// compose writes the message for one kind of change. ok is false when the
// customer doesn't need telling (e.g. "picking").
func compose(kind string, o store.Order, detail map[string]any, b db.GetOrderNotificationContextRow) (notification, bool) {
	loc := time.UTC
	if b.Timezone != nil {
		if l, err := time.LoadLocation(*b.Timezone); err == nil {
			loc = l
		}
	}
	clock := func(t time.Time) string { return t.In(loc).Format("15:04") }
	path := "/orders/" + o.ID.String()
	link := ""
	if b.StorefrontUrl != nil {
		link = strings.TrimRight(*b.StorefrontUrl, "/") + path
	}
	shop := b.BranchName
	ref := o.Reference
	isCollection := o.FulfilmentType == "collection"

	var subject, pushTitle, pushBody string
	var lines []string
	switch kind {
	case "status_accepted":
		subject = fmt.Sprintf("Order %s confirmed", ref)
		pushTitle, pushBody = "Order confirmed", fmt.Sprintf("%s is preparing %s.", shop, ref)
		lines = append(lines, fmt.Sprintf("Good news - %s has accepted your order %s.", shop, ref))
		if isCollection && o.CollectionCode != nil {
			lines = append(lines,
				fmt.Sprintf("Your collection code is %s. Show it at the counter when you collect.", *o.CollectionCode),
				"We'll let you know as soon as it's ready.")
		}
	case "status_rejected":
		subject = fmt.Sprintf("Order %s couldn't be accepted", ref)
		pushTitle, pushBody = "Order declined", fmt.Sprintf("%s couldn't take %s.", shop, ref)
		lines = append(lines, fmt.Sprintf("Sorry - %s couldn't take your order %s.", shop, ref))
		if o.RejectionReason != nil {
			lines = append(lines, "Reason: "+*o.RejectionReason)
		}
		lines = append(lines, "You haven't been charged.")
	case "status_cancelled":
		subject = fmt.Sprintf("Order %s cancelled", ref)
		pushTitle, pushBody = "Order cancelled", fmt.Sprintf("%s was cancelled.", ref)
		switch {
		case o.RejectionReason != nil && *o.RejectionReason == "payment_not_completed":
			lines = append(lines, fmt.Sprintf("Your order %s was cancelled because payment wasn't completed in time.", ref),
				"If you still want it, please place a new order.")
		case o.RejectionReason != nil && *o.RejectionReason == "not_accepted_in_time":
			lines = append(lines, fmt.Sprintf("%s didn't confirm your order %s in time, so it was cancelled automatically.", shop, ref))
		case o.RejectionReason != nil:
			lines = append(lines, fmt.Sprintf("Your order %s was cancelled: %s.", ref, *o.RejectionReason))
		default:
			lines = append(lines, fmt.Sprintf("Your order %s was cancelled.", ref))
		}
		lines = append(lines, "You haven't been charged.")
	case "status_ready":
		if !isCollection {
			return notification{}, false
		}
		subject = fmt.Sprintf("Order %s is ready to collect", ref)
		pushTitle = "Ready to collect"
		pushBody = fmt.Sprintf("%s is ready at %s.", ref, shop)
		lines = append(lines, fmt.Sprintf("Your order %s is ready at %s.", ref, shop))
		if o.CollectBy != nil {
			pushBody = fmt.Sprintf("Collect %s by %s.", ref, clock(*o.CollectBy))
			lines = append(lines, fmt.Sprintf("Please collect it by %s. Orders not collected in time are cancelled and not refunded.", clock(*o.CollectBy)))
		}
		if o.CollectionCode != nil {
			pushBody += " Code " + *o.CollectionCode + "."
			lines = append(lines, fmt.Sprintf("Your collection code is %s.", *o.CollectionCode))
		}
		lines = append(lines, fmt.Sprintf("Total to pay: %s.", money(o.Total)))
	case "status_out_for_delivery":
		subject = fmt.Sprintf("Order %s is on its way", ref)
		pushTitle, pushBody = "On its way", fmt.Sprintf("%s is out for delivery.", ref)
		lines = append(lines, fmt.Sprintf("Your order %s is out for delivery.", ref))
	case "status_no_show":
		subject = fmt.Sprintf("Order %s wasn't collected", ref)
		pushTitle, pushBody = "Order not collected", fmt.Sprintf("%s wasn't collected in time.", ref)
		lines = append(lines,
			fmt.Sprintf("Your order %s wasn't collected in time, so it has been closed.", ref),
			"As set out in the terms you accepted at checkout, uncollected orders aren't refunded.",
			fmt.Sprintf("If something went wrong, please call %s%s.", shop, phoneSuffix(b.BranchPhone)))
	case "substitution_proposed":
		item, sub := findSubstitution(o, detail)
		if sub == nil {
			return notification{}, false
		}
		subject = fmt.Sprintf("%s: please approve a substitute", ref)
		pushTitle = "Please approve a substitute"
		pushBody = fmt.Sprintf("%s is out of stock. Accept %s instead?", item.Name, sub.Name)
		lines = append(lines,
			fmt.Sprintf("%s is out of stock for your order %s.", item.Name, ref),
			fmt.Sprintf("The shop suggests %s × %d instead, for %s (you never pay more than you ordered).", sub.Name, sub.Quantity, money(sub.LineTotal)),
			fmt.Sprintf("Please accept or decline by %s. If we don't hear from you, the item is removed from your order.", clock(sub.ExpiresAt)))
	case "item_removed":
		item := findItem(o, detail)
		if item == nil {
			return notification{}, false
		}
		why := "is unavailable"
		if detail["reason"] == "no_answer" {
			why = "was removed because we didn't hear back about the substitute"
		}
		subject = fmt.Sprintf("%s: an item was removed", ref)
		pushTitle = "Item removed"
		pushBody = fmt.Sprintf("%s %s. New total %s.", item.Name, why, money(o.Total))
		lines = append(lines,
			fmt.Sprintf("%s × %d %s, so it's been taken off your order %s.", item.Name, item.Quantity, why, ref),
			fmt.Sprintf("Your new total is %s.", money(o.Total)))
	default:
		return notification{}, false
	}

	if link != "" {
		lines = append(lines, "", "View your order: "+link)
	}
	text := strings.Join(lines, "\n\n") + "\n\n" + shop + "\n"
	return notification{
		email: &mail.Message{Subject: subject, Text: text, HTML: toHTML(lines, link, shop)},
		push:  push.Message{Title: pushTitle, Body: pushBody, URL: path, Tag: "order-" + o.ID.String()},
	}, true
}

func findItem(o store.Order, detail map[string]any) *store.OrderItem {
	id, _ := detail["item_id"].(string)
	for i := range o.Items {
		if o.Items[i].ID.String() == id {
			return &o.Items[i]
		}
	}
	return nil
}

func findSubstitution(o store.Order, detail map[string]any) (*store.OrderItem, *store.Substitution) {
	id, _ := detail["substitution_id"].(string)
	subID, err := uuid.Parse(id)
	if err != nil {
		return nil, nil
	}
	for i := range o.Items {
		if s := o.Items[i].Substitution; s != nil && s.ID == subID && s.Status == "proposed" {
			return &o.Items[i], s
		}
	}
	return nil, nil // answered or withdrawn before we got to it
}

func money(pence int64) string {
	return fmt.Sprintf("£%d.%02d", pence/100, pence%100)
}

func phoneSuffix(phone *string) string {
	if phone == nil || *phone == "" {
		return ""
	}
	return " on " + *phone
}

func toHTML(lines []string, link, shop string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family:system-ui,sans-serif;font-size:15px;line-height:1.5;color:#111">`)
	for _, l := range lines {
		if l == "" || strings.HasPrefix(l, "View your order: ") {
			continue
		}
		fmt.Fprintf(&b, "<p>%s</p>", escape(l))
	}
	if link != "" {
		fmt.Fprintf(&b, `<p><a href="%s" style="display:inline-block;padding:10px 16px;background:#111;color:#fff;border-radius:8px;text-decoration:none">View your order</a></p>`, escape(link))
	}
	fmt.Fprintf(&b, `<p style="color:#666">%s</p></div>`, escape(shop))
	return b.String()
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
