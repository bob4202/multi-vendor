# Grocery Shop

Online ordering for a multi-branch grocery shop: a storefront per branch, a portal for shop staff and an admin side for running the platform. Products and prices sync from each branch's external inventory system.

| Part | Stack |
|---|---|
| `backend/` | Go, chi, Postgres, sqlc, goose |
| `frontend/` | Next.js 16, shadcn/ui, Tailwind v4 |

## Running locally

Needs Go, Node, Docker and `psql`.

```sh
make setup        # first time: .env files with generated secrets, deps, database, demo data
make dev          # Postgres, API (air live reload) and Next dev server
make reset        # wipe the local database and reseed fresh demo data (asks first)
make help         # all targets: env, migrate, seed, admin, sqlc, typecheck, ...
```

`make setup` creates `backend/.env` and `frontend/.env` from their `.env.example` files. It generates `APP_ENCRYPTION_KEY` and `ADMIN_API_KEY`, and points the local storefront at the demo Stratford branch (`BRANCH_ID`, `STOREFRONT_KEY`). It never overwrites a value you've already set, so it's safe to run again. Use `make env` to do just the `.env` part.

`make reset` deletes everything in the local database and seeds it again. It asks you to type the database name (`FORCE=1 make reset` skips that) and refuses any database that isn't on localhost. Branch IDs and keys change on a reset, so it updates `frontend/.env` too; restart `make dev` afterwards.

### Demo logins

After seeding, every login uses the password `shop-demo-2026` (or `SEED_VENDOR_PASSWORD` if it was set when seeding).

**Vendor portal** (`/vendor`):

| Login | Role | Sees |
|---|---|---|
| `owner@group.example` | owner | Stratford + Ilford (vendor "Grocery Shop Group") |
| `manager.stratford@group.example` | manager | Stratford |
| `staff.ilford@group.example` | staff | Ilford (orders only, store details read-only) |
| `owner@leeds.example` | owner | Leeds (separate vendor "Grocery Shop Leeds Ltd") |

The seed also prints a one-time invite link for `manager.leeds@leeds.example`, to try the invite flow at `/vendor/invite/<token>`.

**Admin side** (`/admin`): only exists where `ADMIN_PORTAL=enabled` is set in `frontend/.env`. Sign in as `admin@groceryshop.example`. For real admins, run `make admin EMAIL=you@example.com NAME="Your Name"`; there is no sign-up page.

**Emails** go to Mailpit locally: http://localhost:8025.

**Inventory sync** works through providers, like payments: each branch picks its inventory system in the admin side, and `INVENTORY_PROVIDERS` in `backend/.env` decides which ones the server offers. Demo branches use `fake` (no records, so "Sync now" works without a real system). `connect` also needs `INVENTORY_CONNECT_URL`, the API's base URL with `{host}` for each branch. A new system is one new package in `backend/internal/inventory/`.

**Card payments** use the `fake` provider locally (`PAYMENT_PROVIDERS=fake` in `backend/.env`), with its own Pay / Decline page.

## Features

**Storefront** (one deployment per branch)
- Shop with categories, search, sorting and a featured row; product pages with gallery and related products
- Cart per branch, with suggestions; checkout for same-day collection or delivery, prices re-checked by the API
- Customer accounts with email-code sign-in and order history
- Live order page (SSE): accept countdown, collection code, substitution answers, email and Web Push updates
- Card payment on the provider's hosted page; the shop only sees an order once the card is held

**Vendor portal** (`/vendor`)
- Owners, managers and staff with their own logins and roles
- Live order queue with accept countdown and new-order sound; status changes with full history
- Collection hand-over by code, substitutions, automatic no-shows
- Card payment status on each order, refunds (owners and managers), and a block on handing over an order whose card couldn't be charged
- Store details, opening hours and closures; products with inventory sync status and "Sync now"

**Platform admin** (`/admin`)
- Onboarding wizard: vendor, branch and inventory connection, store details, owner invite, storefront key, first sync
- Vendors, branches and people; overview of what needs attention (failed syncs, missing setup, stuck invites)
- Every admin action is recorded with who did it

## How it fits together

- Each branch's products sync on a schedule from its inventory system (through an `inventory.Provider`) into `products` (shared catalogue) and `branch_products` (per-branch price and availability). Nothing is ever written back.
- Each storefront deployment is one branch, set by `BRANCH_ID` and `STOREFRONT_KEY`. Only the Next server calls the API, so keys and session tokens never reach the browser.
- Orders wait for the shop to accept them (`ORDER_ACCEPT_TIMEOUT_MINUTES`, default 5), then move through picking and ready to completed. Background jobs handle timeouts, unpaid orders, substitutions and no-shows.
- Emails and push notifications go out from a `notification_outbox` table; payment capture, void and refund calls go through a `payment_jobs` queue.

## Docs

- [`backend/docs/api.md`](backend/docs/api.md): API contracts (storefront, vendor, admin, webhooks)
- [`backend/docs/payments-guide.md`](backend/docs/payments-guide.md): card payments plan and checklist
- [`backend/docs/orders-guide.md`](backend/docs/orders-guide.md), [`backend/docs/featured-sorting-plan.md`](backend/docs/featured-sorting-plan.md): earlier build plans

## Roadmap

- **Card payments:** what happens after a card can't be charged (pay again or cancel), early capture for far-off deliveries, then Stripe (see the payments guide)
- Vendors marking products as featured
- User management: adding, removing and disabling staff, changing roles; password reset by email
- Support for more inventory systems
- Logging and error tracking (Sentry, plus shipping the API's JSON logs)
- Platform billing: monthly fees and/or a per-order cut, payouts via Stripe Connect
- Product search by PLU or barcode
- `/categories` and `/about` pages
- Fix ESLint (eslint 10 doesn't work with the React plugin in eslint-config-next)

## License

Copyright (c) 2026 Thet Naung Soe. All rights reserved. The code is public for viewing only; see [LICENSE](LICENSE).
