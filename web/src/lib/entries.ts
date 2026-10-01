import type { ObservationKind } from './api/enums.gen'
import type { Changeset, Entry, EntryDetail, Observation, Question } from './api/types.gen'
import { paths } from './router'

// Segment is a run of an entry's body: plain text, or the part an observation points at.
export interface Segment {
  text: string
  observation: Observation | null
}

// segments splits an entry body into plain text and observation highlights, in
// order. Spans are UTF-16 offsets (SPEC §10.5), so they slice the body
// directly. Spans outside the body or empty are skipped; where two overlap,
// the later one starts where the earlier one ends.
export function segments(body: string, observations: Observation[]): Segment[] {
  const spans = observations
    .filter((o) => o.span_start >= 0 && o.span_end <= body.length && o.span_start < o.span_end)
    .sort((a, b) => a.span_start - b.span_start || b.span_end - a.span_end)
  const out: Segment[] = []
  let at = 0
  for (const o of spans) {
    const start = Math.max(o.span_start, at)
    if (start >= o.span_end) continue
    if (start > at) out.push({ text: body.slice(at, start), observation: null })
    out.push({ text: body.slice(start, o.span_end), observation: o })
    at = o.span_end
  }
  if (at < body.length) out.push({ text: body.slice(at), observation: null })
  return out
}

// observationHref is the page a highlight opens: its task, else its project
// (which is also where a blocker lives), else the parking lot for an idea.
// null when it links to nothing that has a page.
export function observationHref(o: Observation): string | null {
  const link = (t: string) => o.links.find((l) => l.target_type === t)
  const task = link('task')
  if (task) return paths.task(task.target_id)
  const project = link('project')
  if (project) return paths.project(project.target_id)
  if (link('idea')) return paths.parkingLot
  return null
}

const kindLabels: Record<ObservationKind, string> = {
  progress: 'Progress',
  blocker: 'Blocker',
  idea: 'Idea',
  feeling: 'Feeling',
  decision: 'Decision',
  question: 'Question',
  note: 'Note',
}

// observationLabel describes a highlight for its tooltip.
export function observationLabel(o: Observation): string {
  return `${kindLabels[o.kind]}: ${o.summary}`
}

// shownQuestions are the entry's follow-up questions the timeline shows: open
// ones waiting for an answer, and answered ones, shown quietly so the answer
// visibly landed. Dismissed questions are set aside and not shown.
export function shownQuestions(detail: EntryDetail): Question[] {
  return detail.questions.filter((q) => q.status !== 'dismissed')
}

// pendingChangesets are the entry's changesets still waiting for review,
// with at least one op to decide.
export function pendingChangesets(detail: EntryDetail): Changeset[] {
  return detail.changesets.filter((c) => c.status === 'pending' && c.ops.some((op) => op.status === 'pending'))
}

// RunState is how far Waypoint has got with reading an entry.
export type RunState = 'reading' | 'failed' | 'done'

export function runState(detail: EntryDetail): RunState {
  switch (detail.run?.status) {
    case 'pending':
    case 'running':
      return 'reading'
    case 'failed':
      return 'failed'
    default:
      return 'done'
  }
}

// repliedQuestion finds the question an entry answers among the loaded
// details, or null if it isn't a reply or the question isn't loaded.
export function repliedQuestion(entry: Entry, details: (EntryDetail | null)[]): Question | null {
  if (entry.reply_to_question_id === null) return null
  for (const d of details) {
    const q = d?.questions.find((q) => q.id === entry.reply_to_question_id)
    if (q) return q
  }
  return null
}
