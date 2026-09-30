import { describe, expect, it } from 'vitest'
import { ApiError, getJSON, postJSON } from './client'

function respond(status: number, body: string): typeof fetch {
  return async () => new Response(body, { status, headers: { 'Content-Type': 'application/json' } })
}

async function failure(p: Promise<unknown>): Promise<ApiError> {
  try {
    await p
  } catch (e) {
    if (e instanceof ApiError) return e
    throw e
  }
  throw new Error('expected the request to fail')
}

describe('request', () => {
  it('returns the decoded body of a success', async () => {
    expect(await getJSON('/api/projects', respond(200, '[{"id":1}]'))).toEqual([{ id: 1 }])
  })

  it('sends a JSON body with a POST', async () => {
    let seen: { url: string; init?: RequestInit } = { url: '' }
    const spy: typeof fetch = async (input, init) => {
      seen = { url: String(input), init }
      return new Response('{"id":3}', { status: 201 })
    }
    expect(await postJSON('/api/projects', { title: 'Garden' }, spy)).toEqual({ id: 3 })
    expect(seen.url).toBe('/api/projects')
    expect(seen.init?.method).toBe('POST')
    expect(seen.init?.body).toBe('{"title":"Garden"}')
    expect((seen.init?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('turns the API error shape into an ApiError', async () => {
    const e = await failure(getJSON('/api/projects', respond(409, '{"error":{"code":"active_cap","message":"3 of 3 active projects"}}')))
    expect(e.status).toBe(409)
    expect(e.code).toBe('active_cap')
    expect(e.message).toBe('3 of 3 active projects')
    expect(e.problems).toEqual([])
  })

  it('keeps field problems', async () => {
    const body = '{"error":{"code":"invalid","message":"invalid project","problems":[{"field":"title","problem":"is required"}]}}'
    const e = await failure(postJSON('/api/projects', {}, respond(422, body)))
    expect(e.code).toBe('invalid')
    expect(e.problems).toEqual([{ field: 'title', problem: 'is required' }])
  })

  it('reports unreachable when the request fails outright', async () => {
    const failing: typeof fetch = async () => {
      throw new TypeError('network down')
    }
    const e = await failure(getJSON('/api/projects', failing))
    expect(e.code).toBe('unreachable')
    expect(e.status).toBe(0)
  })

  it('reports unreachable for a response that is not the API talking', async () => {
    expect((await failure(getJSON('/api/projects', respond(502, 'Bad Gateway')))).code).toBe('unreachable')
    expect((await failure(getJSON('/api/projects', respond(500, '{"oops":true}')))).code).toBe('unreachable')
    expect((await failure(getJSON('/api/projects', respond(200, 'not json')))).code).toBe('unreachable')
  })
})
