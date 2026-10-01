// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { STORAGE_KEY } from '../lib/diaryPanel'
import DiaryPanel from './DiaryPanel.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount>

beforeEach(() => {
  // The diary content loads the timeline when the panel opens; leave it loading.
  vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ width: 380, open: false }))
  target = document.createElement('div')
  document.body.append(target)
  component = mount(DiaryPanel, { target })
  flushSync()
})

afterEach(() => {
  unmount(component)
  target.remove()
  localStorage.clear()
  vi.unstubAllGlobals()
})

const openButton = () => target.querySelector<HTMLButtonElement>('button[aria-label="Open the diary"]')

describe('DiaryPanel collapsed', () => {
  it('shows an icon button with a tooltip, not sideways text', () => {
    const button = openButton()
    expect(button).not.toBeNull()
    expect(button!.getAttribute('aria-expanded')).toBe('false')
    expect(button!.querySelector('svg')).not.toBeNull()
    expect(button!.textContent?.trim()).toBe('Open the diary')
    expect(target.innerHTML).not.toContain('writing-mode')
  })

  it('opens the panel when clicked', () => {
    openButton()!.click()
    flushSync()
    expect(openButton()).toBeNull()
    expect(target.querySelector('#diary-panel-title')?.textContent).toBe('Diary')
  })
})
