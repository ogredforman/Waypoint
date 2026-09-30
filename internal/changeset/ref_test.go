package changeset

import (
	"encoding/json"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestRefJSON(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    Ref
		problem string
	}{
		{`7`, ID(7), ""},
		{`"$new:1"`, Temp(1), ""},
		{`"$new:42"`, Temp(42), ""},
		{`-3`, ID(-3), "must be a positive ID"},
		{`null`, Ref{}, "is required"},
		{`0`, Ref{}, `must be a positive ID or "$new:N"`},
		{`1.5`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"7"`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"$new:0"`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"$new:01"`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"$new:-1"`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"$new:1 "`, Ref{}, `must be a positive ID or "$new:N"`},
		{`"$new:99999999999999999999"`, Ref{}, `must be a positive ID or "$new:N"`},
		{`true`, Ref{}, `must be a positive ID or "$new:N"`},
		{`{}`, Ref{}, `must be a positive ID or "$new:N"`},
	} {
		t.Run(tt.in, func(t *testing.T) {
			var r Ref
			if err := json.Unmarshal([]byte(tt.in), &r); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := r.problem(); got != tt.problem {
				t.Errorf("problem %q, want %q", got, tt.problem)
			}
			if tt.problem != "" {
				return
			}
			if r != tt.want {
				t.Errorf("got %+v, want %+v", r, tt.want)
			}
			out, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.in {
				t.Errorf("round trip %s, want %s", out, tt.in)
			}
		})
	}
}

func TestNullableJSON(t *testing.T) {
	type payload struct {
		Size   Nullable[domain.TaskSize] `json:"size,omitzero"`
		Parent Nullable[Ref]             `json:"parent,omitzero"`
	}
	for _, tt := range []struct {
		in   string
		want payload
	}{
		{`{}`, payload{}},
		{`{"size":null,"parent":null}`, payload{Size: Clear[domain.TaskSize](), Parent: Clear[Ref]()}},
		{`{"size":"s","parent":"$new:2"}`, payload{Size: To(domain.SizeS), Parent: To(Temp(2))}},
	} {
		t.Run(tt.in, func(t *testing.T) {
			var got payload
			if err := json.Unmarshal([]byte(tt.in), &got); err != nil {
				t.Fatal(err)
			}
			if got.Size.Set != tt.want.Size.Set || got.Parent.Set != tt.want.Parent.Set ||
				!samePtr(got.Size.Value, tt.want.Size.Value) || !samePtr(got.Parent.Value, tt.want.Parent.Value) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			out, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.in {
				t.Errorf("round trip %s, want %s", out, tt.in)
			}
		})
	}
}

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
