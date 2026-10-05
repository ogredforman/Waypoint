package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func milestoneFromRow(r dbgen.Milestone) domain.Milestone {
	return domain.Milestone{
		ID:          r.ID,
		ProjectID:   r.ProjectID,
		Title:       r.Title,
		Description: r.Description,
		Status:      domain.MilestoneStatus(r.Status),
		Position:    r.Position,
		CreatedAt:   fromMS(r.CreatedAt),
		UpdatedAt:   fromMS(r.UpdatedAt),
	}
}

func (r reader) GetMilestone(ctx context.Context, id int64) (domain.Milestone, error) {
	row, err := r.q.GetMilestone(ctx, id)
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("get milestone %d: %w", id, notFound(err))
	}
	return milestoneFromRow(row), nil
}

// ListMilestonesByProject returns the project's milestones in order.
func (r reader) ListMilestonesByProject(ctx context.Context, projectID int64) ([]domain.Milestone, error) {
	rows, err := r.q.ListMilestonesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list milestones of project %d: %w", projectID, err)
	}
	out := make([]domain.Milestone, len(rows))
	for i, row := range rows {
		out[i] = milestoneFromRow(row)
	}
	return out, nil
}

// CreateMilestone inserts m at the end of its project, ignoring its ID,
// Position, and timestamps.
func (tx *Tx) CreateMilestone(ctx context.Context, m domain.Milestone) (domain.Milestone, error) {
	if err := m.Validate(); err != nil {
		return domain.Milestone{}, fmt.Errorf("create milestone: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateMilestone(ctx, dbgen.CreateMilestoneParams{
		ProjectID:   m.ProjectID,
		Title:       m.Title,
		Description: m.Description,
		Status:      string(m.Status),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("create milestone: %w", err)
	}
	return milestoneFromRow(row), nil
}

// UpdateMilestone overwrites the editable fields of milestone m.ID. ProjectID
// is fixed at creation and ignored here.
func (tx *Tx) UpdateMilestone(ctx context.Context, m domain.Milestone) (domain.Milestone, error) {
	if err := m.Validate(); err != nil {
		return domain.Milestone{}, fmt.Errorf("update milestone %d: %w", m.ID, err)
	}
	row, err := tx.q.UpdateMilestone(ctx, dbgen.UpdateMilestoneParams{
		Title:       m.Title,
		Description: m.Description,
		Status:      string(m.Status),
		Position:    m.Position,
		UpdatedAt:   tx.nowMS(),
		ID:          m.ID,
	})
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("update milestone %d: %w", m.ID, notFound(err))
	}
	return milestoneFromRow(row), nil
}

// DeleteMilestone removes a milestone and the observation links to it, and
// returns the milestone as it was. Its tasks stay in the project with no
// milestone.
func (tx *Tx) DeleteMilestone(ctx context.Context, id int64) (domain.Milestone, error) {
	err := tx.q.DetachTasksFromMilestone(ctx, dbgen.DetachTasksFromMilestoneParams{UpdatedAt: tx.nowMS(), MilestoneID: &id})
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("delete milestone %d: detach tasks: %w", id, err)
	}
	if err := tx.deleteLinksTo(ctx, domain.TargetMilestone, id); err != nil {
		return domain.Milestone{}, fmt.Errorf("delete milestone %d: delete observation links: %w", id, err)
	}
	row, err := tx.q.DeleteMilestone(ctx, id)
	if err != nil {
		return domain.Milestone{}, fmt.Errorf("delete milestone %d: %w", id, notFound(err))
	}
	return milestoneFromRow(row), nil
}
