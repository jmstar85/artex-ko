package agent

// Claude 구독(Pro/Max) OAuth 연결.
//
// API 키 대신 claude.ai 로그인으로 받은 토큰을 Bearer 로 붙여 Anthropic Messages
// API 를 호출한다. 이 OAuth 자격증명은 Anthropic 의 1차 CLI(Claude Code)용이라,
// 요청이 Claude Code 로 식별돼야 인가된다. 실제 요청 변형(헤더 교체 + system 선두
// 블록 주입)은 provider.go 의 quotaAwareTransport 에서 수행한다(norma 가 세팅한
// x-api-key 를 거기서 지우고 Bearer 로 교체할 수 있는 유일한 지점이기 때문).
//
// 플로우는 서버(헤드리스/Docker)에 맞춘 수동 붙여넣기 방식이다: 로그인 URL 을 만들어
// 사용자가 브라우저에서 승인 → 콜백 화면이 보여주는 "code#state" 를 붙여넣으면 서버가
// 토큰으로 교환한다. access token 은 만료되므로 refresh token 으로 갱신한다.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// Claude Code 의 공개 OAuth 클라이언트 식별자(구독 로그인에 쓰이는 공개 상수).
	anthropicOAuthClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicOAuthAuthorize = "https://claude.ai/oauth/authorize"
	anthropicOAuthTokenURL  = "https://console.anthropic.com/v1/oauth/token"
	// 수동 붙여넣기 플로우의 고정 redirect(콜백 화면이 code 를 표시해 준다).
	anthropicOAuthRedirect = "https://console.anthropic.com/oauth/code/callback"
	anthropicOAuthScope    = "org:create_api_key user:profile user:inference"

	// ClaudeCodeSystemIdentity 는 구독 OAuth 요청이 Claude Code 로 인가되도록 요청 본문
	// system 배열 선두에 넣는 신원 문구. 이 블록이 없으면 Anthropic 이 OAuth 자격증명을
	// 거부한다. 트랜스포트의 injectClaudeCodeSystem 이 주입한다.
	ClaudeCodeSystemIdentity = "You are Claude Code, Anthropic's official CLI for Claude."

	// anthropicOAuthBeta 는 OAuth 자격증명을 쓸 때 반드시 함께 보내는 beta 플래그.
	anthropicOAuthBeta = "oauth-2025-04-20"

	// oauthRefreshSkew 는 access token 을 만료 직전에 선제 갱신하는 여유(초).
	oauthRefreshSkew = 120
)

// OAuthToken 는 구독 OAuth 자격증명 한 벌이다. ExpiresAt 은 unix 초(0 = 모름).
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Valid 은 최소한 access token 이 있는지만 본다.
func (t OAuthToken) Valid() bool { return strings.TrimSpace(t.AccessToken) != "" }

// pkcePair 는 PKCE code_verifier 와 S256 code_challenge 한 쌍을 만든다.
func pkcePair() (verifier, challenge string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

// AnthropicAuthURL 는 사용자가 브라우저에서 열 로그인 URL 과, 이후 코드 교환에 쓸
// code_verifier 를 돌려준다. state 에는 verifier 를 그대로 실어(수동 플로우) 콜백에서
// 돌려받은 state 로 요청 위조를 걸러낼 수 있게 한다.
func AnthropicAuthURL() (authURL, verifier string) {
	verifier, challenge := pkcePair()
	q := url.Values{}
	q.Set("code", "true")
	q.Set("client_id", anthropicOAuthClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", anthropicOAuthRedirect)
	q.Set("scope", anthropicOAuthScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", verifier)
	return anthropicOAuthAuthorize + "?" + q.Encode(), verifier
}

// splitAuthCode 는 콜백 화면이 보여주는 "code#state" 입력을 코드와 state 로 나눈다.
// '#' 가 없으면 전체를 코드로 본다.
func splitAuthCode(raw string) (code, state string) {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		return strings.TrimSpace(raw[:i]), strings.TrimSpace(raw[i+1:])
	}
	return raw, ""
}

// ExchangeAnthropicCode 는 인가 코드를 access/refresh 토큰으로 교환한다. verifier 는
// AnthropicAuthURL 이 돌려준 값. 붙여넣은 코드에 "#state" 가 섞여 있으면 분리해 state
// 불일치를 거른다(state 가 비어 있으면 검사하지 않는다 — 수동 복사 중 유실 대비).
func ExchangeAnthropicCode(ctx context.Context, hc *http.Client, rawCode, verifier string) (OAuthToken, error) {
	code, state := splitAuthCode(rawCode)
	if code == "" {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: 빈 인가 코드")
	}
	if state != "" && verifier != "" && state != verifier {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: state 불일치(로그인을 다시 시작하세요)")
	}
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  anthropicOAuthRedirect,
		"client_id":     anthropicOAuthClientID,
		"code_verifier": verifier,
	}
	if state != "" {
		payload["state"] = state
	}
	return anthropicTokenRequest(ctx, hc, payload)
}

// RefreshAnthropicToken 는 refresh token 으로 새 access token 을 받는다.
func RefreshAnthropicToken(ctx context.Context, hc *http.Client, refreshToken string) (OAuthToken, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: refresh token 없음")
	}
	return anthropicTokenRequest(ctx, hc, map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     anthropicOAuthClientID,
	})
}

