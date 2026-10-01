import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { startEvents } from './events'
import { FakeEventSource, fakeOpen } from './events.fake'
import { latest, liveReload } from './live'

let stopEvents: () => void

beforeEach(() => {
  FakeEventSource.all = []
  stopEvents = startEvents(fakeOpen)
})

afterEach(() => stopEvents())

const applied = (...project_ids: number[]) => FakeEventSource.latest().emit('changeset.applied', { changeset_id: 1, project_ids })

describe('liveReload', () => {
  it('reloads on every applied changeset without a project', () => {
    const reload = vi.fn()
    const off = liveReload(reload)
    applied()
    applied(4)
    expect(reload).toHaveBeenCalledTimes(2)
    off()
  })

  it('reloads only for changesets that touched the project', () => {
    const reload = vi.fn()
    const off = liveReload(reload, () => 2)
    applied(1)
    expect(reload).not.toHaveBeenCalled()
    applied(1, 2)
    expect(reload).toHaveBeenCalledTimes(1)
    off()
  })

  it('reloads on any changeset while the project is unknown', () => {
    const reload = vi.fn()
    const off = liveReload(reload, () => undefined)
    applied(9)
    expect(reload).toHaveBeenCalledTimes(1)
    off()
  })

  it('reloads on reconnect, but not on the first open', () => {
    const reload = vi.fn()
    const off = liveReload(reload, () => 2)
    const src = FakeEventSource.latest()
    src.open()
    expect(reload).not.toHaveBeenCalled()
    src.fail()
    src.open()
    expect(reload).toHaveBeenCalledTimes(1)
    off()
  })

  it('stops when unsubscribed', () => {
    const reload = vi.fn()
    liveReload(reload)()
    const src = FakeEventSource.latest()
    applied(1)
    src.open()
    src.open()
    expect(reload).not.toHaveBeenCalled()
  })
})

describe('latest', () => {
  it('marks only the newest request current', () => {
    const loads = latest()
    const first = loads()
    expect(first()).toBe(true)
    const second = loads()
    expect(first()).toBe(false)
    expect(second()).toBe(true)
  })
})
