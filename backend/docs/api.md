# API reference

Contracts between the Go API and the Next.js frontend. They follow the existing conventions: `/v1/store/*`, branch taken from the `Authorization: Bearer <storefront key>` header, snake_case, money in **pence**, 404 for not found. Only the Next server calls the API, so no CORS is needed.

## Batch product lookup

```
GET /v1/store/products?ids=<uuid>,<uuid>,...
```
- Same paginated response as the list endpoint, filtered to those IDs. At most 100 IDs; `page`/`page_size` are ignored.
- **Leave out** IDs the branch doesn't sell, has deleted or has marked unsellable.
- The frontend uses it to refresh cart prices and flag unavailable items on `/cart` and at checkout.

## Store info

```
GET /v1/store/branch
```
```json
{
  "name": "Grocery Shop – Leeds",
  "phone": "0113 000 0000",
  "address": { "line1": "...", "line2": null, "city": "Leeds", "postcode": "LS1 1AA" },
  "fulfilment": {
    "collection": { "enabled": true },
    "delivery": {
      "enabled": true,
      "fee": 350,
      "free_over": 4000,
      "min_order": 2000,
      "postcode_prefixes": ["LS1", "LS2"]
    }
  },
  "opening_hours": [{ "day": 1, "open": "09:00", "close": "18:00" }],
  "closures": [{ "date": "2026-12-25", "closed": true, "open": null, "close": null, "note": "Christmas Day" }]
}
```
`day` is ISO (1 = Monday … 7 = Sunday); a day with no entries is closed. Send `closures` for roughly the next 14 days. `free_over` is `null` when there's no free-delivery threshold.

Schema done: migration `0006_branch_info.sql` adds `branch_info`, `branch_opening_hours` and `branch_hours_exception`; queries are in `queries/branch_info.sql`, and the seed fills in both demo branches.

## Featured filter and sorting

```
GET /v1/store/products?featured=true                    # is_upsell = true
GET /v1/store/products?sort=price_asc|price_desc|name_asc
```
- Both combine with `category`, `q` and pagination; `total_count` reflects the filter. Defaults: all products, `name_asc`.
- Invalid values → `400 {"error": "featured must be true or false"}` / `{"error": "sort must be one of name_asc, price_asc, price_desc"}`.
- Ties sort by name then id, so pages are stable; products without a price come last in both price sorts.
- The admin and vendor product lists accept the same params. `ids=` ignores both.

Frontend: "You might also like" and homepage use featured products; the shop gets a sort dropdown.

Vendors can't mark products as featured yet (Phase 2 of [`featured-sorting-plan.md`](featured-sorting-plan.md)); until then only seeded products are featured, and the homepage row stays hidden.

## Orders

**Step-by-step guide: [`backend/docs/orders-guide.md`](orders-guide.md).** That file holds the full contract (it adds `expected_total` and the issue codes); the summary below is a quick reference.

```
POST /v1/store/orders
Idempotency-Key: <uuid>
```
```json
{
  "items": [{ "product_id": "uuid", "quantity": 2 }],
  "fulfilment": { "type": "delivery", "date": "2026-10-07" },
  "customer": { "name": "...", "email": "...", "phone": "..." },
  "address": { "line1": "...", "city": "...", "postcode": "..." },
  "notes": "optional"
}
```
- **201:** `{ "id", "reference": "TH-10234", "status": "pending", "items": [...], "subtotal", "delivery_fee", "total", "created_at" }`
- **422:** `{ "error": "...", "issues": [{ "product_id": "...", "code": "unavailable" | "price_changed" | "below_min_order" | "outside_delivery_area" }] }`

Rules:
- **Never trust prices from the client.** Read `branch_products.price1` for the key's branch, inside a transaction.
- Reject items that are deleted or have `can_be_sold` set to false.
- Repeating a request with the same `Idempotency-Key` returns the original order.
- Payment is card only, online at checkout (no paying in the shop). Plan: [`backend/docs/payments-guide.md`](payments-guide.md).

