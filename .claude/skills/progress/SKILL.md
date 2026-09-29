---
name: progress
description: Summarize where the Waypoint build stands — current stage, progress, what's next, and recent work. Use when the user runs /progress or asks where things are, especially after time away.
disable-model-invocation: true
---

# Progress

Give a short, calm "where you left off" summary. It should make starting again easy, not list
everything that's behind. No guilt framing.

1. Read `docs/PLAN.md`. Find the current stage and count done vs. total tasks in it.
2. Run `git log --oneline -10` (if this is a git repo) to see recent work, and `git status --short`
   to see anything uncommitted.
3. Reply in this shape, under 15 lines:

```
Stage <n> — <name>: <done>/<total> tasks done
Last done: <most recent ticked task or commit, one line>
Uncommitted: <none | short description>
Next up: <next unchecked task id and title> — <one sentence on what it involves>
Parked: <count> ideas waiting
```

4. If there's uncommitted work, suggest finishing or committing it before starting something new.
5. End with: "Run `/next-task` to start <id>." Do not start it yourself.
