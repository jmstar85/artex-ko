package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnthropicAuthURL(t *testing.T) {
	u, verifier := AnthropicAuthURL()
	if verifier == "" {
		t.Fatal("verifier 가 비어 있음")
	}
	for _, want := range []string{
		anthropicOAuthClientID, "code_challenge=", "code_challenge_method=S256",
		"response_type=code", "state=" + verifier,
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth URL 에 %q 가 없음: %s", want, u)
		}
	}
}

func TestSplitAuthCode(t *testing.T) {
	cases := []struct{ in, code, state string }{
		{"abc#xyz", "abc", "xyz"},
		{"  abc#xyz  ", "abc", "xyz"},
		{"onlycode", "onlycode", ""},
	}
	for _, c := range cases {
		code, state := splitAuthCode(c.in)
		if code != c.code || state != c.state {
			t.Fatalf("splitAuthCode(%q)=(%q,%q) want (%q,%q)", c.in, code, state, c.code, c.state)
		}
	}
}

func TestParseTokenResponse(t *testing.T) {
	tok, err := parseTokenResponse([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "a" || tok.RefreshToken != "r" {
		t.Fatalf("토큰 파싱 결과 어긋남: %+v", tok)
	}
	if tok.ExpiresAt == 0 {
		t.Fatal("expires_in 이 절대 만료시각으로 환산되지 않음")
	}
	if _, err := parseTokenResponse([]byte(`{"refresh_token":"r"}`)); err == nil {
		t.Fatal("access_token 없는 응답이 통과됨")
	}
}

func TestMergeBeta(t *testing.T) {
	if got := mergeBeta("", "oauth-2025-04-20"); got != "oauth-2025-04-20" {
		t.Fatalf("빈 값 병합: %q", got)
	}
	if got := mergeBeta("other-beta", "oauth-2025-04-20"); got != "other-beta,oauth-2025-04-20" {
		t.Fatalf("추가 병합: %q", got)
	}
	if got := mergeBeta("a, oauth-2025-04-20 ,b", "oauth-2025-04-20"); got != "a, oauth-2025-04-20 ,b" {
		t.Fatalf("이미 존재하면 그대로여야 함: %q", got)
	}
}

func systemTexts(t *testing.T, req *http.Request) []string {
	t.Helper()
	b, err := req.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	raw, _ := io.ReadAll(b)
	var m struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("응답 본문 파싱 실패: %s", raw)
	}
	out := make([]string, len(m.System))
	for i, s := range m.System {
		out[i] = s.Text
	}
	return out
}

func TestInjectClaudeCodeSystemArray(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		bytes.NewReader([]byte(`{"model":"x","system":[{"type":"text","text":"orig"}],"messages":[]}`)))
	if err := injectClaudeCodeSystem(req); err != nil {
		t.Fatal(err)
	}
	got := systemTexts(t, req)
	if len(got) != 2 || got[0] != ClaudeCodeSystemIdentity || got[1] != "orig" {
		t.Fatalf("선두 신원 블록 주입 실패: %v", got)
	}
	// 멱등: 다시 돌려도 신원 블록이 중복되지 않아야 한다.
	if err := injectClaudeCodeSystem(req); err != nil {
		t.Fatal(err)
	}
	if got := systemTexts(t, req); len(got) != 2 || got[0] != ClaudeCodeSystemIdentity {
		t.Fatalf("멱등성 위반: %v", got)
	}
}

func TestInjectClaudeCodeSystemAbsent(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		bytes.NewReader([]byte(`{"model":"x","messages":[]}`)))
	if err := injectClaudeCodeSystem(req); err != nil {
		t.Fatal(err)
	}
	if got := systemTexts(t, req); len(got) != 1 || got[0] != ClaudeCodeSystemIdentity {
		t.Fatalf("system 부재 시 신원 블록 생성 실패: %v", got)
	}
}

func TestApplyOAuthHeaders(t *testing.T) {
	tr := quotaAwareTransport{oauth: NewOAuthState(OAuthToken{AccessToken: "tok123"}, "", nil)}
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		bytes.NewReader([]byte(`{"system":[{"type":"text","text":"x"}]}`)))
	req.Header.Set("x-api-key", "KEY")
	if err := tr.applyOAuth(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatal("x-api-key 가 제거되지 않음")
	}
	if req.Header.Get("Authorization") != "Bearer tok123" {
		t.Fatalf("Bearer 토큰 주입 실패: %q", req.Header.Get("Authorization"))
	}
	if !strings.Contains(req.Header.Get("anthropic-beta"), anthropicOAuthBeta) {
		t.Fatalf("oauth beta 플래그 누락: %q", req.Header.Get("anthropic-beta"))
	}
}
