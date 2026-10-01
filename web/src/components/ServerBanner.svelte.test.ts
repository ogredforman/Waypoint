// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { describe, expect, it } from 'vitest'
import type { Health } from '../lib/health'
import ServerBanner from './ServerBanner.svelte'

describe('ServerBanner', () => {
  it('shows nothing while the server is fine, and a calm message when it is not', () => {
    const target = document.body.appendChild(document.createElement('div'))
    const props = $state<{ health: Health }>({ health: 'ok' })
    const c = mount(ServerBanner, { target, props })
    const region = target.querySelector('[role="status"]')!
    expect(region.getAttribute('aria-live')).toBe('polite')
    expect(region.textContent?.trim()).toBe('')

    props.health = 'unreachable'
    flushSync()
    expect(region.textContent).toContain("Can't reach the Waypoint server")
    expect(region.querySelector('.bg-caution')).not.toBeNull()

    props.health = 'unavailable'
    flushSync()
    expect(region.textContent).toContain("can't reach its database")

    props.health = 'ok'
    flushSync()
    expect(region.textContent?.trim()).toBe('')
    unmount(c)
    target.remove()
  })
})
