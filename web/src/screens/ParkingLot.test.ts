// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Idea, Project } from '../lib/api/types.gen'
import ParkingLot from './ParkingLot.svelte'

function project(id: number, status: Project['status'], extra: Partial<Project> = {}): Project {
  return {
    id,
    title: `P${id}`,
    why: '',
    definition_of_done: '',
    status,
    created_at: '',
    updated_at: '',
    ...extra,
  }
}
function idea(id: number, projectId: number | null, extra: Partial<Idea> = {}): Idea {
  return {
    id,
    project_id: projectId,
    title: `I${id}`,
    body: '',
    status: 'parked',
    origin_entry_id: null,
    created_at: '',
    updated_at: '',
    ...extra,
  }
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let ideas: Idea[]
let projects: Project[]
let cap: number
let capOnActivate = false
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  if (method === 'GET') {
    if (url === '/api/settings') return json({ active_project_cap: cap })
    return json(url.startsWith('/api/ideas') ? ideas : projects)
  }
  const id = Number(url.split('/')[3])
  if (method === 'DELETE') return new Response(null, { status: 204 })
  if (method === 'PATCH') {
    if (capOnActivate) return json({ error: { code: 'active_cap', message: 'At the cap.' } }, 409)
    return json({ ...projects.find((p) => p.id === id)!, status: 'active' })
  }
  const req = JSON.parse(init!.body as string)
  const promoted = { ...ideas.find((i) => i.id === id)!, status: 'promoted' }
  if (req.to === 'project')
    return json({
      idea: promoted,
      task: null,
      project: project(50, 'parked', { title: promoted.title }),
    })
  return json({ idea: promoted, task: { id: 60 }, project: null })
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

beforeEach(() => {
  fetchMock.mockClear()
  capOnActivate = false
  cap = 3
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
  component = mount(ParkingLot, { target })
  await settle()
}

function button(text: string, within: ParentNode = target): HTMLButtonElement {
  const b = [...within.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
  if (!b) throw new Error(`no button ${text}`)
  return b
}
async function click(b: HTMLElement) {
  b.click()
  await settle()
}
function writes(): { url: string; method?: string; body: unknown }[] {
  return fetchMock.mock.calls
    .filter(([, init]) => init?.method && init.method !== 'GET')
    .map(([url, init]) => ({
      url,
      method: init?.method,
      body: init?.body ? JSON.parse(init.body as string) : undefined,
    }))
}
function listed(label: string): string[] {
  return [...target.querySelectorAll(`ul[aria-label="${label}"] > li`)].map(
    (li) => li.querySelector('p, a')?.textContent?.trim() ?? '',
  )
}

describe('ParkingLot', () => {
  it('lists parked ideas with their project and the parked projects', async () => {
    ideas = [idea(1, 2), idea(2, null)]
    projects = [project(1, 'active'), project(2, 'parked', { why: 'Someday' }), project(3, 'paused')]
    await render()
    expect(listed('Parked ideas')).toEqual(['I1', 'I2'])
    const rows = target.querySelectorAll('ul[aria-label="Parked ideas"] > li')
    expect(rows[0].textContent).toContain('P2')
    expect(rows[1].textContent).toContain('No project')
    expect(listed('Parked projects')).toEqual(['P2'])
    expect(target.textContent).toContain('Someday')
  })

  it('shows calm empty states', async () => {
    ideas = []
    projects = []
    await render()
    expect(target.textContent).toContain('No parked ideas.')
    expect(target.textContent).toContain('No parked projects.')
  })

  it("promotes an idea to a task in the chosen project, defaulting to the idea's own", async () => {
    ideas = [idea(1, 2)]
    projects = [project(1, 'active'), project(2, 'parked'), project(4, 'done')]
    await render()
    await click(button('Promote'))
    const select = target.querySelector('select')!
    expect([...select.options].map((o) => o.textContent)).toEqual(['P1', 'P2'])
    expect(select.value).toBe('2')
    select.value = '1'
    select.dispatchEvent(new Event('change', { bubbles: true }))
    await click(button('Promote idea'))
    expect(writes()).toEqual([
      {
        url: '/api/ideas/1/promote',
        method: 'POST',
        body: { to: 'task', project_id: 1 },
      },
    ])
    expect(listed('Parked ideas')).toEqual([])
  })

  it('promotes an idea to a new parked project, which joins the parked list', async () => {
    ideas = [idea(1, null)]
    projects = [project(1, 'active')]
    await render()
    await click(button('Promote'))
    const toProject = target.querySelector<HTMLInputElement>('input[value="project"]')!
    toProject.click()
    await settle()
    await click(button('Promote idea'))
    expect(writes()).toEqual([{ url: '/api/ideas/1/promote', method: 'POST', body: { to: 'project' } }])
    expect(listed('Parked ideas')).toEqual([])
    expect(listed('Parked projects')).toEqual(['I1'])
  })

  it('asks before deleting an idea', async () => {
    ideas = [idea(1, null)]
    projects = []
    await render()
    await click(button('Delete'))
    expect(target.textContent).toContain('Delete this idea?')
    await click(button('Keep it'))
    expect(writes()).toEqual([])
    await click(button('Delete'))
    await click(button('Delete idea'))
    expect(writes()).toEqual([{ url: '/api/ideas/1', method: 'DELETE', body: undefined }])
    expect(target.textContent).toContain('No parked ideas.')
  })

  it('activates a parked project', async () => {
    ideas = []
    projects = [project(2, 'parked')]
    await render()
    await click(button('Activate'))
    expect(writes()).toEqual([{ url: '/api/projects/2', method: 'PATCH', body: { status: 'active' } }])
    expect(target.textContent).toContain('No parked projects.')
  })

  it('disables Activate at the cap and explains calmly', async () => {
    ideas = []
    cap = 1
    projects = [project(1, 'active'), project(2, 'parked')]
    await render()
    expect(button('Activate').disabled).toBe(true)
    expect(target.textContent).toContain('the limit is 1')
  })

  it('keeps the project parked when the server reports the cap', async () => {
    ideas = []
    capOnActivate = true
    projects = [project(2, 'parked')]
    await render()
    await click(button('Activate'))
    expect(listed('Parked projects')).toEqual(['P2'])
    expect(target.textContent).toContain('stays parked for now')
    expect(button('Activate').disabled).toBe(true)
  })
})
