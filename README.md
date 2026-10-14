# Waypoint

A local-first, AI-assisted diary that turns what you write about your projects into a plan you can
actually follow: tasks, blockers, parked ideas, and one clear next action.

You write free-form diary entries. A model reads each one and proposes changes (new tasks, a task
done, a blocker, an idea to park) as a changeset you review before anything is applied. The Now
screen keeps a short briefing with one next action.

**Status:** Stage 2 (MVP). Entries are read by a real model, local or hosted; without one, a mock
reads only `todo:`/`done:`-style lines. One Go binary with the web app embedded, SQLite on disk,
single user, no accounts.

- What it is and why: [`docs/SPEC.md`](docs/SPEC.md)
- How it's being built, and what's next: [`docs/PLAN.md`](docs/PLAN.md)
- Instructions for AI coding agents: [`AGENTS.md`](AGENTS.md)

## Install

There are no prebuilt releases yet; build from source.

Requirements: Go 1.26+, GNU `make`, and Node.js 24+ (for the Svelte frontend in `web/`).

```sh
git clone https://github.com/ogredforman/waypoint.git
cd waypoint
make build
```

This builds the frontend and then `bin/waypoint` (`bin\waypoint.exe` on Windows) with it embedded.
The binary is self-contained: copy it anywhere.

## Run

```sh
bin/waypoint
```

Open http://127.0.0.1:7420. The server only listens on `127.0.0.1`, so it's reachable from this
machine only. Stop it with Ctrl+C.

