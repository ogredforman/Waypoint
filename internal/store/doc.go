// Package store is the repository layer over the sqlc-generated queries. It
// takes and returns internal/domain types and hides the dbgen rows.
//
// Reads are available on both *Store and *Tx. Writes exist only on *Tx, so
// every write runs inside a transaction started by Store.InTx.
//
// Projects, tasks, blockers, and ideas are written only by internal/changeset
// (AGENTS.md hard rules); other packages use the read methods. Entries are
// insert-only: there is no update or delete for them.
package store
