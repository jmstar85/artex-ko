"use client"

import { Loader2Icon } from "lucide-react"
import { useTranslations } from "next-intl"

import { cn } from "@/lib/utils"

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  const t = useTranslations("ui")

  return (
    <Loader2Icon data-slot="spinner" role="status" aria-label={t("loading")} className={cn("size-4 animate-spin", className)} {...props} />
  )
}

export { Spinner }
