package apitypes

import "time"

// Export is the body of GET /api/export: everything in one profile's
// database, table by table, for keeping or moving the data elsewhere. Rows
// are as stored, keyed by column name: timestamps are unix milliseconds, JSON
// columns (op payloads, a run's raw output) are strings, NULL is null. API
// keys live in config.toml, never in a database, so no export contains one.
type Export struct {
	// Format is always "waypoint-export".
	Format string `json:"format"`
	// Version is the export format's version, 1. It changes only if this
	// envelope does; the tables follow SchemaVersion.
	Version int `json:"version"`
	// SchemaVersion is the newest database migration applied.
	SchemaVersion int64     `json:"schema_version"`
	ExportedAt    time.Time `json:"exported_at"`
	// Tables holds every row of every table, keyed by table name. Tables are
	// listed in TableOrder, parents before the tables that reference them.
	Tables     map[string][]map[string]any `json:"tables"`
	TableOrder []string                    `json:"table_order"`
}
