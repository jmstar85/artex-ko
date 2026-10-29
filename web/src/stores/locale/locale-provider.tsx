"use client";

import { createContext, type ReactNode, useContext, useEffect, useMemo, useSyncExternalStore } from "react";

import { NextIntlClientProvider } from "next-intl";

import { META_BY_LOCALE } from "@/config/app-config";
import { getClientLocale, MESSAGES, restoreClientLocale, setClientLocale, subscribeClientLocale } from "@/i18n/client";
import { resolveLocale, type SwitchableLocale, TIME_ZONE } from "@/i18n/config";

interface LocaleContextValue {
  locale: SwitchableLocale;
  setLocale: (next: SwitchableLocale) => void;
}

const LocaleContext = createContext<LocaleContextValue | null>(null);

export function useLocale(): LocaleContextValue {
  const ctx = useContext(LocaleContext);
  if (!ctx) throw new Error("useLocale must be used within <LocaleProvider>");
  return ctx;
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  // 정적 HTML과 첫 렌더는 빌드 언어로 맞추고 마운트 후 저장한 선택을 복원한다.
  const locale = useSyncExternalStore(subscribeClientLocale, getClientLocale, resolveLocale);

  useEffect(restoreClientLocale, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const value = useMemo(() => ({ locale, setLocale: setClientLocale }), [locale]);
  const meta = META_BY_LOCALE[locale];

  return (
    <LocaleContext.Provider value={value}>
      {/* React가 정적 HTML과 언어 전환의 메타데이터를 함께 소유해 Next의 뒤늦은 덮어쓰기를 피한다. */}
      <title>{meta.title}</title>
      <meta name="description" content={meta.description} />
      <NextIntlClientProvider locale={locale} messages={MESSAGES[locale]} timeZone={TIME_ZONE}>
        {children}
      </NextIntlClientProvider>
    </LocaleContext.Provider>
  );
}
