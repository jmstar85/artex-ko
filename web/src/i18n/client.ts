import { type AbstractIntlMessages, createTranslator, type MessageKeys, type NestedKeyOf } from "next-intl";

import { isLocale, LOCALE_COOKIE, type Locale, resolveLocale } from "@/i18n/config";
import { getClientCookie, setClientCookie } from "@/lib/cookie.client";

import enMessages from "../../messages/en.json";
import koMessages from "../../messages/ko.json";
import zhMessages from "../../messages/zh.json";

export const MESSAGES = {
  ko: koMessages,
  en: enMessages,
  zh: zhMessages,
} satisfies Record<Locale, AbstractIntlMessages>;

type ClientMessages = (typeof MESSAGES)[Locale];
type ClientMessageKey = MessageKeys<ClientMessages, NestedKeyOf<ClientMessages>>;

const FALLBACK = resolveLocale();
let browserLocale: Locale = FALLBACK;
let restored = false;
const listeners = new Set<() => void>();

// 쿠키는 탭을 처음 복원할 때만 읽는다. 이후 화면과 비React 요청은 이 탭의 상태를 공유한다.
export function getClientLocale(): Locale {
  return typeof document === "undefined" ? FALLBACK : browserLocale;
}

export function subscribeClientLocale(listener: () => void): () => void {
  if (typeof document === "undefined") return () => { /* 서버에서는 구독하지 않으므로 해제할 상태가 없다. */ };
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function publishLocale(locale: Locale): void {
  if (browserLocale === locale) return;
  browserLocale = locale;
  for (const listener of listeners) listener();
}

export function restoreClientLocale(): void {
  if (typeof document === "undefined" || restored) return;
  restored = true;
  const saved = getClientCookie(LOCALE_COOKIE);
  publishLocale(isLocale(saved) ? saved : FALLBACK);
}

export function setClientLocale(locale: Locale): void {
  if (typeof document === "undefined" || !isLocale(locale)) return;
  restored = true;
  setClientCookie(LOCALE_COOKIE, locale);
  publishLocale(locale);
}

export function clientMessage(key: ClientMessageKey, values?: Record<string, string | number>): string {
  const locale = getClientLocale();
  return createTranslator({ locale, messages: MESSAGES[locale] })(key, values);
}
