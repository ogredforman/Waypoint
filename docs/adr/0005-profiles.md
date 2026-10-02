# 0005. Profiles: one database per profile

- **Status:** Accepted
- **Date:** 2026-10-01
- **PLAN task:** 1.5.8

## Context

One person wants separate areas of life (e.g. Personal and Portfolio) that never mix: their own
projects, diary, settings, and briefings. Profiles aren't accounts (SPEC §4): there is still one
user, no auth, and everything is local. Until now the server ran one SQLite file,
`<data_dir>/waypoint.db`, and that database must survive the change intact. The frontend's live
updates use `EventSource`, which can't send custom headers, so the profile can't travel in a header.
Each database runs every migration, so per-profile data can live in the schema.

## Options considered

1. **Scoping column:** a `profile_id` on every table in one database. One file to back up, but every
   query, index, and test needs the scope, and a missed `WHERE` leaks one profile into another.
2. **One database per profile (chosen):** every profile is its own SQLite file with its own store,
   worker, SSE hub, briefing service, and applier. Isolation is structural and the existing code is
   unchanged; the cost is wiring several instances and managing files.

For the details:

- **Profile in the path, a header, or a query parameter:** headers don't work for `EventSource`; a
  query parameter is easy to drop. Path it is.
- **Display names in a registry file or in each database:** a registry (e.g. `profiles.json`) can drift
  from the files on disk. A one-row table in each database makes each file self-describing.
- **Profile IDs random, numbered, or made from the name:** random IDs make the data dir unreadable
  for someone backing up by copying files. Numbered IDs are no better than slugs at avoiding reuse.
  Slugs are readable.

## Decision

- **Files:** `<data_dir>/profiles/<id>.db`. At startup the server opens every `*.db` there whose name is
  a valid ID (others are skipped with a warning), runs migrations, seeds settings from `config.toml`
  only when missing, and starts the profile's services. Pending runs resume in every profile, since
  each has its own worker.
- **IDs:** made from the name at creation (lowercase ASCII letters and digits, other runs as one `-`,
  at most 40 bytes, `profile` if nothing is left, `-2`, `-3`… if taken). IDs never change: renaming
  moves no file and breaks no URL. A deleted profile's ID can be reused by a new one with the same
  name, so a client remembering an ID must check it against the list.
- **Names:** a one-row `profile` table in each database (migration 00007 creates it with the name
  `Personal`). Names are trimmed, 1 to 60 characters, and unique ignoring case across profiles
  (`invalid` on `name` otherwise). The list is in creation order (`profile.created_at`).
- **API:** `GET /api/profiles` (list), `POST /api/profiles {name}` (201), `PATCH /api/profiles/{profile}
  {name}` (rename), `DELETE /api/profiles/{profile}` (204; 409 `last_profile` for the only one). Every
  other route moves unchanged under `/api/profiles/{profile}/`, including `/events` and `/settings`; an
  unknown profile is a 404. The old unprefixed paths are removed, with no alias. `/api/health` stays
  server-wide and pings every profile's database.
- **Moving the existing database:** if `<data_dir>/waypoint.db` exists at startup it is migrated (the
  new migration names it `Personal`), its settings seeded, its write-ahead log checkpointed into the
  main file (`wal_checkpoint(TRUNCATE)`, refusing to continue if the log isn't empty after closing),
  and then renamed in one step to `profiles/personal.db` (or the next free ID, and `Personal 2` if a
  profile already has the name). A crash before the rename leaves it in place for the next start; a
  crash after leaves only empty sidecars, removed on the next start.
- **Creating:** the database is built as `profiles/<id>.db.new` (migrated, named, settings seeded from
  config, checkpointed, closed) and renamed to `<id>.db` once complete, then started. Startup removes
  any leftover `*.db.new*`, so a half-made profile never appears.
- **Deleting:** the profile leaves the list at once (new requests get 404), its SSE streams end,
  requests already running finish, its worker stops, the database closes, and `<id>.db` with its
  `-wal` and `-shm` is removed. The last profile can't be deleted.
- **Code:** `internal/profile` (`Manager`) owns the files and each profile's services, and implements
  `api.Profiles`; `api.NewProfileHandler` is one profile's router. `api` doesn't import `profile`.

## Consequences

- Isolation needs no query changes; a profile's backup is one file.
- Nothing is shared between profiles: an export, or settings that apply to every profile, would need
  their own design. Each profile has its own worker; extraction across profiles runs in parallel.
- Data in the URL: the SPA's own routes stay profile-free, and the API client adds the profile prefix
  in one place (`apiPath` in `web/src/lib/api/client.ts`). Until 1.5.9 the app uses the first profile.
- Deleting is permanent and immediate; the UI (1.5.9) must confirm it.
- A copy of a profile file dropped into `profiles/` with a valid ID becomes a profile on the next
  start, keeping its stored name, which may then duplicate another's. Renaming fixes it.
