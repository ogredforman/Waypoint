// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiPath, useProfile } from './api/client'
import type { Profile } from './api/types.gen'
import { draft } from './diaryDraft.svelte'
import { current, navigate } from './navigation.svelte'
import {
  STORAGE_KEY,
  chooseProfile,
  deleteConfirmed,
  fallbackProfile,
  nameProblem,
  profiles,
  removeProfile,
  renameProfile,
  switchProfile,
} from './profile.svelte'
import { timeline } from './timeline.svelte'

const personal: Profile = { id: 'personal', name: 'Personal' }
const work: Profile = { id: 'work', name: 'Work' }

function memoryStorage(initial: Record<string, string> = {}) {
  const m = new Map(Object.entries(initial))
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
}

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

function listing(list: Profile[]): typeof fetch {
  return async () => json(list)
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})

afterEach(() => {
  useProfile(null)
  vi.restoreAllMocks()
})

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
      return json([personal, work])
    }
    let waited = 0
    const storage = memoryStorage()
    expect(await chooseProfile(fetchFn, 0, () => waited++, storage)).toEqual(personal)
    expect(waited).toBe(1)
    expect(urls).toEqual(['/api/profiles', '/api/profiles'])
    expect(apiPath('/api/projects')).toBe('/api/profiles/personal/projects')
    expect(storage.getItem(STORAGE_KEY)).toBe('personal')
    expect(profiles.active).toEqual(personal)
    expect(profiles.list).toEqual([personal, work])
  })

  it('starts in the remembered profile', async () => {
    const storage = memoryStorage({ [STORAGE_KEY]: 'work' })
    expect(await chooseProfile(listing([personal, work]), 0, undefined, storage)).toEqual(work)
    expect(apiPath('/api/projects')).toBe('/api/profiles/work/projects')
  })

  it('falls back to the first when the remembered profile is gone', async () => {
    const storage = memoryStorage({ [STORAGE_KEY]: 'old' })
    expect(await chooseProfile(listing([personal, work]), 0, undefined, storage)).toEqual(personal)
    expect(storage.getItem(STORAGE_KEY)).toBe('personal')
  })

  it('works without storage', async () => {
    expect(await chooseProfile(listing([work]), 0, undefined, null)).toEqual(work)
  })
})

describe('fallbackProfile', () => {
  it('prefers the remembered profile, then the first, then none', () => {
    expect(fallbackProfile([personal, work], 'work')).toBe(work)
    expect(fallbackProfile([personal, work], null)).toBe(personal)
    expect(fallbackProfile([], 'work')).toBeNull()
  })
})

