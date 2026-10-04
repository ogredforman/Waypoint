// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Project } from '../lib/api/types.gen'
import Projects from './Projects.svelte'

function project(id: number, status: Project['status']): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '', last_activity_at: '' }
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let projects: Project[]
let failPatch = false
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  if (method === 'GET') return json(url === '/api/settings' ? { active_project_cap: 3 } : projects)
  if (failPatch) return json({ error: { code: 'internal', message: 'Nope.' } }, 500)
  const id = Number(url.split('/')[3])
  return json({ ...projects.find((p) => p.id === id)!, ...JSON.parse(init!.body as string) })
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

beforeEach(() => {
  fetchMock.mockClear()
  failPatch = false
  projects = [project(1, 'active'), project(2, 'archived'), project(3, 'parked')]
  vi.stubGlobal('fetch', fetchMock)
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  vi.unstubAllGlobals()
})

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

async function render() {
  component = mount(Projects, { target })
  await settle()
}

function headings(): string[] {
  return [...target.querySelectorAll('h2')].map((h) => h.textContent?.replace(/\s+/g, ' ').trim() ?? '')
}

function titles(): string[] {
  return [...target.querySelectorAll('li a span:first-child')].map((s) => s.textContent ?? '')
}

function toggle(label: string): HTMLButtonElement {
  return [...target.querySelectorAll<HTMLButtonElement>('h2 button')].find((b) => b.textContent?.includes(label))!
}

async function filter(text: string) {
  const box = target.querySelector<HTMLInputElement>('input[aria-label="Filter projects"]')!
  box.value = text
  box.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}

async function openArchived() {
  toggle('Archived').click()
  await settle()
}

function restoreButton(): HTMLButtonElement | undefined {
  return [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Restore')
}

describe('Projects screen', () => {
  it('shows archived projects last, collapsed, each with Restore once opened', async () => {
    await render()
    expect(headings()).toEqual(['Active', 'Parked', 'Archived (1)'])
    expect(toggle('Archived').getAttribute('aria-expanded')).toBe('false')
    expect(restoreButton()).toBeUndefined()
    await openArchived()
    expect(toggle('Archived').getAttribute('aria-expanded')).toBe('true')
    expect(target.querySelectorAll('button[aria-label^="Restore"]').length).toBe(1)
    expect(restoreButton()?.getAttribute('aria-label')).toBe('Restore P2')
  })

  it('restores an archived project to parked', async () => {
    await render()
    await openArchived()
    restoreButton()!.click()
    await settle()
    const patch = fetchMock.mock.calls.find(([, init]) => init?.method === 'PATCH')
    expect(patch?.[0]).toBe('/api/projects/2')
    expect(JSON.parse(patch?.[1]?.body as string)).toEqual({ status: 'parked' })
    expect(headings()).toEqual(['Active', 'Parked'])
    expect(restoreButton()).toBeUndefined()
  })

  it('keeps the project archived and says so when restoring fails', async () => {
    failPatch = true
    await render()
    await openArchived()
    restoreButton()!.click()
    await settle()
    expect(headings()).toEqual(['Active', 'Parked', 'Archived (1)'])
    expect(target.textContent).toContain("Couldn't restore P2.")
  })

  it('starts Done and Archived collapsed and the rest open', async () => {
    projects = [project(1, 'active'), project(2, 'done'), project(3, 'archived'), project(4, 'paused')]
    await render()
    expect(headings()).toEqual(['Active', 'Paused', 'Done (1)', 'Archived (1)'])
    expect(titles()).toEqual(['P1', 'P4'])
    toggle('Done').click()
    await settle()
    expect(titles()).toEqual(['P1', 'P4', 'P2'])
    toggle('Done').click()
    await settle()
    expect(titles()).toEqual(['P1', 'P4'])
  })

  it('lists the most recently active project first in each section', async () => {
    projects = [
      { ...project(1, 'active'), last_activity_at: '2026-10-01T09:00:00Z' },
      { ...project(2, 'active'), last_activity_at: '2026-10-03T09:00:00Z' },
      { ...project(3, 'active'), last_activity_at: '2026-10-02T09:00:00Z' },
    ]
    await render()
    expect(titles()).toEqual(['P2', 'P3', 'P1'])
  })

  it('filters by title, opening collapsed sections that match', async () => {
    projects = [
      { ...project(1, 'active'), title: 'Garden beds' },
      { ...project(2, 'parked'), title: 'Learn Go' },
      { ...project(3, 'archived'), title: 'Old garden shed' },
    ]
    await render()
    await filter('  GARDEN ')
    expect(headings()).toEqual(['Active', 'Archived (1)'])
    expect(titles()).toEqual(['Garden beds', 'Old garden shed'])
    await filter('violin')
    expect(titles()).toEqual([])
    expect(target.textContent).toContain('No projects match “violin”.')
    await filter('')
    expect(titles()).toEqual(['Garden beds', 'Learn Go'])
  })
})
