package agent

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/artex/sidequestion"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
)

// The transport is the only layer that still sees the wire bodies: norma builds
// the request body internally and decodes the SSE response before the recorder
// gets it. This checks the round trip preserves both directions untouched.
func TestRoundTripCapturesRawBodies(t *testing.T) {
	const sse = "event: message_start\ndata: {\"type\":\"message_start\"}\n\nevent: message_stop\ndata: {}\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	defer srv.Close()

	client, err := quotaAwareHTTPClient("", "", nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	const reqBody = `{"model":"claude","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"t","input_schema":{}}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	ctx, capt := llmrec.NewCapture(req.Context())
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = resp.Body.Close()

	if string(got) != sse {
		t.Fatal("capture altered the response delivered to norma")
	}
	if capt.RawRequest() != reqBody {
		t.Fatalf("RawRequest()=%q want %q", capt.RawRequest(), reqBody)
	}
	if capt.RawResponse() != sse {
		t.Fatalf("RawResponse()=%q want the raw SSE frames", capt.RawResponse())
	}
}

// A 429 body is read and replaced in-place by the quota check. Capturing must
// still see it, and the replacement body must remain readable downstream.
func TestRoundTripCaptures429BodyAlongsideQuotaRewrite(t *testing.T) {
	const body = `{"error":{"message":"insufficient_quota"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	client, err := quotaAwareHTTPClient("", "", nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	ctx, capt := llmrec.NewCapture(req.Context())
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	// Quota exhaustion is normalized to 402 so the router fails over.
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status=%d want 402", resp.StatusCode)
	}
	if capt.RawResponse() != body {
		t.Fatalf("RawResponse()=%q want %q", capt.RawResponse(), body)
	}
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read replaced body: %v", err)
	}
	if string(rest) != body {
		t.Fatalf("replaced body=%q want it still readable", rest)
	}
}

// Recording off = no Capture on the context. The transport must behave exactly
// as before, including the quota rewrite.
func TestRoundTripWithoutCaptureIsUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	client, err := quotaAwareHTTPClient("", "", nil)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	resp, err := client.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("body=%q", got)
	}
}

// Capture the real SDK-to-provider request, not a separately built test prompt.
type localeCaptureProvider struct {
	capture func(context.Context, llm.CompletionRequest) error
	reply func() (llm.Message, string)
}

func (p localeCaptureProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	if err := p.capture(ctx, req); err != nil {
		return llm.Message{}, "", llm.Usage{}, err
	}
	if p.reply != nil {
		message, stop := p.reply()
		return message, stop, llm.Usage{}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("done")}}, "end_turn", llm.Usage{}, nil
}

func (p localeCaptureProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		message, stop, _, err := p.Complete(ctx, req)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		for _, block := range message.Content {
			if block.Type == llm.BlockToolUse {
				if !yield(llm.StreamEvent{Type: llm.SEToolUseStart, ToolID: block.ID, ToolName: block.Name}, nil) {
					return
				}
				if !yield(llm.StreamEvent{Type: llm.SEToolInputJSON, Text: string(block.Input)}, nil) {
					return
				}
			} else if !yield(llm.StreamEvent{Type: llm.SETextDelta, Text: block.Text}, nil) {
				return
			}
		}
		if !yield(llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: stop}, nil) {
			return
		}
		yield(llm.StreamEvent{Type: llm.SEMessageStop}, nil)
	}
}

func assertLocaleRequest(t *testing.T, req llm.CompletionRequest, locale string, reminder bool) {
	t.Helper()
	system := strings.Join(req.System, "\n")
	if !strings.Contains(system, langDirective(locale)) {
		t.Errorf("provider system lost locale %q: %q", locale, system)
	}
	for _, other := range []string{"", "ko", "en", "zh"} {
		if other != locale && strings.Contains(system, langDirective(other)) {
			t.Errorf("provider locale %q contaminated by %q", locale, other)
		}
	}
	if reminder {
		var user strings.Builder
		for _, msg := range req.Messages {
			if msg.Role == llm.RoleUser {
				for _, block := range msg.Content {
					user.WriteString(block.Text)
				}
			}
		}
		if !strings.HasSuffix(user.String(), langReminder(locale)) {
			t.Errorf("provider user turn lost final locale %q reminder: %q", locale, user.String())
		}
	}
}

