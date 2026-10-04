// @vitest-environment jsdom
import { mount, unmount } from 'svelte'
import { afterEach, describe, expect, it } from 'vitest'
import type { Briefing, Project, Task } from '../lib/api/types.gen'
import BriefingCard from './BriefingCard.svelte'

const project: Project = {
  id: 3,
  title: 'Waypoint',
  why: '',
  definition_of_done: '',
  status: 'active',
  created_at: '',
  updated_at: '',
  last_activity_at: '',
}

const task = (id: number, title: string): Task =>
  ({ id, project_id: 3, title, status: 'todo', position: id }) as unknown as Task

const daysAgo = (n: number) => new Date(Date.now() - n * 24 * 60 * 60 * 1000).toISOString()

const briefing = (over: Partial<Briefing> = {}): Briefing => ({
  project_id: 3,
  next_task: task(1, 'Write the recap'),
  next_reason: "It's next on your list.",
  next_step: null,
  recap: [],
  source: 'heuristic',
  excerpt: null,
  blockers: [],
  alternatives: [],
  generated_at: '',
  ...over,
})

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined

function show(b: Briefing | null, error = '') {
  target = document.createElement('div')
  document.body.append(target)
  component = mount(BriefingCard, { target, props: { project, briefing: b, error } })
}

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
})

describe('BriefingCard', () => {
  it('shows the next action first, then the recap, then blockers and alternatives', () => {
    show(
      briefing({
        recap: [{ entry_id: 9, created_at: daysAgo(0), points: [{ kind: 'progress', text: 'Wired the hub' }] }],
        blockers: [{ id: 4, description: 'Waiting on a reply' } as Briefing['blockers'][number]],
        alternatives: [task(2, 'Tidy the README')],
      }),
    )
    const text = target.textContent ?? ''
    const order = ['Write the recap', 'Where you left off', 'Wired the hub', 'In the way', 'Tidy the README']
    const at = order.map((s) => text.indexOf(s))
    expect(at.every((i) => i >= 0)).toBe(true)
    expect([...at].sort((a, b) => a - b)).toEqual(at)
    expect(target.querySelector('a[href="/tasks/1"]')).not.toBeNull()
    expect(target.querySelector('a[href="/diary?entry=9"] time')).not.toBeNull()
  })

  it('names the parent when the next action is a step', () => {
    show(briefing({ next_step: { parent_task_id: 5, parent_title: 'Set up CI', number: 2 } }))
    expect(target.textContent).toContain('Step 2 of')
    expect(target.querySelector('a[href="/tasks/5"]')?.textContent).toBe('Set up CI')
  })

  it('lists recent entries newest first, each dated, with its points as separate items', () => {
    show(
      briefing({
        recap: [
          {
            entry_id: 9,
            created_at: daysAgo(0),
            points: [
              { kind: 'progress', text: 'Wired the hub' },
              { kind: 'plan', text: 'Write the tests' },
            ],
          },
          { entry_id: 8, created_at: daysAgo(1), points: [{ kind: 'feeling', text: 'Tired but fine' }] },
        ],
      }),
    )
    const entries = [...target.querySelectorAll('ol > li')]
    expect(entries).toHaveLength(2)
    expect(entries[0].querySelector('time')?.textContent?.trim()).toBe('today')
    expect(entries[1].querySelector('time')?.textContent?.trim()).toBe('yesterday')
    const points = [...entries[0].querySelectorAll('ul > li')]
    expect(points.map((p) => p.textContent?.trim())).toEqual(['Progress: Wired the hub', 'Plan: Write the tests'])
    expect(points[1].className).toContain('bg-plan')
    expect(points[0].className).toContain('bg-accent')
  })

  it('leaves out empty sections', () => {
    show(briefing())
    const text = target.textContent ?? ''
    expect(text).not.toContain('Where you left off')
    expect(text).not.toContain('In the way')
    expect(text).not.toContain('Low on energy')
  })

  it('says so calmly when nothing can be started', () => {
    show(briefing({ next_task: null }))
    expect(target.textContent).toContain('Nothing here is ready to start yet')
  })

  it('shows the error in place of the briefing', () => {
    show(null, "Couldn't load this briefing.")
    expect(target.textContent).toContain("Couldn't load this briefing.")
    expect(target.textContent).not.toContain('Loading')
  })
})
