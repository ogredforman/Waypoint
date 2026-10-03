# Playing with Waypoint on the mock extractor

How to drive the Stage 1 prototype by hand: the exact entry syntax the mock extractor understands,
what each line turns into, and scenarios for exercising the changeset, briefing, question, and
live-update systems. Until Stage 2 adds a real model, every diary entry is read by
`internal/extract/mock`. It's deterministic: the same entry against the same state always gives
the same result. That makes it a good harness for getting the underlying systems to feel right.

The rules here come from the code (`internal/extract/mock/mock.go`, `internal/pipeline/snapshot.go`,
`internal/briefing/heuristic.go`) and SPEC §10.4. If they disagree, the code wins. Update this file
when the mock changes.

---

## 1. Running a sandbox

Use a throwaway data dir so experiments never touch your real database.

**PowerShell**

```powershell
New-Item -ItemType Directory -Force .\tmp\play | Out-Null
Set-Content -Encoding utf8 .\tmp\play\config.toml @'
mock_latency = "1500ms"        # long enough to see the "processing" state
active_project_cap = 3
snapshot_recent_entries = 5
log_level = "debug"
'@
$env:WAYPOINT_DATA_DIR = "$PWD\tmp\play"
$env:WAYPOINT_CONFIG   = "$PWD\tmp\play\config.toml"
make dev        # then open http://localhost:5173
```

**Git Bash**

```sh
mkdir -p tmp/play && printf 'mock_latency = "1500ms"\n' > tmp/play/config.toml
WAYPOINT_DATA_DIR=$PWD/tmp/play WAYPOINT_CONFIG=$PWD/tmp/play/config.toml make dev
```

Or build once and run the binary: `make build`, then
`bin/waypoint -data-dir tmp/play -config tmp/play/config.toml` and open http://127.0.0.1:7420.

- **Reset:** stop the server and delete `tmp/play/waypoint.db*` (the `-wal` and `-shm` files too).
- **Config knobs worth turning:** `mock_latency` (`"0"` for instant, `"20s"` for restart tests),
  `snapshot_recent_entries`. These are read at startup, so restart after changing them.
- **Preferences** (`active_project_cap`, `questions_per_entry`) live in the database. The config values
  only seed a fresh database; on an existing one, change them with `PATCH /api/settings`, e.g.
  `curl -X PATCH localhost:7420/api/settings -d '{"active_project_cap": 1}'` (no restart needed).
- Entry bodies are never logged at info level, but `debug` shows run and changeset activity.

---

## 2. The entry format

An entry is plain text. The mock reads it **one line at a time, top to bottom**. Each non-blank line
is either a *prefixed line* or a *note*.

### 2.1 Line syntax

```
[indent][bullet ]<prefix>:<space?><text>
```

- **Indentation** and a leading **`- ` or `* ` bullet** are stripped. Numbered lists (`1. `) are not,
  so `1. todo: x` is a note.
- **Prefix** is the text before the *first* colon, matched **case-insensitively** and **exactly**:
  `TODO:` works, `todo :` (space before the colon) and `to-do:` don't.
- **Text** is everything after the first colon, trimmed. Later colons are kept:
  `todo: fix: login bug` creates the task `fix: login bug`.
- A prefix with **no text** (`todo:`) is a note.
- An unknown prefix (`note:`, `bug:`, `q:`) makes the whole line a note.
- Blank lines are skipped. CRLF line endings are fine. Unicode is fine.

### 2.2 Prefixes

| Prefix | Observation (highlight) | Proposed op(s) |
|---|---|---|
| `todo: <title>` | `plan` (its own highlight colour) | `create_task` in the entry's project |
| `doing: <title>` | `progress` | `set_task_status → in_progress` on the best-matching open task; if none matches, `create_task` and then `set_task_status` on it |
| `done: <title>` | `progress` | `set_task_status → done` on the best match; if none matches, `create_task` and then `set_task_status` on it |
| `blocked: <description>` | `blocker` | `create_blocker` (project-wide) in the entry's project |
| `unblocked: <description>` | none | `resolve_blocker` on the best-matching open blocker |
| `idea: <title>` | `idea` | `park_idea`, attached to the entry's project if there is one |
| `feel: <text>` | `feeling` | none |
| `decided: <text>` | `decision` | none |
| anything else | `note` | none |