## Vendor portal

Multi-vendor: a **vendor** is a business that owns one or more branches. **Vendor users** log in to `/vendor`: owners see all of their vendor's branches; managers and staff see the branches they're assigned. The database enforces that a user can only be given their own vendor's branches (composite foreign keys in `0009_vendors.sql`). Vendors can't sign themselves up; the platform admin creates them.

| Role | Orders | Store details | Products / sync |
|---|---|---|---|
| owner | view, change status | edit | view, sync |
| manager | view, change status | edit | view, sync |
| staff | view, change status | view | view |

**Auth.** `POST /v1/vendor/auth/login {email, password}` returns `{token, expires_at, user, vendor}`. Everything else needs `Authorization: Bearer <token>`.
- Passwords are argon2id. Sessions are random tokens, stored as SHA-256.
- Sessions expire after 7 days idle (sliding) or 30 days at most. Logout revokes the token.
- 10 wrong passwords in a row lock the account for 15 min (429).
- Login errors never reveal whether an email exists.

The Next server keeps the token in an httpOnly `vendor_session` cookie (path `/vendor`). The browser never sees it.

**Vendor API** (branch-scoped routes return **404** for branches the user can't access, 403 when the role can't do it, 422 `{error, issues:[{field, message}]}` for validation):

```
POST /v1/vendor/auth/login                    POST /v1/vendor/auth/logout
GET  /v1/vendor/me                            → { user, vendor, branches[], permissions{update_order_status, edit_store_info, trigger_sync, refund_orders} }

GET  /v1/vendor/branches/{id}/orders?status=active|done|all&page&page_size
GET  /v1/vendor/branches/{id}/orders/{orderId}          → order + items + events[] + next_statuses[]
POST /v1/vendor/branches/{id}/orders/{orderId}/status   { status, expected_status, reason? }
GET|PUT /v1/vendor/branches/{id}/info                   same shape as /v1/store/branch (hours replaced wholesale)
GET|POST /v1/vendor/branches/{id}/closures              DELETE /v1/vendor/branches/{id}/closures/{closureId}
GET  /v1/vendor/branches/{id}/products | categories | sync-state      POST /v1/vendor/branches/{id}/sync
```

**Order lifecycle** (`internal/portal/orders.go`; the API sends `next_statuses`, so the UI never hard-codes it):

```
pending  → accepted | rejected (reason required) | cancelled
accepted → picking | cancelled
picking  → ready | cancelled
ready    → completed | no_show (collection) · out_for_delivery (delivery). No cancel: the money is taken at `ready`
out_for_delivery → completed
```

`expected_status` is optimistic locking: if someone else changed the order first, you get **409** and nothing changes. Every change is recorded in `order_status_events` with who made it.

**Invites** (public; the token in the URL is the credential): `GET /v1/vendor/invites/{token}` → `{email, name, vendor_name, expires_at}`, `POST /v1/vendor/invites/{token}/accept {password}` → same as login. Unknown tokens get 404; used, expired or replaced ones get 410. Links work once and last 7 days, and issuing a new one cancels the old one.

## Platform admin

The people who run the platform. Admin accounts live in their own table (`platform_admins`, migration `0010`) and use the same login rules as vendor users, from the shared `internal/auth` package: argon2id, lockout, and hashed sessions with idle and absolute expiry.

**Auth.** Every `/v1/admin/*` route accepts **either** an admin session (`Authorization: Bearer <token>` from `POST /v1/admin/auth/login`, which the admin UI uses) **or** the shared `X-Admin-Key` (for scripts and curl). Actions are written to `admin_audit_events` with the admin who did them; the key shows as "API key". A vendor session never works here.

