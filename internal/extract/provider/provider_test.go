package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		wantName string
		wantErr  string
	}{
		{name: "default is mock", opts: Options{}, wantName: "mock"},
		{name: "mock", opts: Options{Provider: "mock"}, wantName: "mock"},
		{name: "openai-compatible", opts: Options{Provider: "openai-compatible", Endpoint: "http://localhost:11434/v1", Model: "qwen3:8b"}, wantName: "openai-compatible"},
		{name: "openai-compatible without model", opts: Options{Provider: "openai-compatible", Endpoint: "http://localhost:11434/v1"}, wantErr: "model"},
		{name: "openai-compatible without endpoint", opts: Options{Provider: "openai-compatible", Model: "qwen3:8b"}, wantErr: "endpoint"},
		{name: "anthropic", opts: Options{Provider: "anthropic", APIKey: "sk-test"}, wantName: "anthropic"},
		{name: "anthropic without key", opts: Options{Provider: "anthropic"}, wantErr: "api_key"},
		{name: "unknown", opts: Options{Provider: "magic"}, wantErr: "unknown provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, err := New(tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if x.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", x.Name(), tt.wantName)
			}
		})
	}
}

func TestNewTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	x, err := New(Options{Provider: "openai-compatible", Endpoint: srv.URL, Model: "m", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Extract(context.Background(), extract.ExtractInput{Entry: domain.Entry{Body: "hi"}})
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Errorf("err = %v, want a timeout", err)
	}
}
