import { describe as group, expect, it } from 'vitest'
import type { OpType } from './api/enums.gen'
import type { ChangeOp, Changeset, Entry } from './api/types.gen'
import {
  accept,
  acceptRest,
  applyRequest,
  describe,
  editedPayload,
  editFields,
  guessedProject,
  noProject,
  reject,
  rejectedUse,
  undecide,
  uses,
  type Choices,
} from './review'

function op(position: number, op_type: OpType, payload: ChangeOp['payload'], refs: Record<string, string> = {}): ChangeOp {
  return { id: position * 10, position, op_type, payload, rationale: null, confidence: null, status: 'pending', applied_payload: null, refs }
}

// A done: line with no matching task (create it, mark it done), a blocker on it, an idea, and a resolve.
const ops = [
  op(1, 'create_task', { project_id: 7, title: 'Write docs' }, { 'project:7': 'Waypoint' }),
  op(2, 'set_task_status', { task_id: '$new:1', status: 'done' }),
  op(3, 'create_blocker', { project_id: 7, task_id: '$new:1', description: 'No wifi' }, { 'project:7': 'Waypoint' }),
  op(4, 'park_idea', { project_id: 7, title: 'Dark mode' }, { 'project:7': 'Waypoint' }),
  op(5, 'resolve_blocker', { blocker_id: 3 }, { 'blocker:3': 'Waiting on review' }),
]

const entry = (project_id: number | null): Entry => ({ id: 1, body: '', project_id, reply_to_question_id: null, created_at: '' })
const cs: Changeset = { id: 1, source: 'extraction', entry_id: 1, parent_changeset_id: null, status: 'pending', ops, created_at: '', resolved_at: null }

group('uses', () => {
  it('finds temporary IDs anywhere in a payload', () => {
    expect(uses({ task_id: '$new:2', nested: [{ x: '$new:4' }], title: '$new:x' })).toEqual([2, 4])
  })
})

group('reject and accept', () => {
  it('rejecting an op rejects everything that uses it', () => {
    expect(reject(ops, {}, 10)).toEqual({ 10: { action: 'reject' }, 20: { action: 'reject' }, 30: { action: 'reject' } })
  })

  it('accepting an op accepts the undecided ops it uses', () => {
    expect(accept(ops, {}, 30)).toEqual({ 30: { action: 'accept' }, 10: { action: 'accept' } })
  })

  it('accepting keeps an edit on what it uses', () => {
    const edit = { action: 'edit' as const, payload: { project_id: 7, title: 'Write the docs' } }
    expect(accept(ops, { 10: edit }, 20)).toEqual({ 10: edit, 20: { action: 'accept' } })
  })

  it('reports a rejected op that blocks accepting', () => {
    const choices = undecide(reject(ops, {}, 10), 20)
    expect(rejectedUse(ops, choices, 20)?.id).toBe(10)
    expect(rejectedUse(ops, choices, 40)).toBeNull()
  })

  it('accepts the rest, keeping decisions', () => {
    const choices = acceptRest(ops, { 50: { action: 'reject' } })
    expect(Object.values(choices).filter((c) => c.action === 'accept')).toHaveLength(4)
    expect(choices[50]).toEqual({ action: 'reject' })
  })
})

