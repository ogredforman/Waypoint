// Package provider builds the configured extractor, so that nothing outside
// internal/extract/* names a provider.
package provider

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/anthropic"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/extract/openaicompat"
)

// Options are the extractor settings from the config. An empty Provider means
// the mock extractor.
type Options struct {
	Provider string // "mock", "anthropic", or "openai-compatible"
	Model    string
	Endpoint string
	APIKey   string
	Timeout  time.Duration // one model request; 0 means the provider's DefaultTimeout

	MockLatency time.Duration // mock only
}

// New returns the extractor Options name, or an error naming what's missing.
func New(o Options) (extract.Extractor, error) {
	var client *http.Client // nil: the provider's default
	if o.Timeout > 0 {
		client = &http.Client{Timeout: o.Timeout}
	}
	switch o.Provider {
	case "", mock.Name:
		return mock.New(o.MockLatency), nil
	case anthropic.Name:
		x, err := anthropic.New(anthropic.Options{Endpoint: o.Endpoint, Model: o.Model, APIKey: o.APIKey, HTTPClient: client})
		if err != nil {
			return nil, err
		}
		return x, nil
	case openaicompat.Name:
		x, err := openaicompat.New(openaicompat.Options{Endpoint: o.Endpoint, Model: o.Model, APIKey: o.APIKey, HTTPClient: client})
		if err != nil {
			return nil, err
		}
		return x, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (want %q, %q, or %q)", o.Provider, mock.Name, anthropic.Name, openaicompat.Name)
	}
}