```
POST /v1/admin/auth/login | /auth/logout          GET /v1/admin/me
GET  /v1/admin/overview        → { counts, attention[{kind, vendor_*, branch_*, detail, since}], recent_activity[] }
GET  /v1/admin/vendors?q=      → [{ id, slug, name, is_active, branch_count, user_count, pending_invites }]
POST /v1/admin/vendors         { slug, name }
GET  /v1/admin/vendors/{id}    → vendor + branches[{ has_store_details, storefront_api_key_prefix, last_sync_* }] + users[{ status: active|invited|invite_expired|locked|disabled }]
PATCH /v1/admin/vendors/{id}   { name?, is_active? }      deactivating signs the vendor's users out at once
POST /v1/admin/vendors/{id}/users                { email, name, role, password?, branch_ids? }
                               → { user, invite: { token, expires_at } | null }   no password = invite
POST /v1/admin/vendors/{id}/users/{userId}/invite → { token, expires_at }        new link, old one stops working
GET|PUT /v1/admin/branches/{id}/info              store details, same shape as the vendor portal
POST /v1/admin/branches        now requires vendor_id (plus the existing branch, key and sync routes)
                               + inventory_provider (optional on create: the server's only provider, or "connect")
                               422 field inventory_provider when the server doesn't offer it; 422 field host when
                               another branch uses the same host in the same system. Responses include inventory_provider.
GET  /v1/admin/inventory-providers → { providers: ["connect", "fake"] }   enabled by INVENTORY_PROVIDERS
```

Attention kinds, most urgent first: `sync_failed`, `account_locked`, `vendor_without_owner`, `invite_expired`, `invite_pending`, `no_store_details`, `no_storefront_key`, `never_synced`.

## Customer side

**Decisions**
- **Customer accounts belong to a vendor** (one per email per vendor), not to a branch or the whole platform. "My orders" shows all of that vendor's orders.
- **Login is a 6-digit email code** (10 min, 5 tries, one code a minute). No passwords. Guest checkout stays; email and phone are always required.
- **Collection is today only**, until the shop's *last orders* time (`last_order_minutes` before the day's last closing time). Delivery is unchanged for now.
- **Collection code**: 6 digits, created when the shop accepts a collection order. Staff check it before handing over. They can override with a reason if the customer lost it.
- **Substitutions**: staff propose a similar product; the customer must accept. The customer pays `min(original line, substitute)`, so the shop covers any increase. No answer within `SUBSTITUTION_TIMEOUT_MINUTES` (10) = the line is removed. The order can't be marked ready while an answer is pending. Checkout has "allow substitutions"; when off, unavailable items are just removed.
- **Collection window**: once ready, collect within 60 min (or by closing time if sooner). 6 min after that it becomes `no_show` automatically. No refund, per the terms accepted at checkout (`terms_version` + time are stored on the order).
- **Live updates**: Server-Sent Events. A Postgres trigger on `orders` sends `NOTIFY order_events`; every API instance listens and streams to its clients. Next route handlers relay the stream, so the storefront key and session tokens stay on the server.
- **Notifications**: a `notification_outbox` table (status changes are written by a trigger, substitutions by the service), worked through by one job using `FOR UPDATE SKIP LOCKED` (safe with several API instances). It sends email over SMTP (Mailpit locally) and Web Push (VAPID).
- **Card payments (later)**: hold the total at checkout, capture the final amount at `ready`, release the hold if the order is cancelled before that. A `ready` order can't be cancelled (fresh goods, labour done); any refund is the store's choice. Substitutions only ever lower the total, so a partial capture always works. Plan: [`backend/docs/payments-guide.md`](payments-guide.md).

**Storefront API additions** (all with the storefront key; a signed-in customer also sends `X-Customer-Session: <token>`)