group('project picker', () => {
  it('guesses only for an entry with no project', () => {
    expect(guessedProject(entry(null), cs)).toBe(7)
    expect(guessedProject(entry(7), cs)).toBeNull()
    expect(guessedProject(entry(null), { ...cs, ops: [ops[4]] })).toBeNull()
  })

  it('none rejects tasks and blockers and what uses them, not ideas', () => {
    expect(Object.keys(noProject(ops, {}, 7)).map(Number).sort()).toEqual([10, 20, 30])
  })

  it('another project rewrites project_id on decided ops', () => {
    const choices: Choices = {
      10: { action: 'accept' },
      20: { action: 'accept' },
      40: { action: 'edit', payload: { project_id: 7, title: 'Light mode' } },
      50: { action: 'reject' },
    }
    expect(applyRequest(ops, choices, { from: 7, to: 9 })).toEqual([
      { id: 10, decision: 'edit', payload: { project_id: 9, title: 'Write docs' } },
      { id: 20, decision: 'accept' },
      { id: 40, decision: 'edit', payload: { project_id: 9, title: 'Light mode' } },
      { id: 50, decision: 'reject' },
    ])
  })

  it('moving a blocker off an existing task leaves it project-wide, but keeps a new task', () => {
    const blockers: ChangeOp[] = [
      { ...ops[0], id: 60, op_type: 'create_blocker', payload: { project_id: 7, task_id: 3, description: 'no key' } },
      { ...ops[0], id: 61, op_type: 'create_blocker', payload: { project_id: 7, task_id: '$new:1', description: 'no van' } },
    ]
    expect(applyRequest(blockers, acceptRest(blockers, {}), { from: 7, to: 9 })).toEqual([
      { id: 60, decision: 'edit', payload: { project_id: 9, description: 'no key' } },
      { id: 61, decision: 'edit', payload: { project_id: 9, task_id: '$new:1', description: 'no van' } },
    ])
  })

  it('none removes the project from ideas', () => {
    expect(applyRequest(ops, acceptRest(ops, {}), { from: 7, to: null })).toEqual([
      { id: 10, decision: 'reject' },
      { id: 20, decision: 'reject' },
      { id: 30, decision: 'reject' },
      { id: 40, decision: 'edit', payload: { title: 'Dark mode' } },
      { id: 50, decision: 'accept' },
    ])
  })

  it('keeping the guess sends plain decisions and leaves undecided ops out', () => {
    expect(applyRequest(ops, { 10: { action: 'accept' } }, { from: 7, to: 7 })).toEqual([{ id: 10, decision: 'accept' }])
  })
})

group('describe', () => {
  it('names existing rows from refs and new ones by their title', () => {
    expect(ops.map((o) => describe(o, ops, {}))).toEqual([
      'New task: Write docs',
      'Mark “Write docs” done',
      'Blocker: No wifi, on “Write docs”',
      'Park idea: Dark mode',
      'Resolve blocker “Waiting on review”',
    ])
  })

  it('uses the edited payload', () => {
    expect(describe(ops[1], ops, { 10: { action: 'edit', payload: { project_id: 7, title: 'Docs' } } })).toBe('Mark “Docs” done')
  })

  it('lists what an update changes', () => {
    const u = op(1, 'update_task', { task_id: 4, title: 'x', notes: 'y' }, { 'task:4': 'Old' })
    expect(describe(u, [u], {})).toBe('Change task “Old” (title, notes)')
  })

  it('says when deleting a project takes its entries too', () => {
    const keep = op(1, 'delete_project', { project_id: 7 }, { 'project:7': 'Garden' })
    const drop = op(1, 'delete_project', { project_id: 7, delete_entries: true }, { 'project:7': 'Garden' })
    expect(describe(keep, [keep], {})).toBe('Delete project “Garden”')
    expect(describe(drop, [drop], {})).toBe('Delete project “Garden” and its diary entries')
  })

  it('describes milestone ops', () => {
    const create = op(1, 'create_milestone', { project_id: 7, title: 'MVP' })
    const update = op(2, 'update_milestone', { milestone_id: 3, status: 'done' }, { 'milestone:3': 'Beta' })
    const del = op(3, 'delete_milestone', { milestone_id: 3 }, { 'milestone:3': 'Beta' })
    expect(describe(create, [create], {})).toBe('New milestone: MVP')
    expect(describe(update, [update], {})).toBe('Change milestone “Beta” (status)')
    expect(describe(del, [del], {})).toBe('Delete milestone “Beta”')
  })
})

group('editing', () => {
  it('offers the text fields of a create op, and only the changed ones of an update', () => {
    expect(editFields(ops[0]).map((f) => f.key)).toEqual(['title', 'notes'])
    expect(editFields(op(1, 'update_idea', { idea_id: 1, body: 'b' })).map((f) => f.key)).toEqual(['body'])
    expect(editFields(ops[1])).toEqual([])
  })

  it('applies edits, dropping an emptied optional field', () => {
    expect(editedPayload(ops[0], { ...ops[0].payload, notes: 'n' }, { title: ' Write docs ', notes: '' })).toEqual({
      project_id: 7,
      title: 'Write docs',
    })
  })
})