Fallbacks, all by design (SPEC §10.4 "a prefixed line that yields nothing becomes a note"):

- `unblocked:` with no matching open blocker becomes a **note**.
- `todo:`, `doing:`, or `done:` with no project to put a new task in (no projects at all) becomes a **note**.
- `blocked:` with no project still gives a `blocker` observation, but no op.
- `idea:` with no project still parks the idea, unattached to any project.
- `doing:` on a task that's already in progress gives the `progress` observation and **no op**.

The observation's **summary** is the text after the prefix (`done: wrote tests` → `wrote tests`). For
notes it's the whole line. Its **highlight span** covers the whole line, prefix included.

### 2.3 Which project things go into

New tasks, blockers, and ideas go into one project per entry, chosen like this:

1. **Scoped entry** (you picked a project in the diary input, or wrote it from a project's screen):
   that project. Confidence `1.0`.
2. **Unscoped entry, one project in the snapshot:** that project, marked *guessed*. Confidence `0.9`.
3. **Unscoped, several projects:** the one with the most *clues*. Confidence `0.7`.
   - +1 for each word of the project's title that appears anywhere in the entry's line texts
     (prefix words don't count, so a project called "Todo app" doesn't win every `todo:` line);
   - +2 for each `done:`/`doing:` line that matches one of its tasks, and each `unblocked:` line
     that matches one of its blockers.
   - Ties go to the first project in the snapshot (lowest ID).
4. **No clues at all:** the first project, confidence `0.3`. Notes and other observations are
   **not** linked to it, since the review UI's project picker can move ops but not observations.

Ops on a guessed project say "project guessed from the entry" in their rationale, and the review UI
shows a **"Project: X (guessed)"** picker. Picking another project rewrites `project_id` on those
ops. Picking **none** rejects the task and blocker ops and detaches the ideas.

**Matching ignores scope.** `done:`, `doing:`, and `unblocked:` search the open tasks and blockers of
*every* project in the snapshot, even in a scoped entry. Scope only decides where *new* things go.

### 2.4 What the mock can see (the snapshot)

Each entry is extracted against a snapshot built at processing time:

- **Projects:** every **active** project, plus the entry's own project whatever its status. Paused,
  parked, and done projects are invisible to an unscoped entry: their tasks can't be matched and
  they can't be guessed.
- **Tasks:** only **open** ones (`todo`, `in_progress`). A `done:` line naming an already-done task
  doesn't match it, so it creates a new task and marks that done.
- **Blockers:** only **open** ones.
- **Ideas:** parked ideas (not used by the mock's rules).
- **Recent entries:** observation summaries of the last `snapshot_recent_entries` entries (not used
  by the mock's rules; there for Stage 2).

### 2.5 Fuzzy matching

`done:`, `doing:`, and `unblocked:` compare the line's text with task titles or blocker descriptions:

1. Both are split into lowercase words (runs of letters and digits). Stop words are dropped:
   `a an and the to of in on for with is it my i at by or be this that`.
2. Score = shared words ÷ all distinct words (Jaccard).
3. If every word of the shorter side appears in the longer side, the score is raised to at least `0.75`.
4. A score of `0.5` or more is a match. The highest score wins; ties go to the lowest ID.
5. The match score becomes the op's confidence.

There's no stemming. What that means in practice:

| Line | Task title | Score | Result |
|---|---|---|---|
| `done: write the mock extractor` | Write the mock extractor | 1.0 | match |
| `done: mock extractor` | Write the mock extractor | 0.75 (contained) | match |
| `done: wrote the mock extractor` | Write the mock extractor | 0.5 (2 of 4) | match, barely |
| `done: wrote mock extractors` | Write the mock extractor | 0.2 (1 of 5) | **no match**: a new task is created and marked done |
| `doing: CI` | Set up CI | 0.75 (contained) | match |
| `done: tests` | Write tests for the API | 0.75 (contained) | match, maybe not the one you meant |

Short lines are greedy: one word that appears in a title is a full containment match. Long, chatty
lines are stingy: extra words dilute the score. The most reliable habit is to **copy the task's key
words**.

Matching also sees tasks created by **earlier lines of the same entry**, so this works:

```
todo: add the SSE hub
doing: SSE hub
```

→ `create_task "add the SSE hub"`, then `set_task_status $new:1 → in_progress`. Rejecting the
create in review also rejects the status change that depends on it.

Blockers created in the same entry are **not** candidates for `unblocked:`, and each blocker can be
resolved at most once per entry (a second `unblocked:` for it becomes a note).

### 2.6 Duplicates

`todo:` never checks for an existing task with the same title. Writing `todo: set up CI` twice gives
two tasks. Reject the duplicate in review, or delete it on the Project screen.

### 2.7 Links (what shows up on the Task screen)

- A `done:`/`doing:` line that matches an **existing** task links its observation to that task and
  its project. These links are what build the **Task detail** timeline.
- Every other observation links to the entry's project (unless that was a no-clue guess).
- A `todo:` line, or a `done:`/`doing:` line that creates its task, links its observation to the
  new task by temp ID (`$new:N`). The link waits on that `create_task` op: applying it makes the
  link real, so the task's timeline starts with this entry; rejecting it drops the link (the
  observation stays). This holds even on a no-clue project guess, since the task moves with its op.
- A `done:`/`doing:` line naming a task created earlier in the same entry links to it the same way.

### 2.8 Follow-up questions

At most 2 per entry (`extract.MaxQuestions`, enforced by the shared validator):

1. **"What's the next step?"** when the entry has no `todo:` or `doing:` line. (`done:` alone doesn't
   count: finishing something isn't naming what's next.)
2. **"Is anything in the way of "<task>"?"** when, in the entry's project (scoped or guessed):
   - a task has been `in_progress` for more than 3 days (by its `updated_at`),
   - with no open blocker on that task, and no project-wide open blocker,
   - the entry doesn't name that task in a `done:`/`doing:` line,
   - and the entry has no `blocked:` line.
   The longest-stalled task is the one asked about.

**Answering** a question in the diary writes a new entry with `reply_to_question_id` set. That entry
is extracted like any other (prefix lines in your answer work), marks the question answered, and is
never asked the *same* question again. A reply with no `todo:`/`doing:` won't be asked "What's the
next step?" a second time. In a reply to "What's the next step?", each plain line (no prefix) counts as
a `todo:` line: answering "write the README" proposes that task. Replies to other questions keep plain
lines as notes. **Dismissing** just closes the question.

### 2.9 Confidence cheat sheet

| Situation | Confidence |
|---|---|
| Create/park in a scoped project | 1.0 |
| Unscoped idea with no project at all | 1.0 |
| Guessed: only project | 0.9 |
| Guessed: from clues | 0.7 |
| Guessed: no clues | 0.3 |
| `done:`/`doing:`/`unblocked:` on a match | the match score (0.5–1.0) |
| `done:`/`doing:` that had to create the task | the project confidence above |

---

## 3. What happens after you hit Cmd/Ctrl+Enter

1. `POST /api/entries` stores the entry (immutable from here on) and a `pending` extraction run, and
   answers `202`. The timeline shows the entry as **processing**.
2. The background worker builds the snapshot, waits `mock_latency`, runs the mock, and passes the
   result through the shared validator (`extract.Validate`).
3. Observations, one pending changeset, and questions are stored in one transaction. SSE sends
   `entry.processed` and `changeset.ready` (or `run.failed`). Briefings refresh: the entry's project,
   or every active project for an unscoped entry, since the recap reads entries.
4. You review the changeset inline under the entry: **accept / reject / edit** each op, or
   **Accept all**. Rejecting an op also rejects ops that use what it creates.
5. **Apply** runs everything in one transaction (all or nothing). Ops you left **undecided** split off
   into a new pending changeset (ADR 0003), so you can come back to them. SSE sends
   `changeset.applied`, and briefings for the touched active projects refresh.

Manual edits on the Project, Task, and Parking lot screens go through the same applier as
`source=manual` changesets, so they behave identically: one transaction, briefing refresh, live update.

---

## 4. The Now screen (heuristic briefing)

One card per **active** project:

- **Next action:** among tasks that can be started now, the most recently updated `in_progress`
  task ("You were in the middle of this."). Otherwise the smallest of the first 3 `todo` tasks by
  position (unsized counts as medium; the mock never sets a size, so this is usually just the top
  todo). A task can't be started if it has an open blocker **on that task**, depends on an open
  task, or has open subtasks.
- **Recap:** the last 3 entries about the project, oldest first ("Earlier: … Then: … Most recently:
  …"), quoting up to the **first 2 observation summaries** of each. Put the line you want remembered
  near the top of the entry.

Mock blockers are always **project-wide** (`blocked:` has no task to attach to), so they never change
the next action. They do suppress the stalled-task question.

---

## 5. Scenarios

Each one targets a system. Run them in order on a fresh sandbox, or pick and mix. "Scoped" means
picking the project in the diary input.

### A. First project, first loop
1. Projects → New project: **Garden shed**, active.
2. Scoped entry:
   ```
   Spent the morning measuring the corner of the yard.
   todo: order lumber
   todo: level the ground
   blocked: waiting for the permit
   idea: a little window box
   feel: excited, a bit overwhelmed
   ```
3. Watch: processing → highlights (note, two plans in their own colour, blocker, idea, feeling) → a
   changeset with 2 `create_task`, `create_blocker`, `park_idea`. No questions (it has `todo:`).
4. Accept all, Apply. The Project screen, Parking lot, and Now card update without a reload. Next
   action: *order lumber*.

### B. Progress and matching
```
doing: lumber
```
→ `in_progress` on *order lumber* (confidence 0.75), progress highlight linked to the task.
"What's the next step?" is **not** asked (`doing:` counts). Apply, then open the task: its timeline
shows this entry. Then try the edges:
- `done: order the lumber` → same words as the title → 1.0 → matches.
- `done: ordered the lumber` → "ordered" isn't "order", 1 of 3 words → 0.33 → **no match**, so it
  creates a new task and marks it done. Reject both ops.
- `done: ground` → matches *level the ground* even though you might mean something else.

### C. Questions
- `feel: tired today` → "What's the next step?". Answer with `todo: buy screws`. The reply makes a
  task and isn't asked the same question again.
- Answer "What's the next step?" with plain lines (`write the README`, `add a logo`): each becomes a
  proposed task. Apply, and the Project screen's Next card shows one of them.
- Answer the stalled question with plain prose ("not sure yet"). It stays a note, and isn't asked again.
- Stalled question: needs a task `in_progress` for 3+ days. Wait, or backdate it (section 6).

### D. Unscoped guessing
1. Create a second active project, **Waypoint planner**, with task *set up CI*.
2. Unscoped entries:
   - `todo: paint the shed door` → guessed **Garden shed** ("shed" is a title word), 0.7.
   - `doing: set up CI` + `todo: add caching` → guessed **Waypoint planner** (+2 for the matched task).
   - `Busy day.` + `todo: call mom` → no clues → first project at 0.3, and the "Busy day." note
     isn't linked to any project.
3. Use the "Project: X (guessed)" picker: move the ops to the other project, or choose **none** and
   watch the task ops get rejected and the idea detached.

### E. Partial apply and split-off
1. An entry with 4 `todo:` lines. Accept 2, reject 1, leave 1 undecided, Apply.
2. The undecided op reappears in a new pending changeset under the same entry. Apply it later.
3. Open each task: the two accepted ones show their `todo:` line; the rejected one was never
   created, and the split-off one shows its line once applied.

### F. The active-project cap
Set the cap to 1 (`curl -X PATCH localhost:7420/api/settings -d '{"active_project_cap": 1}'`, or
`active_project_cap = 1` in the config of a fresh database). Try to create a second active project, change a paused
one to active, or activate a parked project from the Parking lot. Each should explain calmly and
offer to park instead, with no red error states. The mock never creates or activates projects, so
the cap only comes up in manual flows here.

### G. Visibility of non-active projects
Pause **Garden shed**. An unscoped `doing: lumber` no longer matches (the project isn't in the
snapshot) and can't be guessed, and its Now card goes away. A **scoped** entry on Garden shed
still sees it.

### H. Live updates
Open the app in two windows. Apply a changeset or edit a task in one; the other updates. Stop the
server: the stream reconnects when it's back and screens reload.

### I. Crash resume
Set `mock_latency = "20s"`, restart, write an entry, and stop the server while it says
processing. Start it again: the pending run resumes and the entry finishes.

### J. Ideas and the parking lot
`idea: solar lights` (scoped), apply, then in the Parking lot promote it to a task in a project or to
a new parked project, or delete it. Unscoped with no projects at all → an unattached idea.

### K. Reprocess (API only for now)
`POST /api/entries/{id}/reprocess` re-runs extraction on an entry against the *current* state. It
rejects that entry's pending changesets and dismisses its open questions first. Handy after you've
changed tasks to see how the same text reads now.

---

## 6. Poking the internals

### API with curl (Git Bash; in PowerShell use `curl.exe`)

The Go server is on `127.0.0.1:7420` (the Vite dev server on `:5173` proxies `/api` to it).

```sh
API=http://127.0.0.1:7420/api
curl -s $API/projects
curl -s -X POST $API/entries -H 'Content-Type: application/json' \
  -d '{"body":"todo: order lumber\nblocked: no permit","project_id":1}'
curl -s $API/entries/1                     # observations, questions, changesets for one entry
curl -s "$API/changesets?status=pending"
curl -s -X POST $API/changesets/1/apply -H 'Content-Type: application/json' \
  -d '{"ops":[{"id":1,"decision":"accept"},{"id":2,"decision":"reject"}]}'
curl -s -X POST $API/changesets/2/apply -H 'Content-Type: application/json' \
  -d '{"ops":[{"id":3,"decision":"edit","payload":{"project_id":1,"title":"order cedar lumber"}}]}'
curl -s $API/projects/1/briefing
curl -s -X POST $API/entries/1/reprocess
curl -N $API/events                        # watch the SSE stream live
```

Scripting entries this way is the quickest way to load a lot of state: a loop of
`POST /api/entries` calls followed by applies.

### Backdating a task (to trigger the stalled question)

There's no sqlite3 on this machine, but Python's standard library has it. Stop the server first.
Timestamps are Unix **milliseconds**.

```sh
python - <<'EOF'
import sqlite3, time
db = sqlite3.connect("tmp/play/waypoint.db")
four_days_ago = int((time.time() - 4 * 86400) * 1000)
db.execute("UPDATE tasks SET updated_at = ? WHERE status = 'in_progress'", (four_days_ago,))
db.commit()
EOF
```

Restart and write a scoped entry on that project with no `blocked:` line and no mention of the task.
This writes the database behind the changeset system's back, so keep it to sandboxes.

### Unit-level experiments

The fastest loop for "what does the mock do with this text?" is a table case in
`internal/extract/mock/mock_test.go` (`TestPrefixes`, `TestProjectGuess`, `TestQuestions`) and
`go test ./internal/extract/mock -run TestPrefixes -v`. The `snapshot()` fixture there has two
projects with known tasks and a blocker.

---

## 7. Known rough edges (worth noticing while you play)

These are current behaviour, not bugs to fix in Stage 1. Ones that bother you are good `/park`
material or Stage 2 prompt requirements.

- No duplicate check on `todo:`.
- No stemming; chatty `done:` lines miss and silently create a new done task.
- One-word lines can match the wrong task.
- `blocked:` can't name a task, so blockers never affect the next action.
- `done:` on an already-done task creates a duplicate done task.
- `unblocked:` can't resolve a blocker created earlier in the same entry.
- Notes never link to tasks, even when they name one; only `todo:`/`done:`/`doing:` do.
- The recap quotes only the first two observations per entry.
- Reprocess has no button yet.
