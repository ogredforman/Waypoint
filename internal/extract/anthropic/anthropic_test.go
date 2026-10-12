package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

const body = "Finished the login page. Next I want to write tests."

// result is what the fake model passes to the tool: one observation linked to
// task 10 and one op, as the model would send them (a quote, no span).
const result = `{
  "observations": [{"kind": "progress", "summary": "Login page done", "quote": "Finished the login page.",
    "links": [{"target_type": "task", "target_id": 10, "confidence": 0.9}]}],
  "ops": [{"op_type": "create_task", "payload": {"project_id": 1, "title": "Write tests"}, "rationale": "stated next step"}],
  "questions": []
}`

func input() extract.ExtractInput {
	return extract.ExtractInput{
		Entry: domain.Entry{ID: 5, Body: body},
		Snapshot: extract.StateSnapshot{Projects: []extract.SnapshotProject{{
			ID: 1, Title: "Waypoint", Status: domain.ProjectActive,
			Tasks: []extract.SnapshotTask{{ID: 10, Title: "Login page", Status: domain.TaskInProgress}},
		}}},
		MaxQuestions:  1,
		PromptVersion: extract.PromptVersion,
	}
}

// server answers every request with status and reply, and keeps the last
// request it got.
type server struct {
	*httptest.Server
	req    *http.Request
	body   []byte
	status int
	reply  string
}

func newServer(t *testing.T, status int, reply string) *server {
	t.Helper()
	s := &server{status: status, reply: reply}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.req = r
		s.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = io.WriteString(w, s.reply)
	}))
	t.Cleanup(s.Close)
	return s
}

func newExtractor(t *testing.T, s *server, model string) *Extractor {
	t.Helper()
	x, err := New(Options{Endpoint: s.URL + "/v1/", Model: model, APIKey: "sk-secret"})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// reply is a Messages response with the given stop reason and content blocks.
func reply(stop string, blocks ...any) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": DefaultModel,
		"stop_reason": stop, "content": blocks,
	})
	return string(b)
}

