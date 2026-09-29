import { describe, expect, it } from 'vitest'
import { fetchHealth } from './health'

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
