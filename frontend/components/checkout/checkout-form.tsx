"use client"

import {
  startTransition,
  useActionState,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { CircleAlert, Info, Loader2, ShoppingBag } from "lucide-react"

import {
  placeOrder,
  refreshCart,
  type PlaceOrderState,
} from "@/app/(storefront)/checkout/actions"
import { useCart } from "@/components/cart/cart-provider"
import { CartSuggestions } from "@/components/cart/cart-suggestions"
import { CheckoutSection } from "@/components/checkout/checkout-section"
import { CollectionToday } from "@/components/checkout/collection-today"
import { DateChips } from "@/components/checkout/date-chips"
import { FulfilmentPicker } from "@/components/checkout/fulfilment-picker"
import { MobileOrderBar } from "@/components/checkout/mobile-order-bar"
import { CheckboxField } from "@/components/common/checkbox-field"
import { EmptyState } from "@/components/common/empty-state"
import { FormField } from "@/components/common/form-field"
import { Price } from "@/components/common/price"
import { SwipeToConfirm } from "@/components/common/swipe-to-confirm"
import { OrderLineItem } from "@/components/order/order-line-item"
import { PriceBreakdown } from "@/components/order/price-breakdown"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import {
  deliveryFee,
  deliversTo,
  looksLikePostcode,
  upcomingOpenDays,
} from "@/lib/fulfilment"
import { rememberPayingOrder } from "@/lib/cart"
import { formatPrice, formatTime } from "@/lib/format"
import { TERMS_VERSION } from "@/lib/customer/constants"
import type { BranchInfo } from "@/types/branch"
import type { Customer } from "@/types/customer"
import type { FulfilmentType, OrderIssue } from "@/types/order"

const IDLE: PlaceOrderState = { status: "idle" }

function CheckoutForm({
  branch,
  customer,
}: {
  branch: BranchInfo
  customer: Customer | null
}) {
  const router = useRouter()
  const { items, ready, totalPrice, refreshProducts, removeItem } = useCart()
  const { collection, delivery } = branch.fulfilment
  // Collection is same-day only, until the shop's last orders.
  const today = branch.collection_today
  const collectionOpen = collection.enabled && !!today?.accepting

  const [type, setType] = useState<FulfilmentType>(
    collectionOpen || !delivery.enabled ? "collection" : "delivery"
  )
  const days = useMemo(() => upcomingOpenDays(branch), [branch])
  const [date, setDate] = useState(days[0]?.date ?? "")
  const [allowSubstitutions, setAllowSubstitutions] = useState(true)
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [postcode, setPostcode] = useState("")
  // One key per checkout visit: a double-click or a retry after a network
  // blip sends the same key, so the API returns the same order, not two.
  const [idempotencyKey] = useState(() => crypto.randomUUID())

  const [state, formAction, pending] = useActionState(placeOrder, IDLE)
  const formRef = useRef<HTMLFormElement>(null)

  // Re-check prices/availability with the shop as checkout opens.
  const [checked, setChecked] = useState(false)
  const [unavailable, setUnavailable] = useState<Set<string>>(new Set())
  const [pricesChanged, setPricesChanged] = useState(false)
  const checking = useRef(false) // refreshProducts changes `items`; don't refetch for that
  useEffect(() => {
    if (!ready || checked || checking.current || items.length === 0) return
    checking.current = true
    const before = new Map(items.map((i) => [i.product.id, i.product.price]))
    refreshCart(items.map((i) => i.product.id))
      .then((fresh) => {
        const freshIds = new Set(fresh.map((p) => p.id))
        setUnavailable(
          new Set(
            items.map((i) => i.product.id).filter((id) => !freshIds.has(id))
          )
        )
        setPricesChanged(fresh.some((p) => before.get(p.id) !== p.price))
        refreshProducts(fresh)
      })
      .catch(() => {}) // the API re-checks on submit anyway
      .finally(() => {
        checking.current = false
        setChecked(true)
      })
  }, [ready, checked, items, refreshProducts])

  // Order placed: off to the card payment page. The cart stays until the
  // order page sees the order paid, in case the customer backs out.
  useEffect(() => {
    if (state.status !== "ok") return
    rememberPayingOrder(state.orderId)
    if (state.redirectUrl) window.location.assign(state.redirectUrl)
    else router.replace(`/orders/${state.orderId}`)
  }, [state, router])

  // A price changed between checking and submitting - fetch again.
  useEffect(() => {
    if (
      state.status === "error" &&
      state.issues.some((i) => i.code === "price_changed")
    ) {
      setChecked(false)
    }
  }, [state])

  const issues: OrderIssue[] = state.status === "error" ? state.issues : []
  const fieldError = (field: string) =>
    issues.find((i) => i.field === field && !i.product_id)?.message
  const itemIssue = (productId: string) =>
    issues.find((i) => i.product_id === productId)?.message
  const generalIssues = issues.filter((i) => !i.field && !i.product_id)

  if (!ready) return <CheckoutSkeleton />
  if (items.length === 0 && state.status !== "ok") {
    return (
      <EmptyState
        icon={ShoppingBag}
        title="Your basket is empty"
        description="Add something before checking out."
        action={
          <Button nativeButton={false} render={<Link href="/shop" />}>
            Start shopping
          </Button>
        }
      />
    )
  }

  const available = items.filter((i) => !unavailable.has(i.product.id))
  const subtotal = available.reduce(
    (sum, i) => sum + (i.product.price ?? 0) * i.quantity,
    0
  )
  const isDelivery = type === "delivery"
  const fulfilmentDate = isDelivery ? date : (today?.date ?? "")
  const canFulfil = isDelivery ? !!date : collectionOpen
  const fee = isDelivery ? deliveryFee(branch, subtotal) : 0
  const total = subtotal + fee
  const belowMin = isDelivery && subtotal < delivery.min_order
  const postcodeHint =
    isDelivery && looksLikePostcode(postcode)
      ? deliversTo(branch, postcode)
        ? "✓ We deliver to you"
        : "Sorry, we don't deliver here yet - collection is available."
      : undefined

  const placing = pending || state.status === "ok"
  const canSubmit =
    available.length > 0 && !belowMin && canFulfil && termsAccepted

  // Rendered twice: in the summary card from md up, and in the bar pinned
  // to the bottom on phones. Both follow the same state, so whichever is
  // submitted carries the same answer.
  const termsField = (id: string, className?: string) => (
    <CheckboxField
      id={id}
      name="terms"
      className={className}
      checked={termsAccepted}
      onCheckedChange={setTermsAccepted}
      error={fieldError("terms")}
      label={
        <>
          I agree to the{" "}
          <Link
            href="/terms"
            target="_blank"
            className="underline underline-offset-4"
          >
            terms
          </Link>
        </>
      }
      description={
        isDelivery
          ? undefined
          : "Including collecting within an hour of being told it's ready. Uncollected orders aren't refunded."
      }
    />
  )

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    // Submitted by hand (not <form action>) so React doesn't clear the
    // fields if the shop sends back problems to fix.
    const formData = new FormData(event.currentTarget)
    startTransition(() => formAction(formData))
  }

  return (
    <form
      ref={formRef}
      onSubmit={onSubmit}
      // Bottom padding on phones keeps the end of the page clear of the
      // pinned MobileOrderBar.
      className="grid grid-cols-1 items-start gap-6 max-md:pb-64 lg:grid-cols-[minmax(0,1fr)_380px] lg:gap-8"
    >
      <input type="hidden" name="idempotency_key" value={idempotencyKey} />
      <input type="hidden" name="expected_total" value={total} />
      <input type="hidden" name="terms_version" value={TERMS_VERSION} />
      <input
        type="hidden"
        name="items"
        value={JSON.stringify(
          available.map((i) => ({
            product_id: i.product.id,
            quantity: i.quantity,
          }))
        )}
      />

      <div className="flex min-w-0 flex-col gap-4">
        <CheckoutSection step={1} title="How would you like your order?">
          <FulfilmentPicker
            value={type}
            onChange={setType}
            error={fieldError("fulfillment_type")}
            options={[
              {
                value: "collection",
                title: "Collect today",
                description: !collection.enabled
                  ? "Not available from this shop"
                  : collectionOpen
                    ? `Order by ${formatTime(today!.last_order_at)}`
                    : "Closed for today",
                disabled: !collectionOpen,
              },
              {
                value: "delivery",
                title: "Delivery",
                description: delivery.enabled
                  ? delivery.free_over !== null
                    ? `Free on orders over £${(delivery.free_over / 100).toFixed(0)}`
                    : "Local delivery"
                  : "Not available from this shop",
                disabled: !delivery.enabled,
              },
            ]}
          />
        </CheckoutSection>

        <CheckoutSection
          step={2}
          title={isDelivery ? "Delivery day" : "Collection"}
          description="The shop confirms your order within a few minutes."
        >
          {!isDelivery ? (
            <>
              <input
                type="hidden"
                name="fulfilment_date"
                value={fulfilmentDate}
              />
              <CollectionToday
                today={today}
                address={
                  branch.address.line1
                    ? `${branch.address.line1}${branch.address.postcode ? `, ${branch.address.postcode}` : ""}`
                    : undefined
                }
              />
              {fieldError("fulfillment_date") && (
                <p className="text-xs text-destructive">
                  {fieldError("fulfillment_date")}
                </p>
              )}
            </>
          ) : days.length > 0 ? (
            <DateChips
              days={days}
              value={date}
              onChange={setDate}
              error={fieldError("fulfillment_date")}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              The shop has no open days coming up. Please check back soon.
            </p>
          )}
        </CheckoutSection>

        <CheckoutSection
          step={3}
          title="Your details"
          description={
            customer ? undefined : (
              <>
                <Link
                  href="/account/login?next=/checkout"
                  className="font-medium text-foreground underline underline-offset-4"
                >
                  Sign in
                </Link>{" "}
                to fill these in and see your orders later.
              </>
            )
          }
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField
              id="name"
              label="Full name"
              error={fieldError("customer_name")}
              className="sm:col-span-2"
            >
              <Input
                id="name"
                name="name"
                autoComplete="name"
                required
                defaultValue={customer?.name ?? ""}
                aria-invalid={!!fieldError("customer_name")}
              />
            </FormField>
            <FormField
              id="email"
              label="Email"
              error={fieldError("customer_email")}
              hint="For your order updates"
            >
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                required
                defaultValue={customer?.email ?? ""}
                aria-invalid={!!fieldError("customer_email")}
              />
            </FormField>
            <FormField
              id="phone"
              label="Phone"
              error={fieldError("customer_phone")}
              hint="UK number, in case the shop needs to reach you"
            >
              <Input
                id="phone"
                name="phone"
                type="tel"
                autoComplete="tel"
                required
                defaultValue={customer?.phone ?? ""}
                aria-invalid={!!fieldError("customer_phone")}
              />
            </FormField>
          </div>
        </CheckoutSection>

        {isDelivery && (
          <CheckoutSection step={4} title="Delivery address">
            <div className="grid gap-4 sm:grid-cols-2">
              <FormField
                id="postcode"
                label="Postcode"
                error={fieldError("postcode")}
                hint={postcodeHint}
              >
                <Input
                  id="postcode"
                  name="postcode"
                  autoComplete="postal-code"
                  value={postcode}
                  onChange={(e) => setPostcode(e.target.value.toUpperCase())}
                  required
                  aria-invalid={!!fieldError("postcode")}
                />
              </FormField>
              <FormField
                id="city"
                label="Town / city"
                error={fieldError("address_city")}
              >
                <Input
                  id="city"
                  name="city"
                  autoComplete="address-level2"
                  required
                  aria-invalid={!!fieldError("address_city")}
                />
              </FormField>
              <FormField
                id="line1"
                label="Address line 1"
                error={fieldError("address_line_1")}
                className="sm:col-span-2"
              >
                <Input
                  id="line1"
                  name="line1"
                  autoComplete="address-line1"
                  required
                  aria-invalid={!!fieldError("address_line_1")}
                />
              </FormField>
              <FormField
                id="line2"
                label="Address line 2"
                optional
                className="sm:col-span-2"
              >
                <Input id="line2" name="line2" autoComplete="address-line2" />
              </FormField>
            </div>
          </CheckoutSection>
        )}

        <CheckoutSection
          step={isDelivery ? 5 : 4}
          title="Anything we should know?"
        >
          <CheckboxField
            id="allow_substitutions"
            name="allow_substitutions"
            checked={allowSubstitutions}
            onCheckedChange={setAllowSubstitutions}
            label="If something's out of stock, offer me a similar item"
            description={
              allowSubstitutions
                ? "We'll ask you first, and you never pay more than you ordered. If you don't answer in time, the item is removed."
                : "Out-of-stock items will just be removed from your order."
            }
          />
          <FormField
            id="notes"
            label="Order notes"
            optional
            hint="E.g. cut preferences or a parking note"
          >
            <Textarea id="notes" name="notes" rows={3} maxLength={500} />
          </FormField>
        </CheckoutSection>

        <CartSuggestions title="Forgotten something?" />
      </div>

      <Card className="min-w-0 lg:sticky lg:top-20">
        <CardContent className="flex flex-col gap-4">
          <h2 className="font-semibold">Order summary</h2>

          {pricesChanged && (
            <Alert>
              <Info />
              <AlertDescription>
                Some prices have changed since you added them. Totals below are
                up to date.
              </AlertDescription>
            </Alert>
          )}

          <ul className="-my-1 divide-y">
            {items.map((item) => {
              const gone = unavailable.has(item.product.id)
              const warning = gone
                ? "No longer available"
                : itemIssue(item.product.id)
              return (
                <OrderLineItem
                  key={item.product.id}
                  name={item.product.name ?? "Product"}
                  quantity={item.quantity}
                  unitPrice={item.product.price}
                  lineTotal={
                    item.product.price === null
                      ? null
                      : item.product.price * item.quantity
                  }
                  warning={warning}
                />
              )
            })}
          </ul>
          {unavailable.size > 0 && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                unavailable.forEach((id) => removeItem(id))
                setUnavailable(new Set())
              }}
            >
              Remove unavailable items
            </Button>
          )}

          <PriceBreakdown
            subtotal={subtotal}
            delivery={isDelivery ? fee : undefined}
            total={total}
          />

          {belowMin && (
            <p className="text-xs text-muted-foreground">
              Delivery needs a minimum order of{" "}
              <Price pence={delivery.min_order} />. Add{" "}
              <Price pence={delivery.min_order - subtotal} /> more, or choose
              collection.
            </p>
          )}

          {termsField("terms", "max-md:hidden")}

          {state.status === "error" && (
            <Alert variant="destructive" className="animate-in fade-in-0">
              <CircleAlert />
              <AlertTitle>{state.message}</AlertTitle>
              {generalIssues.length > 0 && (
                <AlertDescription>
                  {generalIssues.map((i) => (
                    <p key={i.code}>{i.message}</p>
                  ))}
                </AlertDescription>
              )}
            </Alert>
          )}

          <Button
            type="submit"
            size="lg"
            className="w-full max-md:hidden"
            disabled={placing || !canSubmit}
          >
            {placing ? (
              <>
                <Loader2 className="animate-spin" />
                Opening payment…
              </>
            ) : (
              <>
                Continue to payment · <Price pence={total} />
              </>
            )}
          </Button>
          <p className="text-center text-xs text-muted-foreground">
            Pay by card on the next page. The amount is held on your card and
            only taken when your order is ready, after any changes.
          </p>
        </CardContent>
      </Card>

      <MobileOrderBar
        subtotal={subtotal}
        delivery={isDelivery ? fee : undefined}
        total={total}
      >
        {termsField("terms-mobile")}
        {belowMin ? (
          <p className="text-xs text-muted-foreground">
            Add <Price pence={delivery.min_order - subtotal} /> more for
            delivery, or choose collection.
          </p>
        ) : (
          state.status === "error" && (
            <p className="animate-in text-xs text-destructive fade-in-0">
              {state.message}
            </p>
          )
        )}
        {/* Swipe rather than tap, so a stray touch while scrolling the
            form can't place the order. */}
        <SwipeToConfirm
          label={`Swipe to pay · ${formatPrice(total)}`}
          loading={placing}
          disabled={!canSubmit}
          onConfirm={() => {
            const form = formRef.current
            // Same checks as the button: let the browser point at a
            // missing field, and spring back until it's filled in.
            if (!form?.reportValidity()) return false
            form.requestSubmit()
          }}
        />
      </MobileOrderBar>
    </form>
  )
}

function CheckoutSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_380px] lg:gap-8">
      <div className="flex flex-col gap-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-36 w-full rounded-xl" />
        ))}
      </div>
      <Skeleton className="h-80 rounded-xl" />
    </div>
  )
}

export { CheckoutForm }
