import "server-only"

import { cache } from "react"
import { cookies } from "next/headers"

import { CUSTOMER_SESSION_COOKIE } from "@/lib/customer/constants"
import { externalAPI } from "@/lib/external-api-service"
import type { Customer } from "@/types/customer"

// The signed-in customer, checked against the API once per request. null =
// a guest. Unlike the portals, nothing here redirects: the storefront works
// signed in or not, and pages decide what to show.
export const getCustomer = cache(async (): Promise<Customer | null> => {
  const token = (await cookies()).get(CUSTOMER_SESSION_COOKIE)?.value
  if (!token) return null
  try {
    return await externalAPI.me(token)
  } catch (err) {
    // The API being down shouldn't take the shop with it - act signed out.
    console.error("load customer failed", err)
    return null
  }
})
