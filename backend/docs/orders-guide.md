# Orders: step-by-step guide

You build it; this guide tells you **what** to build and **how to know it's right**, not the code. Each step ends with a **Checkpoint**. Stop there and get it reviewed before moving on; finding a mistake in step 2 is far cheaper than finding it in step 6.

> Hints are written as questions on purpose. Try to answer them before asking.

---

## What you're building

`POST /v1/store/orders`: the storefront sends a cart and customer details, and the API checks everything against the database and saves an order.

**The golden rule:** the client tells you _what_ it wants (product IDs and quantities). The server decides _what it costs_. Never read a price, total or fee from the request.

### Contract (the frontend is built against this, so don't change it without updating the frontend)

Request headers: `Authorization: Bearer <storefront key>`, `Idempotency-Key: <uuid>`

```json
{
  "items": [{ "product_id": "uuid", "quantity": 2 }],
  "fulfilment": { "type": "collection" | "delivery", "date": "2026-10-07" },
  "customer": { "name": "...", "email": "...", "phone": "..." },
  "address": { "line1": "...", "line2": null, "city": "...", "postcode": "..." },
  "notes": "optional",
  "expected_total": 1850
}
```

- `address` is required only for delivery.
- `expected_total` is the total the customer saw, in pence. It's optional. If you get it and it doesn't match your calculation, reject with `price_changed`, so nobody is charged an amount they didn't see.

| Outcome                                      | Status | Body                                                                             |
| -------------------------------------------- | ------ | -------------------------------------------------------------------------------- |
| Created                                      | `201`  | the order (shape below)                                                          |
| Same `Idempotency-Key` sent again, same body | `200`  | the **original** order                                                           |
| Same key, **different** body                 | `409`  | `{ "error": "..." }`                                                             |
| Business rule failed                         | `422`  | `{ "error": "...", "issues": [{ "code", "product_id"?, "field"?, "message" }] }` |
| Malformed JSON, missing header               | `400`  | `{ "error": "..." }`                                                             |

Order response:

```json
{
  "id": "uuid",
  "reference": "TH-10001",
  "status": "pending",
  "fulfilment": { "type": "delivery", "date": "2026-10-07" },
  "items": [
    {
      "product_id": "uuid",
      "name": "Lamb Chops",
      "unit_price": 1400,
      "quantity": 2,
      "line_total": 2800
    }
  ],
  "subtotal": 2800,
  "delivery_fee": 399,
  "total": 3199,
  "created_at": "..."
}
```

Issue `code` values (the frontend shows a different message for each, so use exactly these):

| code                     | meaning                                                  | carries                     |
| ------------------------ | -------------------------------------------------------- | --------------------------- |
| `unavailable`            | product not sold by this branch right now                | `product_id`                |
| `invalid_quantity`       | quantity outside 1–99                                    | `product_id`                |
| `price_changed`          | `expected_total` ≠ your total                            | —                           |
| `fulfilment_unavailable` | branch doesn't offer that type                           | `field: "fulfilment.type"`  |
| `invalid_date`           | past, too far ahead, or branch closed that day           | `field: "fulfilment.date"`  |
| `outside_delivery_area`  | postcode's outward code not in the branch's list         | `field: "address.postcode"` |
| `below_min_order`        | subtotal < `delivery_min_order`                          | —                           |
| `required`               | missing field (name, email, phone, address for delivery) | `field`                     |

---

## Step 1: Migration `0007_orders.sql`

Create two tables, `orders` and `order_items`.

**`orders` needs to hold:** which branch, a human-readable reference, status, fulfilment type and date, customer name/email/phone, the delivery address (nullable), notes, subtotal/delivery fee/total, the idempotency key plus something to detect "same key, different body", and timestamps.

**`order_items` needs to hold:** which order, which product, and a **snapshot** of name, PLU and unit price at the moment of ordering, plus quantity and line total.

Think about:

