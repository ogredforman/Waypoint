import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { on, onDisconnect, onReconnect, retryDelay, startEvents } from './events'
import { FakeEventSource, fakeOpen } from './events.fake'

let stop: () => void = () => {}
const offs: (() => void)[] = []

beforeEach(() => {
  vi.useFakeTimers()
  FakeEventSource.all = []
  stop = startEvents(fakeOpen)
})

afterEach(() => {
  stop()
  offs.splice(0).forEach((off) => off())
  vi.useRealTimers()
})

describe('events', () => {
  it('opens one stream on /api/events', () => {
    expect(FakeEventSource.all.map((s) => s.url)).toEqual(['/api/events'])
  })

  it('passes each event its decoded data, until unsubscribed', () => {
    const got: number[] = []
    const off = on('briefing.updated', (e) => got.push(e.project_id))
    const src = FakeEventSource.latest()
    src.emit('briefing.updated', { project_id: 3 })
    src.emit('entry.processed', { entry_id: 9, run_id: 1 })
    src.emit('briefing.updated', 'not json')
    off()
    src.emit('briefing.updated', { project_id: 4 })
    expect(got).toEqual([3])
  })

  it('calls reconnect handlers only when the stream reopens', () => {
    let reconnects = 0
    offs.push(onReconnect(() => reconnects++))
    const src = FakeEventSource.latest()
    src.open()
    expect(reconnects).toBe(0)
    src.fail()
    src.open()
    expect(reconnects).toBe(1)
  })

  it('calls disconnect handlers once each time an open stream drops', () => {
    let drops = 0
    offs.push(onDisconnect(() => drops++))
    FakeEventSource.latest().fail()
    expect(drops).toBe(0) // never opened: nothing was lost
    FakeEventSource.latest().open()
    FakeEventSource.latest().fail()
    FakeEventSource.latest().fail(true)
    expect(drops).toBe(1)
    vi.advanceTimersByTime(retryDelay)
    FakeEventSource.latest().fail(true)
    expect(drops).toBe(1)
    FakeEventSource.latest().open()
    FakeEventSource.latest().fail()
    expect(drops).toBe(2)
  })

  it('reconnects after the server refuses the stream', () => {
    let reconnects = 0
    offs.push(onReconnect(() => reconnects++))
    FakeEventSource.latest().open()
    FakeEventSource.latest().fail(true)
    vi.advanceTimersByTime(retryDelay)
    expect(FakeEventSource.all).toHaveLength(2)
    FakeEventSource.latest().open()
    expect(reconnects).toBe(1)
  })

  it('closes the stream and stops retrying when stopped', () => {
    const src = FakeEventSource.latest()
    src.fail(true)
    stop()
    vi.advanceTimersByTime(retryDelay)
    expect(src.closed).toBe(true)
    expect(FakeEventSource.all).toHaveLength(1)
  })
})
