import type { Energy, TaskSize, TaskStatus } from './api/enums.gen'
import type { CreateTaskRequest, Task, UpdateTaskRequest } from './api/types.gen'

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

export type Direction = 'up' | 'down'

// neighbour is the task next to t in its status group, or null at the end.
export function neighbour(tasks: Task[], t: Task, dir: Direction): Task | null {
  const group = tasks.filter((x) => x.status === t.status).sort(byPosition)
  const i = group.findIndex((x) => x.id === t.id)
  const j = dir === 'up' ? i - 1 : i + 1
  return i < 0 || j < 0 || j >= group.length ? null : group[j]
}

export interface PositionPatch {
  id: number
  position: number
}

// movePatches are the position changes that move t past its neighbour n:
// the two positions swap. Equal positions (ordered by id) can't swap, so t
// alone steps one past n instead.
export function movePatches(t: Task, n: Task, dir: Direction): PositionPatch[] {
  if (t.position === n.position) {
    return [{ id: t.id, position: dir === 'up' ? n.position - 1 : n.position + 1 }]
  }
  return [
    { id: t.id, position: n.position },
    { id: n.id, position: t.position },
  ]
}

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
