// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Project } from '../lib/api/types.gen'
import Projects from './Projects.svelte'

function project(id: number, status: Project['status']): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '' }
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
  return [...target.querySelectorAll('h2')].map((h) => h.textContent?.trim() ?? '')
}

function restoreButton(): HTMLButtonElement | undefined {
  return [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Restore')
}

describe('Projects screen', () => {
  it('shows archived projects last, each with Restore', async () => {
    await render()
    expect(headings()).toEqual(['Active', 'Parked', 'Archived'])
    expect(target.querySelectorAll('button[aria-label^="Restore"]').length).toBe(1)
    expect(restoreButton()?.getAttribute('aria-label')).toBe('Restore P2')
  })

  it('restores an archived project to parked', async () => {
    await render()
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
    restoreButton()!.click()
    await settle()
    expect(headings()).toEqual(['Active', 'Parked', 'Archived'])
    expect(target.textContent).toContain("Couldn't restore P2.")
  })
})
