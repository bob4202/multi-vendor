import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

interface CheckoutSectionProps {
  step: number
  title: string
  description?: React.ReactNode
  className?: string
  children: React.ReactNode
}

// One numbered block of the checkout form ("1 How would you like it?").
function CheckoutSection({
  step,
  title,
  description,
  className,
  children,
}: CheckoutSectionProps) {
  return (
    <Card className={cn("animate-in duration-300 fade-in-0", className)}>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-start gap-3">
          <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
            {step}
          </span>
          <div className="flex flex-col gap-0.5">
            <h2 className="font-semibold">{title}</h2>
            {description && (
              <p className="text-sm text-muted-foreground">{description}</p>
            )}
          </div>
        </div>
        {children}
      </CardContent>
    </Card>
  )
}

export { CheckoutSection }
