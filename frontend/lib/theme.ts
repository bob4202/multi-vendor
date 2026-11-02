export const THEME_STORAGE_KEY = "theme"

export type Theme = "light" | "dark" | "system"
export type ResolvedTheme = "light" | "dark"

export function resolveTheme(theme: Theme): ResolvedTheme {
  if (theme === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
  }
  return theme
}

// Matches app/globals.css's `.dark` selector (not `data-theme`), so no CSS
// changes were needed when replacing next-themes.
export function applyTheme(resolved: ResolvedTheme) {
  const root = document.documentElement
  root.classList.toggle("dark", resolved === "dark")
  root.style.colorScheme = resolved
}

// Runs inline, before React hydrates, so the correct theme is on the page
// before first paint - no flash. Written as a real function and stringified
// into the <script> tag (same technique next-themes itself used) rather
// than hand-maintained as a string.
function themeInitScript(storageKey: string): void {
  try {
    const stored = localStorage.getItem(storageKey)
    const theme = stored || "system"
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches
    const resolved = theme === "system" ? (prefersDark ? "dark" : "light") : theme
    const root = document.documentElement
    if (resolved === "dark") root.classList.add("dark")
    root.style.colorScheme = resolved
  } catch {
    // localStorage unavailable (privacy mode, disabled storage, etc.) -
    // the page just renders with its default (light) theme.
  }
}

export function getThemeInitScript(): string {
  return `(${themeInitScript.toString()})(${JSON.stringify(THEME_STORAGE_KEY)})`
}
