package llmhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPost(t *testing.T) {
	var got *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	raw, err := Post(context.Background(), srv.Client(), srv.URL+"/x", http.Header{"X-Key": {"k"}}, map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ok":true}` {
		t.Errorf("body = %s", raw)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/x" {
		t.Errorf("request %s %s", got.Method, got.URL.Path)
	}
	if got.Header.Get("X-Key") != "k" || got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", got.Header)
	}
	if string(gotBody) != `{"a":1}` {
		t.Errorf("sent %s", gotBody)
	}
}

func TestPostStatusError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		reply   string
		wantErr string
	}{
		{name: "error message", status: http.StatusNotFound, reply: `{"error":{"message":"model \"m\" not found"}}`, wantErr: `server returned 404 Not Found: model "m" not found`},
		{name: "error as a string", status: http.StatusBadRequest, reply: `{"error":"bad request"}`, wantErr: "400 Bad Request: bad request"},
		{name: "no message", status: http.StatusInternalServerError, reply: `oops`, wantErr: "500 Internal Server Error"},
		{name: "long message is cut short", status: http.StatusBadRequest, reply: `{"error":{"message":"` + strings.Repeat("x", 500) + `"}}`, wantErr: strings.Repeat("x", MaxErrorMessage) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.reply)
			}))
			defer srv.Close()
			_, err := Post(context.Background(), srv.Client(), srv.URL, nil, struct{}{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			if len(err.Error()) > 400 {
				t.Errorf("error is %d bytes long", len(err.Error()))
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
		got, err := NumericIDs(json.RawMessage(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tt.want {
			t.Errorf("NumericIDs(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
