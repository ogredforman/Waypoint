package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestProjects(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()

	var created domain.Project
	inTx(t, s, func(tx *Tx) (err error) {
		created, err = tx.CreateProject(ctx, domain.Project{
			ID: 42, Title: "Learn Go", Why: "Fun", DefinitionOfDone: "Ship a CLI", Status: domain.ProjectParked,
		})
		return err
	})
	want := domain.Project{
		ID: created.ID, Title: "Learn Go", Why: "Fun", DefinitionOfDone: "Ship a CLI",
		Status: domain.ProjectParked, CreatedAt: t0, UpdatedAt: t0,
	}
	if created.ID == 0 || created.ID == 42 || created != want {
		t.Errorf("CreateProject = %+v, want %+v with a fresh ID", created, want)
	}
	if got, err := s.GetProject(ctx, created.ID); err != nil || got != created {
		t.Errorf("GetProject = %+v, %v; want %+v", got, err, created)
	}

	c.t = t0.Add(time.Hour)
	edit := created
	edit.Title, edit.Status = "Learn Go properly", domain.ProjectActive
	var updated domain.Project
	inTx(t, s, func(tx *Tx) (err error) {
		updated, err = tx.UpdateProject(ctx, edit)
		return err
	})
	want.Title, want.Status, want.UpdatedAt = "Learn Go properly", domain.ProjectActive, c.t
	if updated != want {
		t.Errorf("UpdateProject = %+v, want %+v", updated, want)
	}

	second := seedProject(t, s)
	list, err := s.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0] != updated || list[1] != second {
		t.Errorf("ListProjects = %+v, want [%+v %+v]", list, updated, second)
	}
}

func TestCountActiveProjects(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	if n, err := s.CountActiveProjects(ctx); err != nil || n != 0 {
		t.Fatalf("empty DB: CountActiveProjects = %d, %v; want 0", n, err)
	}
	var parked domain.Project
	inTx(t, s, func(tx *Tx) error {
		for _, st := range domain.ProjectStatuses() {
			p, err := tx.CreateProject(ctx, domain.Project{Title: string(st), Status: st})
			if err != nil {
				return err
			}
			if st == domain.ProjectParked {
				parked = p
			}
		}
		_, err := tx.CreateProject(ctx, domain.Project{Title: "second active", Status: domain.ProjectActive})
		return err
	})
	if n, err := s.CountActiveProjects(ctx); err != nil || n != 2 {
		t.Errorf("CountActiveProjects = %d, %v; want 2", n, err)
	}

	// Inside a transaction the count sees the transaction's own writes.
	inTx(t, s, func(tx *Tx) error {
		parked.Status = domain.ProjectActive
		if _, err := tx.UpdateProject(ctx, parked); err != nil {
			return err
		}
		if n, err := tx.CountActiveProjects(ctx); err != nil || n != 3 {
			t.Errorf("in tx: CountActiveProjects = %d, %v; want 3", n, err)
		}
		return nil
	})
}

func TestTasks(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	other := seedProject(t, s)
	e := seedEntry(t, s, "todo: stub the login route")

	// Position is assigned by the store: each project's tasks append at the end.
	first := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Stub /login", Status: domain.TaskTodo, Position: 99})
	elsewhere := seedTask(t, s, domain.Task{ProjectID: other.ID, Title: "Unrelated", Status: domain.TaskTodo})
	second := seedTask(t, s, domain.Task{
		ProjectID: p.ID, ParentTaskID: &first.ID, Title: "Write the handler", Notes: "chi",
		Status: domain.TaskInProgress, Size: ptr(domain.SizeS), Energy: ptr(domain.EnergyLow),
		OriginEntryID: &e.ID,
	})
	if first.Position != 1 || second.Position != 2 || elsewhere.Position != 1 {
		t.Errorf("positions = %d, %d (other project %d), want 1, 2 (1)", first.Position, second.Position, elsewhere.Position)
	}
	want := domain.Task{
		ID: second.ID, ProjectID: p.ID, ParentTaskID: &first.ID, Title: "Write the handler", Notes: "chi",
		Status: domain.TaskInProgress, Size: ptr(domain.SizeS), Energy: ptr(domain.EnergyLow),
		Position: 2, OriginEntryID: &e.ID, CreatedAt: t0, UpdatedAt: t0,
	}
	if !reflect.DeepEqual(second, want) {
		t.Errorf("CreateTask = %+v, want %+v", second, want)
	}
	if got, err := s.GetTask(ctx, second.ID); err != nil || !reflect.DeepEqual(got, second) {
		t.Errorf("GetTask = %+v, %v; want %+v", got, err, second)
	}

	// Moving the second task to the front and clearing optional fields.
	c.t = t0.Add(time.Minute)
	edit := second
	edit.ParentTaskID, edit.Size, edit.Energy = nil, nil, nil
	edit.Status, edit.Position = domain.TaskDone, 0
	edit.ProjectID = other.ID // ignored: fixed at creation
	var updated domain.Task
	inTx(t, s, func(tx *Tx) (err error) {
		updated, err = tx.UpdateTask(ctx, edit)
		return err
	})
	want.ParentTaskID, want.Size, want.Energy = nil, nil, nil
	want.Status, want.Position, want.UpdatedAt = domain.TaskDone, 0, c.t
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("UpdateTask = %+v, want %+v", updated, want)
	}

	list, err := s.ListTasksByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("ListTasksByProject = %+v, want tasks %d then %d", list, second.ID, first.ID)
	}
}

