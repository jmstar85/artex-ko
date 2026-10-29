"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { LOCALE_LABELS, SWITCHABLE_LOCALES } from "@/i18n/config";
import { cn } from "@/lib/utils";
import { useLocale } from "@/stores/locale/locale-provider";

export function LanguageSwitcher({ className }: { className?: string }) {
  const { locale, setLocale } = useLocale();
  const tr = useTranslations("languageSwitcher");
  const next = SWITCHABLE_LOCALES[(SWITCHABLE_LOCALES.indexOf(locale) + 1) % SWITCHABLE_LOCALES.length];

  return (
    <Button
      size="icon"
      variant="ghost"
      className={cn("font-medium text-xs tabular-nums", className)}
      onClick={() => setLocale(next)}
      aria-label={tr("switch", { current: LOCALE_LABELS[locale], next: LOCALE_LABELS[next] })}
      title={`${LOCALE_LABELS[locale]} → ${LOCALE_LABELS[next]}`}
    >
      {locale.toUpperCase()}
    </Button>
  );
}
