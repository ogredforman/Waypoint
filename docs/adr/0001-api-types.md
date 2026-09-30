# 0001. API type sharing between Go and the frontend

- **Status:** Accepted
- **Date:** 2026-09-29
- **PLAN task:** 0.8

## Context

The Svelte SPA talks to the Go server over REST (`/api/*`) and SSE (`/api/events`) (SPEC §10.5).
Every request, response, and event payload is defined twice: once as a Go struct with JSON tags and
once as a TypeScript type. If the two are kept in sync by hand they will drift, and
`.claude/rules/web.md` already forbids hand-duplicating API types. SPEC §13 left the choice open
between writing an OpenAPI spec first and generating TS types from Go structs.

Constraints: one developer, local-first, single user, no third-party API clients. Standard-library-first,
boring Go (AGENTS.md). Code generation already runs through `go tool` (sqlc, staticcheck), and
`make lint` already fails when generated code is stale (`sqlc diff`).

## Options considered

1. **Go structs → TS types with `tygo`.** Go DTOs are the single source of truth; tygo reads a Go
   package and writes TS types. Pros: one source of truth, runs as a `go tool` with no npm dependency,
   little ceremony, and the output is plain TS types with no runtime cost. Cons: only the data shapes
   are typed, not routes or methods; some Go types need explicit mapping rules.
2. **OpenAPI-first.** A hand-written OpenAPI YAML file, with `oapi-codegen` for Go and
   `openapi-typescript` for TS. Pros: routes and methods are typed too, and the contract is a standard.
   Cons: someone has to keep a YAML spec current, it adds two generators (one from npm), and it's more
   process than a local, single-user app needs.
3. **Go → OpenAPI via a framework such as `huma`.** Pros: the spec comes for free. Cons: the framework
   takes over how chi handlers are written, which cuts against "boring Go".
4. **Hand-written TS mirrors.** Pros: no tooling. Cons: the two sides drift apart silently. Already
   ruled out by `.claude/rules/web.md`.

## Decision

Generate TypeScript types from Go structs with `tygo`. The Go DTOs are the source of truth, and the
generated TS file is never edited by hand.

## Consequences

- **Where the types live:** wire DTOs, including SSE event payloads, go in their own package,
  `internal/api/apitypes`. tygo exports every exported type in a package, so keeping DTOs separate
  stops handler internals from leaking into the TS output. For the same reason, tygo must not read
  all of `internal/domain`. Enums need to reach tygo without the rest of that package: either put them
  in a small leaf package (e.g. `internal/domain/enums`) that tygo reads next to `apitypes`, or
  re-declare the wire enums in `apitypes`. Choose one in PLAN 1.1, where the enums are written.
- **Output:** `web/src/lib/api/types.gen.ts`. It is committed and never edited by hand.
- **Keeping it current:** `make gen` regenerates the file, and `make lint` fails if it is stale, the
  same way `sqlc diff` works.
- **Mapping rules may be needed.** tygo's defaults won't fit every Go type. When the first DTOs are
  written, decide on the mapping rules and set them in the tygo config. Expected cases:
  - `time.Time` → an RFC 3339 `string`.
  - `int64` IDs → `number`. This is safe because SQLite rowids stay well under 2^53.
  - Nullable fields use pointers, which become `T | null`. Watch that `omitempty` and optional (`?`)
    fields in the output match what the JSON encoder actually sends.
  - Typed string enums (`type TaskStatus string` + consts): check what tygo emits by default. It may
    be a plain `string` alias plus constants rather than a union type. If so, configure the output
    style so the TS side gets a union type.
  - `sql.Null*` and sqlc-generated types must not appear in DTOs. Convert them in the handler layer.

  Rules that turn out to be non-obvious get recorded here or in a follow-up ADR.
- **What stays hand-written:** route paths and HTTP methods, in a small `web/src/lib/api/client.ts`.
  That's the price of option 1. Revisit OpenAPI (option 2 or 3) if auth, multiple clients, or a
  published API ever need a formal contract.
- **Adoption:** tygo is added as a `go tool` dependency in PLAN 1.14, with the first real endpoints,
  and the commit message gives the one-line reason. Until then, `/api/health` is a temporary
  exception to "never hand-duplicated": in 1.14 its response type moves into `apitypes`, and
  `web/src/lib/health.ts` switches to the generated type.

## Implementation notes (PLAN 1.14)

How the consequences above were settled when the first DTOs landed:

- **Generator:** `internal/api/apitypes/gen` runs tygo as a library, not as a `go tool`. That keeps
  the tygo config in Go next to the enum list, and lets `-check` generate into a temp dir and compare
  with the committed files. Comparing against the working tree (like `sqlc diff`) instead of
  `git diff` means uncommitted but current output doesn't fail lint. `make gen` writes the files;
  `make lint` runs `gen -check`.
- **Enums:** they stay in `internal/domain`. tygo only emits a union for consts named after their type
  (`TaskStatusTodo`), and the domain's are not (`TaskTodo`), so the generator writes
  `web/src/lib/api/enums.gen.ts` itself from the domain's value lists (`domain.TaskStatuses()` and so
  on): `export const taskStatusValues = [...] as const` and `export type TaskStatus = ...`. DTOs use
  the domain enum types directly, and `types.gen.ts` imports them as `domain.TaskStatus`.
- **Mappings:** `time.Time` → `string` (RFC 3339). `int64` → `number`. The generator fails if tygo
  falls back to `any` for an unmapped type.
- **Nullable fields:** tygo makes every pointer field optional (`field?: T`), but `encoding/json`
  always sends a pointer without `omitempty`, as `null` when unset. Such fields are tagged
  `tstype:"T | null,required"`; a test in `apitypes` enforces it. Pointer fields with `omitempty`
  are request fields the client may leave out, so they stay optional. A request field where `null`
  means "clear it" is tagged `tstype:"T | null"` and becomes `field?: T | null`.
- **Raw JSON:** `json.RawMessage` (op payloads) is tagged `tstype:"{ [key: string]: unknown }"`.
  Tag values can't contain commas, so `Record<K, V>`, `Extract`, and `Exclude` can't be used in tags.
- **Request DTOs for PATCH routes** only type the frontend. The handlers read PATCH bodies as raw
  fields so that "absent" and `null` stay distinct, and pass them to the op payload decoder. A test
  checks that each request DTO lists the same fields as its op payload.
