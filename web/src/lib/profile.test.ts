import { afterEach, describe, expect, it } from 'vitest'
import { apiPath, useProfile } from './api/client'
import { chooseProfile } from './profile'

afterEach(() => useProfile(null))

describe('apiPath', () => {
  it('leaves paths alone until a profile is chosen', () => {
    expect(apiPath('/api/projects')).toBe('/api/projects')
  })

  it('puts data paths under the profile', () => {
    useProfile('my-portfolio')
    expect(apiPath('/api/projects/3/tasks')).toBe('/api/profiles/my-portfolio/projects/3/tasks')
    expect(apiPath('/api/entries?limit=20')).toBe('/api/profiles/my-portfolio/entries?limit=20')
    expect(apiPath('/api/events')).toBe('/api/profiles/my-portfolio/events')
    expect(apiPath('/api/settings')).toBe('/api/profiles/my-portfolio/settings')
  })

  it('keeps server-wide paths where they are', () => {
    useProfile('personal')
    expect(apiPath('/api/profiles')).toBe('/api/profiles')
    expect(apiPath('/api/profiles/work')).toBe('/api/profiles/work')
    expect(apiPath('/api/health')).toBe('/api/health')
    expect(apiPath('/somewhere')).toBe('/somewhere')
  })
})

describe('chooseProfile', () => {
  it('uses the first profile, asking again until the server answers', async () => {
    const urls: string[] = []
    let calls = 0
    const fetchFn: typeof fetch = async (input) => {
      urls.push(String(input))
      calls++
      if (calls === 1) throw new TypeError('connection refused')
      return new Response('[{"id":"personal","name":"Personal"},{"id":"work","name":"Work"}]', { status: 200 })
    }
    let waited = 0
    expect(await chooseProfile(fetchFn, 0, () => waited++)).toEqual({ id: 'personal', name: 'Personal' })
    expect(waited).toBe(1)
    expect(urls).toEqual(['/api/profiles', '/api/profiles'])
    expect(apiPath('/api/projects')).toBe('/api/profiles/personal/projects')
  })
})
