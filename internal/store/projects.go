package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func projectFromRow(r dbgen.Project) domain.Project {
	return domain.Project{
		ID:               r.ID,
		Title:            r.Title,
		Why:              r.Why,
		DefinitionOfDone: r.DefinitionOfDone,
		Status:           domain.ProjectStatus(r.Status),
		CreatedAt:        fromMS(r.CreatedAt),
		UpdatedAt:        fromMS(r.UpdatedAt),
	}
}

func (r reader) GetProject(ctx context.Context, id int64) (domain.Project, error) {
	row, err := r.q.GetProject(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("get project %d: %w", id, notFound(err))
	}
	return projectFromRow(row), nil
}

// ListProjects returns every project, oldest first.
func (r reader) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.q.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]domain.Project, len(rows))
	for i, row := range rows {
		out[i] = projectFromRow(row)
	}
	return out, nil
}

// CountActiveProjects returns how many projects are active, for checking the
// active-project cap. Call it on the Tx that makes the change so the count and
// the write agree.
func (r reader) CountActiveProjects(ctx context.Context) (int, error) {
	n, err := r.q.CountProjectsByStatus(ctx, string(domain.ProjectActive))
	if err != nil {
		return 0, fmt.Errorf("count active projects: %w", err)
	}
	return int(n), nil
}

// CreateProject inserts p, ignoring its ID and timestamps.
func (tx *Tx) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if err := p.Validate(); err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateProject(ctx, dbgen.CreateProjectParams{
		Title:            p.Title,
		Why:              p.Why,
		DefinitionOfDone: p.DefinitionOfDone,
		Status:           string(p.Status),
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}
	return projectFromRow(row), nil
}

// UpdateProject overwrites the editable fields of project p.ID.
func (tx *Tx) UpdateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if err := p.Validate(); err != nil {
		return domain.Project{}, fmt.Errorf("update project %d: %w", p.ID, err)
	}
	row, err := tx.q.UpdateProject(ctx, dbgen.UpdateProjectParams{
		Title:            p.Title,
		Why:              p.Why,
		DefinitionOfDone: p.DefinitionOfDone,
		Status:           string(p.Status),
		UpdatedAt:        tx.nowMS(),
		ID:               p.ID,
	})
	if err != nil {
		return domain.Project{}, fmt.Errorf("update project %d: %w", p.ID, notFound(err))
	}
	return projectFromRow(row), nil
}

// ProjectContents is what deleting a project takes with it, or, for Entries,
// what it keeps unscoped or deletes as asked.
type ProjectContents struct {
	Tasks      int
	Blockers   int
	Ideas      int
	Milestones int
	Entries    int
}

// CountProjectContents counts what belongs to project id, for the delete
// confirmation. It returns ErrNotFound if there is no such project.
func (r reader) CountProjectContents(ctx context.Context, id int64) (ProjectContents, error) {
	if _, err := r.GetProject(ctx, id); err != nil {
		return ProjectContents{}, err
	}
	var n ProjectContents
	ref := &id
	counts := []struct {
		what string
		dst  *int
		run  func() (int64, error)
	}{
		{"tasks", &n.Tasks, func() (int64, error) { return r.q.CountProjectTasks(ctx, id) }},
		{"blockers", &n.Blockers, func() (int64, error) { return r.q.CountProjectBlockers(ctx, id) }},
		{"ideas", &n.Ideas, func() (int64, error) { return r.q.CountProjectIdeas(ctx, ref) }},
		{"milestones", &n.Milestones, func() (int64, error) { return r.q.CountProjectMilestones(ctx, id) }},
	}
	for _, c := range counts {
		v, err := c.run()
		if err != nil {
			return ProjectContents{}, fmt.Errorf("count project %d %s: %w", id, c.what, err)
		}
		*c.dst = int(v)
	}
	entries, err := r.ListProjectEntryIDs(ctx, id)
	if err != nil {
		return ProjectContents{}, err
	}
	n.Entries = len(entries)
	return n, nil
}

// ListProjectEntryIDs returns the IDs of the entries scoped to project id,
// oldest first.
func (r reader) ListProjectEntryIDs(ctx context.Context, id int64) ([]int64, error) {
	ids, err := r.q.ListProjectEntryIDs(ctx, &id)
	if err != nil {
		return nil, fmt.Errorf("list project %d entries: %w", id, err)
	}
	return ids, nil
}

// CountRunningProjectRuns returns how many extraction runs of entries scoped
// to project id are running.
func (r reader) CountRunningProjectRuns(ctx context.Context, id int64) (int, error) {
	n, err := r.q.CountRunningProjectRuns(ctx, &id)
	if err != nil {
		return 0, fmt.Errorf("count running runs of project %d: %w", id, err)
	}
	return int(n), nil
}

// DeleteProject removes project id with its tasks, blockers, ideas,
// milestones, and briefing, and the observation links to any of them (ADR
// 0008), and returns the project as it was with what went with it. Entries
// still scoped to it stay, unscoped: that is reference cleanup, not an edit
// of the entry. Callers wanting the entries gone delete them first
// (DeleteEntry). The observations stay; only their links go. It returns
// ErrNotFound if there is no such project.
func (tx *Tx) DeleteProject(ctx context.Context, id int64) (domain.Project, ProjectContents, error) {
	n, err := tx.CountProjectContents(ctx, id)
	if err != nil {
		return domain.Project{}, ProjectContents{}, fmt.Errorf("delete project %d: %w", id, err)
	}
	ref := &id
	steps := []struct {
		what string
		run  func() error
	}{
		{"observation links", func() error { return tx.q.DeleteProjectLinks(ctx, id) }},
		{"briefing", func() error { return tx.q.DeleteProjectBriefing(ctx, id) }},
		{"dependencies", func() error { return tx.q.DeleteProjectDependencies(ctx, id) }},
		{"blockers", func() error { return tx.q.DeleteProjectBlockers(ctx, id) }},
		{"tasks", func() error { return tx.q.DeleteProjectTasks(ctx, id) }},
		{"ideas", func() error { return tx.q.DeleteProjectIdeas(ctx, ref) }},
		{"milestones", func() error { return tx.q.DeleteProjectMilestones(ctx, id) }},
		{"entry scopes", func() error { return tx.q.UnscopeProjectEntries(ctx, ref) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return domain.Project{}, ProjectContents{}, fmt.Errorf("delete project %d: %s: %w", id, s.what, err)
		}
	}
	row, err := tx.q.DeleteProject(ctx, id)
	if err != nil {
		return domain.Project{}, ProjectContents{}, fmt.Errorf("delete project %d: %w", id, notFound(err))
	}
	return projectFromRow(row), n, nil
}
