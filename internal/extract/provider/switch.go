package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/anthropic"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/extract/openaicompat"
)

// SaveFunc stores provider settings: each key ("provider", "model",
// "endpoint", "api_key") set to its value, or removed if the value is empty.
type SaveFunc func(values map[string]string) error

// Switch is the extractor every profile uses, and the provider settings
// behind it, which Update can change while the server runs. Each call goes to
// the extractor in effect when it starts, so a request already sent finishes
// with the old one. As a Caller it returns extract.ErrNoModel while the
// provider is the mock.
type Switch struct {
	mu      sync.RWMutex
	x       extract.Extractor
	opts    Options
	fromEnv []string
	save    SaveFunc
	logger  *slog.Logger
}

// NewSwitch builds the extractor o names. fromEnv lists the keys set by env
// vars, which Update refuses to change; save stores the others (nil if there
// is nowhere to save them).
func NewSwitch(o Options, fromEnv []string, save SaveFunc, logger *slog.Logger) (*Switch, error) {
	x, err := New(o)
	if err != nil {
		return nil, err
	}
	return &Switch{x: x, opts: o, fromEnv: fromEnv, save: save, logger: logger}, nil
}

// Names are the providers that can be chosen.
func Names() []string {
	return []string{mock.Name, anthropic.Name, openaicompat.Name}
}

func (s *Switch) current() extract.Extractor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.x
}

func (s *Switch) Name() string { return s.current().Name() }

// Model is the model in use, or "" for the mock.
func (s *Switch) Model() string {
	if m, ok := s.current().(interface{ Model() string }); ok {
		return m.Model()
	}
	return ""
}

// PromptVersion is the extraction prompt's version, or "" for the mock.
func (s *Switch) PromptVersion() string {
	if p, ok := s.current().(interface{ PromptVersion() string }); ok {
		return p.PromptVersion()
	}
	return ""
}

func (s *Switch) Extract(ctx context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	return s.current().Extract(ctx, in)
}

func (s *Switch) Call(ctx context.Context, c extract.ToolCall) (extract.ToolReply, error) {
	caller, ok := s.current().(extract.Caller)
	if !ok {
		return extract.ToolReply{}, extract.ErrNoModel
	}
	return caller.Call(ctx, c)
}

// Settings are the provider settings in effect, without the key.
func (s *Switch) Settings() domain.ProviderSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings()
}

func (s *Switch) settings() domain.ProviderSettings {
	return domain.ProviderSettings{
		Provider:  s.x.Name(),
		Model:     s.opts.Model,
		Endpoint:  s.opts.Endpoint,
		APIKeySet: s.opts.APIKey != "",
		FromEnv:   append([]string{}, s.fromEnv...),
		Providers: Names(),
	}
}

// Update checks u the way startup checks the config, saves it, and switches
// every later call to the new extractor. A setting from an env var can't
// change, and settings no extractor can be built from are refused; both are
// a *domain.ValidationError and change nothing.
func (s *Switch) Update(_ context.Context, u domain.ProviderUpdate) (domain.ProviderSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.opts
	next.Provider = strings.TrimSpace(u.Provider)
	next.Model = strings.TrimSpace(u.Model)
	next.Endpoint = strings.TrimSpace(u.Endpoint)
	if u.APIKey != nil {
		next.APIKey = strings.TrimSpace(*u.APIKey)
	}
	before, after := values(s.opts), values(next)
	if before["provider"] == "" {
		before["provider"] = mock.Name // the env var said "mock" or nothing
	}
	if after["provider"] == "" {
		after["provider"] = mock.Name
	}

	var problems []domain.FieldError
	for _, k := range s.fromEnv {
		if after[k] != before[k] {
			problems = append(problems, domain.FieldError{Field: k, Problem: "is set by WAYPOINT_" + strings.ToUpper(k) + " and can't be changed here"})
		}
	}
	if len(problems) > 0 {
		return domain.ProviderSettings{}, &domain.ValidationError{Entity: "provider", Problems: problems}
	}
	x, err := New(next)
	if err != nil {
		return domain.ProviderSettings{}, &domain.ValidationError{Entity: "provider", Problems: []domain.FieldError{{Field: "provider", Problem: err.Error()}}}
	}
	if s.save == nil {
		return domain.ProviderSettings{}, &domain.ValidationError{Entity: "provider", Problems: []domain.FieldError{{Field: "provider", Problem: "there is no config file to save to; set WAYPOINT_CONFIG"}}}
	}

	saved := values(next)
	for _, k := range s.fromEnv {
		delete(saved, k) // the file keeps its own value
	}
	if err := s.save(saved); err != nil {
		return domain.ProviderSettings{}, fmt.Errorf("save provider settings: %w", err)
	}
	s.x, s.opts = x, next
	// Never the key.
	s.logger.Info("provider changed", "provider", x.Name(), "model", next.Model, "endpoint", next.Endpoint)
	return s.settings(), nil
}

// values are o's settings by config key.
func values(o Options) map[string]string {
	return map[string]string{"provider": o.Provider, "model": o.Model, "endpoint": o.Endpoint, "api_key": o.APIKey}
}
