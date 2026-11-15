# Weighed items

Plan for selling products priced by weight (the butcher counter): mince, diced meat, steaks, chicken. Decisions were made on 2026-11-11. Backend tickets come from this guide one at a time (see `tickets.md`); the frontend is built alongside.

## The idea in one paragraph

Every product has **one type**: a normal item, ordered by quantity like any online shop, or a **weighed** item, priced per kg and ordered in grams. The customer pays for exactly the weight they ordered, fixed at checkout. At the counter the butcher weighs **at least** that much (never less), types the real weight into the portal, and photographs the scale's printed receipt. The extra weight is the shop's giveaway: it never changes what the customer pays, but it's recorded so management can reconcile those losses later, with the photos as evidence.

## What changes for each person

**Customer.** A weighed product shows its price per kg ("£12.99/kg"). The customer picks a weight: 250 g, 500 g, 1 kg, or their own amount in steps. The price for that weight is shown and is **final**: it doesn't change after weighing. The order page can show the weight they were given ("500 g ordered, 512 g weighed").

**Butcher (vendor portal).** While picking, each weighed line gets a **Weigh** step: type the real weight, which must be **at least** the ordered weight. When the weighing is finished, the scale prints a **receipt listing the weighed products**; the butcher photographs it in the portal. The order can't be marked `ready` until every weighed line has a weight **and** the order has at least one receipt photo.

**Management (owners, managers).** A **weights report** shows, per day and branch, how much was given away (weighed above what was ordered) and what that cost, with links to the orders and their receipt photos. **Receipt photos are for management only; customers never see them.**

## The money rules

- A weighed line costs `price_per_kg × ordered_grams / 1000`, rounded to the penny. That's the line total at checkout, the card hold, and the **final charge**.
- **Weighing never changes the charge.** The real weight must be at least the ordered weight; the extra is the shop's giveaway. An entry below the ordered weight is refused ("Weigh at least 500 g").
- **Hard rule: never charge more than the authorised amount.** It holds automatically here, because weighing doesn't touch `charged_total`. Keep a safety check anyway in `QueueCaptureIfHeld`: if the order total is ever above the payment's `amount_authorised`, don't queue a bigger capture; log an error and return one, so a bug shows up straight away. Never "top up" a charge.
- Giveaway for the report: `price_per_kg × (actual_grams − ordered_grams) / 1000`.

## Which products are weighed

- Each product has one type: `item` or `weight`. It comes from the stock system's **price base** (per kg → `weight`, per item → `item`) on every sync.
- The shop can correct a product's type in the portal if the stock system has it wrong. It's still one type per product, and the correction survives syncs.
- Never name the stock system's supplier in the portal or the shop. Use "your stock system".

**To find out first:** which price base values mean "per kg". The product record only carries a price base id, so the sync needs the stock system's list of price bases (or a fixed mapping per shop). Check the stock system's API for a price base list before ticket W1.

## Data model

```
branch_products (per branch, synced; the correction survives syncs)
  + sold_by_stock     text        -- 'item' | 'weight', from the price base on every sync
  + sold_by_override  text NULL   -- the shop's correction; NULL = follow the stock system
  + min_grams         int         -- smallest weight a customer can order, default 100
  + step_grams        int         -- weight steps, default 50
    sold_by = coalesce(sold_by_override, sold_by_stock)
    price   = per kg when sold_by = 'weight'

order_items
  + sold_by       text NOT NULL DEFAULT 'item'
  + ordered_grams int  NULL       -- weighed lines: what the customer ordered
  + actual_grams  int  NULL       -- set by the butcher, >= ordered_grams
  + weighed_at    timestamptz NULL
  + weighed_by    uuid NULL REFERENCES vendor_users
    weighed lines: quantity = 1, unit_price = price per kg,
                   line_total = charged_total = the price for ordered_grams

order_receipts (new; management only)
  id, order_id, storage_key, content_type, size_bytes,
  uploaded_by (vendor user), created_at
```

Copies on the order line (`unit_price`, `ordered_grams`) are kept, like today, so later price changes don't rewrite old orders.

## Receipt photos

