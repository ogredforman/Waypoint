package briefing

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

var t0 = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// task builds an open todo task; position follows the ID.
func task(id int64, opts ...func(*domain.Task)) domain.Task {
	t := domain.Task{ID: id, ProjectID: 1, Title: "t", Status: domain.TaskTodo, Position: id, UpdatedAt: t0}
	for _, o := range opts {
		o(&t)
	}
	return t
}

func status(s domain.TaskStatus) func(*domain.Task) { return func(t *domain.Task) { t.Status = s } }
func size(s domain.TaskSize) func(*domain.Task)     { return func(t *domain.Task) { t.Size = &s } }
func energy(e domain.Energy) func(*domain.Task)     { return func(t *domain.Task) { t.Energy = &e } }
func parent(id int64) func(*domain.Task)            { return func(t *domain.Task) { t.ParentTaskID = &id } }
func updated(d time.Duration) func(*domain.Task) {
	return func(t *domain.Task) { t.UpdatedAt = t0.Add(d) }
}

func blockerOn(taskID int64) domain.Blocker {
	return domain.Blocker{ID: taskID * 100, ProjectID: 1, TaskID: &taskID, Description: "stuck", Status: domain.BlockerOpen}
}

func dep(taskID, blockedBy int64) domain.TaskDependency {
	return domain.TaskDependency{TaskID: taskID, BlockedByTaskID: blockedBy}
}

func TestHeuristicNext(t *testing.T) {
	tests := []struct {
		name     string
		st       State
		wantNext int64 // 0: none
		reason   string
	}{
		{"no tasks", State{}, 0, reasonNoOpenTasks},
		{"only closed tasks", State{Tasks: []domain.Task{task(1, status(domain.TaskDone)), task(2, status(domain.TaskDropped))}},
			0, reasonNoOpenTasks},
		{"first todo", State{Tasks: []domain.Task{task(1), task(2)}}, 1, reasonNext},
		{"first todo, small", State{Tasks: []domain.Task{task(1, size(domain.SizeS)), task(2, size(domain.SizeS))}},
			1, reasonNextSmall},
		{"in progress beats todo", State{Tasks: []domain.Task{task(1, size(domain.SizeXS)), task(2, status(domain.TaskInProgress))}},
			2, reasonInProgress},
		{"latest in progress", State{Tasks: []domain.Task{
			task(1, status(domain.TaskInProgress), updated(time.Hour)),
			task(2, status(domain.TaskInProgress), updated(2*time.Hour)),
			task(3, status(domain.TaskInProgress)),
		}}, 2, reasonInProgress},
		{"blocked in progress is skipped", State{
			Tasks:    []domain.Task{task(1, status(domain.TaskInProgress)), task(2)},
			Blockers: []domain.Blocker{blockerOn(1)},
		}, 2, reasonNext},
		{"smaller within the window", State{Tasks: []domain.Task{task(1, size(domain.SizeL)), task(2), task(3, size(domain.SizeS))}},
			3, reasonSmall},
		{"smaller but not small", State{Tasks: []domain.Task{task(1, size(domain.SizeL)), task(2, size(domain.SizeM))}},
			2, reasonSmaller},
		{"unsized counts as medium", State{Tasks: []domain.Task{task(1), task(2, size(domain.SizeM))}}, 1, reasonNext},
		{"ties go to position", State{Tasks: []domain.Task{task(1, size(domain.SizeL)), task(2, size(domain.SizeS)), task(3, size(domain.SizeS))}},
			2, reasonSmall},
		{"outside the window doesn't count", State{Tasks: []domain.Task{
			task(1, size(domain.SizeL)), task(2, size(domain.SizeL)), task(3, size(domain.SizeM)), task(4, size(domain.SizeXS)),
		}}, 3, reasonSmaller},
		{"the window skips waiting tasks", State{
			Tasks:    []domain.Task{task(1, size(domain.SizeL)), task(2, size(domain.SizeXS)), task(3, size(domain.SizeL)), task(4, size(domain.SizeS))},
			Blockers: []domain.Blocker{blockerOn(2)},
		}, 4, reasonSmall},
		{"resolved blocker doesn't block", State{
			Tasks:    []domain.Task{task(1)},
			Blockers: []domain.Blocker{func() domain.Blocker { b := blockerOn(1); b.Status = domain.BlockerResolved; return b }()},
		}, 1, reasonNext},
		{"blocker without a task doesn't block", State{
			Tasks:    []domain.Task{task(1)},
			Blockers: []domain.Blocker{{ID: 9, ProjectID: 1, Description: "d", Status: domain.BlockerOpen}},
		}, 1, reasonNext},
		{"waiting on an open task", State{
			Tasks:        []domain.Task{task(1), task(2, status(domain.TaskInProgress))},
			Dependencies: []domain.TaskDependency{dep(2, 1)},
		}, 1, reasonNext},
		{"dependency on a done task is met", State{
			Tasks:        []domain.Task{task(1, status(domain.TaskDone)), task(2, status(domain.TaskInProgress))},
			Dependencies: []domain.TaskDependency{dep(2, 1)},
		}, 2, reasonInProgress},
		{"dependency on a dropped task is met", State{
			Tasks:        []domain.Task{task(1, status(domain.TaskDropped)), task(2)},
			Dependencies: []domain.TaskDependency{dep(2, 1)},
		}, 2, reasonNext},
		{"open subtasks come first", State{Tasks: []domain.Task{task(1, status(domain.TaskInProgress)), task(2, parent(1))}},
			2, reasonNext},
		{"parent is next once subtasks are done", State{Tasks: []domain.Task{task(1), task(2, parent(1), status(domain.TaskDone))}},
			1, reasonNext},
		{"everything waiting", State{
			Tasks:        []domain.Task{task(1), task(2)},
			Blockers:     []domain.Blocker{blockerOn(1)},
			Dependencies: []domain.TaskDependency{dep(2, 1)},
		}, 0, reasonAllWaiting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.st.Project = domain.Project{ID: 1}
			b, err := Heuristic{}.Brief(context.Background(), tt.st)
			if err != nil {
				t.Fatal(err)
			}
			var got int64
			if b.NextTaskID != nil {
				got = *b.NextTaskID
			}
			if got != tt.wantNext || b.NextReason != tt.reason {
				t.Errorf("next = %d (%q), want %d (%q)", got, b.NextReason, tt.wantNext, tt.reason)
			}
			if b.ProjectID != 1 || b.Source != domain.BriefingHeuristic {
				t.Errorf("briefing %+v, want project 1 and heuristic source", b)
			}
		})
	}
}

