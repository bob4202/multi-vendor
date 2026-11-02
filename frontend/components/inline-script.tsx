// A <script> rendered by React triggers a dev-mode warning in this Next.js
// version ("Scripts inside React components are never executed when
// rendering on the client") unless it's inert on the client - hence the
// type swap: real script tag during SSR (so the browser executes it while
// parsing, before hydration), inert text/plain once React is involved.
// suppressHydrationWarning covers the resulting type mismatch.
export function InlineScript({ html }: { html: string }) {
  return (
    <script
      type={typeof window === "undefined" ? "text/javascript" : "text/plain"}
      suppressHydrationWarning
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