- **Storage:** an S3-compatible bucket (for example Cloudflare R2 or AWS S3 in production, MinIO in `docker-compose` locally). The bucket is **private**. The vendor API hands out short-lived links for viewing. The storefront API never returns them.
- **Upload:** the portal sends the photo to a Next route, which streams it to the API (multipart). The API checks the type (JPEG, PNG, HEIC/WebP) and size (≤ 10 MB) and stores it. Phones open the camera directly (`<input type="file" accept="image/*" capture="environment">`).
- At least one receipt per order with weighed lines, more allowed (two printouts, a retake). A receipt can be deleted only while the order hasn't been marked ready.

## API contract (draft)

```
Storefront
GET  /v1/store/products…        + sold_by: "item"|"weight", min_grams, step_grams
                                  price = per kg when sold_by = "weight"
POST /v1/store/orders           weighed products: items[] use grams instead of quantity
                                422 grams below min_grams / not a multiple of step_grams
GET  /v1/store/orders/{id}      items[] + sold_by, ordered_grams, actual_grams
                                (no receipts: management only)

Vendor
GET  …/orders/{orderId}         items[] + sold_by, ordered_grams, actual_grams
                                + receipts[]: { id, url (short-lived), uploaded_by, created_at }
                                next_statuses holds back "ready" until weighed + receipt
PUT  …/orders/{orderId}/items/{itemId}/weight   { grams }   (update_order_status)
                                → 200 order detail · 409 order not accepted/picking
                                · 422 grams below the ordered weight
POST …/orders/{orderId}/receipts       multipart "photo"  → 201 receipt
DELETE …/orders/{orderId}/receipts/{receiptId}             → 204 (not after ready)
POST …/orders/{orderId}/status  ready → 422 field status: "Weigh every item first" /
                                "Add a photo of the scale receipt first"
PUT  …/products/{productId}/sold-by  { sold_by: "item"|"weight"|null, min_grams, step_grams }
                                (edit_store_info; null = follow the stock system)

Reports (owners, managers)
GET  …/reports/weights?from&to  → per day: weighed lines, ordered_grams, actual_grams,
                                  giveaway_grams, giveaway_pence, orders[] with receipt links
```

Substitutions for weighed lines are out of scope at first: a weighed line can be **removed** (out of stock), not substituted. Revisit once the basics work.

## Checklist

**W** = weighed items. **B** = backend (you), **F** = frontend.

### Phase 1: know which products are weighed
- [ ] W1 B Price bases from the stock system → `sold_by_stock` on sync; the new `branch_products` columns
- [ ] W2 B `PUT …/products/{id}/sold-by`; `sold_by`, `min_grams`, `step_grams` on the vendor and store product responses
- [ ] W3 F Portal products page: show and correct each product's type, weight limits

### Phase 2: ordering by weight
- [ ] W4 B Order lines with grams: `POST /orders` validation, price for the weight, the new `order_items` columns
- [ ] W5 F Shop: "/kg" prices, weight picker, cart and checkout with weights
- [ ] W6 F Customer order page: ordered weight, and the weighed weight once known

### Phase 3: weighing at the counter
- [ ] W7 B `PUT …/items/{itemId}/weight` (at least the ordered weight; charge unchanged), history line; capture safety check against `amount_authorised`
- [ ] W8 B `ready` held back until every weighed line has a weight
- [ ] W9 F Portal: Weigh step on each weighed line while picking

### Phase 4: receipt photos
- [ ] W10 B Bucket config, `order_receipts`, upload / list / delete, short-lived links; MinIO in `docker-compose`
- [ ] W11 B `ready` also needs at least one receipt
- [ ] W12 F Portal: take or upload the receipt photo, see and retake it

### Phase 5: reconciliation
- [ ] W13 B Weights report endpoint
- [ ] W14 F Weights report page (per day, giveaway in grams and pounds, links to orders and receipts)

### Later
- [ ] Scan the scale label's barcode (many scales print the weight in it) instead of typing the weight
- [ ] Substitutions for weighed lines
- [ ] Terms update: weighed items are sold by the weight ordered; the shop gives at least that weight (with B24 in the payments guide)

## Open questions

- [ ] **Price bases:** how the stock system lists them (see "Which products are weighed").
- [ ] **Default weights** offered to customers: 250 g / 500 g / 1 kg for every product, or set per product?
- [ ] **Giveaway limit:** flag lines over a percentage (e.g. 10% over) in the report, or just show everything?
- [ ] **How long to keep receipt photos** (disputes and accounting usually need a few years; check what the business needs).
