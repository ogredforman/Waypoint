---
paths:
  - "internal/db/**"
  - "sqlc.yaml"
---

# Database rules

- A schema change is always a new goose migration: `internal/db/migrations/NNNNN_short_name.sql`
  with `-- +goose Up` and `-- +goose Down` sections. Never edit a committed migration (a hook blocks this).
- Migrations must work on a database that already has data. Add columns as nullable or with a default.
- After editing anything in `internal/db/queries/`, run `make sqlc` and commit the generated code with it.
  Never hand-edit generated files.
- Timestamps: store as INTEGER unix milliseconds in UTC. Convert at the API boundary, not in SQL.
- Every table has `id INTEGER PRIMARY KEY` and `created_at`. Mutable tables also have `updated_at`.
- Enums are TEXT with a `CHECK (col IN (...))` constraint that matches the Go enum in `internal/domain`.
- Foreign keys are declared and enforced (`PRAGMA foreign_keys = ON` at connection open).
  Add an index on every foreign key column that is queried.
- `entries` has no UPDATE query for `body`. Do not add one.
- JSON payload columns (`change_ops.payload`, `extraction_runs.raw_output`, ...) are TEXT holding JSON,
  validated in Go before insert.
- Keep queries in SQL files grouped by entity (`tasks.sql`, `entries.sql`, ...), one named query per operation.
