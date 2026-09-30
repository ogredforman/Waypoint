---
name: next-task
description: Pick up and complete the next task from docs/PLAN.md (or a specific one, e.g. /next-task 1.5) using the project's plan, build, verify, review loop.
argument-hint: "[stage.task, e.g. 1.5]"
disable-model-invocation: true
---

# Next task

Complete exactly one task from `docs/PLAN.md`, then stop. Task IDs are `<stage>.<task>`,
e.g. `1.5` is Stage 1, task 5.

Every turn re-reads the whole conversation, so context you load early gets paid for again on
every later turn. Keep the session lean: read only what the task needs, and do the work in
few, well-batched steps. This matters as much as getting it right the first time.

## 1. Pick the task

- If an ID was given (`$ARGUMENTS`), use that task.
- Otherwise use the first unchecked `[ ]` task in the current stage (named at the top of PLAN.md).
  Find it with `grep -n` rather than reading the whole file; read only the current stage's section.
- Check that earlier tasks it depends on are done. If one isn't, say which, and stop.
- If the current stage has no unchecked tasks, check its exit criteria. If they're met, say so and
  suggest updating the "Current stage" line; don't start the next stage yourself.

## 2. Understand it

- Read `docs/CODEMAP.md` first. It says where things live, the key signatures, and which test
  fixtures to reuse.
- Read only the SPEC sections the task touches, by heading (`awk '/^### 7.1/,/^### 7.2/' docs/SPEC.md`),
  not the whole SPEC.
- Look at the existing code the task interacts with, cheaply:
  - Signatures and structure: `go doc ./internal/<pkg>`, `go doc ./internal/<pkg> <Symbol>`, or `grep -n '^func\|^type' <file>`.
  - Bodies: read the specific functions or line ranges you need (`sed -n 120,180p`), not whole files.
  - Read a whole file only if you'll edit much of it. Don't open files you won't change or call.
  - Don't re-read a file you already have in context, or one you just edited.
- Restate, briefly:
  - **Task:** one sentence.
  - **Done when:** 2–5 concrete, checkable acceptance criteria.
  - **Out of scope:** anything nearby that belongs to a later task.
  - **Questions:** real ambiguities only. If none, say "none".

## 3. Plan, then stop

- Propose an implementation plan: files to create or change, key types and functions, and the tests you'll write.
- If the task is large (e.g. many endpoints or screens, or more than ~8 files), propose splitting it
  into smaller PLAN tasks and doing only the first now. Long sessions cost far more per step than short ones.
- **Stop and wait for the user's approval.** Do not write code yet. If the user changes the plan, update it and continue.

## 4. Build

- Implement the approved plan with tests alongside the code.
- Batch the work to keep turns few:
  - Write new files whole, in one step each.
  - Make related edits to a file together, not one line per turn.
  - Don't stop to compile or test after every small change. Finish a coherent piece, then check it.
- While iterating, run only the affected package, with trimmed output:
  `go test ./internal/<pkg>/ 2>&1 | tail -30`. Keep all command output short (`tail`, `grep`, `head`).
- Stay in scope. If you notice something worth doing that isn't part of this task, record it
  with the same format `/park` uses (in PLAN.md's "Parked" section) and move on.

## 5. Verify

- Run `make test` and `make lint` once the work is done, with output trimmed to failures
  (e.g. `make test 2>&1 | grep -v '^ok\|no test files' | tail -30; make lint 2>&1 | tail -15`). Fix failures.
- For UI tasks, tell the user exactly what to click through to confirm it works. Don't drive the
  browser preview tools unless the user asks; their snapshots are large.

## 6. Review

- Invoke the `reviewer` subagent with a brief, so it doesn't redo your orientation:
  ```
  Review PLAN <id>: <task title>
  SPEC sections: <e.g. §7.1, §10.5>
  Changed files: <list, noting which are new>
  make test / make lint: passed   (or: the failures, trimmed)
  Notes: <anything non-obvious: deliberate trade-offs, things parked>
  ```
- Fix every Blocking finding. Fix Should-fix findings unless there's a good reason not to; if you skip one, say why.
- If you made significant fixes, run the reviewer once more, naming only the files you changed since.

## 7. Wrap up

- Tick the task `[x]` in `docs/PLAN.md`.
- Update `docs/SPEC.md` or add an ADR in `docs/adr/` if a design decision changed.
- Update `docs/CODEMAP.md` if you added a package, a public entry point, or a test fixture, or moved one.
  Keep it a map: one line per thing, no detail that `go doc` already gives.
- Give the user a short summary: what was built, anything skipped or parked, and a suggested commit message:
  `<area>: <what changed> (PLAN <id>)`.
- Don't commit unless the user asks. Don't start the next task.
