# 0004. The diary as a panel beside every screen

- **Status:** Accepted
- **Date:** 2026-09-30
- **PLAN task:** 1.16b (app layout and base style)

## Context

SPEC §11 described the diary as its own screen, with the diary input placed under the briefing on
Now. Writing an entry is the main way into the app, so it should be one keystroke away from wherever
the user is: on a Project or Task screen, the thought is usually about what's on screen. Keeping the
diary and a task side by side also makes observation highlights more useful, since clicking one can
open the task in the main pane without losing your place in the timeline.

Only the app shell and the Projects screen existed (1.16), so the layout could still change cheaply,
before the Now screen (1.17) and the diary timeline and review UI (1.18) were built.

## Options considered

1. **Diary as a screen, input on Now (the original SPEC).** Simple routing, lots of room for review.
   But writing from anywhere else means navigating away from what you're looking at.
2. **Diary as a panel only.** Always there, but reviewing a changeset with several ops and edits can
   be cramped in a sidebar, and there's no page to link to.
3. **A panel beside every screen, plus a full-page `/diary`.** The panel is the everyday way in;
   the page gives room for long reviews and links.

## Decision

Option 3. The diary (input, timeline, pending review, follow-up questions) is a collapsible panel on
the right, beside every screen except `/diary`, which shows the same content full-page. The panel is
resized by dragging (or with the arrow keys), within a minimum and maximum width, and its width and
open state are kept in `localStorage`. On narrow windows, below Tailwind's `lg` breakpoint, there is
no sidebar: a floating button in the bottom-right corner opens the diary as a full-height overlay.
The Now screen shows only the briefing.

## Consequences

- Diary components (input, timeline, review) must work at panel width and at page width.
- The diary's scope picker can default to the project on screen, if any.
- The panel's width and open state are browser UI preferences, not server settings; they aren't in
  `config.toml`, the DB, or the export.
- The floating button must not count or badge pending items in a way that nags (SPEC §5).
