package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
)

func TestProjectQueries(t *testing.T) {
	ctx := context.Background()
	q := dbgen.New(migrated(t))

	// The second project is created first but has the earlier timestamp, so
	// ListProjects must order by created_at, not by id.
	params := []dbgen.CreateProjectParams{
		{Title: "Learn Go", Status: "paused", CreatedAt: 200, UpdatedAt: 250},
		{Title: "Build Waypoint", Why: "Follow through", DefinitionOfDone: "I use it daily", Status: "active", CreatedAt: 100, UpdatedAt: 100},
	}
	var created []dbgen.Project
	for _, p := range params {
		got, err := q.CreateProject(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		want := dbgen.Project{
			ID: got.ID, Title: p.Title, Why: p.Why, DefinitionOfDone: p.DefinitionOfDone,
			Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		}
		if got.ID == 0 || got != want {
			t.Errorf("CreateProject = %+v, want %+v with a nonzero ID", got, want)
		}
		created = append(created, got)
	}

	for _, want := range created {
		got, err := q.GetProject(ctx, want.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("GetProject(%d) = %+v, want %+v", want.ID, got, want)
		}
	}

	if _, err := q.GetProject(ctx, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetProject(missing) error = %v, want sql.ErrNoRows", err)
	}

	list, err := q.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0] != created[1] || list[1] != created[0] {
		t.Errorf("ListProjects = %+v, want [%+v %+v]", list, created[1], created[0])
	}
}

func TestProjectQueriesEmpty(t *testing.T) {
	list, err := dbgen.New(migrated(t)).ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("ListProjects on empty DB = %#v, want an empty non-nil slice", list)
	}
}