- Why snapshot the name and price instead of joining to `products` later? (What happens to last month's receipts when you change a price tomorrow?)
- `status`: which values does a butcher's order go through? Constrain it with a `CHECK`. Do the same for fulfilment type.
- **Reference:** `TH-10001`, `TH-10002`, ... Which Postgres feature gives you a gap-tolerant, concurrency-safe counter?
- **Idempotency:** what should be `UNIQUE` so the database itself refuses a second order with the same key from the same branch? Why scope it per branch and not globally?
- How do you store "the body this key was first used with" compactly? (Hint: you don't need the whole body.)
- Money: use the same type as `branch_products.price1`.
- Can you enforce "delivery orders must have an address" with a `CHECK`?
- `ON DELETE` for `order_items.order_id` vs `order_items.product_id`: should deleting a product delete people's order history?
- Remember `-- +goose Up` / `-- +goose Down`, the `set_updated_at()` trigger from 0006, and that **saving the file applies it** (the API runs goose on startup under air).

**Checkpoint 1:** `make migrate` is clean, `\d orders` looks right, and the SQL has been reviewed.

---

## Step 2: Queries `queries/orders.sql`, then `make sqlc`

You need roughly these. Name them yourself:

- insert an order (returning the row)
- insert one order item
- get an order by branch and idempotency key
- get an order by branch and ID (for the confirmation page)
- list an order's items
- get the next reference number

You **don't** need a new product query: `ListBranchProductsByID` already returns price, name and PLU for sellable products only.

Think about:

- Why must "get order by ID" also filter by branch?
- Bulk-inserting items vs one insert per item: which is simpler with sqlc, and does it matter for a cart of at most 50 lines?

**Checkpoint 2:** `make sqlc && go build ./...` passes.

---

## Step 3: Pure rules `internal/store/fulfilment.go`, written test-first

These are plain functions with **no database and no HTTP**, so you can unit-test them in milliseconds. Write the tests first (table-driven, `fulfilment_test.go`), then make them pass.

1. **Outward code from a postcode:** `"e15 1ng"`, `"E151NG"` and `" E15 1NG "` should all give `E15`. Then a membership check against the branch's prefixes.
   - Test: `LS1` must **not** match `LS10 4AB`. Why does "strip the last 3 characters" work for every UK postcode?
2. **Delivery fee** from subtotal and branch info: the fee, `0` when `delivery_free_over` is set and the subtotal reaches it, and the fee when it's `NULL`.
   - Test the boundary: is "free over £50" ≥ or >? Pick one, write it in a comment, and test exactly £50.
3. **Is the branch open on a date?** Given weekly hours, exceptions and a date: an exception for that date wins; otherwise any weekly row for that ISO weekday means open.
   - Tests: a normal Monday, a Sunday with no rows, Christmas closed, an exception with special hours on a normally-closed day.
4. **Is the date allowed?** Not in the past and at most 14 days ahead, both measured **in the branch's timezone**.
   - Why does the timezone matter for an order placed at 23:30 UTC in summer?
   - Make "now" an argument rather than calling `time.Now()` inside, so tests control it.

**Checkpoint 3:** `go test ./internal/store/` passes with at least 12 cases.

---

## Step 4: The service `internal/store/order.go`

One exported method does the work, something like `PlaceOrder(ctx, input) (order, created bool, err)`. The handler only translates HTTP to and from this.

Design the input and output types yourself, keeping them free of `net/http` and of JSON quirks. Define:

- a **validation error type** that holds a list of issues (the handler turns it into 422), and
- a **sentinel error** for an idempotency conflict (409).

Order of operations. Think about why each step comes where it does:

1. **Shape checks**, no DB: at least one item, at most 50 lines, quantity 1–99, required fields, address present for delivery. Merge duplicate product IDs by adding up their quantities. Collect **all** issues; don't stop at the first.
2. **Idempotency lookup:** an existing order for this branch and key means either "return it with created=false" or "conflict", depending on the request fingerprint.
3. **Branch info, hours and exceptions:** is the fulfilment type enabled, and is the date allowed and open (step 3 functions)?
4. **Products:** `ListBranchProductsByID`. Any requested ID missing, or with a nil price, is `unavailable`.
5. **Money:** line totals, subtotal; for delivery, area check, minimum order and fee; total. Then compare with `expected_total` if it was given.
6. If there are any issues, return the validation error. **Nothing has been written yet.**
7. **Transaction:** get a reference, insert the order, insert the items, commit. Use `pool.Begin` and `queries.WithTx(tx)`, with `defer tx.Rollback(ctx)`.
8. **The race:** two identical requests arrive at the same moment, both pass step 2, and one insert hits the unique constraint. Detect that specific Postgres error (which SQLSTATE?) and fall back to step 2's "return the existing order". Don't return a 500.

Think about:

- Why do steps 1–6 happen _before_ the transaction?
- The service needs the pool (for transactions) and `*db.Queries`. How does `sync.NewEngine` get its dependencies? Copy that pattern.
- Where does "now" come from, so tests can control it?

**Checkpoint 4:** `go build ./...` passes and `order.go` has been reviewed _before_ you write the handler.

---

## Step 5: Handler and routes

- `POST /v1/store/orders`: under the existing `/store` group, so `requireStorefrontKey` already gives you the branch.
- `GET /v1/store/orders/{orderID}`: for the confirmation page, scoped to the branch.

Handler responsibilities only:

- Cap the body size (`http.MaxBytesReader`, 64 KB is plenty) and reject unknown JSON fields (`DisallowUnknownFields`).
- Require the `Idempotency-Key` header (400 if missing; also limit its length).
- Map errors: validation → 422 with issues, conflict → 409, anything else → **log it and return a generic 500**. Don't `err.Error()` a database error to the client (step 2a from the hardening list; use the `internalError` helper if you've made it).
- 201 if created, 200 if replayed.
- Wire the service through `NewRouter` and `cmd/api/main.go`.

**Checkpoint 5:** these curl calls behave as in the contract table:

1. A valid collection order → 201 with a `TH-` reference.
2. The same request again with the same key → 200, **same** order ID.
3. Same key, different quantity → 409.
4. An unknown product ID → 422 `unavailable`.
5. Delivery to `SW1A 1AA` (Stratford) → 422 `outside_delivery_area`.
6. Delivery with Ilford's key → 422 `fulfilment_unavailable`.
7. Delivery under £20 → 422 `below_min_order`.
8. A wrong `expected_total` → 422 `price_changed`.
9. `date` = 25 December → 422 `invalid_date`.
10. No `Idempotency-Key` header → 400.

---

## Step 6: Integration test `internal/store/order_integration_test.go`

Follow `internal/sync/dedup_integration_test.go`: skip when `DATABASE_URL` is unset, and create your own branch, products and branch info so the test doesn't depend on seed data.

Cover at least:

- the happy path, with totals worked out from **database** prices
- replaying the same key returns the same order, with exactly one row in the database
- **concurrent** replay: fire the same request from 2 goroutines and assert exactly one order exists (this proves step 4.8)
- one unavailable item rejects the whole order and writes nothing

**Checkpoint 6:** `make test` passes. Then the frontend checkout gets wired to the real endpoint.

---

## Out of scope for today

Card payments, order emails, admin order management, editing or cancelling orders, stock decrements. Status starts at `pending`, and staff handle the rest for now. Note anything you're tempted to add in `README.md` under **Later** instead of building it.
