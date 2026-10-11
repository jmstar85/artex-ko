"use client";

import { Button } from "@/components/ui/button";
import { LOCALE_LABELS, type SwitchableLocale } from "@/i18n/config";
import { cn } from "@/lib/utils";
import { useLocale } from "@/stores/locale/locale-provider";

export function LanguageSwitcher({ className }: { className?: string }) {
  const { locale, setLocale } = useLocale();
  const next: SwitchableLocale = locale === "ko" ? "en" : "ko";

  return (
    <Button
      size="icon"
      variant="ghost"
      className={cn("font-medium text-xs tabular-nums", className)}
      onClick={() => setLocale(next)}
      aria-label={`언어 전환 (현재 ${LOCALE_LABELS[locale]} → ${LOCALE_LABELS[next]})`}
      title={`${LOCALE_LABELS[locale]} → ${LOCALE_LABELS[next]}`}
    >
      {locale.toUpperCase()}
    </Button>
  );
}
