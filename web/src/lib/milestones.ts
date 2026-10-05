import type { MilestoneStatus } from './api/enums.gen'
import type { Milestone, Task, UpdateMilestoneRequest } from './api/types.gen'
import type { TaskGroup, TaskTree } from './tasks'

// MilestoneSection is one part of the Project screen's task list: a
// milestone's top-level tasks grouped by status, or, with milestone null,
// the tasks in no milestone.
export interface MilestoneSection {
  milestone: Milestone | null
  groups: TaskGroup[]
}

export const milestoneStatusLabels: { status: MilestoneStatus; label: string }[] = [
  { status: 'open', label: 'Open' },
  { status: 'done', label: 'Done' },
  { status: 'dropped', label: 'Dropped' },
]

function byPosition(a: Milestone, b: Milestone): number {
  return a.position - b.position || a.id - b.id
}

// sortMilestones orders milestones as the server does.
export function sortMilestones(milestones: Milestone[]): Milestone[] {
  return [...milestones].sort(byPosition)
}

// milestoneSections splits a task tree's top-level groups by milestone: one
// section per milestone in order (empty ones too), then the tasks in no
// milestone (or in one that isn't listed) last, left out when there are
// none unless there are no milestones at all. Steps stay with their parent
// whatever their own milestone, through tree.steps.
export function milestoneSections(tree: TaskTree, milestones: Milestone[]): MilestoneSection[] {
  const known = new Set(milestones.map((m) => m.id))
  const pick = (keep: (t: Task) => boolean): TaskGroup[] =>
    tree.groups.map((g) => ({ ...g, tasks: g.tasks.filter(keep) })).filter((g) => g.tasks.length > 0)
  const sections: MilestoneSection[] = sortMilestones(milestones).map((m) => ({
    milestone: m,
    groups: pick((t) => t.milestone_id === m.id),
  }))
  const rest = pick((t) => t.milestone_id === null || !known.has(t.milestone_id))
  if (rest.length > 0 || sections.length === 0) sections.push({ milestone: null, groups: rest })
  return sections
}

// milestoneIsClosed says whether a milestone is done or dropped; its section
// starts folded.
export function milestoneIsClosed(m: Milestone): boolean {
  return m.status !== 'open'
}

// milestoneLabel names a milestone in a picker, with its status when it's closed.
export function milestoneLabel(m: Milestone): string {
  return milestoneIsClosed(m) ? `${m.title} (${m.status})` : m.title
}

export interface MilestoneForm {
  title: string
  description: string
}

// milestonePatch builds the PATCH /api/milestones/{id} body from the edit
// form: only the fields that changed, trimmed, or null when nothing did.
export function milestonePatch(m: Milestone, form: MilestoneForm): UpdateMilestoneRequest | null {
  const req: UpdateMilestoneRequest = {}
  const title = form.title.trim()
  const description = form.description.trim()
  if (title !== m.title) req.title = title
  if (description !== m.description) req.description = description
  return Object.keys(req).length > 0 ? req : null
}
