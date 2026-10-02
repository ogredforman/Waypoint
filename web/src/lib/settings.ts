import { ApiError } from './api/client'
import type { Settings, UpdateSettingsRequest } from './api/types.gen'

// SettingsForm is what the Settings screen edits. The cap is kept as the
// field's raw value so a half-typed number isn't lost.
export interface SettingsForm {
  cap: string | number
  questions: number
}

export function settingsForm(s: Settings): SettingsForm {
  return { cap: String(s.active_project_cap), questions: s.questions_per_entry }
}

// parseCap reads the cap field: a whole number, 1 or more, or null.
export function parseCap(value: string | number): number | null {
  const s = String(value).trim()
  if (!/^[0-9]+$/.test(s)) return null
  const n = Number(s)
  return Number.isSafeInteger(n) && n >= 1 ? n : null
}

// FieldProblems maps an API field name to what's wrong with it.
export type FieldProblems = Partial<Record<keyof Settings | 'form', string>>

export const capProblem = 'Enter a whole number, 1 or more.'

// settingsPatch is the PATCH body for the fields that changed, or null when
// nothing did. A cap that isn't a valid number is a problem, not a change.
export function settingsPatch(
  saved: Settings,
  form: SettingsForm,
): { patch: UpdateSettingsRequest | null; problems: FieldProblems } {
  const cap = parseCap(form.cap)
  if (cap === null) return { patch: null, problems: { active_project_cap: capProblem } }
  const patch: UpdateSettingsRequest = {}
  if (cap !== saved.active_project_cap) patch.active_project_cap = cap
  if (form.questions !== saved.questions_per_entry) patch.questions_per_entry = form.questions
  return { patch: Object.keys(patch).length > 0 ? patch : null, problems: {} }
}

// capNote explains, calmly, a cap below the number of active projects:
// nothing is paused, and the cap applies to the next activation. Null when
// the cap isn't below the count or the count is unknown.
export function capNote(activeCount: number | null, cap: number | null): string | null {
  if (activeCount === null || cap === null || activeCount <= cap) return null
  return `You have ${activeCount} active projects, more than a limit of ${cap}. Nothing gets paused: they all stay active, and the limit applies the next time you make a project active.`
}

// fieldProblems turns a failed save into messages per field. Problems with
// fields the form doesn't show, and other errors, go under "form".
export function fieldProblems(err: unknown): FieldProblems {
  if (!(err instanceof ApiError)) return { form: "Couldn't save your settings." }
  if (err.code !== 'invalid' || err.problems.length === 0) return { form: err.message }
  const out: FieldProblems = {}
  for (const p of err.problems) {
    const text = p.problem.charAt(0).toUpperCase() + p.problem.slice(1) + '.'
    if (p.field === 'active_project_cap' || p.field === 'questions_per_entry') out[p.field] = text
    else out.form = out.form ? `${out.form} ${text}` : text
  }
  return out
}

// questionChoices are the options for follow-up questions per entry, up to
// the most Waypoint ever asks (SPEC §5).
export const questionChoices: { value: number; label: string; hint: string }[] = [
  { value: 0, label: 'None', hint: "Waypoint doesn't ask follow-up questions." },
  { value: 1, label: 'One', hint: 'At most one question per entry.' },
  { value: 2, label: 'Two', hint: 'Up to two questions per entry, the most Waypoint asks.' },
]
