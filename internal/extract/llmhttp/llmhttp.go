// Package llmhttp holds the HTTP and JSON plumbing the model extractors share:
// posting a request, reporting a server's error without echoing the request,
// and fixing IDs a model wrote as strings. It knows no provider.
package llmhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MaxResponse caps how much of a response is read: far more than any
// extractor's output cap, so only a misbehaving server reaches it (and then
// fails to decode).
const MaxResponse = 8 << 20

// MaxErrorMessage caps how much of a server's error message goes into a run's
// error, in case the server echoes part of the request back.
const MaxErrorMessage = 200

// Post sends req as JSON to url with header and returns the response body. A
// non-2xx response is an error carrying the server's message, cut short.
func Post(ctx context.Context, client *http.Client, url string, header http.Header, req any) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range header {
		r.Header[k] = v
	}
	r.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponse))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, statusError(resp.StatusCode, raw)
	}
	return raw, nil
}

// statusError reports a non-2xx response with the server's error message,
// cut short, and nothing else from the body. Both {"error":{"message":…}}
// (OpenAI, Anthropic) and {"error":"…"} are understood.
func statusError(status int, raw []byte) error {
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	var msg string
	if json.Unmarshal(raw, &body) == nil && len(body.Error) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body.Error, &e) == nil {
			msg = e.Message
		} else {
			_ = json.Unmarshal(body.Error, &msg) // some servers send a bare string
		}
	}
	if r := []rune(msg); len(r) > MaxErrorMessage {
		msg = string(r[:MaxErrorMessage]) + "…"
	}
	if msg == "" {
		return fmt.Errorf("server returned %d %s", status, http.StatusText(status))
	}
	return fmt.Errorf("server returned %d %s: %s", status, http.StatusText(status), msg)
}

// NumericIDs turns IDs written as strings of digits ("1") into numbers, in
// target_id and every other "_id" field. Tool input that isn't constrained to
// the schema lets models quote them. Anything else, including "$new:N", is
// left for the validator.
func NumericIDs(args json.RawMessage) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(fixIDs(v))
}

func fixIDs(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			if s, ok := e.(string); ok && strings.HasSuffix(k, "_id") && isPositiveInt(s) {
				v[k] = json.Number(s)
			} else {
				v[k] = fixIDs(e)
			}
		}
	case []any:
		for i, e := range v {
			v[i] = fixIDs(e)
		}
	}
	return v
}

func isPositiveInt(s string) bool {
	if s == "" || len(s) > 18 || s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
