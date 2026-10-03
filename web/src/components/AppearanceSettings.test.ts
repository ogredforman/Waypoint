// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Settings } from '../lib/api/types.gen'
import { fonts } from '../lib/appearance'
import AppearanceSettings from './AppearanceSettings.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let stored: Settings
let saved: Settings[]
// Each PATCH waits on gate (open by default), then fails if failNext is set.
let gate: Promise<void>
let failNext: boolean
const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
  await gate
  if (failNext) {
    failNext = false
    return new Response(JSON.stringify({ error: { code: 'internal', message: 'boom' } }), { status: 500 })
  }
  if (init?.method === 'PATCH') stored = { ...stored, ...JSON.parse(init.body as string) }
  return new Response(JSON.stringify(stored), { status: 200 })
})

beforeEach(() => {
  fetchMock.mockClear()
  stored = {
    active_project_cap: 3,
    questions_per_entry: 2,
    theme: 'midnight',
    interface_font: 'inter',
    diary_font: 'literata',
  }
  saved = []
  gate = Promise.resolve()
  failNext = false
  vi.stubGlobal('fetch', fetchMock)
  document.documentElement.removeAttribute('style')
  document.documentElement.removeAttribute('data-theme')
  localStorage.clear()
  target = document.createElement('div')
  document.body.append(target)
  component = mount(AppearanceSettings, { target, props: { settings: stored, onsaved: (s: Settings) => saved.push(s) } })
  flushSync()
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

function choose(group: string, value: string) {
  const radio = target.querySelector<HTMLInputElement>(`input[name="${group}"][value="${value}"]`)!
  radio.click()
  flushSync()
}

function preview(id: string): HTMLElement {
  return target.querySelector<HTMLElement>(`[data-testid="${id}"]`)!
}

function checked(group: string): string | undefined {
  return target.querySelector<HTMLInputElement>(`input[name="${group}"]:checked`)?.value
}

function patches(): unknown[] {
  return fetchMock.mock.calls.map(([, init]) => JSON.parse(init!.body as string))
}

describe('AppearanceSettings', () => {
  it('shows each font option in its own font, with the saved ones chosen, and no Save button', () => {
    const label = target.querySelector('input[name="diary-font"][value="manrope"]')!.nextElementSibling as HTMLElement
    expect(label.style.fontFamily).toContain('Manrope Variable')
    expect(checked('interface-font')).toBe('inter')
    expect(checked('diary-font')).toBe('literata')
    expect(target.querySelector('button')).toBeNull()
  })

  it('shows a swatch per theme, each in its own colours, with the saved one chosen', () => {
    const swatches = [...target.querySelectorAll<HTMLElement>('[data-testid="theme-swatch"]')]
    expect(swatches.map((s) => s.dataset.theme)).toEqual(['midnight', 'forest', 'paper', 'sage', 'dawn'])
    expect(checked('theme')).toBe('midnight')
  })

  it('applies a chosen theme to the app at once, saves it, and remembers it for the next load', async () => {
    choose('theme', 'sage')
    expect(document.documentElement.dataset.theme).toBe('sage')
    expect(preview('interface-preview').dataset.theme).toBe('sage')
    await settle()
    expect(patches()).toEqual([{ theme: 'sage' }])
    expect(localStorage.getItem('waypoint.theme')).toBe('sage')
    expect(saved.map((s) => s.theme)).toEqual(['sage'])
    expect(target.textContent).toContain('Saved.')
  })

  it('applies and saves only the changed font', async () => {
    choose('diary-font', 'atkinson-hyperlegible-next')
    expect(document.documentElement.style.getPropertyValue('--font-diary')).toBe(fonts['atkinson-hyperlegible-next'].stack)
    expect(preview('diary-preview').style.fontFamily).toContain('Atkinson Hyperlegible Next Variable')
    await settle()
    const [url] = fetchMock.mock.calls[0]
    expect(url).toMatch(/\/settings$/)
    expect(patches()).toEqual([{ diary_font: 'atkinson-hyperlegible-next' }])
    expect(document.documentElement.style.getPropertyValue('--font-sans')).toBe(fonts.inter.stack)
    expect(saved.map((s) => s.diary_font)).toEqual(['atkinson-hyperlegible-next'])
  })

  it('puts the previous choice back when a save fails, and says so in one line', async () => {
    failNext = true
    choose('theme', 'dawn')
    expect(document.documentElement.dataset.theme).toBe('dawn')
    await settle()
    expect(checked('theme')).toBe('midnight')
    expect(document.documentElement.dataset.theme).toBe('midnight')
    expect(localStorage.getItem('waypoint.theme')).toBe('midnight')
    expect(saved).toEqual([])
    expect(target.textContent).toContain("Couldn't save that, so the previous choice is back.")
  })

  it('saves one change at a time, sending what was chosen meanwhile once the first save answers', async () => {
    let open!: () => void
    gate = new Promise((r) => (open = r))
    choose('theme', 'paper')
    choose('theme', 'forest')
    choose('interface-font', 'geist')
    await settle()
    expect(patches()).toEqual([{ theme: 'paper' }])
    expect(document.documentElement.dataset.theme).toBe('forest')
    open()
    await settle()
    expect(patches()).toEqual([{ theme: 'paper' }, { theme: 'forest', interface_font: 'geist' }])
    expect(stored.theme).toBe('forest')
    expect(stored.interface_font).toBe('geist')
    expect(checked('theme')).toBe('forest')
  })

  it('leaves the page alone when a save fails after Settings has closed', async () => {
    let open!: () => void
    gate = new Promise((r) => (open = r))
    failNext = true
    choose('theme', 'dawn')
    unmount(component!)
    component = undefined
    document.documentElement.dataset.theme = 'forest'
    open()
    await settle()
    expect(document.documentElement.dataset.theme).toBe('forest')
  })
})
