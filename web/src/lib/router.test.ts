import { describe, expect, it } from 'vitest'
import { matchRoute, pageTitle, paths, sectionOf, shouldIntercept, type Click, type Link } from './router'

describe('matchRoute', () => {
  it('matches the top-level screens', () => {
    expect(matchRoute('/')).toEqual({ name: 'now' })
    expect(matchRoute('/diary')).toEqual({ name: 'diary' })
    expect(matchRoute('/projects')).toEqual({ name: 'projects' })
    expect(matchRoute('/parking-lot')).toEqual({ name: 'parkingLot' })
    expect(matchRoute('/settings')).toEqual({ name: 'settings' })
  })

  it('ignores a trailing slash', () => {
    expect(matchRoute('/diary/')).toEqual({ name: 'diary' })
    expect(matchRoute('/projects//')).toEqual({ name: 'projects' })
    expect(matchRoute('/projects/7/')).toEqual({ name: 'project', id: 7 })
  })

  it('matches project and task pages by ID', () => {
    expect(matchRoute('/projects/12')).toEqual({ name: 'project', id: 12 })
    expect(matchRoute('/tasks/3')).toEqual({ name: 'task', id: 3 })
  })

  it('rejects IDs the API never hands out', () => {
    for (const path of ['/projects/0', '/projects/-1', '/projects/01', '/projects/1.5', '/projects/abc', '/tasks/1e3', '/tasks/99999999999999999999']) {
      expect(matchRoute(path), path).toEqual({ name: 'notFound' })
    }
  })

  it('reports anything else as not found', () => {
    for (const path of ['/nope', '/projects/1/tasks', '/diaryx', '/tasks', '/Parking-Lot']) {
      expect(matchRoute(path), path).toEqual({ name: 'notFound' })
    }
  })

  it('round-trips the paths it builds', () => {
    expect(matchRoute(paths.now)).toEqual({ name: 'now' })
    expect(matchRoute(paths.project(42))).toEqual({ name: 'project', id: 42 })
    expect(matchRoute(paths.task(5))).toEqual({ name: 'task', id: 5 })
    expect(matchRoute(paths.parkingLot)).toEqual({ name: 'parkingLot' })
    expect(matchRoute(paths.settings)).toEqual({ name: 'settings' })
  })
})

describe('sectionOf', () => {
  it('puts project and task pages under Projects', () => {
    expect(sectionOf({ name: 'project', id: 1 })).toBe('projects')
    expect(sectionOf({ name: 'task', id: 1 })).toBe('projects')
  })

  it('maps top-level screens to themselves and not found to none', () => {
    expect(sectionOf({ name: 'now' })).toBe('now')
    expect(sectionOf({ name: 'diary' })).toBe('diary')
    expect(sectionOf({ name: 'parkingLot' })).toBe('parkingLot')
    expect(sectionOf({ name: 'settings' })).toBe('settings')
    expect(sectionOf({ name: 'notFound' })).toBeNull()
  })
})

describe('pageTitle', () => {
  it('names the screen', () => {
    expect(pageTitle({ name: 'now' })).toBe('Waypoint')
    expect(pageTitle({ name: 'parkingLot' })).toBe('Parking lot · Waypoint')
    expect(pageTitle({ name: 'settings' })).toBe('Settings · Waypoint')
  })
})

describe('shouldIntercept', () => {
  const origin = 'http://127.0.0.1:7420'
  const click: Click = { button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, defaultPrevented: false }
  const link: Link = { href: `${origin}/diary`, target: '', hasDownload: false }

  it('takes a plain left click on an app link', () => {
    expect(shouldIntercept(click, link, origin)).toBe(true)
    expect(shouldIntercept(click, { ...link, target: '_self' }, origin)).toBe(true)
  })

  it('leaves modified and non-left clicks to the browser', () => {
    for (const k of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey'] as const) {
      expect(shouldIntercept({ ...click, [k]: true }, link, origin), k).toBe(false)
    }
    expect(shouldIntercept({ ...click, button: 1 }, link, origin)).toBe(false)
    expect(shouldIntercept({ ...click, defaultPrevented: true }, link, origin)).toBe(false)
  })

  it('leaves new-tab and download links to the browser', () => {
    expect(shouldIntercept(click, { ...link, target: '_blank' }, origin)).toBe(false)
    expect(shouldIntercept(click, { ...link, hasDownload: true }, origin)).toBe(false)
  })

  it('leaves other origins and the API to the browser', () => {
    expect(shouldIntercept(click, { ...link, href: 'https://example.com/diary' }, origin)).toBe(false)
    expect(shouldIntercept(click, { ...link, href: 'http://127.0.0.1:9999/diary' }, origin)).toBe(false)
    expect(shouldIntercept(click, { ...link, href: `${origin}/api/export` }, origin)).toBe(false)
    expect(shouldIntercept(click, { ...link, href: `${origin}/api` }, origin)).toBe(false)
    expect(shouldIntercept(click, { ...link, href: 'mailto:me@example.com' }, origin)).toBe(false)
  })

  it('still takes app paths that merely start with "api"', () => {
    expect(shouldIntercept(click, { ...link, href: `${origin}/apiary` }, origin)).toBe(true)
  })
})
