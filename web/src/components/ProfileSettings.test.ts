// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useProfile } from '../lib/api/client'
import type { Profile } from '../lib/api/types.gen'
import { chooseProfile, profiles } from '../lib/profile.svelte'
import ProfileSettings from './ProfileSettings.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let list: Profile[]
let calls: { method: string; url: string; body?: unknown; active?: string }[]
let reply: Response | null

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  const body = init?.body ? JSON.parse(init.body as string) : undefined
  calls.push({ method, url, body, active: profiles.active?.id })
  if (reply) return reply
  if (method === 'GET') return json(list)
  if (method === 'POST') return json({ id: body.name.toLowerCase(), name: body.name }, 201)
  if (method === 'PATCH') return json({ id: url.split('/').pop(), name: body.name })
  return new Response(null, { status: 204 })
})

beforeEach(async () => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  calls = []
  reply = null
  list = [
    { id: 'personal', name: 'Personal' },
    { id: 'work', name: 'Work' },
  ]
  vi.stubGlobal('fetch', fetchMock)
  await chooseProfile(undefined, 0, undefined, null)
  calls = []
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  useProfile(null)
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

async function render() {
  component = mount(ProfileSettings, { target })
  await settle()
}

function row(name: string): HTMLElement | null {
  return target.querySelector<HTMLElement>(`li[aria-label="${name}"]`)
}
function button(scope: HTMLElement | null, text: string): HTMLButtonElement | undefined {
  return [...(scope?.querySelectorAll('button') ?? [])].find((b) => b.textContent?.trim() === text)
}
function input(id: string): HTMLInputElement {
  return target.querySelector<HTMLInputElement>(`#${id}`)!
}
async function type(el: HTMLInputElement, v: string) {
  el.value = v
  el.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}
async function click(b: HTMLElement | undefined) {
  b!.click()
  await settle()
}
async function submit(form: HTMLFormElement | null) {
  form!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await settle()
}

describe('ProfileSettings', () => {
  it('lists the profiles and marks the one in use', async () => {
    await render()
    expect(row('Personal')!.textContent).toContain('In use')
    expect(button(row('Personal'), 'Switch to')).toBeUndefined()
    expect(button(row('Work'), 'Switch to')).toBeDefined()
  })

  it('adds a profile without switching to it', async () => {
    await render()
    await type(input('new-profile-name'), ' Portfolio ')
    await submit(input('new-profile-name').form)
    // A new profile starts with the appearance of the one in use.
    expect(calls).toEqual([
      { method: 'POST', url: '/api/profiles', body: { name: 'Portfolio', appearance_from: 'personal' }, active: 'personal' },
    ])
    expect(row('Portfolio')).not.toBeNull()
    expect(profiles.active?.id).toBe('personal')
    expect(target.textContent).toContain('Added Portfolio.')
  })

  it("shows the server's problem with a new name", async () => {
    await render()
    reply = json(
      {
        error: {
          code: 'invalid',
          message: 'Invalid.',
          problems: [{ field: 'name', problem: 'is already used by another profile' }],
        },
      },
      422,
    )
    await type(input('new-profile-name'), 'work')
    await submit(input('new-profile-name').form)
    expect(target.querySelector('#new-profile-problem')?.textContent).toBe('Is already used by another profile.')
  })

  it('renames a profile', async () => {
    await render()
    await click(button(row('Work'), 'Rename'))
    expect(input('profile-work-name').value).toBe('Work')
    await type(input('profile-work-name'), 'Portfolio')
    await submit(input('profile-work-name').form)
    expect(calls).toEqual([
      { method: 'PATCH', url: '/api/profiles/work', body: { name: 'Portfolio' }, active: 'personal' },
    ])
    expect(row('Portfolio')).not.toBeNull()
  })

  it('deletes only once the name is typed, saying everything in it goes', async () => {
    await render()
    await click(button(row('Work'), 'Delete…'))
    const warning = target.querySelector('#profile-work-warning')!.textContent!
    expect(warning).toContain('removes everything in it')
    expect(warning).toContain("can't be undone")
    const confirm = button(row('Work'), 'Delete Work')!
    expect(confirm.disabled).toBe(true)
    expect(confirm.className).toContain('bg-danger')
    await type(input('profile-work-confirm'), 'work')
    expect(confirm.disabled).toBe(true)
    await type(input('profile-work-confirm'), 'Work')
    expect(confirm.disabled).toBe(false)
    await submit(input('profile-work-confirm').form)
    expect(calls).toEqual([{ method: 'DELETE', url: '/api/profiles/work', body: undefined, active: 'personal' }])
    expect(row('Work')).toBeNull()
    expect(target.textContent).toContain('Deleted Work.')
  })

  it('deleting the profile in use switches to another first', async () => {
    await render()
    await click(button(row('Personal'), 'Delete…'))
    expect(target.querySelector('#profile-personal-warning')!.textContent).toContain('switch to Work first')
    await type(input('profile-personal-confirm'), 'Personal')
    await submit(input('profile-personal-confirm').form)
    expect(calls).toEqual([{ method: 'DELETE', url: '/api/profiles/personal', body: undefined, active: 'work' }])
    expect(profiles.active?.id).toBe('work')
    expect(row('Personal')).toBeNull()
  })

  it('offers no delete for the only profile', async () => {
    list = [{ id: 'personal', name: 'Personal' }]
    await chooseProfile(undefined, 0, undefined, null)
    await render()
    expect(button(row('Personal'), 'Delete…')).toBeUndefined()
    expect(target.textContent).toContain("This is your only profile, so it can't be deleted.")
  })
})
