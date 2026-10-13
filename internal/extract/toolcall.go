package extract

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolCall is one request that asks the model to call a single tool: a
// system prompt, a user message, and the tool's name, description, and input
// schema.
type ToolCall struct {
	System      string
	User        string
	Tool        string
	Description string
	Schema      json.RawMessage
}

// ToolReply is what the model passed to the tool, with quoted "_id" values
// turned into numbers, and the provider's whole reply.
type ToolReply struct {
	Input    json.RawMessage
	Response json.RawMessage
}

// Caller makes a ToolCall against the configured model, so code outside
// internal/extract/* can use the model without knowing the provider. The
// model extractors implement it; the mock doesn't. Once the provider has
// answered, an error is a *ResponseError carrying the reply.
type Caller interface {
	Call(ctx context.Context, c ToolCall) (ToolReply, error)
}

// ExtractWith runs an extraction through c: one ToolCall with the extraction
// prompt and ResultSchema, then the tool input decoded and its quotes
// resolved into spans. The model extractors' Extract is this.
func ExtractWith(ctx context.Context, c Caller, in ExtractInput) (ExtractionResult, error) {
	reply, err := c.Call(ctx, ToolCall{
		System:      SystemPrompt(in.MaxQuestions),
		User:        UserMessage(in),
		Tool:        ToolName,
		Description: ToolDescription,
		Schema:      ResultSchema(),
	})
	if err != nil {
		return ExtractionResult{}, err
	}
	var res ExtractionResult
	if err := json.Unmarshal(reply.Input, &res); err != nil {
		return ExtractionResult{}, &ResponseError{Response: reply.Response, Err: fmt.Errorf("decode %s input: %w", ToolName, err)}
	}
	ResolveQuotes(in.Entry.Body, res.Observations)
	res.Response = reply.Response
	return res, nil
}
