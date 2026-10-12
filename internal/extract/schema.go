package extract

import (
	_ "embed"
	"encoding/json"
)

// ToolName and ToolDescription name the tool an LLM extractor asks the model
// to call with its result; ResultSchema is the tool's input schema.
const (
	ToolName        = "propose_changes"
	ToolDescription = "Return the observations, proposed ops, and follow-up questions for the entry."
)

// resultSchema is the JSON schema of the model's output: an ExtractionResult
// whose observations carry a quote instead of a span (ResolveQuotes turns one
// into the other). It uses only keywords that Anthropic tool use and
// OpenAI-compatible function calling (including llama.cpp grammars) accept,
// and lists only the ops extraction may propose. Payloads mirror the
// changeset payload structs; schema_test.go keeps them in step.
//
//go:embed schema.json
var resultSchema []byte

// ResultSchema returns a copy of the output schema, so a caller can't change
// it for everyone.
func ResultSchema() json.RawMessage {
	return json.RawMessage(append([]byte(nil), resultSchema...))
}