func TestBlockers(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Deploy", Status: domain.TaskTodo})
	origin := seedEntry(t, s, "blocked: no server")
	resolving := seedEntry(t, s, "got a server")

	var b domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		b, err = tx.CreateBlocker(ctx, domain.Blocker{
			ProjectID: p.ID, TaskID: &task.ID, Description: "No server", Status: domain.BlockerOpen, OriginEntryID: &origin.ID,
		})
		return err
	})
	want := domain.Blocker{
		ID: b.ID, ProjectID: p.ID, TaskID: &task.ID, Description: "No server", Status: domain.BlockerOpen,
		OriginEntryID: &origin.ID, CreatedAt: t0, UpdatedAt: t0,
	}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("CreateBlocker = %+v, want %+v", b, want)
	}
	if got, err := s.GetBlocker(ctx, b.ID); err != nil || !reflect.DeepEqual(got, b) {
		t.Errorf("GetBlocker = %+v, %v; want %+v", got, err, b)
	}

	c.t = t0.Add(time.Hour)
	edit := b
	edit.Status, edit.ResolvedEntryID = domain.BlockerResolved, &resolving.ID
	var updated domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		updated, err = tx.UpdateBlocker(ctx, edit)
		return err
	})
	want.Status, want.ResolvedEntryID, want.UpdatedAt = domain.BlockerResolved, &resolving.ID, c.t
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("UpdateBlocker = %+v, want %+v", updated, want)
	}

	list, err := s.ListBlockersByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !reflect.DeepEqual(list[0], updated) {
		t.Errorf("ListBlockersByProject = %+v, want [%+v]", list, updated)
	}
}

func TestIdeas(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)

	var loose, scoped domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		if loose, err = tx.CreateIdea(ctx, domain.Idea{Title: "A bread app", Status: domain.IdeaParked}); err != nil {
			return err
		}
		scoped, err = tx.CreateIdea(ctx, domain.Idea{ProjectID: &p.ID, Title: "Dark mode", Body: "later", Status: domain.IdeaParked})
		return err
	})
	want := domain.Idea{ID: loose.ID, Title: "A bread app", Status: domain.IdeaParked, CreatedAt: t0, UpdatedAt: t0}
	if !reflect.DeepEqual(loose, want) {
		t.Errorf("CreateIdea = %+v, want %+v", loose, want)
	}
	if got, err := s.GetIdea(ctx, scoped.ID); err != nil || !reflect.DeepEqual(got, scoped) {
		t.Errorf("GetIdea = %+v, %v; want %+v", got, err, scoped)
	}

	// Promoting the loose idea into the project.
	c.t = t0.Add(time.Hour)
	edit := loose
	edit.ProjectID, edit.Status = &p.ID, domain.IdeaPromoted
	var promoted domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		promoted, err = tx.UpdateIdea(ctx, edit)
		return err
	})
	want.ProjectID, want.Status, want.UpdatedAt = &p.ID, domain.IdeaPromoted, c.t
	if !reflect.DeepEqual(promoted, want) {
		t.Errorf("UpdateIdea = %+v, want %+v", promoted, want)
	}

	parked, err := s.ListIdeasByStatus(ctx, domain.IdeaParked)
	if err != nil {
		t.Fatal(err)
	}
	if len(parked) != 1 || parked[0].ID != scoped.ID {
		t.Errorf("ListIdeasByStatus(parked) = %+v, want only idea %d", parked, scoped.ID)
	}
	byProject, err := s.ListIdeasByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byProject) != 2 || byProject[0].ID != loose.ID || byProject[1].ID != scoped.ID {
		t.Errorf("ListIdeasByProject = %+v, want ideas %d then %d", byProject, loose.ID, scoped.ID)
	}
}

