# Payments: where we are

Handoff notes for picking up the payments work on another machine. The plan and full checklist are in [`payments-guide.md`](payments-guide.md); this file is the short "what's done, what's next" on top of it.

Last updated: 2026-10-31.

## How we work on this

- Work goes ticket by ticket, each reviewed before merging.
- One small step at a time, each with a "done check" (`go build ./...`, a test, or a curl).

## Setting up the other laptop

1. Commit and push everything here first (most of B6 and B7 is uncommitted), then pull there.
2. `.env` is gitignored. Add to `backend/.env` there:
   ```
   PAYMENT_PROVIDERS=fake
   PAYMENT_WINDOW_MINUTES=30
   ```
   and make sure `APP_ENCRYPTION_KEY` is set (the seed encrypts the fake keys with it).
3. Tools: `mise use -g go@1.27` and `mise use -g sqlc@1.31.1` (sqlc's version is pinned; a Go upgrade can lose the sqlc binary, so reinstall it after one).
4. Migrations changed (`0011` trigger now sends `old_status`): reset the dev DB, then `make seed`. The seed gives every vendor an active `fake` payment account (`cmd/seed/payments.go`).
5. Done check: `go build ./... && go test ./internal/payment/...`, then start the API and look for `"payment providers","enabled":["fake"]` in the log.

## Done

- **B1–B6**: migration, queries, `internal/payment` with the fake provider, `awaiting_payment` hidden from vendors, state rules, `PlaceOrder` creating `awaiting_payment` orders (503 when the vendor has no active account).
- **B7** (2026-10-31): `payment.Service.StartCheckout` (`checkout.go`, split into `StartCheckout` + `newSession`) and `POST /v1/store/orders/{orderID}/payment-session` (`httpapi/payments.go`).
- **B8**: `payment.Service.HandleWebhook` (`webhook.go`): event dedupe + payment lock in one transaction; `authorised` moves the order to `pending` with a fresh accept deadline; money held for an order already paid or dropped queues a `void` job. Route `POST /v1/webhooks/{provider}/{accountID}` (`httpapi/webhooks.go`). Fake Pay / Decline page at `/fake-pay/{ref}` (`httpapi/fakepay.go`, `payment/fakepay.go`), only routed when `fake` is enabled; it sends signed webhooks through the real route.
- **B9**: `store.ExpireUnpaid` in `RunExpiry`; email wording for `payment_not_completed`. Orders dropped unpaid are hidden from vendor lists, detail and the vendor SSE feed.
- **B10**: `payment` + `payment_deadline` on storefront orders, `payment` on vendor order detail, `payments.enabled` on `/store/branch`, `needs_action` while awaiting payment.
- **F1–F3**: checkout goes to the payment page and keeps the cart until paid; the order page shows confirming / not finished / declined states with "Pay now" / "Try again"; checkout is closed when `payments.enabled` is false; terms updated (`TERMS_VERSION` 2026-10-31).
- Design decision for the open point: the session ends at `payment_deadline`. A provider with a longer minimum (Stripe: 30 min) clamps it in its adapter, and a payment that arrives after the order was dropped is voided by the webhook.

## Test it (step 7)

1. `backend/.env`: `PAYMENT_PROVIDERS=fake`, `PAYMENT_WINDOW_MINUTES=30`; `make seed` if the vendor has no `fake` account.
2. Place an order in the storefront → you land on `http://localhost:8080/fake-pay/fake_<id>`.
3. **Pay** → back on the order page: "Confirming your payment", then "Waiting for the shop" and the vendor screen beeps.
4. Try **Decline** ("Your card was declined" + Try again), **Back to the shop** (Pay now reuses the same session), and leaving an order unpaid past the window (cancelled, email in Mailpit).

## Next: phase 3 (capture, void, refund)

- [ ] **B11** `payment_jobs` worker. `void` jobs are already being queued (late or double payments) and nothing runs them yet.
- [ ] **B12** Queue `capture` at `ready`, `void` on reject/cancel/accept timeout, `refund` on cancel after `ready`, in the same transaction as the status change.
- [ ] B13–B15, F4–F5: see `payments-guide.md`.

## Optional tidy-ups (suggested, not required)

- Rename `PaymentKind` → `Status`, `store.AwaitingPayment` → `StatusAwaitingPayment`, fake `subtotal` → `capturedAmount`.
- `state.go`: validate refund amounts once at the top, and simplify the `Refunded <= 0` branch. The webhook relies on the SQL guards in `payments.sql`; `NextStatus` isn't called yet.
- An integration test for the webhook (duplicate event, late payment → void) with the fake provider.
