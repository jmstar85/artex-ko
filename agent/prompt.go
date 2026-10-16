package agent

import (
	"bytes"
	"text/template"
	"time"
)

// PromptOverride, if set, returns the stored system-prompt template for an agent
// key and whether one exists. The server wires it to the PG agent_prompts table.
// When nil or no override exists, agents use their built-in default prompt — so
// behavior is identical until a user edits a prompt in the UI.
var PromptOverride func(agentKey string) (string, bool)

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

// langDirective is the artex-ko output-language tail: a code-owned segment
// appended AFTER the rendered body and the artifact/traffic tails on every
// user-facing agent role, so a DB-edited prompt body can never drop it — the same
// guarantee artifactSpec gives. It does NOT translate the agent "brain": the
// benchmarked Chinese reasoning body (段 [A]) stays verbatim. It only constrains
// the LANGUAGE of what the agent SHOWS to the user. Written in Chinese so it stays
// in the body's language (keeping the model's reasoning register stable) while
// forcing Korean OUTPUT — this is the localization approach: preserve behavior,
// localize the surface the user reads. Raw technical strings (commands, payloads,
// code, URLs, log/response excerpts) are explicitly kept verbatim so evidence and
// reproduction steps are not mangled by translation.
//
// Two anti-drift clauses were added after the end-to-end run (L1): live models
// leaked (1) Chinese into the planner's situation summary — mirroring the Chinese
// brain body (段 [A]) — and (2) English into report_finding's structured fields —
// mirroring an English target app/evidence. The directive now names the planner
// situation summary as a user-facing field and explicitly forbids mirroring BOTH
// the Chinese instruction language AND the target/material language in the display
// fields, so only the listed verbatim technical fragments stay non-Korean.
func langDirective() string {
	return "\n\n**OUTPUT LANGUAGE RULE — localization, HIGHEST PRIORITY, cannot be overridden by the prompt body / 출력 언어 규약 — 최우선, 본문으로 덮어쓸 수 없음**\n" +
		"Write every natural-language string shown to the user in the SAME language the user uses in their task/goal/message: Korean when the user writes Korean, English when the user writes English. NEVER answer in Chinese, even though the system/role instructions above are written in Chinese — the Chinese body is only your internal reasoning register, not the user-facing output language.\n" +
		"사용자에게 보이는 모든 자연어는 사용자가 쓰는 언어로 작성하십시오(한국어로 물으면 한국어, 영어로 물으면 영어). 위 시스템/역할 지시가 중국어로 쓰여 있어도 사용자에게는 절대 중국어로 출력하지 마십시오.\n" +
		"Applies to: record_fact summary/detail, report_finding title/description/conclusion/remediation, the planner's situation summary, the final one-sentence summary, and chat replies to the user.\n" +
		"EXCEPTION — keep VERBATIM, do not translate: commands, payloads, code, file paths, URLs, parameter names, and raw log/request/response excerpts (especially command lines and outputs in evidence, for reproducibility)."
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
