package openaicompat

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

func newExtractor(t *testing.T, s *server, key string) *Extractor {
	t.Helper()
	x, err := New(Options{Endpoint: s.URL + "/v1/", Model: "test-model", APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// toolReply is a chat completion whose message calls the tool with args.
func toolReply(args json.RawMessage) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
			"id": "call_1", "type": "function",
			"function": map[string]any{"name": extract.ToolName, "arguments": args},
		}}},
		"finish_reason": "tool_calls",
	}}})
	return string(b)
}

// stringArgs encodes args as a JSON string, as the OpenAI API sends them.
func stringArgs(args string) json.RawMessage {
	b, _ := json.Marshal(args)
	return b
}

func contentReply(content string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"role": "assistant", "content": content},
	}}})
	return string(b)
}

func TestExtractRequest(t *testing.T) {
	for _, tt := range []struct {
		name, key, wantAuth string
	}{
		{name: "with key", key: "sk-test", wantAuth: "Bearer sk-test"},
		{name: "without key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, http.StatusOK, toolReply(stringArgs(result)))
			in := input()
			if _, err := newExtractor(t, s, tt.key).Extract(context.Background(), in); err != nil {
				t.Fatal(err)
			}
			if s.req.Method != http.MethodPost || s.req.URL.Path != "/v1/chat/completions" {
				t.Errorf("request %s %s, want POST /v1/chat/completions", s.req.Method, s.req.URL.Path)
			}
			if got := s.req.Header.Get("Authorization"); got != tt.wantAuth {
				t.Errorf("Authorization = %q, want %q", got, tt.wantAuth)
			}

			var req struct {
				Model    string `json:"model"`
				Messages []struct {
					Role, Content string
				} `json:"messages"`
				Tools []struct {
					Type     string `json:"type"`
					Function struct {
						Name       string          `json:"name"`
						Parameters json.RawMessage `json:"parameters"`
					} `json:"function"`
				} `json:"tools"`
				ToolChoice struct {
					Type     string `json:"type"`
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_choice"`
				Temperature *float64 `json:"temperature"`
				MaxTokens   int      `json:"max_tokens"`
			}
			if err := json.Unmarshal(s.body, &req); err != nil {
				t.Fatal(err)
			}
			if req.Model != "test-model" {
				t.Errorf("model = %q", req.Model)
			}
			if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
				t.Fatalf("messages = %+v, want system then user", req.Messages)
			}
			if req.Messages[0].Content != extract.SystemPrompt(1) || req.Messages[1].Content != extract.UserMessage(in) {
				t.Error("messages are not the extraction prompt for this input")
			}
			if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != extract.ToolName {
				t.Fatalf("tools = %+v, want the %s function", req.Tools, extract.ToolName)
			}
			if !jsonEqual(t, req.Tools[0].Function.Parameters, extract.ResultSchema()) {
				t.Error("tool parameters are not ResultSchema()")
			}
			if req.ToolChoice.Type != "function" || req.ToolChoice.Function.Name != extract.ToolName {
				t.Errorf("tool_choice = %+v, want the %s function", req.ToolChoice, extract.ToolName)
			}
			if req.Temperature == nil || *req.Temperature != 0 {
				t.Errorf("temperature = %v, want 0", req.Temperature)
			}
			if req.MaxTokens != MaxTokens {
				t.Errorf("max_tokens = %d, want %d", req.MaxTokens, MaxTokens)
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
	xs, _ := json.Marshal(x)
	ys, _ := json.Marshal(y)
	return string(xs) == string(ys)
}

func TestExtractResult(t *testing.T) {
	tests := []struct {
		name  string
		reply string
	}{
		{name: "arguments as a string", reply: toolReply(stringArgs(result))},
		{name: "arguments as an object", reply: toolReply(json.RawMessage(result))},
		{name: "JSON content instead of a tool call", reply: contentReply("Here you go:\n```json\n" + result + "\n```")},
		{name: "JSON content after thinking", reply: contentReply("<think>The {entry} is short.</think>\n" + result)},
		{name: "IDs as strings", reply: toolReply(stringArgs(strings.NewReplacer(`"target_id": 10`, `"target_id": "10"`, `"project_id": 1`, `"project_id": "1"`).Replace(result)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, http.StatusOK, tt.reply)
			in := input()
			res, err := newExtractor(t, s, "").Extract(context.Background(), in)
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
			if err := extract.Validate(in, res); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

func TestNumericIDs(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"task_id":"12","n":[{"target_id":"3"}]}`, `{"n":[{"target_id":3}],"task_id":12}`},
		{`{"task_id":"$new:2","project_id":"01","milestone_id":"0","blocker_id":"x1"}`, `{"blocker_id":"x1","milestone_id":"0","project_id":"01","task_id":"$new:2"}`},
		{`{"title":"12","task_id":12,"confidence":0.25,"parent_task_id":null}`, `{"confidence":0.25,"parent_task_id":null,"task_id":12,"title":"12"}`},
	}
	for _, tt := range tests {
		got, err := numericIDs(json.RawMessage(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tt.want {
			t.Errorf("numericIDs(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		reply   string
		wantErr string
	}{
		{name: "server error message", status: http.StatusNotFound, reply: `{"error":{"message":"model \"test-model\" not found"}}`, wantErr: `404 Not Found: model "test-model" not found`},
		{name: "server error as a string", status: http.StatusBadRequest, reply: `{"error":"bad request"}`, wantErr: "400 Bad Request: bad request"},
		{name: "server error without a message", status: http.StatusInternalServerError, reply: `oops`, wantErr: "500 Internal Server Error"},
		{name: "long server error is cut short", status: http.StatusBadRequest, reply: `{"error":{"message":"` + strings.Repeat("x", 500) + `"}}`, wantErr: strings.Repeat("x", maxErrorMessage) + "…"},
		{name: "not JSON", status: http.StatusOK, reply: `<html>`, wantErr: "decode response"},
		{name: "no choices", status: http.StatusOK, reply: `{"choices":[]}`, wantErr: "no choices"},
		{name: "no tool call", status: http.StatusOK, reply: contentReply("I can't help with that."), wantErr: "did not call " + extract.ToolName},
		{name: "arguments not JSON", status: http.StatusOK, reply: toolReply(stringArgs("{not json")), wantErr: "decode " + extract.ToolName + " arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newServer(t, tt.status, tt.reply)
			_, err := newExtractor(t, s, "sk-secret").Extract(context.Background(), input())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			if len(err.Error()) > 400 {
				t.Errorf("error is %d bytes long", len(err.Error()))
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
	s := newServer(t, http.StatusOK, toolReply(stringArgs(result)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newExtractor(t, s, "").Extract(ctx, input())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNew(t *testing.T) {
	if _, err := New(Options{Model: "m"}); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("no endpoint: err = %v", err)
	}
	if _, err := New(Options{Endpoint: "http://localhost:11434/v1", Model: " "}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("no model: err = %v", err)
	}
	x, err := New(Options{Endpoint: "http://localhost:11434/v1", Model: "qwen3:8b"})
	if err != nil {
		t.Fatal(err)
	}
	if x.Name() != Name || x.Model() != "qwen3:8b" || x.PromptVersion() != extract.PromptVersion {
		t.Errorf("Name, Model, PromptVersion = %q, %q, %q", x.Name(), x.Model(), x.PromptVersion())
	}
	if x.url != "http://localhost:11434/v1/chat/completions" {
		t.Errorf("url = %q", x.url)
	}
}
