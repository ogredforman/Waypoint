package extract

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

type schemaNode = map[string]any

// loadSchema parses ResultSchema and returns it with a resolver for its
// local "$ref"s.
func loadSchema(t *testing.T) (schemaNode, func(schemaNode) schemaNode) {
	t.Helper()
	var root schemaNode
	if err := json.Unmarshal(ResultSchema(), &root); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	var resolve func(schemaNode) schemaNode
	resolve = func(n schemaNode) schemaNode {
		t.Helper()
		ref, ok := n["$ref"].(string)
		if !ok {
			return n
		}
		name, ok := strings.CutPrefix(ref, "#/$defs/")
		def, found := root["$defs"].(schemaNode)[name].(schemaNode)
		if !ok || !found {
			t.Fatalf("unresolved $ref %q", ref)
		}
		return resolve(def)
	}
	return root, resolve
}

func props(t *testing.T, n schemaNode) schemaNode {
	t.Helper()
	p, ok := n["properties"].(schemaNode)
	if !ok {
		t.Fatalf("node has no properties: %v", n)
	}
	return p
}

func strs(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	slices.Sort(out)
	return out
}

func sorted[T ~string](vs []T) []string {
	var out []string
	for _, v := range vs {
		out = append(out, string(v))
	}
	slices.Sort(out)
	return out
}

// enumOf returns the enum values of a node, looking through a nullable anyOf.
func enumOf(t *testing.T, resolve func(schemaNode) schemaNode, n schemaNode) []string {
	t.Helper()
	n = resolve(n)
	if alts, ok := n["anyOf"].([]any); ok {
		return enumOf(t, resolve, alts[0].(schemaNode))
	}
	if n["enum"] == nil {
		t.Fatalf("node has no enum: %v", n)
	}
	return strs(n["enum"])
}

// extractableOps are the op types extraction may propose: not deleting a
// project (ADR 0008).
func extractableOps() []domain.OpType {
	return slices.DeleteFunc(domain.OpTypes(), func(op domain.OpType) bool { return op == domain.OpDeleteProject })
}

// payloadTypes maps each extractable op type to its payload struct.
var payloadTypes = map[domain.OpType]reflect.Type{
	domain.OpCreateTask:      reflect.TypeFor[changeset.CreateTask](),
	domain.OpUpdateTask:      reflect.TypeFor[changeset.UpdateTask](),
	domain.OpSetTaskStatus:   reflect.TypeFor[changeset.SetTaskStatus](),
	domain.OpSplitTask:       reflect.TypeFor[changeset.SplitTask](),
	domain.OpAddDependency:   reflect.TypeFor[changeset.AddDependency](),
	domain.OpDeleteTask:      reflect.TypeFor[changeset.DeleteTask](),
	domain.OpCreateBlocker:   reflect.TypeFor[changeset.CreateBlocker](),
	domain.OpUpdateBlocker:   reflect.TypeFor[changeset.UpdateBlocker](),
	domain.OpResolveBlocker:  reflect.TypeFor[changeset.ResolveBlocker](),
	domain.OpDeleteBlocker:   reflect.TypeFor[changeset.DeleteBlocker](),
	domain.OpParkIdea:        reflect.TypeFor[changeset.ParkIdea](),
	domain.OpUpdateIdea:      reflect.TypeFor[changeset.UpdateIdea](),
	domain.OpDeleteIdea:      reflect.TypeFor[changeset.DeleteIdea](),
	domain.OpCreateProject:   reflect.TypeFor[changeset.CreateProject](),
	domain.OpUpdateProject:   reflect.TypeFor[changeset.UpdateProject](),
	domain.OpCreateMilestone: reflect.TypeFor[changeset.CreateMilestone](),
	domain.OpUpdateMilestone: reflect.TypeFor[changeset.UpdateMilestone](),
	domain.OpDeleteMilestone: reflect.TypeFor[changeset.DeleteMilestone](),
}

// jsonFields returns a struct's JSON field names and which of them are
// required (no omitempty or omitzero).
func jsonFields(typ reflect.Type) (all, required []string) {
	for f := range typ.Fields() {
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		all = append(all, name)
		if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
			required = append(required, name)
		}
	}
	slices.Sort(all)
	slices.Sort(required)
	return all, required
}

// checkObject checks that a schema object's properties are fields of typ and
// that it requires exactly typ's required fields.
func checkObject(t *testing.T, where string, n schemaNode, typ reflect.Type) {
	t.Helper()
	all, required := jsonFields(typ)
	for name := range props(t, n) {
		if !slices.Contains(all, name) {
			t.Errorf("%s: property %q is not a field of %s", where, name, typ)
		}
	}
	if got := strs(n["required"]); !slices.Equal(got, required) {
		t.Errorf("%s: required = %v, want %v", where, got, required)
	}
	if n["additionalProperties"] != false {
		t.Errorf("%s: additionalProperties should be false", where)
	}
}

