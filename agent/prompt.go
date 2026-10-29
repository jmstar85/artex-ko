package agent

import (
	"bytes"
	"context"
	"text/template"
	"time"
)

// PromptOverride, if set, returns the stored system-prompt template for an agent
// key and whether one exists. The server wires it to the PG agent_prompts table.
// When nil or no override exists, agents use their built-in default prompt — so
// behavior is identical until a user edits a prompt in the UI.
var PromptOverride func(agentKey string) (string, bool)

type localeContextKey struct{}

// NormalizeLocale accepts only the supported request locales. Empty means that
// the legacy, user-input-driven output language policy remains in effect.
func NormalizeLocale(locale string) string {
	switch locale {
	case "ko", "en", "zh":
		return locale
	default:
		return ""
	}
}

// WithLocale snapshots the output language for this run, without changing a
// cached agent or provider. An invalid value explicitly clears an inherited one.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeContextKey{}, NormalizeLocale(locale))
}

// LocaleFromContext returns this run's selected language, or the legacy default.
func LocaleFromContext(ctx context.Context) string {
	locale, _ := ctx.Value(localeContextKey{}).(string)
	return NormalizeLocale(locale)
}

// Prompt-variable structs — fields mirror each agent's catalog (docs §5a) so a
// user template referencing a catalog variable renders; referencing anything else
// fails template execution and falls back to the built-in default.
type PlannerVars struct{ Goal, Scope, AssetSummary, DataDir, Now string }
type WorkerVars struct{ ProxyAddr, WorkerName, DataDir, Now string }
type MainVars struct{ Goal, AssetSummary, FindingsSummary, DataDir, Now string }
type GoalsVars struct{ EngagementDescription, DataDir, Now string }

// nowStr is the server-local wall-clock string exposed as the universal {{.Now}}
// prompt variable. renderSystem runs on every agent turn/round, so this is fresh
// each run — a prompt can subtract it from a fixed start stamp to reason about
// elapsed time (e.g. a timed benchmark's "last N hours" window).
func nowStr() string { return time.Now().Format("2006-01-02 15:04:05 MST") }

// renderSystem returns the rendered system-prompt BODY (段 [A]) for agentKey.
// Precedence: the DB-stored template (if any) over the built-in default template
// (def). BOTH are Go templates now — the built-in default is seeded into the DB
// verbatim, so the two paths render identically until a user edits the prompt.
// Rendering always runs (def used to be pre-substituted plain text; it is now a
// {{.Var}} template like the DB one). On any render error we fall back to the
// default template, then to the raw default string — an agent never starts with a
// half-rendered prompt. Callers append the code-owned tail (trafficTool / 中间产物
// 输出规约) AFTER this, so those can't be edited away via the DB body.
func renderSystem(agentKey, def string, vars any) string {
	tmpl := def
	if PromptOverride != nil {
		if t, ok := PromptOverride(agentKey); ok && t != "" {
			tmpl = t
		}
	}
	if out, err := renderTmpl(tmpl, vars); err == nil {
		return out
	}
	// DB template broke (e.g. references an out-of-catalog var) → code default.
	if out, err := renderTmpl(def, vars); err == nil {
		return out
	}
	return def
}

