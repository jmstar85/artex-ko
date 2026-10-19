package server

// Claude 구독(Pro/Max) OAuth 연결의 서버측 플로우.
//
// 수동 붙여넣기 방식: start 가 로그인 URL + PKCE verifier 를 주고, 사용자가 브라우저에서
// 승인한 뒤 콜백 코드를 붙여넣으면 complete 가 토큰으로 교환한다. 교환한 토큰은 브라우저로
// 내보내지 않고 1회용 ticket 으로 서버측에 잠깐 보관했다가, 프로필 저장 시
// UpdateProfileOAuth 로 해당 프로필에 심는다.

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// oauthTicket 는 complete~save 사이에 토큰을 잠깐 보관하는 1회용 표(TTL 10분).
type oauthTicket struct {
	tok     agent.OAuthToken
	expires time.Time
}

var (
	oauthTicketMu sync.Mutex
	oauthTickets  = map[string]oauthTicket{}
)

func newTicketID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func putOAuthTicket(tok agent.OAuthToken) string {
	id := newTicketID()
	oauthTicketMu.Lock()
	defer oauthTicketMu.Unlock()
	now := time.Now()
	for k, v := range oauthTickets {
		if now.After(v.expires) {
			delete(oauthTickets, k)
		}
	}
	oauthTickets[id] = oauthTicket{tok: tok, expires: now.Add(10 * time.Minute)}
	return id
}

func takeOAuthTicket(id string) (agent.OAuthToken, bool) {
	oauthTicketMu.Lock()
	defer oauthTicketMu.Unlock()
	t, ok := oauthTickets[id]
	if !ok {
		return agent.OAuthToken{}, false
	}
	delete(oauthTickets, id)
	if time.Now().After(t.expires) {
		return agent.OAuthToken{}, false
	}
	return t.tok, true
}

// peekOAuthTicket 는 티켓을 소모하지 않고 토큰만 읽는다. 모델 목록 조회처럼 프로필 저장
// 전에 토큰이 필요하지만 이후 저장(takeOAuthTicket)도 남겨야 하는 경우에 쓴다.
func peekOAuthTicket(id string) (agent.OAuthToken, bool) {
	oauthTicketMu.Lock()
	defer oauthTicketMu.Unlock()
	t, ok := oauthTickets[id]
	if !ok || time.Now().After(t.expires) {
		return agent.OAuthToken{}, false
	}
	return t.tok, true
}

// pgOAuthAnthropicStart 는 사용자가 브라우저에서 열 로그인 URL 과 PKCE verifier 를 준다.
// verifier 는 PKCE 교환에만 쓰이는 1회용 값이라 브라우저로 돌려줘도 된다(비밀 자격증명 아님).
func (s *Server) pgOAuthAnthropicStart(w http.ResponseWriter, r *http.Request) {
	authURL, verifier := agent.AnthropicAuthURL()
	writeJSON(w, 200, map[string]any{"auth_url": authURL, "verifier": verifier})
}

// pgOAuthAnthropicComplete 는 붙여넣은 콜백 코드를 토큰으로 교환하고 ticket 으로 보관한다.
// 토큰 자체는 응답에 담지 않는다 — 프로필 저장 시 ticket 으로 서버측에서 심는다.
func (s *Server) pgOAuthAnthropicComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Verifier) == "" {
		writeErr(w, 400, "code 와 verifier 가 필요합니다")
		return
	}
	tok, err := agent.ExchangeAnthropicCode(r.Context(), nil, req.Code, req.Verifier)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ticket": putOAuthTicket(tok), "expires_at": tok.ExpiresAt})
}

// applyProfileAuth 는 프로필의 인증 방식을 cfg 에 반영한다. anthropic + oauth 프로필이면
// 저장된 토큰으로 OAuthState 를 달고, 실행 중 자동 refresh 가 갱신값을 이 프로필(id)에
// 되쓰도록 persist 콜백을 건다. apikey 프로필이면 아무것도 하지 않는다.
func (s *Server) applyProfileAuth(cfg *agent.Config, p *db.LLMProfile) {
	cfg.AuthMode = p.AuthMode
	if p.AuthMode != "oauth" || p.Format != "anthropic" {
		return
	}
	var persist func(agent.OAuthToken) error
	if id := p.ID; id != 0 && s.m != nil && s.m.pg != nil {
		persist = func(tok agent.OAuthToken) error {
			return s.m.pg.UpdateProfileOAuth(id, tok.AccessToken, tok.RefreshToken, tok.ExpiresAt)
		}
	}
	cfg.OAuthState = agent.NewOAuthState(agent.OAuthToken{
		AccessToken:  p.OAuthAccess,
		RefreshToken: p.OAuthRefresh,
		ExpiresAt:    p.OAuthExpiresAt,
	}, p.Proxy, persist)
}
