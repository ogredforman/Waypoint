import type { Energy, TaskSize, TaskStatus } from './api/enums.gen'
import type { CreateTaskRequest, LinkedObservation, MoveDirection, StepOf, Task, UpdateTaskRequest } from './api/types.gen'

export interface TaskGroup {
  status: TaskStatus
  label: string
  tasks: Task[]
}

const groupOrder: { status: TaskStatus; label: string }[] = [
  { status: 'in_progress', label: 'In progress' },
  { status: 'todo', label: 'To do' },
  { status: 'done', label: 'Done' },
  { status: 'dropped', label: 'Dropped' },
]

export const statusLabels: { status: TaskStatus; label: string }[] = groupOrder

function byPosition(a: Task, b: Task): number {
  return a.position - b.position || a.id - b.id
}

// groupTasks sorts a project's tasks into the Project screen's sections, in
// progress first, each ordered by position (ties by id, as the server does).
// Empty sections are left out.
export function groupTasks(tasks: Task[]): TaskGroup[] {
  return groupOrder
    .map(({ status, label }) => ({ status, label, tasks: tasks.filter((t) => t.status === status).sort(byPosition) }))
    .filter((g) => g.tasks.length > 0)
}

export interface TaskTree {
  groups: TaskGroup[]
  // steps holds each task's subtasks by parent id, ordered by position.
  steps: Map<number, Task[]>
}

// nestTasks is groupTasks for a list with subtasks: only top-level tasks are
// grouped by status, and each task's steps sit under it whatever their
// status. A task whose parent isn't in the list counts as top-level.
export function nestTasks(tasks: Task[]): TaskTree {
  const ids = new Set(tasks.map((t) => t.id))
  const steps = new Map<number, Task[]>()
  const top: Task[] = []
  for (const t of tasks) {
    if (t.parent_task_id !== null && ids.has(t.parent_task_id)) {
      steps.set(t.parent_task_id, [...(steps.get(t.parent_task_id) ?? []), t])
    } else top.push(t)
  }
  for (const list of steps.values()) list.sort(byPosition)
  return { groups: groupTasks(top), steps }
}

// sortSteps orders a task's steps as the server does.
export function sortSteps(steps: Task[]): Task[] {
  return [...steps].sort(byPosition)
}

// stepStatus is the status a step's checkbox sets: done when ticked, back to
// todo when not.
export function stepStatus(checked: boolean): TaskStatus {
  return checked ? 'done' : 'todo'
}

// newStepRequest builds the POST /api/projects/{id}/tasks body for a step of
// the parent task.
export function newStepRequest(title: string, parentTaskId: number): CreateTaskRequest {
  return { title: title.trim(), parent_task_id: parentTaskId }
}

// stepLabel is how a briefing names a recommended step's place: "Step 2 of",
// followed by the parent's title.
export function stepLabel(s: StepOf): string {
  return `Step ${s.number} of`
}

export type Direction = MoveDirection

export interface TaskForm {
  title: string
  notes: string
  size: TaskSize | ''
  energy: Energy | ''
  // milestone is a milestone id, or '' for none.
  milestone: number | ''
}

export function taskFields(t: Task): TaskForm {
  return { title: t.title, notes: t.notes, size: t.size ?? '', energy: t.energy ?? '', milestone: t.milestone_id ?? '' }
}

// taskPatch builds the PATCH /api/tasks/{id} body from the edit form: only
// the fields that changed, trimmed. An empty size, energy, or milestone
// clears it (null). It returns null when nothing changed.
export function taskPatch(t: Task, form: TaskForm): UpdateTaskRequest | null {
  const req: UpdateTaskRequest = {}
  const title = form.title.trim()
  const notes = form.notes.trim()
  const size = form.size === '' ? null : form.size
  const energy = form.energy === '' ? null : form.energy
  const milestone = form.milestone === '' ? null : form.milestone
  if (title !== t.title) req.title = title
  if (notes !== t.notes) req.notes = notes
  if (size !== t.size) req.size = size
  if (energy !== t.energy) req.energy = energy
  if (milestone !== t.milestone_id) req.milestone_id = milestone
  return Object.keys(req).length > 0 ? req : null
}

// newTaskRequest builds the POST /api/projects/{id}/tasks body. Unset size,
// energy, and milestone are left out; the task starts as todo.
export function newTaskRequest(form: Pick<TaskForm, 'title' | 'size' | 'energy'> & { milestone?: number | '' }): CreateTaskRequest {
  const req: CreateTaskRequest = { title: form.title.trim() }
  if (form.size) req.size = form.size
  if (form.energy) req.energy = form.energy
  if (form.milestone) req.milestone_id = form.milestone
  return req
}

// TimelineGroup is the observations one entry made about a task, under that entry's date.
export interface TimelineGroup {
  entryId: number
  at: string
  observations: LinkedObservation[]
}

// observationTimeline groups linked observations (a task's, or a project's) by the entry they
// came from, keeping the API's order. An entry's observations
// arrive together, so only consecutive ones are merged.
export function observationTimeline(observations: LinkedObservation[]): TimelineGroup[] {
  const groups: TimelineGroup[] = []
  for (const o of observations) {
    const last = groups[groups.length - 1]
    if (last && last.entryId === o.entry_id) last.observations.push(o)
    else groups.push({ entryId: o.entry_id, at: o.entry_created_at, observations: [o] })
  }
  return groups
}