On first run Waypoint creates its config and data folders (see [Configuration](#configuration)) and a
profile called "Personal". The Now screen opens on a short welcome with a form for your first
project. Then write an entry on the Diary screen, review the proposed changes, and apply them.

Until a provider is set up, entries are read by the **mock**, which understands only lines like
`todo: write the README` or `done: write the README` (the full syntax is in
[`docs/MOCK-TESTING.md`](docs/MOCK-TESTING.md)). Set up a model to write naturally.

## Configure a provider

A provider is the model that reads your entries and writes the briefing. Three choices:

| `provider` | What it is | Needs |
|---|---|---|
| `mock` (default) | No model; reads `todo:`/`done:`-style lines only | nothing |
| `anthropic` | Claude, via the Anthropic API | an API key |
| `openai-compatible` | Any server with an OpenAI-style `/chat/completions` and tool calling: Ollama, LM Studio, llama.cpp's `llama-server`, OpenAI, and hosted services like it | an endpoint and a model; a key if the server wants one |

The provider applies to every profile. Set it in one of three places:

1. **In the app:** Settings → Model. Pick the provider, fill in model, endpoint, and key, and save.
   The change takes effect immediately, no restart, and is saved to `config.toml`. The key is
   shown only as set or not set.
2. **In `config.toml`** (in the config folder below), then restart Waypoint.
3. **As environment variables:** `WAYPOINT_PROVIDER`, `WAYPOINT_MODEL`, `WAYPOINT_ENDPOINT`,
   `WAYPOINT_API_KEY`. An env var wins over `config.toml`, and the Settings screen shows that field
   read-only.

There are no command-line flags for these, so a key never ends up in shell history.

### Anthropic

```toml
provider = "anthropic"
api_key  = "sk-ant-..."
# model    = "claude-haiku-4-5"              # the default
# endpoint = "https://api.anthropic.com/v1"  # the default
```

Get a key from the Anthropic Console. The default model is the smallest and cheapest Claude model;
move to a larger one only if it misses too much.

### Ollama (local, free)

```sh
ollama pull qwen3:8b
```

```toml
provider = "openai-compatible"
endpoint = "http://localhost:11434/v1"
model    = "qwen3:8b"
```

**Set Ollama's context length to 16k or more** (in the Ollama app's settings, or
`OLLAMA_CONTEXT_LENGTH=16384` before `ollama serve`). The prompt and tool schema take about 5k
tokens before your projects are added; with Ollama's default of 4096 the model never sees the entry,
and the run "succeeds" with nothing proposed. Pick a model that supports tool calling.

### LM Studio, llama.cpp, OpenAI, and other OpenAI-compatible servers

Same as Ollama, with that server's base URL (the part before `/chat/completions`) and model name:

| Server | `endpoint` | `api_key` |
|---|---|---|
| LM Studio | `http://localhost:1234/v1` | not needed |
| llama.cpp `llama-server` | `http://localhost:8080/v1` | not needed |
| OpenAI | `https://api.openai.com/v1` | required |

For local servers, the same context-length advice applies: give the model at least 16k.

### Timeouts

`timeout` in `config.toml` (or `WAYPOINT_TIMEOUT`), a duration such as `"10m"`, bounds each model
request. The default is 5 minutes for `openai-compatible` (local models can be slow) and 2 minutes
for `anthropic`. If a run fails or times out, the entry says so and offers Read again; the entry
itself is always saved.

### Where the key lives

Only in `config.toml` (written with owner-only permissions) or the environment. It is never stored
in a database, never included in an export, and never logged.

### Checking a model

From a clone of the repo, `waypoint eval` runs the sample entries in `testdata/entries` against a
provider and scores what it proposes:

```sh
bin/waypoint eval                                   # the configured provider
bin/waypoint eval -provider openai-compatible -endpoint http://localhost:11434/v1 -model qwen3:8b
bin/waypoint eval -run reply -v                     # only fixtures named *reply*, showing what was missed
```

The key and timeout come from `config.toml` or the env vars, as for the server.

## Configuration

Each setting comes from the first place that sets it: command-line flag, then `WAYPOINT_*` environment
variable, then `config.toml`, then the default. Run `waypoint -h` to list the flags.

| Flag | Env | `config.toml` | Default |
|---|---|---|---|
| `-config` | `WAYPOINT_CONFIG` | n/a | `config.toml` in the OS config folder (see below) |
| `-port` | `WAYPOINT_PORT` | `port` | `7420` |
| `-data-dir` | `WAYPOINT_DATA_DIR` | `data_dir` | OS data folder (see below) |
| `-log-level` | `WAYPOINT_LOG_LEVEL` | `log_level` | `info` |
| n/a | `WAYPOINT_PROVIDER` | `provider` | `mock` (see [Configure a provider](#configure-a-provider)) |
| n/a | `WAYPOINT_MODEL` | `model` | the provider's (`claude-haiku-4-5` for `anthropic`) |
| n/a | `WAYPOINT_ENDPOINT` | `endpoint` | the provider's (`https://api.anthropic.com/v1` for `anthropic`) |
| n/a | `WAYPOINT_API_KEY` | `api_key` | none |
| n/a | `WAYPOINT_TIMEOUT` | `timeout` | `5m` for `openai-compatible`, `2m` for `anthropic` |
| n/a | n/a | `active_project_cap` | `3` (how many projects can be active at once; seeds a new database only, see below) |
| n/a | n/a | `questions_per_entry` | `2` (follow-up questions per entry, `0` to `2`; seeds a new database only) |
| n/a | n/a | `snapshot_recent_entries` | `5` (how many earlier entries the model sees for context; `0` for none) |
| n/a | n/a | `stalled_after_days` | `14` (days an in-progress task can go unchanged before the model may suggest splitting it) |
| n/a | n/a | `mock_latency` | `0` (artificial delay per mock extraction, e.g. `"1500ms"`) |

`active_project_cap` and `questions_per_entry` are preferences, stored in the database. The config
values only seed a profile that has no settings yet (including every new profile); after that, change
them on the Settings screen.

| OS | Config folder | Data folder (holds `profiles/<id>.db`, one database per profile) |
|---|---|---|
| Windows | `%APPDATA%\Waypoint` | `%LOCALAPPDATA%\Waypoint` |
| macOS | `~/Library/Application Support/Waypoint` | `~/Library/Application Support/Waypoint` |
| Linux | `$XDG_CONFIG_HOME/waypoint` or `~/.config/waypoint` | `$XDG_DATA_HOME/waypoint` or `~/.local/share/waypoint` |

A relative `data_dir` in `config.toml` is relative to that file.

## Backups

Two ways, both per profile:

- **Copy the database file.** Quit Waypoint, then copy `profiles/<id>.db` from the data folder above
  (Settings → Data names the file). Copy it only while Waypoint is stopped: the database runs in WAL
  mode, so a copy made while it runs can miss recent changes. To restore, quit Waypoint and put the
  file back.
- **Export as JSON.** Settings → Data → Export as JSON (or `GET /api/profiles/<id>/export`) downloads
  every table of the profile in one file, readable outside Waypoint. Importing it comes later.

Neither contains an API key: keys live in `config.toml`, never in a database.

## Development

Run `npm --prefix web install` once, then:

| Target | What it does |
|---|---|
| `make dev` | Run the Go server and the Vite dev server together; open http://localhost:5173 (Vite proxies `/api` to Go) |
| `make build` | Build the frontend, then `bin/waypoint` with it embedded; the binary serves it at http://127.0.0.1:7420 |
| `make test` | All Go and frontend tests |
| `make e2e` | Playwright smoke test of write → review → apply against the real binary |
| `make lint` | `go vet`, `staticcheck`, generated code is current, `svelte-check` |
| `make sqlc` | Regenerate query code after editing `internal/db/queries/` |
| `make gen` | Regenerate the frontend's API types after editing `internal/api/apitypes` or the domain enums |

`staticcheck` and `sqlc` are pinned as tool dependencies in `go.mod` (`go tool ...`), so they need
no separate install.

### Workflow (Claude Code)

| Command | What it does |
|---|---|
| `/devflow:next-task` | Picks the next unchecked task in `docs/PLAN.md`, plans it, waits for approval, builds, tests, reviews |
| `/devflow:next-task 1.5` | Same, for a specific task |
| `/devflow:park <idea>` | Saves an idea to PLAN.md's Parked section without acting on it |
| `/devflow:progress` | Where things stand and what's next |

Requirements for the hooks in `.claude/settings.json`: Go on your PATH, and `git`.
