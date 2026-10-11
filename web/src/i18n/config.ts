// 지원 locale 과 기본값. 이 저장소는 ARTEX 한국어판이므로 기본 locale 은 "ko" 이고,
// 원문 대조(상류 업데이트 비교)를 위해 중국어 "zh" 를 함께 둔다.
export const LOCALES = ["ko", "en", "zh"] as const;
export type Locale = (typeof LOCALES)[number];
export const DEFAULT_LOCALE: Locale = "ko";

// 인앱 언어 전환기가 제공하는 로케일. 원문 중국어 "zh" 는 상류 대조용 참조로만 두고
// 사용자용 전환 대상에서는 제외한다(KO <-> EN 토글).
export const SWITCHABLE_LOCALES = ["ko", "en"] as const;
export type SwitchableLocale = (typeof SWITCHABLE_LOCALES)[number];

// next-intl 에 넘길 고정 시간대. 서버 프리렌더와 브라우저가 같은 값을 써야 하이드레이션 불일치가 없다.
export const TIME_ZONE = "Asia/Seoul";

// 런타임 locale 선택을 저장하는 쿠키 키. 테마·레이아웃 설정과 동일한 client-cookie 패턴을 따른다.
export const LOCALE_COOKIE = "locale";

// 전환기 버튼에 표시할 각 로케일의 자기 이름.
export const LOCALE_LABELS: Record<SwitchableLocale, string> = {
  ko: "한국어",
  en: "English",
};

// 활성 locale 을 빌드 시점에 결정한다. 정적 내보내기(next.config 의 output: "export")와
// 호환되어야 하므로 cookies()·headers() 같은 동적 API 를 쓰지 않고 환경변수만 읽는다.
// NEXT_PUBLIC_LOCALE 이 비었거나 지원 목록 밖이면 기본값(ko)으로 떨어진다.
export function resolveLocale(): Locale {
  const raw = process.env.NEXT_PUBLIC_LOCALE;
  return LOCALES.includes(raw as Locale) ? (raw as Locale) : DEFAULT_LOCALE;
}