```
POST  /v1/store/auth/code      { email }              → 202 {} · 422 bad email · 429 { error, retry_after }
POST  /v1/store/auth/verify    { email, code }        → 200 { token, expires_at, customer } · 401 wrong/expired · 429 too many tries
POST  /v1/store/auth/logout                           → 204
GET   /v1/store/me                                    → customer { id, email, name, phone }
PATCH /v1/store/me             { name?, phone? }      → customer · 422 issues
GET   /v1/store/me/orders?page&page_size              → Paginated<{ id, reference, status, branch_name, fulfilment, item_count, total, created_at, needs_action }>

GET   /v1/store/branch         + fulfilment.collection.last_order_minutes
                               + collection_today: { date, last_order_at, closes_at, accepting } | null
                                 (null = closed today or collection off; accepting = false once last orders have passed)
POST  /v1/store/orders         + terms_version (required), allow_substitutions (default true)
                               collection: date must be today and before last_order_at (422 code "ordering_closed")
                               phone: UK number, stored as E.164 (422 field customer_phone)
GET   /v1/store/orders/{id}    order + branch_name, allow_substitutions, collection_code (accepted..ready, collection only),
                               ready_at, collect_by, awaiting_customer, and per item:
                               id, status ok|awaiting_customer|substituted|removed, charged_total,
                               substitution { id, product_id, name, quantity, unit_price, line_total, status, expires_at } | null
GET   /v1/store/orders/{id}/events                    → text/event-stream, "event: order" + the order JSON on connect and on every change
POST  /v1/store/orders/{id}/substitutions/{subId}  { accept } → order · 409 already answered/expired
GET   /v1/store/push/config                           → { public_key }   (VAPID; "" = push not configured)
POST  /v1/store/push-subscriptions { subscription: { endpoint, keys: { p256dh, auth } }, order_id? } → 204
DELETE /v1/store/push-subscriptions { endpoint }      → 204
```
A signed-in customer can also open their orders from the vendor's other branches by ID.

**Vendor API additions**

