"use client";

import { useCallback } from "react";

import { useMessages } from "next-intl";

// 빌트인 시드 문구(도구 설명, 에이전트 이름·프롬프트, 가로채기 규칙 등)는 DB/백엔드가 원문 그대로
// 내려준다. messages 의 builtin.text 는 "원문의 해시 → 현재 로케일 번역" 표이며, 원문과 정확히
// 일치할 때만 치환하므로 사용자가 직접 수정한 값은 입력한 그대로 보인다.
// 키 = FNV-1a 32bit(코드포인트 단위, base36 7자리) + 길이(base36). 형식을 바꾸면 builtin.text 전체를 다시 생성해야 한다.
export function builtinTextKey(text: string): string {
  let h = 0x811c9dc5;
  for (const ch of text) {
    h ^= ch.codePointAt(0) ?? 0;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(36).padStart(7, "0") + text.length.toString(36);
}

export function useBuiltinText(): (text: string | null | undefined) => string {
  const messages = useMessages() as { builtin?: { text?: Record<string, string> } };
  // 중국어 내장 본문은 원문으로 표시하고 한국어 기본 마무리 문구 등만 번역표에서 찾는다.
  const table = messages.builtin?.text;
  return useCallback((text) => (text ? (table?.[builtinTextKey(text)] ?? text) : ""), [table]);
}
