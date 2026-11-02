import { cn } from "@/lib/utils"

// Shared responsive gutter/max-width used by the header, footer, and
// (optionally) page sections. `main` itself stays full-bleed so a page can
// still render edge-to-edge sections (e.g. a hero banner) and opt into this
// container only where it wants the standard content width.
function Container({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn("mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8", className)}
      {...props}
    />
  )
}

export { Container }
