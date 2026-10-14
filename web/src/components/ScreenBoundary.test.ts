// @vitest-environment jsdom
import { createRawSnippet, flushSync, mount, unmount } from 'svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ScreenBoundary from './ScreenBoundary.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined

afterEach(() => {
  if (component) unmount(component)
  target.remove()
  vi.restoreAllMocks()
})

// screen is a children snippet that throws while broken is true.
function screen(state: { broken: boolean }) {
  return createRawSnippet(() => ({
    render: () => {
      if (state.broken) throw new Error('boom')
      return '<p>the screen</p>'
    },
  }))
}

describe('ScreenBoundary', () => {
  it('shows a calm message for a screen that fails, and tries again on request', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const state = { broken: true }
    target = document.createElement('div')
    document.body.append(target)
    component = mount(ScreenBoundary, { target, props: { screen: 'now', children: screen(state) } })
    flushSync()
    expect(target.textContent).toContain('This screen ran into a problem')
    expect(target.textContent).not.toContain('boom')

    state.broken = false
    const retry = [...target.querySelectorAll('button')].find((b) => b.textContent?.includes('Try again'))
    retry?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    flushSync()
    expect(target.textContent).toContain('the screen')
  })
})
