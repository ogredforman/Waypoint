// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Each test loads a fresh navigation module, as if the app had just started at /.
async function load() {
  vi.resetModules()
  return import('./navigation.svelte')
}

// heading puts an <h1> on the page, the way a screen renders one.
function heading(): HTMLElement {
  const h = document.createElement('h1')
  document.body.append(h)
  return h
}

describe('pageHeading', () => {
  beforeEach(() => {
    history.replaceState(null, '', '/')
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  })
  afterEach(() => {
    document.body.replaceChildren()
    vi.restoreAllMocks()
  })

  it('makes the heading focusable from script only', async () => {
    const { pageHeading } = await load()
    const h = heading()
    pageHeading(h)
    expect(h.tabIndex).toBe(-1)
  })

  it('leaves focus alone on the first load', async () => {
    const { pageHeading } = await load()
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).not.toBe(h)
  })

  it('focuses the new screen heading after navigate', async () => {
    const { navigate, pageHeading } = await load()
    navigate('/projects')
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).toBe(h)
  })

  it('leaves focus alone when only the query string changes', async () => {
    const { navigate, pageHeading } = await load()
    navigate('/?tab=recap')
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).not.toBe(h)
  })

  it('focuses the heading after back or forward to another page', async () => {
    const { pageHeading, startRouting } = await load()
    const stop = startRouting()
    history.pushState(null, '', '/diary')
    window.dispatchEvent(new PopStateEvent('popstate'))
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).toBe(h)
    stop()
  })

  it('moves focus from the clicked link to the heading', async () => {
    const { navigate, pageHeading } = await load()
    const link = document.createElement('a')
    link.href = '/projects'
    document.body.append(link)
    link.focus()
    navigate('/projects')
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).toBe(h)
  })

  it("doesn't take focus from where the user moved it while the screen loaded", async () => {
    const { navigate, pageHeading } = await load()
    navigate('/projects/1')
    const input = document.createElement('textarea')
    document.body.append(input)
    input.focus()
    const h = heading()
    pageHeading(h)
    expect(document.activeElement).toBe(input)
  })

  it('focuses only the first heading after a page change', async () => {
    const { navigate, pageHeading } = await load()
    navigate('/projects/1')
    pageHeading(heading())
    ;(document.activeElement as HTMLElement).blur()
    const again = heading()
    pageHeading(again)
    expect(document.activeElement).not.toBe(again)
  })
})
