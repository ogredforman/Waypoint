package changeset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

// Ref points at a project, task, blocker, or idea from an op payload. In JSON
// it is either an existing row's ID (a positive integer) or a temp ID,
// "$new:N", naming the entity created by the op at position N earlier in the
// same changeset. Op positions start at 1.
//
// A malformed ref still decodes, marked invalid, so payload validation can
// report it against its field instead of failing the whole decode.
type Ref struct {
	ID      int64 // set for an existing row
	Temp    int64 // set for a temp ID: the position of the creating op
	invalid bool
}

// ID returns a ref to an existing row.
func ID(id int64) Ref { return Ref{ID: id} }

// Temp returns a ref to the entity created by the op at position pos.
func Temp(pos int64) Ref { return Ref{Temp: pos} }

// IsTemp reports whether r is a temp ID.
func (r Ref) IsTemp() bool { return r.Temp != 0 }

func (r Ref) String() string {
	if r.IsTemp() {
		return "$new:" + strconv.FormatInt(r.Temp, 10)
	}
	return strconv.FormatInt(r.ID, 10)
}

// Problem describes what's wrong with r, or returns "" if it is well formed.
// It never echoes the raw value, which may come from model output.
func (r Ref) Problem() string {
	switch {
	case r.invalid || r.Temp < 0:
		return `must be a positive ID or "$new:N"`
	case r.Temp > 0:
		return ""
	case r.ID == 0:
		return "is required"
	case r.ID < 0:
		return "must be a positive ID"
	}
	return ""
}

var tempRef = regexp.MustCompile(`^\$new:([1-9][0-9]{0,17})$`)

func (r *Ref) UnmarshalJSON(b []byte) error {
	*r = Ref{}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		m := tempRef.FindStringSubmatch(s)
		if m == nil {
			r.invalid = true
			return nil
		}
		pos, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			r.invalid = true
			return nil
		}
		r.Temp = pos
		return nil
	}
	if bytes.Equal(b, []byte("null")) {
		return nil // same as absent; a required ref then reports "is required"
	}
	var id int64
	if err := json.Unmarshal(b, &id); err != nil {
		r.invalid = true
		return nil
	}
	if id == 0 {
		r.invalid = true // 0 is never a row ID; don't let it read as "absent"
	}
	r.ID = id
	return nil
}

func (r Ref) MarshalJSON() ([]byte, error) {
	if r.invalid {
		return nil, fmt.Errorf("marshal invalid ref")
	}
	if r.IsTemp() {
		return json.Marshal(r.String())
	}
	return json.Marshal(r.ID)
}

// Nullable is an update field that can be left alone, cleared, or set: absent
// from the JSON (Set false), null (Set true, Value nil), or a value.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// Clear returns a Nullable that clears the field.
func Clear[T any]() Nullable[T] { return Nullable[T]{Set: true} }

// To returns a Nullable that sets the field to v.
func To[T any](v T) Nullable[T] { return Nullable[T]{Set: true, Value: &v} }

// IsZero reports whether the field is absent, so omitzero leaves it out.
func (n Nullable[T]) IsZero() bool { return !n.Set }

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	n.Value = nil
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.Value)
}
