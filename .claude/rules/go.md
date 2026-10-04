---
paths:
  - "**/*.go"
---

# Go rules

- To orient, prefer `go doc ./internal/<pkg>` and `go doc ./internal/<pkg> <Symbol>` or
  `grep -n '^func\|^type' <file>` over reading whole files. Read bodies by line range.
- While iterating, test only the package you changed: `go test ./internal/<pkg>/ 2>&1 | tail -30`.
- Wrap errors with context: `fmt.Errorf("apply op %d: %w", opID, err)`. Never `panic` in request or worker paths.
- `context.Context` is the first parameter of every function that does I/O, and is passed through, not stored.
- Logging is `log/slog` only, with structured fields. No `fmt.Println` or `log.Printf` outside `main`.
- No package-level mutable state. All dependencies are constructed and wired in `cmd/waypoint/main.go`.
- HTTP handlers stay thin: decode and validate the request, call a service, encode the response.
  Business logic lives in services (`internal/changeset`, `internal/pipeline`, `internal/briefing`, ...).
- API errors use one JSON shape: `{"error": {"code": "...", "message": "..."}}` with a matching HTTP status.
- Use `database/sql` transactions via a helper; never leave a transaction open across a network call (LLM requests).
- Accept interfaces at package boundaries, return concrete types. Define interfaces where they are consumed.
- Tests:
  - Table-driven, next to the code (`foo_test.go`).
  - Store and service tests use a real temp SQLite DB with migrations applied (`t.TempDir()`), not mocks.
  - Pipeline tests use the mock extractor; no test may call a real LLM.
  - Use `t.Helper()` in helpers and `t.Cleanup()` for teardown.
- Goroutines must have an owner that can stop them (context cancellation) and must not leak in tests.
