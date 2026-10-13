package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
)

// testProvider is an in-memory api.Provider. provider.Switch is the real
// one; its tests cover the checks and saving.
type testProvider struct {
	s   domain.ProviderSettings
	got []domain.ProviderUpdate
}

func (p *testProvider) Settings() domain.ProviderSettings { return p.s }

func (p *testProvider) Update(_ context.Context, u domain.ProviderUpdate) (domain.ProviderSettings, error) {
	if u.Provider == "gpt" {
		return domain.ProviderSettings{}, &domain.ValidationError{Entity: "provider", Problems: []domain.FieldError{{Field: "provider", Problem: "unknown provider"}}}
	}
	p.got = append(p.got, u)
	p.s.Provider, p.s.Model, p.s.Endpoint = u.Provider, u.Model, u.Endpoint
	if u.APIKey != nil {
		p.s.APIKeySet = *u.APIKey != ""
	}
	return p.s, nil
}

func TestProviderRoutes(t *testing.T) {
	logger, logs := jsonLogger()
	p := &testProvider{s: domain.ProviderSettings{Provider: "mock", Providers: []string{"mock", "anthropic"}}}
	s := &server{t: t, h: NewRouter(Deps{Logger: logger, Provider: p}), logs: logs}

	got := ok[apitypes.Provider](s, http.StatusOK, "GET", "/api/provider", nil)
	if got.Provider != "mock" || got.FromEnv == nil || !slices.Equal(got.Providers, []string{"mock", "anthropic"}) {
		t.Errorf("get = %+v (from_env must be [] not null)", got)
	}

	got = ok[apitypes.Provider](s, http.StatusOK, "PUT", "/api/provider", `{"provider":"anthropic","model":"claude-haiku-4-5","endpoint":"","api_key":"sk-secret"}`)
	if got.Provider != "anthropic" || !got.APIKeySet {
		t.Errorf("put = %+v", got)
	}
	if rec := s.call("GET", "/api/provider", nil); strings.Contains(rec.Body.String()+logs.String(), "sk-secret") {
		t.Error("the key came back or was logged")
	}
	ok[apitypes.Provider](s, http.StatusOK, "PUT", "/api/provider", `{"provider":"anthropic"}`)
	if last := p.got[len(p.got)-1]; last.APIKey != nil {
		t.Errorf("no api_key gave %q, want nil (keep the key)", *last.APIKey)
	}

	if e := s.fails(http.StatusUnprocessableEntity, "invalid", "PUT", "/api/provider", `{"provider":"gpt"}`); len(e.Problems) != 1 || e.Problems[0].Field != "provider" {
		t.Errorf("problems = %+v", e.Problems)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "PUT", "/api/provider", `{"provider":"mock","temperature":1}`)

	// A cross-site form or text/plain fetch needs no preflight, so it's refused.
	req := httptest.NewRequest("PUT", "/api/provider", strings.NewReader(`{"provider":"openai-compatible","endpoint":"http://evil.example/v1","model":"m"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType || len(p.got) != 2 {
		t.Errorf("text/plain PUT: %d, %d updates; want 415 and no update", rec.Code, len(p.got))
	}
	s.fails(http.StatusMethodNotAllowed, "method_not_allowed", "PATCH", "/api/provider", `{}`)
}
