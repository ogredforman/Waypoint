package store

import (
	"context"
	"fmt"
	"slices"
)

// ExportTables lists every table of a profile database, in an order where
// each table comes after the tables it references, so an import can insert
// them as listed. The one exception is entries.reply_to_question_id: a reply
// names a question from an earlier entry's run, so the tables reference each
// other and an import needs deferred foreign keys. goose's own version table
// isn't data and isn't listed.
// export_test.go fails when a migration adds a table this list misses.
func ExportTables() []string {
	return []string{
		"profile",
		"settings",
		"projects",
		"milestones",
		"entries",
		"tasks",
		"task_dependencies",
		"extraction_runs",
		"follow_up_questions",
		"observations",
		"observation_links",
		"blockers",
		"ideas",
		"changesets",
		"change_ops",
		"pending_observation_links",
		"briefings",
		"briefing_recap_points",
	}
}

// ExportTable returns every row of table, one map per row keyed by column
// name, in rowid order. Values are as stored: integers (timestamps are unix
// milliseconds), text (JSON columns stay strings), and nil for NULL. Read
// all tables in one InReadTx so they agree with each other.
func (r reader) ExportTable(ctx context.Context, table string) ([]map[string]any, error) {
	if !slices.Contains(ExportTables(), table) {
		return nil, fmt.Errorf("export %s: not an exported table", table)
	}
	// The name is one of ExportTables, so it is safe to put in the query.
	rows, err := r.db.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY rowid")
	if err != nil {
		return nil, fmt.Errorf("export %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("export %s: %w", table, err)
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("export %s: %w", table, err)
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			row[c] = vals[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export %s: %w", table, err)
	}
	return out, nil
}

// SchemaVersion is the newest migration applied to the database.
func (r reader) SchemaVersion(ctx context.Context) (int64, error) {
	var v int64
	if err := r.db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("schema version: %w", err)
	}
	return v, nil
}