func TestEntries(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)

	var entries []domain.Entry
	for i, projectID := range []*int64{nil, &p.ID, &p.ID} {
		c.t = t0.Add(time.Duration(i) * time.Minute)
		inTx(t, s, func(tx *Tx) error {
			e, err := tx.CreateEntry(ctx, domain.Entry{ID: 7, Body: "day " + string(rune('1'+i)), ProjectID: projectID})
			entries = append(entries, e)
			return err
		})
	}
	want := domain.Entry{ID: entries[1].ID, Body: "day 2", ProjectID: &p.ID, CreatedAt: t0.Add(time.Minute)}
	if !reflect.DeepEqual(entries[1], want) {
		t.Errorf("CreateEntry = %+v, want %+v", entries[1], want)
	}
	if got, err := s.GetEntry(ctx, entries[0].ID); err != nil || !reflect.DeepEqual(got, entries[0]) {
		t.Errorf("GetEntry = %+v, %v; want %+v", got, err, entries[0])
	}

	recent, err := s.ListRecentEntries(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != entries[2].ID || recent[1].ID != entries[1].ID {
		t.Errorf("ListRecentEntries(2) = %+v, want entries %d then %d", recent, entries[2].ID, entries[1].ID)
	}
	scoped, err := s.ListRecentEntriesByProject(ctx, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 2 || scoped[0].ID != entries[2].ID || scoped[1].ID != entries[1].ID {
		t.Errorf("ListRecentEntriesByProject = %+v, want entries %d then %d", scoped, entries[2].ID, entries[1].ID)
	}
	if _, err := s.ListRecentEntries(ctx, 0); err == nil {
		t.Error("ListRecentEntries(0) succeeded, want an error")
	}
}

func TestObservationsAndLinks(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Stub /login", Status: domain.TaskTodo})
	e := seedEntry(t, s, "Stubbed login. Feeling good.")
	run := seedRun(t, sqlDB, e.ID)

	var progress, feeling domain.Observation
	var links []domain.ObservationLink
	inTx(t, s, func(tx *Tx) (err error) {
		// Inserted out of text order; the list sorts by span.
		if feeling, err = tx.CreateObservation(ctx, domain.Observation{
			EntryID: e.ID, ExtractionRunID: run, Kind: domain.ObservationFeeling, Summary: "Good", SpanStart: 15, SpanEnd: 28,
		}); err != nil {
			return err
		}
		if progress, err = tx.CreateObservation(ctx, domain.Observation{
			EntryID: e.ID, ExtractionRunID: run, Kind: domain.ObservationProgress, Summary: "Login stubbed", SpanStart: 0, SpanEnd: 14,
		}); err != nil {
			return err
		}
		for _, l := range []domain.ObservationLink{
			{ObservationID: progress.ID, TargetType: domain.TargetTask, TargetID: task.ID, Confidence: 0.9},
			{ObservationID: progress.ID, TargetType: domain.TargetProject, TargetID: p.ID, Confidence: 1},
		} {
			created, err := tx.CreateObservationLink(ctx, l)
			if err != nil {
				return err
			}
			links = append(links, created)
		}
		return nil
	})

	wantObs := domain.Observation{
		ID: progress.ID, EntryID: e.ID, ExtractionRunID: run, Kind: domain.ObservationProgress,
		Summary: "Login stubbed", SpanStart: 0, SpanEnd: 14, CreatedAt: t0,
	}
	if progress != wantObs {
		t.Errorf("CreateObservation = %+v, want %+v", progress, wantObs)
	}
	if got, err := s.GetObservation(ctx, feeling.ID); err != nil || got != feeling {
		t.Errorf("GetObservation = %+v, %v; want %+v", got, err, feeling)
	}
	obs, err := s.ListObservationsByEntry(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 || obs[0] != progress || obs[1] != feeling {
		t.Errorf("ListObservationsByEntry = %+v, want [%+v %+v]", obs, progress, feeling)
	}

	wantLink := domain.ObservationLink{
		ID: links[0].ID, ObservationID: progress.ID, TargetType: domain.TargetTask, TargetID: task.ID, Confidence: 0.9, CreatedAt: t0,
	}
	if links[0] != wantLink {
		t.Errorf("CreateObservationLink = %+v, want %+v", links[0], wantLink)
	}
	byObs, err := s.ListLinksByObservation(ctx, progress.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(byObs, links) {
		t.Errorf("ListLinksByObservation = %+v, want %+v", byObs, links)
	}
	byTarget, err := s.ListLinksByTarget(ctx, domain.TargetTask, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byTarget) != 1 || byTarget[0] != links[0] {
		t.Errorf("ListLinksByTarget = %+v, want [%+v]", byTarget, links[0])
	}
}
