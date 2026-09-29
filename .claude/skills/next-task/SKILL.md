---
name: next-task
description: Pick up and complete the next task from docs/PLAN.md (or a specific one, e.g. /next-task 1.5) using the project's plan, build, verify, review loop.
argument-hint: "[stage.task, e.g. 1.5]"
disable-model-invocation: true
---

# Next task

Complete exactly one task from `docs/PLAN.md`, then stop. Task IDs are `<stage>.<task>`,
e.g. `1.5` is Stage 1, task 5.

## 1. Pick the task

- If an ID was given (`$ARGUMENTS`), use that task.
- Otherwise use the first unchecked `[ ]` task in the current stage (named at the top of PLAN.md).
- Check that earlier tasks it depends on are done. If one isn't, say which, and stop.
- If the current stage has no unchecked tasks, check its exit criteria. If they're met, say so and
  suggest updating the "Current stage" line; don't start the next stage yourself.

## 2. Understand it

- Read the task and the SPEC sections it touches (not the whole SPEC).
- Look at the existing code it will interact with.
- Restate, briefly:
  - **Task:** one sentence.
  - **Done when:** 2–5 concrete, checkable acceptance criteria.
  - **Out of scope:** anything nearby that belongs to a later task.
  - **Questions:** real ambiguities only. If none, say "none".

## 3. Plan, then stop

- Propose an implementation plan: files to create or change, key types and functions, and the tests you'll write.
- **Stop and wait for the user's approval.** Do not write code yet. If the user changes the plan, update it and continue.

## 4. Build

- Implement the approved plan with tests alongside the code.
- Stay in scope. If you notice something worth doing that isn't part of this task, record it
  with the same format `/park` uses (in PLAN.md's "Parked" section) and move on.

## 5. Verify

- Run `make test` and `make lint` (once they exist). Fix failures.
- For UI tasks, tell the user exactly what to click through to confirm it works, since you can't see it.

## 6. Review

- Invoke the `reviewer` subagent with: "Review PLAN <id>: <task title>".
- Fix every Blocking finding. Fix Should-fix findings unless there's a good reason not to; if you skip one, say why.
- If you made significant fixes, run the reviewer once more.

## 7. Wrap up

- Tick the task `[x]` in `docs/PLAN.md`.
- Update `docs/SPEC.md` or add an ADR in `docs/adr/` if a design decision changed.
- Give the user a short summary: what was built, anything skipped or parked, and a suggested commit message:
  `<area>: <what changed> (PLAN <id>)`.
- Don't commit unless the user asks. Don't start the next task.
