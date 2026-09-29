# Migrations

Goose SQL migrations, embedded into the binary and applied at startup (`db.Migrate`).

- Name: `NNNNN_short_name.sql` (e.g. `00001_core_schema.sql`), numbered in order.
- Each file has `-- +goose Up` and `-- +goose Down` sections.
- Never edit a migration that's already committed. Add a new one. See `.claude/rules/db.md`.

Goose only reads `.sql` files here; this README is ignored (it also keeps `go:embed` happy
before the first migration exists).
