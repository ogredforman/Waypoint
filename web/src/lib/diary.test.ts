import { describe, expect, it } from 'vitest'
import type { ProjectStatus } from './api/enums.gen'
import type { Project } from './api/types.gen'
import { entryRequest, isSubmitShortcut, knownScope, resolveScope, scopeOptions, screenProjectID } from './diary'

function project(id: number, status: ProjectStatus): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '', last_activity_at: '' }
}

describe('isSubmitShortcut', () => {
  const key = (k: Partial<Parameters<typeof isSubmitShortcut>[0]>) => ({ key: 'Enter', ctrlKey: false, metaKey: false, ...k })

  it('submits on Ctrl+Enter and Cmd+Enter', () => {
    expect(isSubmitShortcut(key({ ctrlKey: true }))).toBe(true)
    expect(isSubmitShortcut(key({ metaKey: true }))).toBe(true)
  })

  it('leaves plain Enter, other keys, and IME composition alone', () => {
    expect(isSubmitShortcut(key({}))).toBe(false)
    expect(isSubmitShortcut(key({ key: 'a', ctrlKey: true }))).toBe(false)
    expect(isSubmitShortcut(key({ ctrlKey: true, isComposing: true }))).toBe(false)
  })
})

describe('screenProjectID', () => {
  it('is the project on a project page and nothing elsewhere', () => {
    expect(screenProjectID({ name: 'project', id: 4 })).toBe(4)
    expect(screenProjectID({ name: 'now' })).toBeNull()
    expect(screenProjectID({ name: 'task', id: 4 })).toBeNull()
  })
})

describe('resolveScope', () => {
  it('defaults to the project on screen', () => {
    expect(resolveScope(null, '/projects/2', 2)).toBe(2)
    expect(resolveScope(null, '/', null)).toBeNull()
  })

  it('uses the pick made on this page, including no project', () => {
    expect(resolveScope({ projectID: 5, path: '/' }, '/', null)).toBe(5)
    expect(resolveScope({ projectID: null, path: '/projects/2' }, '/projects/2', 2)).toBeNull()
  })

  it('goes back to the default on another page', () => {
    expect(resolveScope({ projectID: 5, path: '/' }, '/projects/2', 2)).toBe(2)
  })
})

describe('scopeOptions', () => {
  it('lists active projects first and leaves out done ones', () => {
    const opts = scopeOptions([project(1, 'parked'), project(2, 'done'), project(3, 'active')], null)
    expect(opts.map((p) => p.id)).toEqual([3, 1])
  })

  it('leaves out archived projects unless one is the current scope', () => {
    const all = [project(1, 'archived'), project(2, 'parked')]
    expect(scopeOptions(all, null).map((p) => p.id)).toEqual([2])
    expect(scopeOptions(all, 1).map((p) => p.id)).toEqual([2, 1])
  })

  it('keeps a done project that is the current scope', () => {
    expect(scopeOptions([project(2, 'done')], 2).map((p) => p.id)).toEqual([2])
  })
})

describe('entryRequest', () => {
  it('trims the body and leaves out a missing project', () => {
    expect(entryRequest('  fixed the bug \n', null)).toEqual({ body: 'fixed the bug' })
  })

  it('scopes the entry to a project', () => {
    expect(entryRequest('todo: write tests', 3)).toEqual({ body: 'todo: write tests', project_id: 3 })
  })

  it('has nothing to send for a blank body', () => {
    expect(entryRequest(' \n ', 3)).toBeNull()
  })
})

describe('knownScope', () => {
  it('keeps a scope until the project list has loaded', () => {
    expect(knownScope(3, null)).toBe(3)
  })
  it('drops a scope whose project is gone', () => {
    expect(knownScope(3, [project(1, 'active')])).toBeNull()
    expect(knownScope(1, [project(1, 'active')])).toBe(1)
    expect(knownScope(null, [project(1, 'active')])).toBeNull()
  })
})
