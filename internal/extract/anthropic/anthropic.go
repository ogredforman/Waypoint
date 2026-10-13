package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/llmhttp"
)

// Name is the provider name runs record.
const Name = "anthropic"

// DefaultEndpoint and DefaultModel apply when Options leave them empty.
const (
	DefaultEndpoint = "https://api.anthropic.com/v1"
	DefaultModel    = "claude-haiku-4-5"
)

// APIVersion is sent as the anthropic-version header.
const APIVersion = "2023-06-01"

// DefaultTimeout bounds one request when Options.HTTPClient is nil.
const DefaultTimeout = 2 * time.Minute

// MaxTokens caps the model's reply. A result is a few thousand tokens; newer
// models also think before answering, which counts against the cap.
const MaxTokens = 16000

// Options configure an Extractor. Endpoint is the API's base URL;
// "/messages" is appended. APIKey is required.
type Options struct {
	Endpoint   string // empty means DefaultEndpoint
	Model      string // empty means DefaultModel
	APIKey     string
	HTTPClient *http.Client // nil means a client with DefaultTimeout
}

// Extractor asks the model to call the extract.ToolName tool with the result,
// and resolves the observations' quotes into spans. It is also an
// extract.Caller, for other uses of the model.
type Extractor struct {
	url    string
	model  string
	apiKey string
	client *http.Client
}

// New returns an Extractor. APIKey is required.
func New(o Options) (*Extractor, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, errors.New("anthropic extractor: api_key is required")
	}
	endpoint := strings.TrimSpace(o.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	model := strings.TrimSpace(o.Model)
	if model == "" {
		model = DefaultModel
	}
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	return &Extractor{
		url:    strings.TrimRight(endpoint, "/") + "/messages",
		model:  model,
		apiKey: o.APIKey,
		client: client,
	}, nil
}

func (x *Extractor) Name() string          { return Name }
func (x *Extractor) Model() string         { return x.model }
func (x *Extractor) PromptVersion() string { return extract.PromptVersion }

// Extract makes the extraction tool call and decodes it.
func (x *Extractor) Extract(ctx context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	return extract.ExtractWith(ctx, x, in)
}

// Call sends one Messages request offering c.Tool and returns the input the
// model passed to it. It makes Extractor an extract.Caller.
func (x *Extractor) Call(ctx context.Context, c extract.ToolCall) (extract.ToolReply, error) {
	header := http.Header{}
	header.Set("x-api-key", x.apiKey)
	header.Set("anthropic-version", APIVersion)
	raw, err := llmhttp.Post(ctx, x.client, x.url, header, x.request(c))
	if err != nil {
		return extract.ToolReply{}, err
	}

	// From here on the provider has answered, so a failure keeps its reply.
	args, err := toolInput(raw, c.Tool)
	if err == nil {
		if args, err = llmhttp.NumericIDs(args); err != nil {
			err = fmt.Errorf("decode %s input: %w", c.Tool, err)
		}
	}
	if err != nil {
		return extract.ToolReply{}, &extract.ResponseError{Response: raw, Err: err}
	}
	return extract.ToolReply{Input: args, Response: raw}, nil
}

type request struct {
	Model       string      `json:"model"`
	MaxTokens   int         `json:"max_tokens"`
	System      []textBlock `json:"system"`
	Messages    []message   `json:"messages"`
	Tools       []tool      `json:"tools"`
	ToolChoice  toolChoice  `json:"tool_choice"`
	Temperature *float64    `json:"temperature,omitempty"`
}

// textBlock's cache_control on the system prompt caches the tools and the
// system prompt together (tools come first), so repeat entries pay for that
// prefix at the cache rate. A prefix below the model's minimum isn't cached.
type textBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

func (x *Extractor) request(c extract.ToolCall) request {
	r := request{
		Model:     x.model,
		MaxTokens: MaxTokens,
		System: []textBlock{{
			Type:         "text",
			Text:         c.System,
			CacheControl: &cacheControl{Type: "ephemeral"},
		}},
		Messages: []message{{Role: "user", Content: c.User}},
		Tools: []tool{{
			Name:        c.Tool,
			Description: c.Description,
			InputSchema: c.Schema,
		}},
		ToolChoice: toolChoice{Type: "auto"},
	}
	if pinsOutput(x.model) {
		zero := 0.0
		r.ToolChoice = toolChoice{Type: "tool", Name: c.Tool}
		r.Temperature = &zero
	}
	return r
}

// pinsOutput reports whether model takes a forced tool_choice and temperature
// 0, the most repeatable output. Newer models reject one or both with a 400;
// they get tool_choice auto, their default sampling, and the system prompt's
// instruction to call the tool.
func pinsOutput(model string) bool {
	for _, p := range []string{"claude-haiku-", "claude-3", "claude-sonnet-4"} {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}

type response struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

// toolInput returns the input the model passed to the named tool.
func toolInput(raw []byte, name string) (json.RawMessage, error) {
	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	switch resp.StopReason {
	case "max_tokens":
		return nil, fmt.Errorf("the reply was cut off at %d tokens", MaxTokens)
	case "refusal":
		return nil, errors.New("the model declined the request")
	}
	for _, b := range resp.Content {
		if b.Type == "tool_use" && b.Name == name {
			return b.Input, nil
		}
	}
	return nil, fmt.Errorf("the model did not call %s", name)
}
