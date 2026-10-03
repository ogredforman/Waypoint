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
const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
  if (init?.method === 'PATCH') stored = { ...stored, ...JSON.parse(init.body as string) }
  return new Response(JSON.stringify(stored), { status: 200 })
})

beforeEach(() => {
  fetchMock.mockClear()
  stored = { active_project_cap: 3, questions_per_entry: 2, interface_font: 'inter', diary_font: 'literata' }
  saved = []
  vi.stubGlobal('fetch', fetchMock)
  document.documentElement.removeAttribute('style')
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

function saveButton(): HTMLButtonElement {
  return [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Save')!
}

describe('AppearanceSettings', () => {
  it('shows each font option in its own font, with the saved ones chosen', () => {
    const label = target.querySelector('input[name="diary-font"][value="manrope"]')!.nextElementSibling as HTMLElement
    expect(label.style.fontFamily).toContain('Manrope Variable')
    expect(target.querySelector<HTMLInputElement>('input[name="interface-font"][value="inter"]')!.checked).toBe(true)
    expect(target.querySelector<HTMLInputElement>('input[name="diary-font"][value="literata"]')!.checked).toBe(true)
    expect(saveButton().disabled).toBe(true)
  })

  it('previews a choice before it is saved, without applying it to the app', () => {
    choose('interface-font', 'geist')
    choose('diary-font', 'ibm-plex-sans')
    expect(preview('interface-preview').style.fontFamily).toContain('Geist Variable')
    expect(preview('diary-preview').style.fontFamily).toContain('IBM Plex Sans Variable')
    expect(document.documentElement.style.getPropertyValue('--font-sans')).toBe('')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('saves only the changed font and applies it to the app', async () => {
    choose('diary-font', 'atkinson-hyperlegible-next')
    saveButton().click()
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toMatch(/\/settings$/)
    expect(JSON.parse(init!.body as string)).toEqual({ diary_font: 'atkinson-hyperlegible-next' })
    expect(document.documentElement.style.getPropertyValue('--font-diary')).toBe(fonts['atkinson-hyperlegible-next'].stack)
    expect(document.documentElement.style.getPropertyValue('--font-sans')).toBe(fonts.inter.stack)
    expect(saved.map((s) => s.diary_font)).toEqual(['atkinson-hyperlegible-next'])
    expect(target.textContent).toContain('Saved.')
  })
})
