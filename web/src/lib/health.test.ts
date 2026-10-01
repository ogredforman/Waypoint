import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchHealth, healthRetryDelay, watchHealth, type Health } from './health'

function respond(status: number, body: string): typeof fetch {
  return async () => new Response(body, { status, headers: { 'Content-Type': 'application/json' } })
}

describe('fetchHealth', () => {
  it('reports ok for a healthy server', async () => {
    expect(await fetchHealth(respond(200, '{"status":"ok"}'))).toBe('ok')
  })

  it('reports unavailable when the server cannot reach its database', async () => {
    expect(await fetchHealth(respond(503, '{"status":"unavailable"}'))).toBe('unavailable')
  })

  it('reports unreachable when the request fails', async () => {
    const failing: typeof fetch = async () => {
      throw new TypeError('network down')
    }
    expect(await fetchHealth(failing)).toBe('unreachable')
  })

  it('reports unreachable for a response that is not the health shape', async () => {
    // e.g. the Vite proxy's 502 page when the Go server isn't running.
    expect(await fetchHealth(respond(502, 'Bad Gateway'))).toBe('unreachable')
    expect(await fetchHealth(respond(200, '{"status":"weird"}'))).toBe('unreachable')
  })

  it('asks for /api/health', async () => {
    let url = ''
    const spy: typeof fetch = async (input) => {
      url = String(input)
      return new Response('{"status":"ok"}')
    }
    await fetchHealth(spy)
    expect(url).toBe('/api/health')
  })
})

describe('watchHealth', () => {
  let answers: Health[]
  let calls: number
  let seen: Health[]
  // Answers each request with the next queued answer, then keeps repeating the last.
  const fake: typeof fetch = async () => {
    calls++
    const h = answers.length > 1 ? answers.shift()! : answers[0]
    if (h === 'unreachable') throw new TypeError('network down')
    return new Response(JSON.stringify({ status: h }), { status: h === 'ok' ? 200 : 503 })
  }
  const settle = () => vi.advanceTimersByTimeAsync(0)

  beforeEach(() => {
    vi.useFakeTimers()
    calls = 0
    seen = []
  })
  afterEach(() => vi.useRealTimers())

  it('asks once and does not poll while the server is fine', async () => {
    answers = ['ok']
    const w = watchHealth((h) => seen.push(h), fake)
    await settle()
    await vi.advanceTimersByTimeAsync(healthRetryDelay * 5)
    expect(seen).toEqual(['ok'])
    expect(calls).toBe(1)
    w.stop()
  })

  it('keeps asking while the server is unreachable, and stops once it is back', async () => {
    answers = ['unreachable', 'unavailable', 'ok']
    const w = watchHealth((h) => seen.push(h), fake)
    await settle()
    expect(seen).toEqual(['unreachable'])
    await vi.advanceTimersByTimeAsync(healthRetryDelay)
    expect(seen).toEqual(['unreachable', 'unavailable'])
    await vi.advanceTimersByTimeAsync(healthRetryDelay)
    expect(seen).toEqual(['unreachable', 'unavailable', 'ok'])
    await vi.advanceTimersByTimeAsync(healthRetryDelay * 5)
    expect(calls).toBe(3)
    w.stop()
  })

  it('asks again on check, e.g. when the event stream drops', async () => {
    answers = ['ok', 'unreachable']
    const w = watchHealth((h) => seen.push(h), fake)
    await settle()
    w.check()
    await settle()
    expect(seen).toEqual(['ok', 'unreachable'])
    w.stop()
  })

  it('reports nothing and stops retrying once stopped', async () => {
    answers = ['unreachable']
    const w = watchHealth((h) => seen.push(h), fake)
    w.stop()
    await vi.advanceTimersByTimeAsync(healthRetryDelay * 3)
    expect(seen).toEqual([])
    expect(calls).toBe(1)
  })
})
