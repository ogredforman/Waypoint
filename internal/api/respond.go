package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/store"
)

// maxBodyBytes caps request bodies. Diary entries are the largest; a megabyte
// is far more than anyone writes in one go.
const maxBodyBytes = 1 << 20

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an API error with the given status, code, and message.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apitypes.ErrorBody{Error: apitypes.ErrorDetail{Code: code, Message: message}})
}

// badRequest is a request the server couldn't make sense of: malformed JSON
// or a bad query parameter. It becomes a 400.
type badRequest struct{ msg string }

func (e *badRequest) Error() string { return e.msg }

func badRequestf(format string, args ...any) error {
	return &badRequest{msg: fmt.Sprintf(format, args...)}
}

// invalid returns a validation error with one problem, for request checks
// the handlers make themselves.
func invalid(entity, field, problem string) error {
	return &domain.ValidationError{Entity: entity, Problems: []domain.FieldError{{Field: field, Problem: problem}}}
}

// fail writes the response for err. Errors the client can act on map to 4xx
// with their message; anything else is logged and becomes a bare 500.
// Messages never carry diary text: validation problems don't echo values.
func (h *handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var (
		bad *badRequest
		ve  *domain.ValidationError
		ce  *domain.ActiveCapError
	)
	switch {
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, "bad_request", bad.msg)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, domain.ErrLastProfile):
		writeError(w, http.StatusConflict, "last_profile", "the last profile can't be deleted")
	case errors.As(err, &ce):
		writeError(w, http.StatusConflict, "active_cap", ce.Error())
	case errors.As(err, &ve):
		problems := make([]apitypes.FieldProblem, len(ve.Problems))
		for i, p := range ve.Problems {
			problems[i] = apitypes.FieldProblem{Field: p.Field, Problem: p.Problem}
		}
		writeJSON(w, http.StatusUnprocessableEntity, apitypes.ErrorBody{Error: apitypes.ErrorDetail{
			Code: "invalid", Message: ve.Error(), Problems: problems,
		}})
	case errors.Is(err, changeset.ErrNotPending):
		writeError(w, http.StatusConflict, "not_pending", "the changeset was already resolved")
	case errors.Is(err, pipeline.ErrRunInProgress):
		writeError(w, http.StatusConflict, "run_in_progress", "an entry is already being processed; try again when it's done")
	default:
		h.logger.LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("request_id", middleware.GetReqID(r.Context())),
			slog.Any("err", err),
		)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

// readJSON decodes the request body into v, rejecting unknown fields (as a
// validation error) and trailing data.
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return decodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return badRequestf("request body has data after the JSON value")
	}
	return nil
}

// readObject decodes the request body as a JSON object and returns its
// fields, for PATCH-style bodies where absent and null differ.
func readObject(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := readJSON(w, r, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, badRequestf("request body must be a JSON object")
	}
	return body, nil
}

func decodeError(err error) error {
	var (
		syntax  *json.SyntaxError
		typ     *json.UnmarshalTypeError
		tooBig  *http.MaxBytesError
		unknown = "json: unknown field "
	)
	switch {
	case errors.Is(err, io.EOF):
		return badRequestf("request body is empty")
	case errors.As(err, &tooBig):
		return badRequestf("request body is larger than %d bytes", tooBig.Limit)
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return badRequestf("request body is not valid JSON")
	case errors.As(err, &typ):
		if typ.Field == "" {
			return badRequestf("request body has the wrong type")
		}
		return badRequestf("field %s has the wrong type", typ.Field)
	case strings.HasPrefix(err.Error(), unknown):
		// A 422 like the op-backed routes give for unknown fields.
		return invalid("request", strings.Trim(strings.TrimPrefix(err.Error(), unknown), `"`), "is not a known field")
	}
	return badRequestf("request body can't be decoded")
}

// take removes field name from body and returns its value.
func take(body map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	v, ok := body[name]
	delete(body, name)
	return v, ok
}

// isJSONString reports whether raw is the JSON string s.
func isJSONString(raw json.RawMessage, s string) bool {
	var got string
	return json.Unmarshal(raw, &got) == nil && got == s
}

// opFrom builds a change op of type t from request fields plus the fields
// the route fills in, such as the ID from the path. The changeset package
// decodes and validates it, so request fields follow the op's payload shape
// (SPEC §6.2). A request that sets a field the route fills in is rejected
// like any unknown field.
func opFrom(t domain.OpType, body map[string]json.RawMessage, set map[string]any) (changeset.Payload, error) {
	fields := make(map[string]any, len(body)+len(set))
	for k, v := range body {
		fields[k] = v
	}
	for k, v := range set {
		if _, ok := body[k]; ok {
			return nil, invalid(string(t)+" op", k, "is not a known field")
		}
		fields[k] = v
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode %s op: %w", t, err)
	}
	p, err := changeset.DecodePayload(t, raw)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// pathID returns the positive integer path parameter name. Anything else
// can't name a row, so it is not found.
func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("path %s: %w", name, store.ErrNotFound)
	}
	return id, nil
}

// queryID returns the positive integer query parameter name, or nil if it
// is absent.
func queryID(r *http.Request, name string) (*int64, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return nil, badRequestf("query parameter %s must be a positive integer", name)
	}
	return &id, nil
}
