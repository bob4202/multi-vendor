import type { Step } from "@/components/common/stepper"

// The onboarding wizard's steps, shared by /vendors/new (step 1) and the
// setup page (the rest), so the stepper looks the same on both.

export const SETUP_STEPS = [
  "Vendor",
  "Branch",
  "Store details",
  "Owner",
  "Go live",
] as const

export type SetupStep = 0 | 1 | 2 | 3 | 4

// done = which steps are complete; current = the one being shown.
export function setupStepper(
  done: boolean[],
  current: SetupStep,
  hrefFor?: (step: SetupStep) => string
): Step[] {
  return SETUP_STEPS.map((label, i) => ({
    label,
    state: i === current ? "current" : done[i] ? "done" : "upcoming",
    href: hrefFor && i > 0 ? hrefFor(i as SetupStep) : undefined,
  }))
}
