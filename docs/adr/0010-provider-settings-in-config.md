# 0010. Provider settings: changed in the app, saved in config.toml

- **Status:** Accepted
- **Date:** 2026-10-13
- **PLAN task:** 2.12

## Context

The provider, model, endpoint, and API key were read from `config.toml` or `WAYPOINT_*` env vars at
startup only (SPEC §8). PLAN 2.12 asks for them on the Settings screen, with the key kept in config,
never shown, and never exported. Every profile shares one extractor (ADR 0005), so these settings are
server-wide, unlike the per-profile `settings` table.

## Options considered

1. **A per-profile `settings` column.** Fits the Settings API, but puts the key in the database (and so
   near the export and the DB-file backup), and profiles would need their own extractors.
2. **A separate app-written file (e.g. `provider.toml`).** Easy to rewrite whole, but splits one kind of
   setting across two files and the SPEC says the key lives in config.
3. **Write the four keys back into `config.toml`.** One place for them, as today. Re-encoding the whole
   file would drop the user's comments, so only those keys' lines are edited.

## Decision

Option 3. `GET`/`PUT /api/provider` (server-wide, beside `/api/profiles`) read and change them;
`config.SaveProvider` replaces, adds, or removes only the top-level `provider`, `model`, `endpoint`, and
`api_key` lines, checks the result decodes to those values, and writes the file atomically with mode
0600. A setting from an env var wins over the file, so the app shows it read-only, refuses to change
it, and never copies its value into the file. `provider.Switch` is the extractor every profile uses:
an update is checked the way startup checks the config, saved, then swapped in for later calls. While
the provider is the mock, its `Call` returns `extract.ErrNoModel` and the LLM briefer quietly keeps
the heuristic's briefing.

## Consequences

- No restart after changing provider; a model request already sent finishes on the old extractor.
- The key never enters a database, so the export (2.13) and a copied DB file can't contain it.
- `config.toml` may now be written by the server; a hand-edited multi-line value on one of the four
  keys makes the save fail rather than corrupt the file.
