@AGENTS.md

## Claude Code

- Use plan mode for any task that touches the schema, `internal/changeset`, or the extraction pipeline.
- Workflows (devflow plugin): `/devflow:next-task` (do the next PLAN task), `/devflow:park <idea>`
  (capture an idea without doing it), `/devflow:progress` (where things stand).
- The `devflow:reviewer` subagent checks diffs against the hard rules above. Use it before finishing any task.
- Hooks in `.claude/settings.json` auto-format Go, protect committed migrations, and run
  `go build` + `go vet` when you stop. If a hook blocks you, fix the cause; don't work around it.
