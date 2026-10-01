import type { OpType, TaskStatus } from './api/enums.gen'
import type { ChangeOp, Changeset, Entry, OpDecision } from './api/types.gen'

// Reviewing a pending changeset (SPEC §7.1 step 7): the user accepts, rejects,
// or edits each op, then applies what they decided. Ops left undecided split
// off into a new pending changeset on the server (ADR 0003).

export type Payload = { [key: string]: unknown }

// Choice is the user's call on one op; an op with no Choice is undecided.
export type Choice = { action: 'accept' } | { action: 'reject' } | { action: 'edit'; payload: Payload }

// Choices are keyed by op ID.
export type Choices = Record<number, Choice>

const tempRef = /^\$new:(\d+)$/

// uses lists the positions of the ops whose new rows this op refers to
// ("$new:N", SPEC §6.2), wherever they sit in its payload.
export function uses(payload: unknown): number[] {
  const out: number[] = []
  const walk = (v: unknown) => {
    if (typeof v === 'string') {
      const m = tempRef.exec(v)
      if (m) out.push(Number(m[1]))
    } else if (Array.isArray(v)) {
      v.forEach(walk)
    } else if (v && typeof v === 'object') {
      Object.values(v).forEach(walk)
    }
  }
  walk(payload)
  return out
}

// payloadOf is what an op would apply: its edited payload, else the proposed one.
export function payloadOf(op: ChangeOp, choices: Choices): Payload {
  const c = choices[op.id]
  return c?.action === 'edit' ? c.payload : op.payload
}

// reject rejects the op and, since nothing can use what a rejected op creates,
// every op that uses it, directly or through others.
export function reject(ops: ChangeOp[], choices: Choices, id: number): Choices {
  const out = { ...choices }
  const queue = [id]
  while (queue.length > 0) {
    const next = queue.shift()
    const cur = ops.find((o) => o.id === next)
    if (!cur || out[cur.id]?.action === 'reject') continue
    out[cur.id] = { action: 'reject' }
    for (const o of ops) {
      if (uses(payloadOf(o, out)).includes(cur.position)) queue.push(o.id)
    }
  }
  return out
}

// rejectedUse is the first rejected op this op uses, directly or through
// others, so it can't be accepted; null if there is none.
export function rejectedUse(ops: ChangeOp[], choices: Choices, id: number): ChangeOp | null {
  const seen = new Set<number>()
  const queue = [id]
  while (queue.length > 0) {
    const next = queue.shift()
    const cur = ops.find((o) => o.id === next)
    if (!cur || seen.has(cur.id)) continue
    seen.add(cur.id)
    for (const pos of uses(payloadOf(cur, choices))) {
      const dep = ops.find((o) => o.position === pos)
      if (!dep) continue
      if (choices[dep.id]?.action === 'reject') return dep
      queue.push(dep.id)
    }
  }
  return null
}

// accept accepts the op (keeping an edit, if it has one) and the undecided ops
// it uses, since an op can't be applied before what it uses exists. Check
// rejectedUse first: an op that uses a rejected op can't be accepted.
export function accept(ops: ChangeOp[], choices: Choices, id: number): Choices {
  const out = { ...choices }
  const queue = [id]
  while (queue.length > 0) {
    const next = queue.shift()
    const cur = ops.find((o) => o.id === next)
    if (!cur) continue
    if (cur.id === id ? out[id]?.action !== 'edit' : !out[cur.id]) out[cur.id] = { action: 'accept' }
    for (const pos of uses(payloadOf(cur, out))) {
      const dep = ops.find((o) => o.position === pos)
      if (dep && !out[dep.id]) queue.push(dep.id)
    }
  }
  return out
}

// undecide takes back the call on one op. Ops rejected along with it stay rejected.
export function undecide(choices: Choices, id: number): Choices {
  const out = { ...choices }
  delete out[id]
  return out
}

// acceptRest accepts every op not yet decided.
export function acceptRest(ops: ChangeOp[], choices: Choices): Choices {
  const out = { ...choices }
  for (const o of ops) out[o.id] ??= { action: 'accept' }
  return out
}

