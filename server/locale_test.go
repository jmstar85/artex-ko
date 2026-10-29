package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"strconv"
	"testing"

	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/artex/db"
)

func newLocaleServer(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" { t.Fatal("ARTEX_PG_DSN must point to an isolated test database") }
	pg, err := db.Open(dsn)
	if err != nil { t.Fatal("isolated database unavailable") }
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{ctx: ctx, m: &Manager{pg: pg, dir: t.TempDir(), tasks: map[string]*Task{}},
		chatBusy: map[string]bool{}, chatCancel: map[string]context.CancelCauseFunc{},
		triggerQ: map[string][]triggeredRun{}, triggerCfg: map[string]triggerBehavior{}, triggerActive: map[string]int{}}
	s.engine = NewEngine(s.m)
	t.Cleanup(func() { cancel(); waitRetestIdle(t, s); pg.Close() })
	return s
}

func TestHTTPLocalePersistence(t *testing.T) {
	s := newLocaleServer(t)
	for _, value := range []string{"ko", "en", "zh", "", "ZH", "fr", "zh-CN"} {
		request := httptest.NewRequest("POST", "/", nil)
		request.Header.Set("X-Artex-Locale", value)
		want := value
		if value != "ko" && value != "en" && value != "zh" { want = "" }
		if got := requestLocale(request); got != want { t.Fatalf("header %q normalized to %q", value, got) }
	}
	if _, err := s.m.pg.Exec(`INSERT INTO agents(key,name,role) VALUES ('locale_regression','locale','assistant') ON CONFLICT (key) DO NOTHING`); err != nil { t.Fatal(err) }
	t.Cleanup(func() { s.m.pg.Exec(`DELETE FROM agents WHERE key='locale_regression'`) })
	r := httptest.NewRequest("POST", "/api/conversations", strings.NewReader(`{"agent_key":"locale_regression","title":"locale"}`))
	r.Header.Set("X-Artex-Locale", "zh")
	w := httptest.NewRecorder()
	s.pgCreateConversation(w, r)
	if w.Code != 200 { t.Fatalf("create status=%d body=%s", w.Code, w.Body) }
	var conv db.Conversation
	if err := json.Unmarshal(w.Body.Bytes(), &conv); err != nil { t.Fatal(err) }
	t.Cleanup(func() { s.m.pg.DeleteConversation(conv.ID) })
	got, err := s.m.pg.GetConversation(conv.ID)
	if err != nil || got == nil || got.Locale != "zh" { t.Fatalf("persisted locale: %+v, %v", got, err) }
	t.Run("task and child lifetime", func(t *testing.T) {
		defer restoreConcurrencySetting(t, s.m)()
		if err := s.m.SetConcurrency(true, 1); err != nil { t.Fatal(err) }
		taskRequest := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"description":"locale task","goal":"locale goal"}`))
		taskRequest.Header.Set("X-Artex-Locale", "zh")
		taskResponse := httptest.NewRecorder()
		s.createTask(taskResponse, taskRequest)
		if taskResponse.Code != 201 { t.Fatalf("task create: %d %s", taskResponse.Code, taskResponse.Body) }
		var dto TaskDTO
		if err := json.Unmarshal(taskResponse.Body.Bytes(), &dto); err != nil { t.Fatal(err) }
		task, ok := s.m.Task(dto.ID)
		if !ok || dto.Locale != "zh" || task.Locale != "zh" { t.Fatal("HTTP task locale lost") }
		t.Cleanup(func() { s.engine.StopTask(task.ID); s.m.pg.DeleteTask(mustLocaleID(t, task.ID)) })
		stored, err := s.m.pg.GetTask(mustLocaleID(t, task.ID))
		if err != nil { t.Fatal(err) }
		reloaded := taskFromPG(stored, task.Store, nil)
		if reloaded.Locale != "zh" { t.Fatal("reload lost locale") }
		oldChat := s.chatAgent
		setRetestProvider(s, retestProvider{})
		s.triggerActive["locale_regression"] = 1
		s.StartTriggeredRun("locale_regression", "source report", "source report", stored.ID, true, task.Description, task.Goal)
		s.chatAgent = oldChat
		queued := s.triggerQ["locale_regression"]
		if len(queued) != 1 || queued[0].locale != "zh" { t.Fatal("trigger lost source task locale") }
		delete(s.triggerQ, "locale_regression")
		delete(s.triggerActive, "locale_regression")
		s.engine.Resume(task)
		ctx := s.engine.execContextFor(context.Background(), task.ID)
		if agent.LocaleFromContext(ctx) != "zh" { t.Fatal("execution lost locale") }
		s.engine.Pause(task.ID, context.Canceled)
		s.engine.Resume(task)
		resumed := s.engine.execContextFor(context.Background(), task.ID)
		if resumed == ctx || ctx.Err() == nil || agent.LocaleFromContext(resumed) != "zh" { t.Fatal("resume lost locale or cancellation") }
		result, err := s.toolSpawnTask().Call(agent.WithLocale(context.Background(), "zh"), json.RawMessage(`{"description":"locale child","goal":"locale child"}`), nil)
		if err != nil || result.IsError { t.Fatalf("spawn: %+v %v", result, err) }
		children := 0
		for _, child := range s.m.tasks {
			if child.ID != task.ID {
				children++
				if child.Locale != "zh" { t.Fatal("child locale lost") }
				t.Cleanup(func() { s.engine.StopTask(child.ID); s.m.pg.DeleteTask(mustLocaleID(t, child.ID)) })
			}
		}
		if children != 1 { t.Fatalf("children=%d", children) }
		fid, err := s.m.pg.AddFinding(mustLocaleID(t, task.ID), 0, "locale", "locale", "low", "summary", "evidence", "worker", nil)
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { s.m.pg.DeleteFinding(fid) })
		_, retestConv, created, err := s.m.pg.CreateFindingRetest(context.Background(), fid, "")
		if err != nil || !created || retestConv.Locale != "zh" { t.Fatalf("retest locale: %+v %v", retestConv, err) }
		t.Cleanup(func() { s.m.pg.DeleteConversation(retestConv.ID) })
	})
	t.Run("provider turn snapshots", func(t *testing.T) {
		oldPrompt, oldAugment, oldResolve := agent.PromptOverride, agent.ToolAugment, agent.ToolResolve
		agent.PromptOverride = func(string) (string, bool) { return "preserve-user-prompt", true }
		agent.ToolAugment, agent.ToolResolve = nil, nil
		defer func() { agent.PromptOverride, agent.ToolAugment, agent.ToolResolve = oldPrompt, oldAugment, oldResolve }()
		type observed struct { locale string; request llm.CompletionRequest }
		requests := make(chan observed, 8)
		release := make(chan struct{})
		released := false
		defer func() { if !released { close(release) }; waitRetestIdle(t, s) }()
		setRetestProvider(s, retestProvider{complete: func(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
			requests <- observed{agent.LocaleFromContext(ctx), req}
			select { case <-release: case <-ctx.Done(): return llm.Message{}, "", llm.Usage{}, ctx.Err() }
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("done")}}, "end_turn", llm.Usage{}, nil
		}})
		other, err := s.m.pg.CreateConversationWithLocale("locale_regression", "other", nil, "ko")
		if err != nil { t.Fatal(err) }
		defer s.m.pg.DeleteConversation(other.ID)
		send := func(id int64, locale string) int {
			r := httptest.NewRequest("POST", "/", strings.NewReader(`{"message":"keep original user message"}`))
			r.SetPathValue("id", strconv.FormatInt(id, 10))
			r.Header.Set("X-Artex-Locale", locale)
			w := httptest.NewRecorder()
			s.pgSendConversationMessage(w, r)
			return w.Code
		}
		if code := send(conv.ID, "zh"); code != 202 { t.Fatalf("zh send=%d", code) }
		if code := send(other.ID, "ko"); code != 202 { t.Fatalf("ko send=%d", code) }
		if code := send(conv.ID, "en"); code != 409 { t.Fatalf("busy send=%d", code) }
		check := func(want map[string]bool) {
			t.Helper()
			for range len(want) {
				select {
				case got := <-requests:
					if !want[got.locale] { t.Fatalf("unexpected locale %q", got.locale) }
					delete(want, got.locale)
					body, err := json.Marshal(got.request)
					if err != nil { t.Fatal(err) }
					rule := map[string]string{"zh":"本次执行选择的输出语言是中文。", "ko":"이번 실행의 출력 언어는 한국어입니다.", "en":"The selected output language for this run is English."}[got.locale]
					if !strings.Contains(string(body), rule) || !strings.Contains(string(body), "preserve-user-prompt") { t.Fatalf("provider missing selected rule or original prompt for %q", got.locale) }
				case <-time.After(5*time.Second): t.Fatal("provider request missing")
				}
			}
		}
		check(map[string]bool{"zh":true,"ko":true})
		persisted, err := s.m.pg.GetConversation(conv.ID)
		if err != nil || persisted.Locale != "zh" { t.Fatal("rejected turn changed stored locale") }
		close(release)
		released = true
		waitRetestIdle(t, s)
		if code := send(conv.ID, "en"); code != 202 { t.Fatalf("en send=%d", code) }
		check(map[string]bool{"en":true})
		waitRetestIdle(t, s)
		if code := send(conv.ID, "invalid"); code != 202 { t.Fatalf("stored locale send=%d", code) }
		check(map[string]bool{"en":true})
		waitRetestIdle(t, s)
		s.runTriggeredRun(triggeredRun{agentKey:"locale_regression",locale:"zh",title:"locale report",message:"write report"})
		check(map[string]bool{"zh":true})
		rows, err := s.m.pg.ListConversations()
		if err != nil { t.Fatal(err) }
		for _, row := range rows {
			if row.Title == "locale report" {
				defer s.m.pg.DeleteConversation(row.ID)
				if row.Locale != "zh" { t.Fatal("report locale lost") }
			}
		}
		if err := s.seedFindingRetester(); err != nil { t.Fatal(err) }
		fid, err := s.m.pg.AddFinding(0, 0, "locale", "locale", "low", "summary", "evidence", "worker", nil)
		if err != nil { t.Fatal(err) }
		defer s.m.pg.DeleteFinding(fid)
		_, retestConv, created, err := s.m.pg.CreateFindingRetestWithLocale(context.Background(), fid, "", "zh")
		if err != nil || !created || retestConv.Locale != "zh" { t.Fatalf("unbound retest locale: %+v %v", retestConv, err) }
		defer s.m.pg.DeleteConversation(retestConv.ID)
		s.runConversationSync(retestConv, "retest", s.convBusyKey(retestConv.ID))
		check(map[string]bool{"zh":true})
	})
	t.Run("trigger merge isolation", func(t *testing.T) {
		for _, mode := range []string{"all","by_task"} {
			s.triggerQ["locale"] = []triggeredRun{
				{agentKey:"locale",locale:"zh",taskID:1,mergeable:true,message:"first"},
				{agentKey:"locale",locale:"ko",taskID:1,mergeable:true,message:"second"},
				{agentKey:"locale",locale:"zh",taskID:1,mergeable:true,message:"third"},
			}
			first := s.nextTriggerRun("locale", triggerBehavior{runMode:"serial",mergeMode:mode})
			if first.locale != "zh" || strings.Contains(first.message,"second") || !strings.Contains(first.message,"third") { t.Fatalf("%s cross-language merge", mode) }
			second := s.nextTriggerRun("locale", triggerBehavior{runMode:"serial",mergeMode:mode})
			if second.locale != "ko" || second.message != "second" { t.Fatalf("%s lost queued language", mode) }
		}
	})
}

func mustLocaleID(t *testing.T, raw string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil { t.Fatal(err) }
	return id
}
