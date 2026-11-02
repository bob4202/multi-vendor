# Featured filter and sorting: plan

Featured filter and sorting (see [`api.md`](api.md)). Split into backend and frontend phases.

## Where things stand

- `GET /v1/store/products` takes `category`, `q`, `page`, `page_size` (and `ids` for the cart lookup). It always sorts by `products.name`.
- "Featured" already exists as data: `branch_product_content.is_upsell` (migration `0003`, with a partial index `branch_product_content_upsell_idx`). Every product response already has `is_upsell`.
- **Nothing sets `is_upsell` except the seed** (`cmd/seed/main.go:395`). A real branch has no featured products, so a featured filter would come back empty until vendors can mark products (Phase 2).
- One query path serves three endpoints: `listBranchProducts` in `internal/httpapi/products.go:97`, called by the storefront (`storefront.go:73`), admin (`branches.go:385`) and vendor (`vendor_store.go:84`). Changing it once covers all three.

## Phase 1: Featured filter and sorting (backend)

### Contract

```
GET /v1/store/products?featured=true
GET /v1/store/products?sort=price_asc|price_desc|name_asc
```

| Param      | Values                                | Default                        | Invalid value                                                          |
| ---------- | ------------------------------------- | ------------------------------ | ---------------------------------------------------------------------- |
| `featured` | `true`, `false`                       | `false` (all products)         | `400 {"error": "featured must be true or false"}`                      |
| `sort`     | `name_asc`, `price_asc`, `price_desc` | `name_asc` (today's behaviour) | `400 {"error": "sort must be one of name_asc, price_asc, price_desc"}` |

- Both combine with `category`, `q` and pagination. `total_count`/`total_pages` must reflect the filter.
- `ids=` stays as it is: it ignores `featured` and `sort`.
- Response shape is unchanged.
- The admin and vendor product lists accept the same params for free (shared helper).

### Rules

- **Stable order for pagination.** Always end `ORDER BY` with `products.id`. Today it's only `products.name`, so two products with the same name can swap between pages, showing one twice and skipping the other. This fixes that too.
- **Price ties** fall back to name, then id.
- **Missing prices** (`price1` is a nullable `BIGINT`) go last in both price sorts (`NULLS LAST`).
- `featured=true` means `COALESCE(branch_product_content.is_upsell, false) = true` for this branch.

### Steps

**1. Queries** (`internal/db/queries/products.sql`)

sqlc can't bind a column name into `ORDER BY`, so use one `CASE` per sort option. Only the matching one produces values; the others are `NULL` for every row and have no effect.

`ListBranchProducts`: add to the `WHERE`:

```sql
    AND (NOT sqlc.arg (featured_only)::boolean
        OR COALESCE(branch_product_content.is_upsell, FALSE))
```

and replace the `ORDER BY`:

```sql
ORDER BY
    CASE WHEN sqlc.arg (sort)::text = 'price_asc'  THEN branch_products.price1 END ASC NULLS LAST,
    CASE WHEN sqlc.arg (sort)::text = 'price_desc' THEN branch_products.price1 END DESC NULLS LAST,
    products.name,
    products.id
```

`CountBranchProducts`: it has no join to `branch_product_content` yet. Add the join and the same `featured_only` condition, otherwise the totals are wrong:

```sql
    LEFT JOIN branch_product_content ON branch_product_content.branch_id = branch_products.branch_id
        AND branch_product_content.product_id = products.id
```

Then `make sqlc`.

**2. Parsing** (`internal/httpapi/products.go`)

`parseProductQuery` already returns four values. Rather than adding two more, group them in a struct:

```go
type productQuery struct {
	Category     *string
	Search       *string
	FeaturedOnly bool
	Sort         string // "name_asc" | "price_asc" | "price_desc"
	Page         int
	PageSize     int
}

func parseProductQuery(r *http.Request) (productQuery, error)
```

- `featured`: `""` → false; otherwise `strconv.ParseBool`, and an error → the 400 above.
- `sort`: `""` → `"name_asc"`; anything outside the three values → the 400 above. Keep the allowed values in one place (a `map[string]bool` or a `switch`); the SQL compares against the same strings.

**3. Shared helper**

Change `listBranchProducts(ctx, branchID, category, search, includeUnavailable, page, pageSize)` to `listBranchProducts(ctx, branchID, q productQuery, includeUnavailable)` and pass `FeaturedOnly` and `Sort` to both queries. Update the three callers; each now returns 400 when parsing fails.

**4. Tests**

The existing tests for the product endpoints are the place for these. Cover:

- `featured=true` returns only `is_upsell` products, and `total_count` matches.
- `sort=price_asc` and `price_desc` order correctly; a product with no price comes last in both.
- Two products with the same name don't repeat or disappear across pages.
- `sort=bogus` and `featured=maybe` → 400.
- No params → same result as today.

**5. Check with curl**

```sh
curl -H "Authorization: Bearer $STOREFRONT_KEY" "localhost:8080/v1/store/products?featured=true&page_size=4"
curl -H "Authorization: Bearer $STOREFRONT_KEY" "localhost:8080/v1/store/products?sort=price_desc&category=<id>"
```

## Phase 2: Vendors mark products as featured (backend)

Optional for the first release, but without it only seeded data is ever featured.

### Contract

```
PUT /v1/vendor/branches/{branchID}/products/{productID}/featured
{ "featured": true }
```

- **200:** the product (same shape as `GET …/products/{id}`), with the new `is_upsell`.
- **404:** the branch doesn't sell this product.
- **403:** the role can't do this.

### Steps

1. **Permission.** Add an `EditProducts` action in `internal/portal/authz.go` for owners (automatic) and managers. Staff stay read-only, matching `EditStoreInfo`.
2. **Query.** Write an upsert that leaves the other override columns alone:
   ```sql
   -- name: SetBranchProductFeatured :exec
   INSERT INTO branch_product_content (branch_id, product_id, is_upsell)
   VALUES ($1, $2, $3)
   ON CONFLICT (branch_id, product_id) DO UPDATE SET
       is_upsell = EXCLUDED.is_upsell,
       updated_at = now();
   ```
   Check the product is in `branch_products` for that branch first (→ 404). Otherwise you could insert content for a product the branch doesn't sell.
3. **Route.** Next to the vendor `GET /products`, add `r.With(requirePermission(portal.EditProducts)).Put("/products/{productID}/featured", …)`.
4. **Response.** Vendor responses add `can_edit_products` wherever the portal reads the user's capabilities, so the toggle can be hidden from staff. If there's no such field yet, the frontend can hide it by role instead. Note in the PR which you went with.

## Phase 3: Frontend

Once Phase 1 ships:

1. **API client** (`lib/external-api-service.ts`). Add `featured?: boolean` and `sort?: ProductSort` to `ProductsQuery`, and send them as `featured=true` / `sort=…`. Put `type ProductSort = "name_asc" | "price_asc" | "price_desc"` in `types/product.ts`.
2. **Shop sort dropdown** (`/shop`).
   - A shadcn `Select` next to the search box, with "Name (A–Z)", "Price: low to high" and "Price: high to low".
   - The choice lives in the URL as `?sort=`, like `category` and `q`. Changing it resets `page` to 1. Unknown values fall back to the default instead of erroring.
   - On phones it sits on the same row as the search box and stays usable at 320px.
3. **"You might also like"** (`components/product/related-products.tsx`).
   - Show featured products in the same category first.
   - If there aren't enough, fill up from the same category as today, deduplicated, without the product being viewed.
4. **Homepage** (`app/(storefront)/page.tsx`).
   - Today it lists the first 100 products.
   - Add a "Featured" row (`featured=true`, about 8 items) above it, with a link through to `/shop`. If nothing is featured, hide the row instead of showing an empty one.
5. **Vendor toggle** (after Phase 2).
   - A "Featured" switch on each row of the vendor products view, hidden from staff.
   - Add a `featured=true` filter chip to the list so vendors can see what's featured.
6. `npm run typecheck`, then screenshots from you.

## Done when

- [ ] Phase 1 shipped and checked with curl (code done; tests and curl left)
- [x] Phase 3 steps 1–4 shipped
- [ ] Phase 2 and the vendor toggle shipped
- [ ] README §3 marked done and the contract updated; Status lists the sort dropdown and featured row