// Ops that put something new into a project, which the project picker moves.
const projectOps: OpType[] = ['create_task', 'create_blocker', 'park_idea']

function inProject(op: ChangeOp, projectID: number): boolean {
  return projectOps.includes(op.op_type) && op.payload.project_id === projectID
}

// guessedProject is the project an extraction put an unscoped entry's new
// tasks, blockers, and ideas into, or null if the entry has a project or
// the changeset puts nothing into one.
export function guessedProject(entry: Entry, cs: Changeset): number | null {
  if (entry.project_id !== null) return null
  for (const op of cs.ops) {
    if (projectOps.includes(op.op_type) && typeof op.payload.project_id === 'number') return op.payload.project_id
  }
  return null
}

// needsProject says whether the op creates a task or blocker in the guessed
// project, which the picker's "none" leaves nowhere to go.
export function needsProject(op: ChangeOp, guessed: number): boolean {
  return op.op_type !== 'park_idea' && inProject(op, guessed)
}

// noProject is the picker's "none": tasks and blockers need a project, so the
// ones in the guessed project are rejected (with what uses them). Ideas stay,
// with no project, when applied.
export function noProject(ops: ChangeOp[], choices: Choices, guessed: number): Choices {
  let out = choices
  for (const op of ops) {
    if (needsProject(op, guessed)) out = reject(ops, out, op.id)
  }
  return out
}

// ProjectMove is the picker's choice: move what the extraction put into
// `from` into `to`, or into no project when `to` is null.
export interface ProjectMove {
  from: number
  to: number | null
}

// applyRequest turns the decided ops into the body of POST
// /api/changesets/{id}/apply. Undecided ops are left out. A project move
// rewrites project_id on the decided ops that put something into the guessed
// project, which turns an accept into an edit.
export function applyRequest(ops: ChangeOp[], choices: Choices, move: ProjectMove | null): OpDecision[] {
  const moving = move !== null && move.from !== move.to
  const decided = moving && move.to === null ? noProject(ops, choices, move.from) : choices
  const out: OpDecision[] = []
  for (const op of ops) {
    const c = decided[op.id]
    if (!c) continue
    if (c.action === 'reject') {
      out.push({ id: op.id, decision: 'reject' })
      continue
    }
    let payload = c.action === 'edit' ? c.payload : null
    if (moving && inProject(op, move.from)) {
      payload = { ...(payload ?? op.payload) }
      if (move.to === null) delete payload.project_id
      else payload.project_id = move.to
    }
    out.push(payload ? { id: op.id, decision: 'edit', payload } : { id: op.id, decision: 'accept' })
  }
  return out
}

const statusLabels: Record<TaskStatus, string> = {
  todo: 'to do',
  in_progress: 'in progress',
  done: 'done',
  dropped: 'dropped',
}

const quote = (s: string) => `“${s}”`

// refName names a row an op refers to: an existing one from the op's refs, or
// one created earlier in the changeset ("$new:N") by the title it will have.
function refName(op: ChangeOp, ops: ChangeOp[], choices: Choices, kind: string, value: unknown): string {
  if (typeof value === 'number') {
    const name = op.refs[`${kind}:${value}`]
    return name === undefined ? `a ${kind} that's gone` : quote(name)
  }
  const m = typeof value === 'string' ? tempRef.exec(value) : null
  const created = m && ops.find((o) => o.position === Number(m[1]))
  if (created) {
    const p = payloadOf(created, choices)
    const name = p.title ?? p.description
    if (typeof name === 'string') return quote(name)
  }
  return `the new ${kind}`
}

const fieldNames: Record<string, string> = {
  title: 'title',
  notes: 'notes',
  size: 'size',
  energy: 'energy',
  parent_task_id: 'parent task',
  position: 'order',
  description: 'description',
  task_id: 'task',
  body: 'text',
  project_id: 'project',
  status: 'status',
  why: 'why',
  definition_of_done: 'definition of done',
}

