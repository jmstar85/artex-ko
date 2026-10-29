import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

// LocaleProvider가 정적 HTML부터 쿠키 복원·언어 전환까지 같은 제목과 설명을 선언한다.
// Next 정적 metadata와 별도로 등록하지 않아 문서 메타데이터의 소유자를 하나로 유지한다.
// ko 제목은 엠대시 대신 콜론을 써서 "제품명: 설명" 형태로 둔다.
export const META_BY_LOCALE = {
  ko: {
    title: "ARTEX: 자율 침투 테스트 콘솔",
    description: "LLM 기반으로 자율 침투 테스트를 수행하는 시스템 콘솔",
  },
  en: {
    title: "ARTEX: Autonomous Penetration Testing Console",
    description: "System console for LLM-driven autonomous penetration testing",
  },
  zh: {
    title: "ARTEX — 自主渗透测试控制台",
    description: "LLM 驱动的自主渗透测试系统控制台",
  },
} as const;

export const APP_CONFIG = {
  name: "ARTEX",
  version: packageJson.version,
  copyright: `© ${currentYear}, ARTEX.`,
};
