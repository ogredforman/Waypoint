// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Project, Settings as SettingsBody } from '../lib/api/types.gen'
import Settings from './Settings.svelte'

function project(id: number, status: Project['status']): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '' }
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let settings: SettingsBody
let projects: Project[]
let patchReply: Response | null
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  if (url === '/api/projects') return json(projects)
  if (method === 'PATCH') {
    if (patchReply) return patchReply
    settings = { ...settings, ...JSON.parse(init!.body as string) }
  }
  return json(settings)
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

beforeEach(() => {
  fetchMock.mockClear()
  settings = { active_project_cap: 3, questions_per_entry: 2, interface_font: 'inter', diary_font: 'literata' }
  projects = [project(1, 'active'), project(2, 'active'), project(3, 'parked')]
  patchReply = null
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
  component = mount(Settings, { target })
  await settle()
}

function capInput(): HTMLInputElement {
  return target.querySelector<HTMLInputElement>('#setting-cap')!
}
function radio(value: number): HTMLInputElement {
  return target.querySelector<HTMLInputElement>(`input[name="questions"][value="${value}"]`)!
}
function saveButton(): HTMLButtonElement {
  return [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Save')!
}
async function setCap(v: string) {
  const input = capInput()
  input.value = v
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}
async function pick(v: number) {
  radio(v).click()
  await settle()
}
async function save() {
  saveButton().click()
  await settle()
}
function patches(): unknown[] {
  return fetchMock.mock.calls
    .filter(([, init]) => init?.method === 'PATCH')
    .map(([, init]) => JSON.parse(init!.body as string))
}

describe('Settings', () => {
  it('shows the stored settings, with nothing to save yet', async () => {
    await render()
    expect(capInput().value).toBe('3')
    expect(radio(2).checked).toBe(true)
    expect(saveButton().disabled).toBe(true)
  })

  it('saves only the field that changed', async () => {
    await render()
    await pick(1)
    expect(saveButton().disabled).toBe(false)
    await save()
    expect(patches()).toEqual([{ questions_per_entry: 1 }])
    expect(radio(1).checked).toBe(true)
    expect(target.textContent).toContain('Saved.')
    expect(saveButton().disabled).toBe(true)
  })

  it('explains calmly that a cap below the active projects pauses nothing', async () => {
    await render()
    expect(target.querySelector('[role="status"]')).toBeNull()
    await setCap('1')
    const note = target.querySelector('[role="status"]')?.textContent ?? ''
    expect(note).toContain('You have 2 active projects')
    expect(note).toContain('Nothing gets paused')
    await save()
    expect(patches()).toEqual([{ active_project_cap: 1 }])
    // The note stays after saving, since the stored cap is now below the count.
    expect(target.querySelector('[role="status"]')?.textContent).toContain('the next time you make a project active')
  })

  it("doesn't send a cap that isn't a whole number of at least 1", async () => {
    await render()
    await setCap('0')
    await save()
    expect(patches()).toEqual([])
    expect(target.textContent).toContain('Enter a whole number, 1 or more.')
    expect(capInput().getAttribute('aria-invalid')).toBe('true')
  })

  it("shows the server's problem next to its field", async () => {
    patchReply = json(
      {
        error: {
          code: 'invalid',
          message: 'Invalid settings.',
          problems: [{ field: 'active_project_cap', problem: 'must be at least 1, got 0' }],
        },
      },
      422,
    )
    await render()
    await setCap('5')
    await save()
    expect(document.getElementById('setting-cap-problem')?.textContent).toBe('Must be at least 1, got 0.')
    expect(target.textContent).not.toContain('Saved.')
  })

  it('offers to try again when the settings fail to load', async () => {
    fetchMock.mockImplementationOnce(async () => {
      throw new TypeError('offline')
    })
    await render()
    expect(target.textContent).toContain("Can't reach the Waypoint server.")
    const retry = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Try again')!
    retry.click()
    await settle()
    expect(capInput().value).toBe('3')
  })
})
