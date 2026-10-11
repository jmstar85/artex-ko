"use client";

import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from "react";

import { NextIntlClientProvider } from "next-intl";

import { DEFAULT_LOCALE, LOCALE_COOKIE, SWITCHABLE_LOCALES, type SwitchableLocale, TIME_ZONE } from "@/i18n/config";
import { getClientCookie, setClientCookie } from "@/lib/cookie.client";

import enMessages from "../../../messages/en.json";
import koMessages from "../../../messages/ko.json";

// 전환 가능한 두 로케일의 메시지를 정적으로 번들한다. 정적 내보내기(output: "export")에서는
// 서버가 쿠키를 읽을 수 없으므로, 런타임 전환은 전적으로 클라이언트에서 처리한다.
const MESSAGES: Record<SwitchableLocale, Record<string, unknown>> = {
  ko: koMessages as Record<string, unknown>,
  en: enMessages as Record<string, unknown>,
};

const FALLBACK: SwitchableLocale = (SWITCHABLE_LOCALES as readonly string[]).includes(DEFAULT_LOCALE)
  ? (DEFAULT_LOCALE as SwitchableLocale)
  : "ko";

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

function readLocaleCookie(): SwitchableLocale {
  if (typeof document === "undefined") return FALLBACK;
  const value = getClientCookie(LOCALE_COOKIE);
  return value && (SWITCHABLE_LOCALES as readonly string[]).includes(value) ? (value as SwitchableLocale) : FALLBACK;
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  // 서버 렌더와 클라이언트 첫 렌더는 기본 locale 로 맞춰 하이드레이션 불일치를 피하고,
  // 마운트 후 쿠키 값이 다르면 전환한다(EN 사용자는 첫 페인트에서 한 번 깜빡일 수 있음).
  const [locale, setLocaleState] = useState<SwitchableLocale>(FALLBACK);

  useEffect(() => {
    const cookieLocale = readLocaleCookie();
    setLocaleState((prev) => (cookieLocale !== prev ? cookieLocale : prev));
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: SwitchableLocale) => {
    setClientCookie(LOCALE_COOKIE, next);
    setLocaleState(next);
  }, []);

  const value = useMemo(() => ({ locale, setLocale }), [locale, setLocale]);

  return (
    <LocaleContext.Provider value={value}>
      <NextIntlClientProvider locale={locale} messages={MESSAGES[locale]} timeZone={TIME_ZONE}>
        {children}
      </NextIntlClientProvider>
    </LocaleContext.Provider>
  );
}