func TestAlternatives(t *testing.T) {
	ids := func(ts []domain.Task) []int64 {
		out := []int64{}
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return out
	}
	tests := []struct {
		name string
		st   State
		next *int64
		want []int64
	}{
		{"none", State{}, nil, []int64{}},
		{"small or low energy, not next", State{Tasks: []domain.Task{
			task(1, size(domain.SizeXS)),
			task(2, size(domain.SizeL)),
			task(3, size(domain.SizeL), energy(domain.EnergyLow)),
			task(4),
		}}, ptr(int64(1)), []int64{3}},
		{"at most two", State{Tasks: []domain.Task{
			task(1, size(domain.SizeS)), task(2, size(domain.SizeS)), task(3, size(domain.SizeS)),
		}}, nil, []int64{1, 2}},
		{"only startable", State{
			Tasks: []domain.Task{
				task(1, size(domain.SizeS)),
				task(2, size(domain.SizeS), status(domain.TaskDone)),
				task(3, energy(domain.EnergyLow)),
				task(4, size(domain.SizeXS)),
			},
			Blockers: []domain.Blocker{blockerOn(1)},
		}, nil, []int64{3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(alternatives(tt.st, tt.next)); !slices.Equal(got, tt.want) {
				t.Errorf("alternatives = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecap(t *testing.T) {
	obs := func(texts ...string) []domain.RecapPoint {
		out := []domain.RecapPoint{}
		for _, s := range texts {
			out = append(out, domain.RecapPoint{Kind: domain.ObservationProgress, Text: s})
		}
		return out
	}
	entry := func(id int64, texts ...string) RecentEntry { return RecentEntry{ID: id, Observations: obs(texts...)} }
	want := func(id int64, texts ...string) domain.RecapEntry {
		return domain.RecapEntry{EntryID: id, Points: obs(texts...)}
	}
	tests := []struct {
		name    string
		entries []RecentEntry // newest first
		want    []domain.RecapEntry
	}{
		{"none", nil, []domain.RecapEntry{}},
		{"no summaries", []RecentEntry{entry(1), entry(2, " ")}, []domain.RecapEntry{}},
		{"one, trimmed", []RecentEntry{entry(1, " Wired the SSE hub. ")}, []domain.RecapEntry{want(1, "Wired the SSE hub.")}},
		{"three points per entry at most", []RecentEntry{entry(1, "A", "", "B", "C", "D")}, []domain.RecapEntry{want(1, "A", "B", "C")}},
		{"newest first", []RecentEntry{entry(3, "C"), entry(2, "B"), entry(1, "A")},
			[]domain.RecapEntry{want(3, "C"), want(2, "B"), want(1, "A")}},
		{"empty entries are skipped", []RecentEntry{entry(3, "C"), entry(2), entry(1, "A")},
			[]domain.RecapEntry{want(3, "C"), want(1, "A")}},
		{"at most three", []RecentEntry{entry(4, "D"), entry(3, "C"), entry(2, "B"), entry(1, "A")},
			[]domain.RecapEntry{want(4, "D"), want(3, "C"), want(2, "B")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recap(tt.entries); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("recap = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRecapKeepsKinds(t *testing.T) {
	got := recap([]RecentEntry{{ID: 1, Observations: []domain.RecapPoint{
		{Kind: domain.ObservationFeeling, Text: "Tired"},
		{Kind: domain.ObservationPlan, Text: "Write tests"},
	}}})
	if len(got) != 1 || got[0].Points[0].Kind != domain.ObservationFeeling || got[0].Points[1].Kind != domain.ObservationPlan {
		t.Errorf("recap = %+v, want the observations' kinds", got)
	}
}
