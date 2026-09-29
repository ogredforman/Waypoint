package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Projects have no delete: entries reference them and entries are immutable.
// Parking or finishing a project takes it out of view instead.

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
