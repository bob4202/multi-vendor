"use server"

import { redirect } from "next/navigation"

import {
  CUSTOMER_LOGIN_PATH,
  CUSTOMER_SESSION_COOKIE,
  safeCustomerRedirect,
} from "@/lib/customer/constants"
import { externalAPI } from "@/lib/external-api-service"
import {
  browserUserAgent,
  clearSessionCookie,
  setSessionCookie,
} from "@/lib/portal-session"
import type { Customer } from "@/types/customer"

// Customer sign-in: email -> 6-digit code by email -> session cookie.

export interface LoginState {
  step: "email" | "code"
  email?: string
  error?: string
  // Seconds before another code can be sent.
  retryAfter?: number
}

function text(formData: FormData, key: string) {
  const v = formData.get(key)
  return typeof v === "string" ? v.trim() : ""
}

// Both steps share one form state: "send" emails a code, "verify" checks it.
export async function customerLoginAction(
  prev: LoginState,
  formData: FormData
): Promise<LoginState> {
  const intent = text(formData, "intent")
  const email = text(formData, "email") || prev.email || ""

  if (intent === "send" || intent === "resend") {
    if (!email) return { step: "email", error: "Enter your email address." }
    const res = await externalAPI.sendLoginCode(email).catch(() => null)
    if (!res) {
      return { step: prev.step, email, error: "We couldn't reach the shop. Please try again." }
    }
    if (res.ok) return { step: "code", email }
    if (res.status === 429) {
      // Already sent one moments ago - it's still on its way.
      return {
        step: "code",
        email,
        retryAfter: 60,
        error: intent === "resend" ? "Please wait a minute before asking for another code." : undefined,
      }
    }
    return {
      step: "email",
      email,
      error: res.issues[0]?.message ?? "Enter a valid email address.",
    }
  }

  const code = text(formData, "code").replace(/\s+/g, "")
  if (!/^\d{6}$/.test(code)) {
    return { step: "code", email, error: "Enter the 6-digit code from the email." }
  }
  const res = await externalAPI
    .verifyLoginCode(email, code, await browserUserAgent())
    .catch(() => null)
  if (!res) {
    return { step: "code", email, error: "We couldn't reach the shop. Please try again." }
  }
  if (!res.ok) {
    return {
      step: "code",
      email,
      error:
        res.status === 429
          ? "Too many wrong codes. Ask for a new one."
          : "That code is wrong or has expired.",
    }
  }
  await setSessionCookie(CUSTOMER_SESSION_COOKIE, "/", res.data.token)
  redirect(safeCustomerRedirect(text(formData, "next")))
}

export async function customerLogoutAction() {
  const token = await clearSessionCookie(CUSTOMER_SESSION_COOKIE, "/")
  if (token) await externalAPI.logout(token).catch(() => {})
  redirect(CUSTOMER_LOGIN_PATH)
}

export interface ProfileState {
  ok?: boolean
  error?: string
  fieldErrors?: Record<string, string>
  customer?: Customer
}

export async function updateProfileAction(
  _prev: ProfileState,
  formData: FormData
): Promise<ProfileState> {
  const res = await externalAPI
    .updateMe({ name: text(formData, "name"), phone: text(formData, "phone") })
    .catch(() => null)
  if (!res) return { error: "We couldn't reach the shop. Please try again." }
  if (!res.ok) {
    if (res.status === 401) redirect(`${CUSTOMER_LOGIN_PATH}?next=/account`)
    return {
      error: res.issues.length ? undefined : res.error,
      fieldErrors: Object.fromEntries(res.issues.map((i) => [i.field, i.message])),
    }
  }
  return { ok: true, customer: res.data }
}