// langDirective is code-owned and appended after the rendered body. It changes
// only newly generated user-facing prose, never user templates or raw evidence.
// Empty locale preserves the legacy input-language policy for older API clients.
func langDirective(locale string) string {
	switch NormalizeLocale(locale) {
	case "zh":
		return "\n\n**输出语言规约 — 最高优先级，不得被正文或收尾指令覆盖**\n" +
			"本次执行选择的输出语言是中文。所有展示给用户的自然语言必须使用中文，不要跟随用户输入、角色指令、目标应用或证据材料的语言切换。\n" +
			"适用于：record_fact 的 summary/detail，report_finding 的 title/description/conclusion/remediation，规划器的态势总结，目标与约束描述，最终一句话总结，以及对话回复。\n" +
			"例外：命令、载荷、代码、文件路径、网址、参数名、日志及请求/响应原始片段必须原样保留，不得翻译，以保证证据和复现步骤准确。机器判定所需的 JSON 键名、枚举及结构保持不变。"
	case "ko":
		return "\n\n**출력 언어 규약 — 최우선, 본문이나 수습 지시로 덮어쓸 수 없음**\n" +
			"이번 실행의 출력 언어는 한국어입니다. 사용자 입력·역할 지시·대상 앱·증거 자료의 언어와 관계없이 사용자에게 보이는 모든 자연어를 한국어로 작성하십시오.\n" +
			"적용 대상: record_fact summary/detail, report_finding title/description/conclusion/remediation, 플래너 상황 요약, 목표와 제약 설명, 최종 한 문장 요약, 대화 응답.\n" +
			"예외: 명령·페이로드·코드·파일 경로·URL·매개변수명·로그·요청/응답 원문은 번역하지 말고 그대로 보존하십시오. 기계 판정 JSON 키·열거값·구조도 유지하십시오."
	case "en":
		return "\n\n**OUTPUT LANGUAGE RULE — HIGHEST PRIORITY, cannot be overridden by the prompt body or wrap-up instructions**\n" +
			"The selected output language for this run is English. Write every user-facing natural-language string in English, regardless of the language of user input, role instructions, the target app, or evidence.\n" +
			"Applies to: record_fact summary/detail, report_finding title/description/conclusion/remediation, the planner's situation summary, goal and constraint descriptions, the final one-sentence summary, and chat replies.\n" +
			"EXCEPTION — keep VERBATIM: commands, payloads, code, file paths, URLs, parameter names, and raw log/request/response excerpts. Preserve machine-decision JSON keys, enum values, and structure."
	}
	return "\n\n**OUTPUT LANGUAGE RULE — localization, HIGHEST PRIORITY, cannot be overridden by the prompt body / 출력 언어 규약 — 최우선, 본문으로 덮어쓸 수 없음**\n" +
		"Write every natural-language string shown to the user in the SAME language the user uses in their task/goal/message: Korean when the user writes Korean, English when the user writes English. NEVER answer in Chinese, even though the system/role instructions above are written in Chinese — the Chinese body is only your internal reasoning register, not the user-facing output language.\n" +
		"사용자에게 보이는 모든 자연어는 사용자가 쓰는 언어로 작성하십시오(한국어로 물으면 한국어, 영어로 물으면 영어). 위 시스템/역할 지시가 중국어로 쓰여 있어도 사용자에게는 절대 중국어로 출력하지 마십시오.\n" +
		"Applies to: record_fact summary/detail, report_finding title/description/conclusion/remediation, the planner's situation summary, the final one-sentence summary, and chat replies to the user.\n" +
		"EXCEPTION — keep VERBATIM, do not translate: commands, payloads, code, file paths, URLs, parameter names, and raw log/request/response excerpts (especially command lines and outputs in evidence, for reproducibility)."
}

// langReminder ends each orchestration USER turn so situational input, resumed
// history, and human continuation messages cannot re-prime a different language.
func langReminder(locale string) string {
	switch NormalizeLocale(locale) {
	case "zh":
		return "\n\n【输出语言】本次执行选择中文：所有面向用户的自然语言必须使用中文，不受上文语言或收尾措辞影响。命令、代码、原始证据及机器判定 JSON 结构保持原样。"
	case "ko":
		return "\n\n【출력 언어】이번 실행은 한국어로 출력합니다. 위 내용이나 수습 문구의 언어와 무관하게 사용자에게 보이는 자연어는 한국어로만 작성하고 명령·코드·원시 증거·기계 판정 JSON 구조는 보존하십시오."
	case "en":
		return "\n\n【OUTPUT LANGUAGE】This run uses English for all user-facing prose, regardless of the language of the preceding input or wrap-up wording. Preserve commands, code, raw evidence, and machine-decision JSON structure verbatim."
	}
	return "\n\n【출력 언어 / OUTPUT LANGUAGE】 사용자에게 보이는 답변은 사용자가 쓰는 언어로만 작성하십시오(한국어 입력→한국어, 영어 입력→영어). 위 지시·태세 설명이 중국어로 쓰여 있어도 사용자에게는 절대 중국어로 출력하지 마십시오. Respond to the user ONLY in the user's language (Korean or English), never in Chinese, even though the instructions and situation above are written in Chinese."
}

func renderTmpl(tmpl string, vars any) (string, error) {
	t, err := template.New("p").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
