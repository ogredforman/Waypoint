// Package db opens the SQLite database and runs the embedded goose migrations.
// The sqlc query files live in queries/ and the code generated from them in dbgen/.
//
// Write queries for projects, tasks, blockers, and ideas are for the changeset
// applier (internal/changeset) only; nothing else may mutate those tables.
package db