function changed(p: Payload, idField: string): string {
  const names = Object.keys(p)
    .filter((k) => k !== idField)
    .map((k) => fieldNames[k] ?? k)
  return names.length > 0 ? ` (${names.join(', ')})` : ''
}

// describe says what an op will do, in one plain sentence.
export function describe(op: ChangeOp, ops: ChangeOp[], choices: Choices): string {
  const p = payloadOf(op, choices)
  const name = (kind: string, v: unknown) => refName(op, ops, choices, kind, v)
  const text = (v: unknown) => (typeof v === 'string' ? v : '')
  switch (op.op_type) {
    case 'create_task':
      return `New task: ${text(p.title)}${p.parent_task_id != null ? `, under ${name('task', p.parent_task_id)}` : ''}`
    case 'update_task':
      return `Change task ${name('task', p.task_id)}${changed(p, 'task_id')}`
    case 'set_task_status':
      return `Mark ${name('task', p.task_id)} ${statusLabels[p.status as TaskStatus] ?? text(p.status)}`
    case 'split_task': {
      const n = Array.isArray(p.subtasks) ? p.subtasks.length : 0
      return `Split ${name('task', p.task_id)} into ${n === 1 ? '1 step' : `${n} steps`}`
    }
    case 'add_dependency':
      return `${name('task', p.task_id)} waits on ${name('task', p.blocked_by_task_id)}`
    case 'delete_task':
      return `Delete task ${name('task', p.task_id)}`
    case 'create_blocker':
      return `Blocker: ${text(p.description)}${p.task_id != null ? `, on ${name('task', p.task_id)}` : ''}`
    case 'update_blocker':
      return `Change blocker ${name('blocker', p.blocker_id)}${changed(p, 'blocker_id')}`
    case 'resolve_blocker':
      return `Resolve blocker ${name('blocker', p.blocker_id)}`
    case 'delete_blocker':
      return `Delete blocker ${name('blocker', p.blocker_id)}`
    case 'park_idea':
      return `Park idea: ${text(p.title)}`
    case 'update_idea':
      return `Change idea ${name('idea', p.idea_id)}${changed(p, 'idea_id')}`
    case 'delete_idea':
      return `Delete idea ${name('idea', p.idea_id)}`
    case 'create_project':
      return `New project: ${text(p.title)}`
    case 'update_project':
      return `Change project ${name('project', p.project_id)}${changed(p, 'project_id')}`
  }
}

// EditField is one text field the review lets the user change.
export interface EditField {
  key: string
  label: string
  long: boolean
  required: boolean
}

const textFields: Partial<Record<OpType, string[]>> = {
  create_task: ['title', 'notes'],
  update_task: ['title', 'notes'],
  create_blocker: ['description'],
  update_blocker: ['description'],
  park_idea: ['title', 'body'],
  update_idea: ['title', 'body'],
  create_project: ['title', 'why'],
  update_project: ['title', 'why', 'definition_of_done'],
}

const fieldLabels: Record<string, string> = {
  title: 'Title',
  notes: 'Notes',
  description: 'Description',
  body: 'Details',
  why: 'Why',
  definition_of_done: 'Definition of done',
}

// editFields are the text fields of an op the user can edit: all of them for
// an op that creates something, the ones it changes for an update.
export function editFields(op: ChangeOp): EditField[] {
  const keys = textFields[op.op_type] ?? []
  const update = op.op_type.startsWith('update_')
  return keys
    .filter((k) => !update || typeof op.payload[k] === 'string')
    .map((k) => ({
      key: k,
      label: fieldLabels[k],
      long: k !== 'title',
      required: k === 'title' || k === 'description',
    }))
}

// editedPayload applies the edited text fields to the payload; an emptied
// optional field on a create op is left out.
export function editedPayload(op: ChangeOp, base: Payload, values: Record<string, string>): Payload {
  const out = { ...base }
  for (const f of editFields(op)) {
    const v = values[f.key]?.trim() ?? ''
    if (v === '' && !f.required && !op.op_type.startsWith('update_')) delete out[f.key]
    else out[f.key] = v
  }
  return out
}
