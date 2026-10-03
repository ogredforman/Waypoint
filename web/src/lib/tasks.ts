import type { Energy, TaskSize, TaskStatus } from './api/enums.gen'
import type { CreateTaskRequest, LinkedObservation, MoveDirection, Task, UpdateTaskRequest } from './api/types.gen'

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

export type Direction = MoveDirection

export interface TaskForm {
  title: string
  notes: string
  size: TaskSize | ''
  energy: Energy | ''
}

export function taskFields(t: Task): TaskForm {
  return { title: t.title, notes: t.notes, size: t.size ?? '', energy: t.energy ?? '' }
}

// taskPatch builds the PATCH /api/tasks/{id} body from the edit form: only
// the fields that changed, trimmed. An empty size or energy clears it
// (null). It returns null when nothing changed.
export function taskPatch(t: Task, form: TaskForm): UpdateTaskRequest | null {
  const req: UpdateTaskRequest = {}
  const title = form.title.trim()
  const notes = form.notes.trim()
  const size = form.size === '' ? null : form.size
  const energy = form.energy === '' ? null : form.energy
  if (title !== t.title) req.title = title
  if (notes !== t.notes) req.notes = notes
  if (size !== t.size) req.size = size
  if (energy !== t.energy) req.energy = energy
  return Object.keys(req).length > 0 ? req : null
}

// newTaskRequest builds the POST /api/projects/{id}/tasks body. Unset size
// and energy are left out; the task starts as todo.
export function newTaskRequest(form: Pick<TaskForm, 'title' | 'size' | 'energy'>): CreateTaskRequest {
  const req: CreateTaskRequest = { title: form.title.trim() }
  if (form.size) req.size = form.size
  if (form.energy) req.energy = form.energy
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
