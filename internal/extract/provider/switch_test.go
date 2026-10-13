package provider

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// saved records what a Switch saves.
type saved struct {
	calls []map[string]string
	err   error
}

func (s *saved) save(v map[string]string) error {
	s.calls = append(s.calls, maps.Clone(v))
	return s.err
}

func newSwitch(t *testing.T, o Options, fromEnv []string) (*Switch, *saved, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	sv := &saved{}
	sw, err := NewSwitch(o, fromEnv, sv.save, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return sw, sv, &logs
}

func ptr(s string) *string { return &s }

func TestSwitchUpdate(t *testing.T) {
	ctx := context.Background()
	sw, sv, logs := newSwitch(t, Options{}, nil)

	if _, err := sw.Call(ctx, extract.ToolCall{}); !errors.Is(err, extract.ErrNoModel) {
		t.Errorf("mock Call err = %v, want ErrNoModel", err)
	}
	if got := sw.Settings(); got.Provider != "mock" || got.APIKeySet || !slices.Equal(got.Providers, Names()) {
		t.Errorf("settings = %+v", got)
	}

	got, err := sw.Update(ctx, domain.ProviderUpdate{Provider: "anthropic", Model: " claude-haiku-4-5 ", APIKey: ptr("sk-secret")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "anthropic" || got.Model != "claude-haiku-4-5" || !got.APIKeySet {
		t.Errorf("after update = %+v", got)
	}
	if sw.Name() != "anthropic" || sw.Model() != "claude-haiku-4-5" || sw.PromptVersion() != extract.PromptVersion {
		t.Errorf("switch is %s %s %s", sw.Name(), sw.Model(), sw.PromptVersion())
	}
	want := map[string]string{"provider": "anthropic", "model": "claude-haiku-4-5", "endpoint": "", "api_key": "sk-secret"}
	if len(sv.calls) != 1 || !maps.Equal(sv.calls[0], want) {
		t.Errorf("saved %v, want %v", sv.calls, want)
	}
	if strings.Contains(logs.String(), "sk-secret") {
		t.Error("the key was logged")
	}

	// No api_key keeps the key; an empty one removes it.
	if got, err := sw.Update(ctx, domain.ProviderUpdate{Provider: "anthropic"}); err != nil || !got.APIKeySet {
		t.Errorf("kept key: %+v, %v", got, err)
	}
	if got, err := sw.Update(ctx, domain.ProviderUpdate{Provider: "mock", APIKey: ptr("")}); err != nil || got.APIKeySet || got.Provider != "mock" {
		t.Errorf("cleared key: %+v, %v", got, err)
	}
	if last := sv.calls[len(sv.calls)-1]; last["api_key"] != "" {
		t.Errorf("last save %v, want the key removed", last)
	}
}

func TestSwitchUpdateRefuses(t *testing.T) {
	ctx := context.Background()
	start := Options{Provider: "openai-compatible", Model: "qwen3:8b", Endpoint: "http://localhost:11434/v1", APIKey: "from-env"}
	tests := []struct {
		name      string
		u         domain.ProviderUpdate
		wantField string
	}{
		{name: "unknown provider", u: domain.ProviderUpdate{Provider: "gpt", Model: "qwen3:8b"}, wantField: "provider"},
		{name: "openai-compatible without endpoint", u: domain.ProviderUpdate{Provider: "openai-compatible", Model: "qwen3:8b"}, wantField: "provider"},
		{name: "key from env", u: domain.ProviderUpdate{Provider: "openai-compatible", Model: "qwen3:8b", Endpoint: "http://localhost:11434/v1", APIKey: ptr("other")}, wantField: "api_key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sw, sv, _ := newSwitch(t, start, []string{"api_key"})
			_, err := sw.Update(ctx, tt.u)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Field != tt.wantField {
				t.Fatalf("err = %v, want a problem with %s", err, tt.wantField)
			}
			if len(sv.calls) != 0 || sw.Settings().Model != "qwen3:8b" {
				t.Errorf("refused update changed something: saved %v", sv.calls)
			}
		})
	}
}

func TestSwitchUpdateKeepsEnvOutOfFile(t *testing.T) {
	ctx := context.Background()
	sw, sv, _ := newSwitch(t, Options{Provider: "anthropic", APIKey: "from-env"}, []string{"api_key"})
	if _, err := sw.Update(ctx, domain.ProviderUpdate{Provider: "anthropic", Model: "claude-sonnet-4-5"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sv.calls[0]["api_key"]; ok {
		t.Errorf("saved %v; an env var's value must not be written to the file", sv.calls[0])
	}
}

func TestSwitchUpdateSaveFails(t *testing.T) {
	sw, sv, _ := newSwitch(t, Options{}, nil)
	sv.err = errors.New("disk full")
	if _, err := sw.Update(context.Background(), domain.ProviderUpdate{Provider: "anthropic", APIKey: ptr("k")}); err == nil {
		t.Fatal("no error")
	}
	if sw.Name() != "mock" {
		t.Errorf("switched to %s though saving failed", sw.Name())
	}

	nowhere, err := NewSwitch(Options{}, nil, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	var ve *domain.ValidationError
	if _, err := nowhere.Update(context.Background(), domain.ProviderUpdate{Provider: "mock"}); !errors.As(err, &ve) {
		t.Errorf("no save func: err = %v, want a validation error", err)
	}
}