// anthropicTokenRequest 는 토큰 엔드포인트에 JSON 으로 POST 하고 응답을 OAuthToken 으로
// 파싱한다. 토큰 엔드포인트는 Messages API 가 아니므로 OAuth 주입 트랜스포트를 타면 안
// 된다 — 호출부가 평범한(주입 없는) 클라이언트를 넘긴다.
func anthropicTokenRequest(ctx context.Context, hc *http.Client, payload map[string]string) (OAuthToken, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return OAuthToken{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicOAuthTokenURL, bytes.NewReader(b))
	if err != nil {
		return OAuthToken{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return OAuthToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseTokenResponse(body)
}

// parseTokenResponse 는 토큰 엔드포인트 JSON 응답을 OAuthToken 으로 바꾼다. expires_in(초)
// 은 호출 시점 기준 절대 만료시각으로 환산한다.
func parseTokenResponse(body []byte) (OAuthToken, error) {
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: 토큰 응답 파싱 실패: %w", err)
	}
	if strings.TrimSpace(r.AccessToken) == "" {
		return OAuthToken{}, fmt.Errorf("anthropic oauth: access_token 이 비어 있습니다")
	}
	tok := OAuthToken{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	if r.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Unix() + r.ExpiresIn
	}
	return tok, nil
}

// OAuthState 는 트랜스포트가 쓰는 토큰 매니저다. 현재 토큰을 들고, 만료(임박) 시
// refresh 로 갱신한 뒤 persist 콜백으로 저장소(DB)에 되쓴다. 포인터로 agent.Config 에
// 실리므로 Config 의 == 비교(값 동등성)를 깨지 않는다(apikey 모드는 nil).
type OAuthState struct {
	mu      sync.Mutex
	tok     OAuthToken
	hc      *http.Client           // 토큰 갱신용(주입 트랜스포트가 아닌 평범한 클라이언트)
	persist func(OAuthToken) error // 갱신값 영속화. nil 가능(연결 테스트 등 비영속 경로)
}

// NewOAuthState 는 토큰 매니저를 만든다. proxy 는 토큰 갱신 요청에 쓸 아웃바운드 프록시
// (빈 값=직결). persist 는 갱신된 토큰을 저장소에 반영하는 콜백(nil 허용).
func NewOAuthState(tok OAuthToken, proxy string, persist func(OAuthToken) error) *OAuthState {
	hc, err := quotaAwareHTTPClient(proxy, "", nil)
	if err != nil || hc == nil {
		hc = http.DefaultClient
	}
	return &OAuthState{tok: tok, hc: hc, persist: persist}
}

// Token 은 현재 보관 중인 토큰 사본을 돌려준다(테스트/표시용).
func (o *OAuthState) Token() OAuthToken {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.tok
}

// bearer 는 유효한 access token 을 돌려준다. 만료(임박)면 refresh 후 저장한다. 동시
// 요청이 몰려도 갱신은 한 번만 일어나도록 잠금 아래에서 처리한다.
func (o *OAuthState) bearer(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if strings.TrimSpace(o.tok.AccessToken) == "" {
		return "", fmt.Errorf("anthropic oauth: access token 이 없습니다(구독 로그인이 필요합니다)")
	}
	if o.tok.ExpiresAt > 0 && time.Now().Unix() >= o.tok.ExpiresAt-oauthRefreshSkew {
		if strings.TrimSpace(o.tok.RefreshToken) == "" {
			return "", fmt.Errorf("anthropic oauth: access token 이 만료됐고 refresh token 이 없습니다(다시 로그인하세요)")
		}
		nt, err := RefreshAnthropicToken(ctx, o.hc, o.tok.RefreshToken)
		if err != nil {
			return "", err
		}
		// 일부 응답은 refresh token 을 재발급하지 않는다 — 그럴 땐 기존 값을 유지.
		if strings.TrimSpace(nt.RefreshToken) == "" {
			nt.RefreshToken = o.tok.RefreshToken
		}
		o.tok = nt
		if o.persist != nil {
			_ = o.persist(nt)
		}
	}
	return o.tok.AccessToken, nil
}
