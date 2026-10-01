// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Blocker, Idea } from '../lib/api/types.gen'
import ProjectBlockers from './ProjectBlockers.svelte'
import ProjectIdeas from './ProjectIdeas.svelte'

function blocker(id: number, status: Blocker['status'], extra: Partial<Blocker> = {}): Blocker {
  return {
    id,
    project_id: 7,
    task_id: null,
    description: `B${id}`,
    status,
    origin_entry_id: null,
    resolved_entry_id: null,
    created_at: `2026-09-0${id}`,
    updated_at: `2026-09-0${id}`,
    ...extra,
  }
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let blockers: Blocker[]
let ideas: Idea[]
let failWrites = false
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  if (method === 'GET') {
    if (url === '/api/projects') return json([])
    return json(url.startsWith('/api/ideas') ? ideas : blockers)
  }
  if (failWrites) return json({ error: { code: 'internal', message: 'Something went wrong.' } }, 500)
  const id = Number(url.split('/').pop())
  if (method === 'DELETE') return new Response(null, { status: 204 })
  return json({ ...blockers.find((b) => b.id === id)!, status: 'resolved', updated_at: '2026-09-30' })
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

beforeEach(() => {
  fetchMock.mockClear()
  failWrites = false
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

async function render(bs: Blocker[]) {
  blockers = bs
  component = mount(ProjectBlockers, { target, props: { projectId: 7 } })
  await settle()
}

function buttons(text: string): HTMLButtonElement[] {
  return [...target.querySelectorAll('button')].filter((b) => b.textContent?.trim() === text)
}
function writes(): { url: string; method?: string; body: unknown }[] {
  return fetchMock.mock.calls
    .filter(([, init]) => init?.method && init.method !== 'GET')
    .map(([url, init]) => ({ url, method: init?.method, body: init?.body ? JSON.parse(init.body as string) : undefined }))
}
function descriptions(group: string): string[] {
  return [...target.querySelectorAll(`ul[aria-label="${group}"] > li p:first-child`)].map((p) => p.textContent ?? '')
}

describe('ProjectBlockers', () => {
  it('lists open blockers, then resolved ones without a resolve button', async () => {
    await render([blocker(1, 'resolved'), blocker(2, 'open', { task_id: 5 }), blocker(3, 'open')])
    expect(descriptions('Open')).toEqual(['B2', 'B3'])
    expect(descriptions('Resolved')).toEqual(['B1'])
    expect(buttons('Resolve')).toHaveLength(2)
    expect(target.querySelector('ul[aria-label="Open"] a')?.getAttribute('href')).toBe('/tasks/5')
  })

  it('shows a calm empty state', async () => {
    await render([])
    expect(target.textContent).toContain('Nothing in the way.')
  })

  it('resolves a blocker and moves it to Resolved', async () => {
    await render([blocker(1, 'open')])
    buttons('Resolve')[0].click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/blockers/1', method: 'PATCH', body: { status: 'resolved' } }])
    expect(descriptions('Open')).toEqual([])
    expect(descriptions('Resolved')).toEqual(['B1'])
  })

  it('says so when resolving fails, keeping the blocker open', async () => {
    await render([blocker(1, 'open')])
    failWrites = true
    buttons('Resolve')[0].click()
    await settle()
    expect(target.querySelector('[role="status"]')?.textContent).toBe('Something went wrong.')
    expect(descriptions('Open')).toEqual(['B1'])
  })

  it('deletes only after confirming', async () => {
    await render([blocker(1, 'open')])
    buttons('Delete')[0].click()
    flushSync()
    expect(writes()).toEqual([])
    buttons('Keep it')[0].click()
    flushSync()
    expect(target.textContent).not.toContain('Delete this blocker?')
    buttons('Delete')[0].click()
    flushSync()
    buttons('Delete blocker')[0].click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/blockers/1', method: 'DELETE', body: undefined }])
    expect(target.textContent).toContain('Nothing in the way.')
  })
})

describe('ProjectIdeas', () => {
  it("lists this project's parked ideas", async () => {
    ideas = [
      {
        id: 1,
        project_id: 7,
        title: 'Rain barrel',
        body: 'Under the gutter',
        status: 'parked',
        origin_entry_id: null,
        created_at: '',
        updated_at: '',
      },
    ]
    component = mount(ProjectIdeas, { target, props: { projectId: 7 } })
    await settle()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/ideas?project_id=7')
    expect(target.textContent).toContain('Rain barrel')
    expect(target.textContent).toContain('Under the gutter')
  })

  it('shows a calm empty state', async () => {
    ideas = []
    component = mount(ProjectIdeas, { target, props: { projectId: 7 } })
    await settle()
    expect(target.textContent).toContain('No parked ideas for this project.')
  })
})
