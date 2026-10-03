package db

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

// checkIn matches `CHECK (col IN ('a', 'b', ...))` in a CREATE TABLE statement.
var checkIn = regexp.MustCompile(`(?i)CHECK\s*\(\s*(\w+)\s+IN\s*\(([^)]*)\)\s*\)`)

// sqlEnums returns the allowed values of every CHECK (... IN ...) column in the
// migrated schema, keyed by "table.column".
func sqlEnums(t *testing.T) map[string][]string {
	t.Helper()
	sqlDB := migrated(t)
	rows, err := sqlDB.QueryContext(context.Background(),
		`SELECT name, sql FROM sqlite_master WHERE type = 'table' AND sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var table, ddl string
		if err := rows.Scan(&table, &ddl); err != nil {
			t.Fatal(err)
		}
		for _, m := range checkIn.FindAllStringSubmatch(ddl, -1) {
			var values []string
			for v := range strings.SplitSeq(m[2], ",") {
				values = append(values, strings.Trim(strings.TrimSpace(v), "'"))
			}
			out[table+"."+m[1]] = values
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func strs[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// TestEnumsMatchSchema keeps the Go enums in internal/domain and the SQL CHECK
// lists in the migrations identical, values and order.
func TestEnumsMatchSchema(t *testing.T) {
	goEnums := map[string][]string{
		"projects.status":                       strs(domain.ProjectStatuses()),
		"settings.theme":                        strs(domain.Themes()),
		"settings.interface_font":               strs(domain.InterfaceFonts()),
		"settings.diary_font":                   strs(domain.DiaryFonts()),
		"milestones.status":                     strs(domain.MilestoneStatuses()),
		"tasks.status":                          strs(domain.TaskStatuses()),
		"tasks.size":                            strs(domain.TaskSizes()),
		"tasks.energy":                          strs(domain.Energies()),
		"extraction_runs.status":                strs(domain.ExtractionRunStatuses()),
		"follow_up_questions.status":            strs(domain.QuestionStatuses()),
		"observations.kind":                     strs(domain.ObservationKinds()),
		"briefing_recap_points.kind":            strs(domain.ObservationKinds()),
		"observation_links.target_type":         strs(domain.TargetTypes()),
		"pending_observation_links.target_type": strs(domain.TargetTypes()),
		"blockers.status":                       strs(domain.BlockerStatuses()),
		"ideas.status":                          strs(domain.IdeaStatuses()),
		"changesets.source":                     strs(domain.ChangesetSources()),
		"changesets.status":                     strs(domain.ChangesetStatuses()),
		"change_ops.op_type":                    strs(domain.OpTypes()),
		"change_ops.status":                     strs(domain.ChangeOpStatuses()),
		"briefings.source":                      strs(domain.BriefingSources()),
	}
	sql := sqlEnums(t)

	for col, want := range sql {
		got, ok := goEnums[col]
		if !ok {
			t.Errorf("%s has a CHECK IN list %v but no Go enum mapped in this test", col, want)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: Go enum %v, SQL CHECK %v", col, got, want)
		}
	}
	for col := range goEnums {
		if _, ok := sql[col]; !ok {
			t.Errorf("%s: no CHECK IN list found in the schema", col)
		}
	}
}
