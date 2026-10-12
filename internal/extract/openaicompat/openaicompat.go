package openaicompat

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
const Name = "openai-compatible"

// DefaultTimeout bounds one request when Options.HTTPClient is nil. A local
// model may need to load first and think before it answers.
const DefaultTimeout = 5 * time.Minute

// MaxTokens caps the model's reply. A result is a few thousand tokens at
// most, plus any thinking; the cap stops a local model that loops from
// holding the GPU until the timeout.
const MaxTokens = 8192

// Options configure an Extractor. Endpoint is the API's base URL, such as
// "http://localhost:11434/v1" for Ollama; "/chat/completions" is appended.
// APIKey is sent as a bearer token when set; local servers need none.
type Options struct {
	Endpoint   string
	Model      string
	APIKey     string
	HTTPClient *http.Client // nil means a client with DefaultTimeout
}

// Extractor asks the model to call the extract.ToolName tool with the result,
// and resolves the observations' quotes into spans.
type Extractor struct {
	url    string
	model  string
	apiKey string
	client *http.Client
}

// New returns an Extractor. Endpoint and Model are required.
func New(o Options) (*Extractor, error) {
	if strings.TrimSpace(o.Endpoint) == "" {
		return nil, errors.New("openai-compatible extractor: endpoint is required")
	}
	if strings.TrimSpace(o.Model) == "" {
		return nil, errors.New("openai-compatible extractor: model is required")
	}
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	return &Extractor{
		url:    strings.TrimRight(o.Endpoint, "/") + "/chat/completions",
		model:  o.Model,
		apiKey: o.APIKey,
		client: client,
	}, nil
}

func (x *Extractor) Name() string          { return Name }
func (x *Extractor) Model() string         { return x.model }
func (x *Extractor) PromptVersion() string { return extract.PromptVersion }

// Extract sends one chat completion request and decodes the tool call.
func (x *Extractor) Extract(ctx context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	header := http.Header{}
	if x.apiKey != "" {
		header.Set("Authorization", "Bearer "+x.apiKey)
	}
	raw, err := llmhttp.Post(ctx, x.client, x.url, header, x.request(in))
	if err != nil {
		return extract.ExtractionResult{}, err
	}

	// From here on the provider has answered, so a failure keeps its reply.
	fail := func(err error) (extract.ExtractionResult, error) {
		return extract.ExtractionResult{}, &extract.ResponseError{Response: raw, Err: err}
	}
	args, err := toolArguments(raw)
	if err != nil {
		return fail(err)
	}
	if args, err = llmhttp.NumericIDs(args); err != nil {
		return fail(fmt.Errorf("decode %s arguments: %w", extract.ToolName, err))
	}
	var res extract.ExtractionResult
	if err := json.Unmarshal(args, &res); err != nil {
		return fail(fmt.Errorf("decode %s arguments: %w", extract.ToolName, err))
	}
	extract.ResolveQuotes(in.Entry.Body, res.Observations)
	res.Response = raw
	return res, nil
}

// request asks for temperature 0, the most repeatable output. Greedy decoding
// can make a thinking model such as Qwen3 loop; MaxTokens ends that. Leaving
// temperature out doesn't help: Ollama then uses 1.0, not the model's default.
type request struct {
	Model       string     `json:"model"`
	Messages    []message  `json:"messages"`
	Tools       []tool     `json:"tools"`
	ToolChoice  toolChoice `json:"tool_choice"`
	Temperature float64    `json:"temperature"`
	MaxTokens   int        `json:"max_tokens"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type function struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolChoice struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

func (x *Extractor) request(in extract.ExtractInput) request {
	r := request{
		Model: x.model,
		Messages: []message{
			{Role: "system", Content: extract.SystemPrompt(in.MaxQuestions)},
			{Role: "user", Content: extract.UserMessage(in)},
		},
		Tools: []tool{{Type: "function", Function: function{
			Name:        extract.ToolName,
			Description: extract.ToolDescription,
			Parameters:  extract.ResultSchema(),
		}}},
		ToolChoice: toolChoice{Type: "function"},
		MaxTokens:  MaxTokens,
	}
	r.ToolChoice.Function.Name = extract.ToolName
	return r
}

type response struct {
	Choices []struct {
		Message struct {
			Content   *string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// toolArguments returns the JSON object the model passed to the tool. The
// spec has arguments as a string of JSON; some servers send the object
// itself. A model that ignores tool_choice and answers with the JSON as its
// message content (some local models do) is accepted too.
func toolArguments(raw []byte) (json.RawMessage, error) {
	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("response has no choices")
	}
	msg := resp.Choices[0].Message
	for _, call := range msg.ToolCalls {
		if call.Function.Name != extract.ToolName {
			continue
		}
		args := call.Function.Arguments
		var s string
		if json.Unmarshal(args, &s) == nil {
			args = json.RawMessage(s)
		}
		return args, nil
	}
	if msg.Content != nil {
		if obj, ok := jsonObject(*msg.Content); ok {
			return obj, nil
		}
	}
	return nil, fmt.Errorf("the model did not call %s", extract.ToolName)
}

// jsonObject finds a JSON object in free text: whatever lies between the
// first "{" and the last "}" after any <think> block, if it's valid JSON.
func jsonObject(s string) (json.RawMessage, bool) {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, false
	}
	obj := json.RawMessage(s[start : end+1])
	return obj, json.Valid(obj)
}
