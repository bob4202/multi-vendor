import type { Metadata } from "next"

import { Container } from "@/components/layout/container"
import { TERMS_VERSION } from "@/lib/customer/constants"
import { formatDateLabel } from "@/lib/format"

export const metadata: Metadata = { title: "Terms" }

// Plain-English ordering terms. Customers tick these at checkout and the
// order records TERMS_VERSION, so bump it whenever this page changes.
// Draft wording: have it checked before launch.
const SECTIONS: { title: string; body: string[] }[] = [
  {
    title: "Placing an order",
    body: [
      "When you place an order, the shop has a few minutes to accept it. If it doesn't, the order is cancelled automatically and you aren't charged.",
      "Collection orders are for the same day only, until the shop's last-orders time shown at checkout.",
    ],
  },
  {
    title: "Out-of-stock items and substitutes",
    body: [
      "Our products are fresh, so occasionally something runs out after you order.",
      "If you allow substitutes, the shop may offer a similar product. We always ask you first, and you never pay more than the price of what you originally ordered. If the substitute costs less, you pay the lower price.",
      "If you don't answer in time, or you turn off substitutes at checkout, the item is removed and your total goes down.",
    ],
  },
  {
    title: "Collecting your order",
    body: [
      "We'll tell you when your order is ready, with a collection code. Please show the code at the counter.",
      "Please collect within one hour of being told your order is ready, or by the shop's closing time if that's sooner. The time is shown on your order page.",
      "Because the products are perishable and prepared for you, orders that aren't collected in time are cancelled and are not refunded.",
    ],
  },
  {
    title: "Paying",
    body: [
      "You pay by card when you place your order. The shop takes the payment, through its own card payment provider.",
      "The order total is held on your card, not taken. Your order goes to the shop once the hold is in place; if it isn't paid by the time shown on your order page, the order is cancelled.",
      "When your order is ready, the shop takes the final total shown on your order page, after any substitutes or removed items. The rest of the hold is released.",
      "If the shop declines or cancels your order before it's ready, the hold is released and nothing is taken.",
    ],
  },
  {
    title: "Questions",
    body: [
      "If something isn't right with your order, please call the shop. You'll find the number in your order emails.",
    ],
  },
]

export default function TermsPage() {
  return (
    <Container className="max-w-2xl py-10">
      <h1 className="text-2xl font-semibold">Ordering terms</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Version of {formatDateLabel(TERMS_VERSION)}
      </p>
      <div className="mt-8 flex flex-col gap-8">
        {SECTIONS.map((section) => (
          <section key={section.title} className="flex flex-col gap-2">
            <h2 className="font-semibold">{section.title}</h2>
            {section.body.map((p) => (
              <p key={p} className="text-sm leading-relaxed text-muted-foreground">
                {p}
              </p>
            ))}
          </section>
        ))}
      </div>
    </Container>
  )
}