func toolUse(input json.RawMessage) map[string]any {
	return map[string]any{"type": "tool_use", "id": "toolu_1", "name": extract.ToolName, "input": input}
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func TestExtractRequest(t *testing.T) {
	tests := []struct {
		model      string
		wantChoice string
		wantTool   string
		wantPinned bool // tool_choice forced and temperature 0
	}{
		{model: "claude-haiku-4-5", wantChoice: "tool", wantTool: extract.ToolName, wantPinned: true},
		{model: "claude-sonnet-5-5", wantChoice: "auto"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			s := newServer(t, http.StatusOK, reply("tool_use", toolUse(json.RawMessage(result))))
			in := input()
			if _, err := newExtractor(t, s, tt.model).Extract(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			if s.req.Method != http.MethodPost || s.req.URL.Path != "/v1/messages" {
				t.Errorf("request %s %s, want POST /v1/messages", s.req.Method, s.req.URL.Path)
			}
			if got := s.req.Header.Get("x-api-key"); got != "sk-secret" {
				t.Errorf("x-api-key = %q", got)
			}
			if got := s.req.Header.Get("anthropic-version"); got != APIVersion {
				t.Errorf("anthropic-version = %q", got)
			}

			var req struct {
				Model     string `json:"model"`
				MaxTokens int    `json:"max_tokens"`
				System    []struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					CacheControl *struct {
						Type string `json:"type"`
					} `json:"cache_control"`
				} `json:"system"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
				Tools []struct {
					Name        string          `json:"name"`
					Description string          `json:"description"`
					InputSchema json.RawMessage `json:"input_schema"`
				} `json:"tools"`
				ToolChoice struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"tool_choice"`
				Temperature *float64 `json:"temperature"`
			}
			if err := json.Unmarshal(s.body, &req); err != nil {
				t.Fatal(err)
			}
			if req.Model != tt.model || req.MaxTokens != MaxTokens {
				t.Errorf("model, max_tokens = %q, %d", req.Model, req.MaxTokens)
			}
			if len(req.System) != 1 || req.System[0].Type != "text" || req.System[0].Text != extract.SystemPrompt(1) ||
				req.System[0].CacheControl == nil || req.System[0].CacheControl.Type != "ephemeral" {
				t.Errorf("system = %+v, want the system prompt, cached", req.System)
			}
			if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != extract.UserMessage(in) {
				t.Errorf("messages = %+v, want the user message", req.Messages)
			}
			if len(req.Tools) != 1 || req.Tools[0].Name != extract.ToolName || req.Tools[0].Description != extract.ToolDescription ||
				!jsonEqual(t, req.Tools[0].InputSchema, extract.ResultSchema()) {
				t.Errorf("tools = %+v, want %s with the result schema", req.Tools, extract.ToolName)
			}
			if req.ToolChoice.Type != tt.wantChoice || req.ToolChoice.Name != tt.wantTool {
				t.Errorf("tool_choice = %+v, want %q %q", req.ToolChoice, tt.wantChoice, tt.wantTool)
			}
			if tt.wantPinned && (req.Temperature == nil || *req.Temperature != 0) {
				t.Errorf("temperature = %v, want 0", req.Temperature)
			}
			if !tt.wantPinned && req.Temperature != nil {
				t.Errorf("temperature = %v, want none", *req.Temperature)
			}
		})
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

func TestExtractResult(t *testing.T) {
	quotedIDs := strings.NewReplacer(`"target_id": 10`, `"target_id": "10"`, `"project_id": 1`, `"project_id": "1"`).Replace(result)
	tests := []struct {
		name  string
		reply string
	}{
		{name: "tool call", reply: reply("tool_use", toolUse(json.RawMessage(result)))},
		{name: "text before the tool call", reply: reply("tool_use", text("Here is what I found."), toolUse(json.RawMessage(result)))},
		{name: "IDs as strings", reply: reply("tool_use", toolUse(json.RawMessage(quotedIDs)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, http.StatusOK, tt.reply)
			in := input()
			res, err := newExtractor(t, s, DefaultModel).Extract(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Observations) != 1 || len(res.Ops) != 1 || len(res.Questions) != 0 {
				t.Fatalf("got %+v, want 1 observation, 1 op, no questions", res)
			}
			o := res.Observations[0]
			if want := int64(len("Finished the login page.")); o.SpanStart != 0 || o.SpanEnd != want {
				t.Errorf("span = %d-%d, want 0-%d (from the quote)", o.SpanStart, o.SpanEnd, want)
			}
			if res.Ops[0].OpType != domain.OpCreateTask {
				t.Errorf("op type = %q", res.Ops[0].OpType)
			}
			if string(res.Response) != tt.reply {
				t.Errorf("response = %s, want the reply", res.Response)
			}
			if err := extract.Validate(in, res); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		reply   string
		wantErr string
	}{
		{name: "API error", status: http.StatusUnauthorized, reply: `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, wantErr: "401 Unauthorized: invalid x-api-key"},
		{name: "overloaded", status: 529, reply: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, wantErr: "529"},
		{name: "not JSON", status: http.StatusOK, reply: `<html>`, wantErr: "decode response"},
		{name: "cut off", status: http.StatusOK, reply: reply("max_tokens", text("Let me think")), wantErr: "cut off"},
		{name: "refusal", status: http.StatusOK, reply: reply("refusal"), wantErr: "declined"},
		{name: "no tool call", status: http.StatusOK, reply: reply("end_turn", text("I cannot help with that.")), wantErr: "did not call " + extract.ToolName},
		{name: "input not an object", status: http.StatusOK, reply: reply("tool_use", toolUse(json.RawMessage(`"oops"`))), wantErr: "decode " + extract.ToolName + " input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, tt.status, tt.reply)
			_, err := newExtractor(t, s, DefaultModel).Extract(context.Background(), input())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			// A reply the model sent is kept for the run; a server error's
			// body isn't a reply.
			want := ""
			if tt.status == http.StatusOK {
				want = tt.reply
			}
			if got := string(extract.ResponseOf(err)); got != want {
				t.Errorf("response = %q, want %q", got, want)
			}
			for _, secret := range []string{body, "sk-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error %q contains %q", err, secret)
				}
			}
		})
	}
}

func TestExtractCanceled(t *testing.T) {
	s := newServer(t, http.StatusOK, reply("tool_use", toolUse(json.RawMessage(result))))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newExtractor(t, s, DefaultModel).Extract(ctx, input())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNew(t *testing.T) {
	if _, err := New(Options{Model: "m", APIKey: " "}); err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("no key: err = %v", err)
	}
	x, err := New(Options{APIKey: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	if x.Name() != Name || x.Model() != DefaultModel || x.PromptVersion() != extract.PromptVersion {
		t.Errorf("Name, Model, PromptVersion = %q, %q, %q", x.Name(), x.Model(), x.PromptVersion())
	}
	if x.url != DefaultEndpoint+"/messages" {
		t.Errorf("url = %q", x.url)
	}
	x, err = New(Options{Endpoint: "https://proxy.example/v1/", Model: "claude-sonnet-5-5", APIKey: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	if x.url != "https://proxy.example/v1/messages" || x.Model() != "claude-sonnet-5-5" {
		t.Errorf("url, model = %q, %q", x.url, x.Model())
	}
}