func TestSchemaOpsMatchPayloads(t *testing.T) {
	root, resolve := loadSchema(t)
	variants := resolve(props(t, root)["ops"].(schemaNode)["items"].(schemaNode))["anyOf"].([]any)

	var got []domain.OpType
	for _, v := range variants {
		v := v.(schemaNode)
		p := props(t, v)
		op := domain.OpType(p["op_type"].(schemaNode)["const"].(string))
		got = append(got, op)

		typ, ok := payloadTypes[op]
		if !ok {
			t.Errorf("%s: no payload type", op)
			continue
		}
		payload := resolve(p["payload"].(schemaNode))
		checkObject(t, string(op), payload, typ)
		if op == domain.OpSplitTask {
			sub := props(t, payload)["subtasks"].(schemaNode)["items"].(schemaNode)
			checkObject(t, "split_task subtasks", sub, reflect.TypeFor[changeset.Subtask]())
		}
		if r := strs(v["required"]); !slices.Equal(r, []string{"confidence", "op_type", "payload", "rationale"}) {
			t.Errorf("%s: required = %v", op, r)
		}
	}
	if !slices.Equal(sorted(got), sorted(extractableOps())) {
		t.Errorf("op types = %v, want %v", sorted(got), sorted(extractableOps()))
	}
}

func TestSchemaEnumsMatchDomain(t *testing.T) {
	root, resolve := loadSchema(t)
	defs := root["$defs"].(schemaNode)
	obs := props(t, defs["observation"].(schemaNode))
	link := props(t, obs["links"].(schemaNode)["items"].(schemaNode))

	payloadProp := func(op domain.OpType, field string) schemaNode {
		for _, v := range defs["op"].(schemaNode)["anyOf"].([]any) {
			p := props(t, v.(schemaNode))
			if p["op_type"].(schemaNode)["const"] == string(op) {
				return props(t, resolve(p["payload"].(schemaNode)))[field].(schemaNode)
			}
		}
		t.Fatalf("no %s variant", op)
		return nil
	}

	tests := []struct {
		name string
		node schemaNode
		want []string
	}{
		{"observation kind", obs["kind"].(schemaNode), sorted(domain.ObservationKinds())},
		{"link target_type", link["target_type"].(schemaNode), sorted(domain.TargetTypes())},
		{"size", defs["size"].(schemaNode), sorted(domain.TaskSizes())},
		{"nullable size", defs["nullable_size"].(schemaNode), sorted(domain.TaskSizes())},
		{"energy", defs["energy"].(schemaNode), sorted(domain.Energies())},
		{"nullable energy", defs["nullable_energy"].(schemaNode), sorted(domain.Energies())},
		{"task status", payloadProp(domain.OpSetTaskStatus, "status"), sorted(domain.TaskStatuses())},
		{"idea status", payloadProp(domain.OpUpdateIdea, "status"), sorted(domain.IdeaStatuses())},
		{"project status", payloadProp(domain.OpUpdateProject, "status"), sorted(domain.ProjectStatuses())},
		{"milestone status", payloadProp(domain.OpUpdateMilestone, "status"), sorted(domain.MilestoneStatuses())},
	}
	for _, tt := range tests {
		if got := enumOf(t, resolve, tt.node); !slices.Equal(got, tt.want) {
			t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
		}
	}

	if got := props(t, root)["questions"].(schemaNode)["maxItems"]; got != float64(MaxQuestions) {
		t.Errorf("questions maxItems = %v, want %d", got, MaxQuestions)
	}
	checkObject(t, "link", obs["links"].(schemaNode)["items"].(schemaNode), reflect.TypeFor[LinkDraft]())
}

func TestResultSchemaIsACopy(t *testing.T) {
	s := ResultSchema()
	s[0] = 'x'
	if ResultSchema()[0] != '{' {
		t.Error("changing a returned schema changed the next one")
	}
}

// TestSchemaShapedOutput decodes output shaped like the schema, as a model's
// tool call would be, and checks it resolves and validates.
func TestSchemaShapedOutput(t *testing.T) {
	const out = `{
	  "observations": [
	    {"kind": "progress", "summary": "Finished the shipping work.", "quote": "done: ship it",
	     "links": [{"target_type": "task", "target_id": 10, "confidence": 0.9}]},
	    {"kind": "plan", "summary": "Plans a coffee-themed idea.", "quote": "café",
	     "links": [{"target_type": "idea", "target_id": "$new:2", "confidence": 0.6}]}
	  ],
	  "ops": [
	    {"op_type": "set_task_status", "payload": {"task_id": 10, "status": "done"}, "rationale": "It shipped.", "confidence": 0.9},
	    {"op_type": "park_idea", "payload": {"project_id": 1, "title": "Coffee mode"}, "rationale": "A new idea.", "confidence": 0.5},
	    {"op_type": "update_task", "payload": {"task_id": 10, "size": null}, "rationale": "Size no longer applies.", "confidence": 0.4}
	  ],
	  "questions": [{"text": "What's next after shipping?", "reason": "No next step is clear."}]
	}`
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	var res ExtractionResult
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	in := input()
	ResolveQuotes(in.Entry.Body, res.Observations)
	if err := Validate(in, res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if o := res.Observations[0]; body[o.SpanStart:o.SpanEnd] != "done: ship it" {
		t.Errorf("span = %q", body[o.SpanStart:o.SpanEnd])
	}
}
