"use client";

import Link from "next/link";

import { useTranslations } from "next-intl";

import { LanguageSwitcher } from "@/components/language-switcher";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  const t = useTranslations("notFound");
  return (
    <div className="flex h-dvh flex-col items-center justify-center space-y-2 text-center">
      <div className="fixed top-4 right-4 z-50">
        <LanguageSwitcher />
      </div>
      <h1 className="font-semibold text-2xl">{t("title")}</h1>
      <p className="text-muted-foreground">{t("description")}</p>
      <Link prefetch={false} replace href="/function/tasks">
        <Button variant="outline">{t("back")}</Button>
      </Link>
    </div>
  );
}