func TestCachedChatLocaleProviderIsolation(t *testing.T) {
	previous := PromptOverride
	t.Cleanup(func() { PromptOverride = previous })
	const customBody = "CUSTOM BODY: keep /data/evidence and SELECT 1 unchanged"
	PromptOverride = func(string) (string, bool) { return customBody, true }
	for _, nonStreaming := range []bool{false, true} {
		for _, role := range []string{"chat", "custom", "reporter", "auto"} {
			t.Run(role+map[bool]string{false: "/stream", true: "/complete"}[nonStreaming], func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				started := make(chan struct{}, 4)
				release := make(chan struct{})
				provider := localeCaptureProvider{capture: func(ctx context.Context, req llm.CompletionRequest) error {
					assertLocaleRequest(t, req, LocaleFromContext(ctx), false)
					if !strings.Contains(strings.Join(req.System, "\n"), customBody) {
						t.Error("provider lost custom prompt body")
					}
					started <- struct{}{}
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}}
				bound := sidequestion.Bind(provider, sidequestion.Model{Model: "fixture", Streaming: !nonStreaming})
				dir := t.TempDir()
				chat := NewChatAgent(bound, "fixture", dir, transcript.NewStore(dir+"/transcripts"), 100000)
				chat.SetNonStreaming(func() bool { return nonStreaming })
				done := make(chan error, 4)
				for i, locale := range []string{"zh", "ko", "en", ""} {
					go func() {
						var snapshot sidequestion.Snapshot
						runCtx := sidequestion.WithPublisher(WithLocale(ctx, locale), func(s sidequestion.Snapshot) { snapshot = s })
						_, err := chat.Chat(runCtx, role, "conv-"+strconv.Itoa(i+1), "original user input", 3, 0, false, nil)
						if err == nil {
							req, buildErr := sidequestion.BuildRequest(snapshot, nil, "side question")
							if buildErr != nil {
								err = buildErr
							} else {
								assertLocaleRequest(t, req, locale, false)
							}
						}
						done <- err
					}()
				}
				for range 4 {
					select {
					case <-started:
					case <-ctx.Done():
						t.Fatal("concurrent runs did not reach the provider")
					}
				}
				close(release)
				for range 4 {
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestAutonomousLocaleProviderRequests(t *testing.T) {
	// Never fall back to the application's operational database. The integration
	// gate supplies a disposable PostgreSQL database explicitly.
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated ARTEX_PG_DSN")
	}
	d, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	expID, err := d.CreateExploration("locale fixture", "original user description")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM explorations WHERE id=$1`, expID) })
	ts := d.Exploration(expID)
	payload := map[string]any{"summary": "original intent"}
	intentID, err := ts.AddNode("intent", payload, 0, "running", "planner", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh", "ko", "en", ""} {
		t.Run(locale, func(t *testing.T) {
			ctx := WithLocale(t.Context(), locale)
			for _, role := range []string{"planner", "worker", "goals", "mainagent"} {
				t.Run(role, func(t *testing.T) {
					calls := 0
					provider := localeCaptureProvider{capture: func(_ context.Context, req llm.CompletionRequest) error {
						calls++
						assertLocaleRequest(t, req, locale, role != "mainagent")
						return nil
					}}
					dir := t.TempDir()
					var err error
					switch role {
					case "planner":
						p := NewPlanner(provider, "fixture", dir, nil, 100000, 3)
						_, _, err = p.Plan(ctx, expID, nil, ts, "original goal", nil, nil)
					case "worker":
						w := NewWorker(provider, "fixture", dir, transcript.NewStore(dir+"/transcripts"), 100000, 3)
						intent := &db.Node{ID: intentID, Payload: raw}
						_, _, err = w.Execute(ctx, "fixture-worker", expID, nil, ts, intent, nil, nil, nil, nil)
						if err == nil {
							_, _, err = w.Execute(ctx, "fixture-worker", expID, nil, ts, intent, nil, nil, nil, nil)
						}
						if err == nil {
							_, _, err = w.ExecuteWithMessage(ctx, "fixture-worker", expID, nil, ts, intent, nil, nil, nil, nil, "locale-turn", "original continuation")
						}
						if calls != 3 {
							t.Errorf("worker initial/resume/continuation calls = %d, want 3", calls)
						}
					case "goals":
						DecomposeGoalsWithProvider(ctx, provider, dir, "original goal", "", nil, ts, expID, false, 0, nil)
					case "mainagent":
						m := NewMainAgent(provider, "fixture", dir, nil, 100000, 3)
						_, err = m.Chat(ctx, expID, 0, nil, ts, "original goal", "original message", nil, nil, nil, nil, nil)
					}
					if err != nil {
						t.Fatal(err)
					}
					if calls == 0 {
						t.Fatal("runtime did not reach the provider")
					}
				})
			}
		})
	}
}

func TestLocaleProviderSettlementPreservesLanguage(t *testing.T) {
	for _, locale := range []string{"zh", "ko", "en", ""} {
		t.Run(locale, func(t *testing.T) {
			dir := t.TempDir()
			fixture := dir + "/fixture.txt"
			if err := os.WriteFile(fixture, []byte("raw evidence"), 0600); err != nil {
				t.Fatal(err)
			}
			input, err := json.Marshal(map[string]string{"file_path": fixture})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			provider := localeCaptureProvider{
				capture: func(_ context.Context, req llm.CompletionRequest) error {
					calls++
					assertLocaleRequest(t, req, locale, false)
					if calls == 2 {
						wire, err := json.Marshal(req.Messages)
						if err != nil {
							return err
						}
						if !strings.Contains(string(wire), WrapupDefault("custom")) {
							t.Error("budget exhaustion did not deliver the wrap-up instruction")
						}
					}
					return nil
				},
				reply: func() (llm.Message, string) {
					if calls == 1 {
						return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
							Type: llm.BlockToolUse, ID: "locale-read", Name: "Read", Input: input,
						}}}, "tool_use"
					}
					return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("done")}}, "end_turn"
				},
			}
			chat := NewChatAgent(provider, "fixture", dir, nil, 100000)
			_, err = chat.Chat(WithLocale(t.Context(), locale), "custom", "settlement", "Read the fixture", 1, 0, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("provider calls = %d, want initial plus settlement", calls)
			}
		})
	}
}