```
GET|PUT /v1/vendor/branches/{id}/info      + fulfilment.collection.last_order_minutes (0-480)
GET  …/orders/lookup?code=123456           → { data: [order summary] } (open collection orders only) · 404
POST …/orders/{orderId}/status             + collection_code | code_override_reason   (required for ready → completed on collection)
                                           422 field collection_code if wrong; ready is refused (422) while awaiting_customer
POST …/orders/{orderId}/items/{itemId}/unavailable  { substitute: { product_id, quantity } | null } → order detail
DELETE …/orders/{orderId}/items/{itemId}/substitution                                            → order detail (withdraw a proposal)
GET  …/events                              → text/event-stream, "event: order" { id, reference, status, op } for the branch
```
Order summary and detail gain `awaiting_customer`, `collect_by`; detail also `allow_substitutions`, `ready_at`, `requires_collection_code` (staff never see the code itself), and items gain the same `id/status/charged_total/substitution` fields as the storefront. `next_statuses` leaves out `ready` while an answer is pending. Events gain `kind` (`status` | `substitution` | `removed` | `code_override` | `payment`; `payment` events don't change the status, so `from_status` = `to_status`, and the note says what happened).

**Admin API addition**: branches get `storefront_url` (used for links in emails).

**Config (backend `.env`)**: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM`, `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT`, `SUBSTITUTION_TIMEOUT_MINUTES` (10), `COLLECTION_WINDOW_MINUTES` (60), `COLLECTION_GRACE_MINUTES` (6). Mailpit runs in `docker-compose` (SMTP 1025, inbox http://localhost:8025).

## Card payments

Card is the only way to pay. Each vendor brings their own Stripe (later Dojo) keys; the total is held at checkout, captured at `ready` and released if the order is cancelled before then. A `ready` order can't be cancelled, so the money is kept; the store can choose to refund part of it. Full plan, data model and checklist: [`backend/docs/payments-guide.md`](payments-guide.md). Built so far: taking a hold, releasing it when an order is rejected, cancelled or not accepted in time, and capturing the total (after substitutions) at `ready`. Store-issued refunds can be requested through the vendor API; the Refund button and Stripe are next.

**Storefront API** (storefront key)

```
GET  /v1/store/branch          + payments: { enabled }   (false = no active payment account: checkout is closed)
POST /v1/store/orders          → 201 with status "awaiting_payment" and payment_deadline; the shop can't see it yet
                               503 "This shop isn't taking online orders right now…" when payments are off
POST /v1/store/orders/{id}/payment-session  { success_url, cancel_url }  (http(s) URLs)
                               → 201 { redirect_url, expires_at }; calling it again while a session is open returns the same one
                               404 not found · 409 not awaiting payment, payment window closed, or another request is creating it
                               503 no active payment account
GET  /v1/store/orders/{id}     + payment_deadline (awaiting_payment only) | null
                               + payment: { status, amount_authorised, amount_captured, amount_refunded } | null
                                 status: created | authorised | failed | expired | captured | voided
                                         | partially_refunded | refunded | capture_failed
GET  /v1/store/me/orders       needs_action = true while awaiting_payment too
```
The storefront sends the customer back to `/orders/{id}?payment=return` (paid) or `?payment=cancelled` (backed out). The order page shows "Confirming your payment" until the webhook moves the order to `pending`.

**Webhooks** (no storefront key; signed by the provider)

```
POST /v1/webhooks/{provider}/{account_id}   → 200 once stored (also duplicates and events we don't use)
                                            404 unknown account / provider mismatch · 400 bad signature · 500 our DB failed (provider retries)
```
`authorised` → payment `authorised`, order `awaiting_payment` → `pending` with a fresh `accept_deadline` (history: "Card held £x"). Money held for an order that was already paid or cancelled is queued to be voided.

**Vendor API**: order detail, the order list and lookup by code gain `payment` (same shape; `null` for orders from before card payments). `/me` permissions gain `refund_orders` (owners and managers). Orders that were never paid (`awaiting_payment`, or cancelled with `rejection_reason = "payment_not_completed"`) never appear in vendor lists, detail or the live feed.

```
POST /v1/vendor/branches/{id}/orders/{orderId}/refunds    (owner, manager)
  { "amount": 500, "reason": "Damaged item" }      amount in pence
  → 202 order detail (refund queued; the worker gives the money back a few seconds later)
  404 order not found (or not in this branch)
  409 the money hasn't been taken yet (cancel the order instead)
  422 field amount: 0 or less, or more than is left to refund (refunds still queued count as used)
  422 field reason: empty
```
```
POST /v1/vendor/branches/{id}/orders/{orderId}/status
  409 "the card couldn't be charged for this order - don't hand it over"
      (status completed or out_for_delivery while the payment is capture_failed; no_show is still allowed)
```
An order can be refunded more than once, up to what was captured. Each request adds a history event with `kind` `payment` ("Refund of £5.00 requested: Damaged item"); the payment becomes `partially_refunded` or `refunded` when the job finishes.

**Jobs**: unpaid orders are cancelled after `payment_deadline` with `rejection_reason = "payment_not_completed"` (the customer gets an email saying so). When an order with a card hold is rejected or cancelled by a vendor or an admin, or isn't accepted in time, a `void` job is queued in the same transaction as the status change. Marking an order `ready` queues a `capture` of its total the same way. A worker runs the jobs, retrying after 1, 3 and 5 minutes. If a `capture` gives up (the hold expired or the card was closed, or it ran out of retries), the job is marked `failed`, the payment `capture_failed` and the history gets a `payment` line ("Couldn't take £x from the card. Don't hand the order over."), all in one transaction. The order stays `ready` and can't be handed over; what happens next is still to be decided (PAY-20).

**Local development**: `PAYMENT_PROVIDERS=fake` in `backend/.env`. `make seed` gives every vendor an active `fake` account. The fake payment page lives on the API at `/fake-pay/{ref}` (only routed when `fake` is enabled); Pay and Decline send a signed webhook through the real webhook route.

**Config (backend `.env`)**: `PAYMENT_PROVIDERS` (required, e.g. `fake` or `stripe`), `PAYMENT_WINDOW_MINUTES` (30).