describe('switching profiles', () => {
  let storage: ReturnType<typeof memoryStorage>

  beforeEach(async () => {
    storage = memoryStorage({ [STORAGE_KEY]: 'personal' })
    await chooseProfile(listing([personal, work]), 0, undefined, storage)
    draft.body = ''
    draft.choice = null
    navigate('/')
  })

  it('points the API at the new profile and remembers it', () => {
    switchProfile('work')
    expect(profiles.active).toEqual(work)
    expect(apiPath('/api/projects')).toBe('/api/profiles/work/projects')
    expect(storage.getItem(STORAGE_KEY)).toBe('work')
  })

  it('ignores unknown profiles and the one in use', () => {
    switchProfile('nope')
    switchProfile('personal')
    expect(profiles.active).toEqual(personal)
    expect(storage.getItem(STORAGE_KEY)).toBe('personal')
  })

  it("empties the diary timeline so the old profile's entries don't show", () => {
    timeline.status = 'ready'
    timeline.items = [{ entry: { id: 1 } as never, detail: null, error: '' }]
    switchProfile('work')
    expect(timeline.items).toEqual([])
    expect(timeline.status).toBe('idle')
  })

  it('keeps one diary draft per profile', () => {
    draft.body = 'about personal things'
    draft.choice = { projectID: 4, path: '/' }
    switchProfile('work')
    expect(draft.body).toBe('')
    expect(draft.choice).toBeNull()
    draft.body = 'about work'
    switchProfile('personal')
    expect(draft.body).toBe('about personal things')
    expect(draft.choice).toEqual({ projectID: 4, path: '/' })
    switchProfile('work')
    expect(draft.body).toBe('about work')
  })

  it("leaves a project or task page, which belongs to the old profile", () => {
    navigate('/projects/3')
    switchProfile('work')
    expect(current.path).toBe('/')
    navigate('/tasks/9')
    switchProfile('personal')
    expect(current.path).toBe('/')
  })

  it('stays on a section page', () => {
    navigate('/settings')
    switchProfile('work')
    expect(current.path).toBe('/settings')
  })

  it('renaming the profile in use renames it everywhere', async () => {
    await renameProfile('personal', 'Home', async () => json({ id: 'personal', name: 'Home' }))
    expect(profiles.active).toEqual({ id: 'personal', name: 'Home' })
    expect(profiles.list.map((p) => p.name)).toEqual(['Home', 'Work'])
  })

  it('deleting the profile in use switches to another before the delete is sent', async () => {
    const seen: { method: string; url: string; active: string | undefined; data: string }[] = []
    const fetchFn: typeof fetch = async (input, init) => {
      seen.push({
        method: init?.method ?? 'GET',
        url: String(input),
        active: profiles.active?.id,
        data: apiPath('/api/projects'),
      })
      return new Response(null, { status: 204 })
    }
    await removeProfile('personal', fetchFn)
    expect(seen).toEqual([
      { method: 'DELETE', url: '/api/profiles/personal', active: 'work', data: '/api/profiles/work/projects' },
    ])
    expect(profiles.list).toEqual([work])
    expect(storage.getItem(STORAGE_KEY)).toBe('work')
  })

  it('deleting another profile stays put', async () => {
    await removeProfile('work', async () => new Response(null, { status: 204 }))
    expect(profiles.active).toEqual(personal)
    expect(profiles.list).toEqual([personal])
  })

  it('treats a profile already gone as deleted', async () => {
    const gone = async () => json({ error: { code: 'not_found', message: 'No such profile.' } }, 404)
    await removeProfile('work', gone)
    expect(profiles.list).toEqual([personal])
  })

  it('keeps the profile when the delete fails', async () => {
    const fails = async () => json({ error: { code: 'internal', message: 'Something went wrong.' } }, 500)
    await expect(removeProfile('work', fails)).rejects.toThrow('Something went wrong.')
    expect(profiles.list).toEqual([personal, work])
  })

  it("won't delete the only profile", async () => {
    await removeProfile('work', async () => new Response(null, { status: 204 }))
    let asked = false
    await expect(
      removeProfile('personal', async () => {
        asked = true
        return new Response(null, { status: 204 })
      }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(asked).toBe(false)
    expect(profiles.active).toEqual(personal)
  })
})

describe('deleteConfirmed', () => {
  it('needs the exact name, spaces around it aside', () => {
    expect(deleteConfirmed('Work', 'Work')).toBe(true)
    expect(deleteConfirmed('  Work ', 'Work')).toBe(true)
    expect(deleteConfirmed('work', 'Work')).toBe(false)
    expect(deleteConfirmed('Wor', 'Work')).toBe(false)
    expect(deleteConfirmed('', 'Work')).toBe(false)
  })
})

describe('nameProblem', () => {
  it("uses the server's problem with the name", () => {
    const err = new ApiError(422, 'invalid', 'Invalid profile.', [
      { field: 'name', problem: 'is already used by another profile' },
    ])
    expect(nameProblem(err, 'fallback')).toBe('Is already used by another profile.')
  })

  it('falls back to the message, or the given text', () => {
    expect(nameProblem(new ApiError(500, 'internal', 'Something went wrong.'), 'fallback')).toBe('Something went wrong.')
    expect(nameProblem(new Error('x'), 'fallback')).toBe('fallback')
  })
})
