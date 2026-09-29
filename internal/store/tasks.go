package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func taskFromRow(r dbgen.Task) domain.Task {
	return domain.Task{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		MilestoneID:   r.MilestoneID,
		ParentTaskID:  r.ParentTaskID,
		Title:         r.Title,
		Notes:         r.Notes,
		Status:        domain.TaskStatus(r.Status),
		Size:          stringPtrToEnum[domain.TaskSize](r.Size),
		Energy:        stringPtrToEnum[domain.Energy](r.Energy),
		Position:      r.Position,
		OriginEntryID: r.OriginEntryID,
		CreatedAt:     fromMS(r.CreatedAt),
		UpdatedAt:     fromMS(r.UpdatedAt),
	}
}

func (r reader) GetTask(ctx context.Context, id int64) (domain.Task, error) {
	row, err := r.q.GetTask(ctx, id)
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task %d: %w", id, notFound(err))
	}
	return taskFromRow(row), nil
}

// ListTasksByProject returns the project's tasks in position order.
func (r reader) ListTasksByProject(ctx context.Context, projectID int64) ([]domain.Task, error) {
	rows, err := r.q.ListTasksByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks of project %d: %w", projectID, err)
	}
	out := make([]domain.Task, len(rows))
	for i, row := range rows {
		out[i] = taskFromRow(row)
	}
	return out, nil
}

// CreateTask inserts t at the end of its project, ignoring its ID, Position,
// and timestamps.
func (tx *Tx) CreateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	if err := t.Validate(); err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateTask(ctx, dbgen.CreateTaskParams{
		ProjectID:     t.ProjectID,
		MilestoneID:   t.MilestoneID,
		ParentTaskID:  t.ParentTaskID,
		Title:         t.Title,
		Notes:         t.Notes,
		Status:        string(t.Status),
		Size:          enumPtrToString(t.Size),
		Energy:        enumPtrToString(t.Energy),
		OriginEntryID: t.OriginEntryID,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}
	return taskFromRow(row), nil
}

// UpdateTask overwrites the editable fields of task t.ID. ProjectID and
// OriginEntryID are fixed at creation and ignored here.
func (tx *Tx) UpdateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	if err := t.Validate(); err != nil {
		return domain.Task{}, fmt.Errorf("update task %d: %w", t.ID, err)
	}
	row, err := tx.q.UpdateTask(ctx, dbgen.UpdateTaskParams{
		MilestoneID:  t.MilestoneID,
		ParentTaskID: t.ParentTaskID,
		Title:        t.Title,
		Notes:        t.Notes,
		Status:       string(t.Status),
		Size:         enumPtrToString(t.Size),
		Energy:       enumPtrToString(t.Energy),
		Position:     t.Position,
		UpdatedAt:    tx.nowMS(),
		ID:           t.ID,
	})
	if err != nil {
		return domain.Task{}, fmt.Errorf("update task %d: %w", t.ID, notFound(err))
	}
	return taskFromRow(row), nil
}

// DeleteTask removes a task and returns it as it was. What pointed at it is
// cleaned up first (SPEC §6.2): subtasks become top-level, dependencies in
// either direction are removed, blockers stay on the project without a task,
// briefings lose it as the next action, and observation links to it are
// removed. The observations and entries themselves stay.
func (tx *Tx) DeleteTask(ctx context.Context, id int64) (domain.Task, error) {
	now := tx.nowMS()
	cleanups := []struct {
		what string
		run  func() error
	}{
		{"detach subtasks", func() error {
			return tx.q.DetachSubtasks(ctx, dbgen.DetachSubtasksParams{UpdatedAt: now, ParentTaskID: &id})
		}},
		{"delete dependencies", func() error { return tx.q.DeleteTaskDependencies(ctx, id) }},
		{"detach blockers", func() error {
			return tx.q.DetachBlockersFromTask(ctx, dbgen.DetachBlockersFromTaskParams{UpdatedAt: now, TaskID: &id})
		}},
		{"clear briefings", func() error {
			return tx.q.ClearBriefingNextTask(ctx, dbgen.ClearBriefingNextTaskParams{UpdatedAt: now, NextTaskID: &id})
		}},
		{"delete observation links", func() error { return tx.deleteLinksTo(ctx, domain.TargetTask, id) }},
	}
	for _, c := range cleanups {
		if err := c.run(); err != nil {
			return domain.Task{}, fmt.Errorf("delete task %d: %s: %w", id, c.what, err)
		}
	}

	row, err := tx.q.DeleteTask(ctx, id)
	if err != nil {
		return domain.Task{}, fmt.Errorf("delete task %d: %w", id, notFound(err))
	}
	return taskFromRow(row), nil
}
