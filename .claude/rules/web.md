---
paths:
  - "web/**"
---

# Frontend rules

- Svelte 5 with runes (`$state`, `$derived`, `$effect`, `$props`). No legacy `export let` or `$:` syntax.
- TypeScript strict mode. No `any` without a comment explaining why.
- All API calls go through the typed client in `web/src/lib/api/`. No raw `fetch` in components.
  API types come from the shared-types mechanism chosen in ADR 0001, never hand-duplicated.
- Exactly one SSE connection for the app, exposed as a store in `web/src/lib/events`.
  Components subscribe to it; they never open their own `EventSource`.
- Diary input: Cmd/Ctrl+Enter submits. Submitting clears the box immediately and shows a
  "processing" state on the new entry; it never waits on extraction.
- Product principles are UI requirements:
  - Show one next action prominently; alternatives are secondary and visually quieter.
  - No "overdue", streak counters, red failure badges, or shaming copy anywhere.
  - Empty states encourage starting small ("Write what you're thinking about").
  - Follow-up questions are always dismissible.
- Keep components small. Page components live in `web/src/screens`, reusable ones in `web/src/components` (shared UI primitives in `web/src/components/ui`).
- Styling is Tailwind utility classes. Use the shared color tokens; don't introduce one-off colors.
- Accessibility basics: every input has a label, every interactive element is keyboard reachable.
