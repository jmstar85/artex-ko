package agent

import (
	"strings"
	"testing"
)

func TestRenderSystemOverrideAndFallback(t *testing.T) {
	t.Cleanup(func() { PromptOverride = nil })

	// no override → built-in default
	PromptOverride = nil
	if got := renderSystem("planner", "DEFAULT", PlannerVars{Goal: "g"}); got != "DEFAULT" {
		t.Fatalf("no override should give default, got %q", got)
	}

	// override → rendered with vars
	PromptOverride = func(k string) (string, bool) {
		if k == "planner" {
			return "目标:{{.Goal}} 范围:{{.Scope}}", true
		}
		return "", false
	}
	if got := renderSystem("planner", "DEFAULT", PlannerVars{Goal: "拿下X", Scope: "*.x.com"}); got != "目标:拿下X 范围:*.x.com" {
		t.Fatalf("override render: %q", got)
	}

	// override referencing a non-catalog var → execution error → fallback to default
	PromptOverride = func(k string) (string, bool) { return "{{.NotInCatalog}}", true }
	if got := renderSystem("planner", "DEFAULT", PlannerVars{Goal: "x"}); got != "DEFAULT" {
		t.Fatalf("bad var should fall back to default, got %q", got)
	}

	// full plannerSystem path: DB body [A] is honored, then the code-owned tail
	// [C] (中间产物输出规约) is ALWAYS appended — editing the body can't drop it.
	PromptOverride = func(k string) (string, bool) { return "PLANNER {{.Goal}}", true }
	got := plannerSystem("拿下X", "/data", "/data", "")
	if !strings.HasPrefix(got, "PLANNER 拿下X") {
		t.Fatalf("plannerSystem body not honored: %q", got)
	}
	if !strings.Contains(got, "中间产物输出规约") || !strings.Contains(got, "/data") {
		t.Fatalf("plannerSystem missing code-owned artifact tail: %q", got)
	}

	// worker dual-text via {{if .ProxyAddr}} in a user template, plus the code tail:
	// [B] trafficTool present only when RECORDING (caCert set — the MITM is on, so
	// the traffic_* tools exist), [C] artifact spec always present. The trafficTool
	// block is gated on the CA (arg 2), NOT on ProxyAddr — a global egress proxy
	// with capture off routes traffic but records nothing.
	PromptOverride = func(k string) (string, bool) {
		return "{{if .ProxyAddr}}走代理 {{.ProxyAddr}}{{else}}手动{{end}}", true
	}
	recording := workerSystem("127.0.0.1:8080", "/ca.pem", "/data", "/data", "")
	if !strings.HasPrefix(recording, "走代理 127.0.0.1:8080") {
		t.Fatalf("worker proxy branch body: %q", recording)
	}
	if !strings.Contains(recording, "traffic_search") {
		t.Fatalf("worker while recording should inject trafficTool: %q", recording)
	}
	if strings.Contains(recording, "traffic_refs") {
		t.Fatalf("worker bypassed shared optional evidence policy: %q", recording)
	}
	if !strings.Contains(recording, "中间产物输出规约") {
		t.Fatalf("worker missing artifact tail: %q", recording)
	}
	// Egress proxy set but capture OFF (no CA): the ProxyAddr template branch still
	// renders, but the trafficTool block must NOT — those tools are not registered.
	egressOnly := workerSystem("127.0.0.1:8080", "", "/data", "/data", "")
	if !strings.HasPrefix(egressOnly, "走代理 127.0.0.1:8080") {
		t.Fatalf("worker egress-only branch body: %q", egressOnly)
	}
	if strings.Contains(egressOnly, "traffic_search") {
		t.Fatalf("worker without recording must NOT inject trafficTool: %q", egressOnly)
	}
	noProxy := workerSystem("", "", "/data", "/data", "")
	if !strings.HasPrefix(noProxy, "手动") {
		t.Fatalf("worker no-proxy branch body: %q", noProxy)
	}
	if strings.Contains(noProxy, "traffic_search") {
		t.Fatalf("worker without proxy must NOT inject trafficTool: %q", noProxy)
	}
}

// Custom bodies remain intact while every user-facing role receives the selected
// run's code-owned policy after its body and artifact/scope instructions.
func TestLocaleDirectiveAppendedToUserFacingRoles(t *testing.T) {
	previous := PromptOverride
	t.Cleanup(func() { PromptOverride = previous })
	const body = "USER BODY: keep paths /data/example and payload SELECT 1 verbatim"
	PromptOverride = func(string) (string, bool) { return body, true }
	for _, locale := range []string{"", "ko", "en", "zh"} {
		t.Run(locale, func(t *testing.T) {
			cases := map[string]string{
				"worker": workerSystem("", "", "/data", "/data", locale),
				"planner": plannerSystem("g", "/data", "/data", locale),
				"mainagent": mainAgentSystem("g", "/data", "/data", locale),
				"chat": chatSystem("chat", "/data", "/data", locale),
				"custom": chatSystem("custom", "/data", "/data", locale),
				"reporter": chatSystem("reporter", "/data", "/data", locale),
				"auto": chatSystem("auto", "/data", "/data", locale),
				"goals": goalsSystem("/data", true, locale),
			}
			for role, sys := range cases {
				if !strings.HasPrefix(sys, body) || !strings.HasSuffix(sys, langDirective(locale)) {
					t.Fatalf("%s/%s: custom body or selected language policy lost: %q", role, locale, sys)
				}
			}
		})
	}
	if langDirective("invalid") != langDirective("") || langReminder("invalid") != langReminder("") {
		t.Fatal("invalid locale must retain the legacy language policy")
	}
	for _, policy := range []string{langDirective("zh"), langReminder("zh")} {
		if !strings.Contains(policy, "中文") || strings.Contains(policy, "NEVER answer in Chinese") ||
			strings.Contains(policy, "never in Chinese") || strings.Contains(policy, "절대 중국어로") {
			t.Fatalf("Chinese policy missing or contradicted: %q", policy)
		}
	}
}
