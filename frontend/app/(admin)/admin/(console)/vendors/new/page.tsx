import type { Metadata } from "next"

import { PageHeader } from "@/components/common/page-header"
import { Stepper } from "@/components/common/stepper"
import { setupStepper } from "@/components/admin/setup-steps"
import { VendorForm } from "@/components/admin/vendor-form"
import { Card, CardContent } from "@/components/ui/card"

export const metadata: Metadata = { title: "Onboard a vendor" }

export default function NewVendorPage() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <PageHeader
        className="mb-0"
        title="Onboard a vendor"
        description="Five short steps. You can stop at any point and pick up where you left off from the vendor's page."
      />
      <Stepper steps={setupStepper([], 0)} />
      <Card>
        <CardContent>
          <VendorForm />
        </CardContent>
      </Card>
    </div>
  )
}
