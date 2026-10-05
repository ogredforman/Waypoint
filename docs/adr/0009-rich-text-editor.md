# 0009. Rich text editor for notes

- **Status:** Accepted
- **Date:** 2026-10-04
- **PLAN task:** 1.5.17

## Context

Task notes, a project's why and definition of done, and an idea's body were plain text areas.
The owner's Stage 1 review asked for word-processor editing there: headings, bold and italic,
bulleted, numbered, and check lists, and links. The stored text is read by the extraction
snapshot and, from Stage 2, by a model, so it should stay plain, readable text, not HTML or an
editor's JSON. The app is local-first and runs offline; nothing may load from a CDN. The diary
input is deliberately a plain writing surface (SPEC §5).

## Options considered

1. **Markdown text area with a preview** (e.g. `marked` + a sanitiser). Small (~40 KB). But it
   isn't word-processor editing: the user writes the markup.
2. **Tiptap (ProseMirror) with `@tiptap/markdown`.** Real WYSIWYG editing, keyboard shortcuts,
   accessible check lists, and markdown in and out through the official extension. Framework-
   agnostic, so it works in Svelte 5 with a plain `new Editor({ element })`, no wrapper package.
   About 140 KB gzipped.
3. **Lexical, Milkdown, or a contenteditable of our own.** Lexical has no Svelte binding and a
   weaker markdown story; Milkdown is ProseMirror with more layers; our own contenteditable is
   a long tail of selection and paste bugs.

## Decision

Tiptap 3 (`@tiptap/core`, `@tiptap/pm`, `@tiptap/starter-kit`, `@tiptap/extension-list` for
check lists, `@tiptap/markdown`), stored as markdown in the existing text columns. One shared
extension list (`web/src/lib/richtext.ts`) drives both the editor (`components/ui/RichTextEditor`)
and the read-only view (`components/ui/RichText`, a non-editable editor). The diary stays plain text.

## Consequences

- No schema, API, or extractor change: the columns hold markdown, which the snapshot passes on as text.
- Text written before this reads as it did: single line breaks are kept (`marked` `breaks`), and
  opening an untouched field and saving sends nothing, because only edits write the value.
- Showing text goes through the editor's schema, so raw HTML in stored text never reaches the page.
- Markdown that has no button (blockquotes, code, which the starter kit parses) still shows and
  round-trips. Underline is off because markdown can't store it. Headings are levels 2 and 3 only.
- The bundle grows by about 140 KB gzipped, built as its own `richtext` chunk (`vite.config.ts`).
  It loads with the app; served from localhost that's cheap. Lazy-loading it is possible later.
- A rich text field is named with `aria-labelledby` (a `<label for>` can't name a contenteditable).
- Check lists in notes are free-form detail; steps the system tracks are subtasks.
