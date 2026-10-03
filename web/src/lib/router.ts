// The app's routes and the rules for following links, kept free of the DOM
// so they can be tested directly. navigation.svelte.ts wires them to the browser.

export type Route =
  | { name: 'now' }
  | { name: 'diary' }
  | { name: 'projects' }
  | { name: 'project'; id: number }
  | { name: 'task'; id: number }
  | { name: 'parkingLot' }
  | { name: 'settings' }
  | { name: 'notFound' }

// Section is a top-level area of the app: one per nav item.
export type Section = 'now' | 'diary' | 'projects' | 'parkingLot' | 'settings'

export const paths = {
  now: '/',
  diary: '/diary',
  diaryEntry: (id: number) => `/diary?entry=${id}`,
  projects: '/projects',
  project: (id: number) => `/projects/${id}`,
  task: (id: number) => `/tasks/${id}`,
  parkingLot: '/parking-lot',
  settings: '/settings',
} as const

// matchRoute finds the route for a URL path. A trailing slash is ignored.
export function matchRoute(pathname: string): Route {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  switch (path) {
    case '':
    case '/':
      return { name: 'now' }
    case paths.diary:
      return { name: 'diary' }
    case paths.projects:
      return { name: 'projects' }
    case paths.parkingLot:
      return { name: 'parkingLot' }
    case paths.settings:
      return { name: 'settings' }
  }
  const m = /^\/(projects|tasks)\/([^/]+)$/.exec(path)
  if (m) {
    const id = parseID(m[2])
    if (id !== null) return { name: m[1] === 'projects' ? 'project' : 'task', id }
  }
  return { name: 'notFound' }
}

// entryParam reads the entry a /diary link points at (?entry=N) from a URL's
// query string. null when there isn't one, or it isn't an ID.
export function entryParam(search: string): number | null {
  const s = new URLSearchParams(search).get('entry')
  return s === null ? null : parseID(s)
}

// parseID accepts the positive integer IDs the API hands out, nothing else.
function parseID(s: string): number | null {
  if (!/^[1-9][0-9]*$/.test(s)) return null
  const n = Number(s)
  return Number.isSafeInteger(n) ? n : null
}

// sectionOf says which nav item a route belongs to. Tasks live in projects.
export function sectionOf(route: Route): Section | null {
  switch (route.name) {
    case 'now':
    case 'diary':
    case 'projects':
    case 'parkingLot':
    case 'settings':
      return route.name
    case 'project':
    case 'task':
      return 'projects'
    case 'notFound':
      return null
  }
}

const titles: Record<Route['name'], string> = {
  now: 'Now',
  diary: 'Diary',
  projects: 'Projects',
  project: 'Project',
  task: 'Task',
  parkingLot: 'Parking lot',
  settings: 'Settings',
  notFound: 'Not found',
}

// pageTitle is the document title for a route.
export function pageTitle(route: Route): string {
  return route.name === 'now' ? 'Waypoint' : `${titles[route.name]} · Waypoint`
}

// Click is the part of a MouseEvent that decides whether to follow a link in-app.
export interface Click {
  button: number
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  defaultPrevented: boolean
}

// Link is the part of an <a> element that decides the same. href is absolute,
// as HTMLAnchorElement.href reports it.
export interface Link {
  href: string
  target: string
  hasDownload: boolean
}

// shouldIntercept reports whether a click on a link should be handled by the
// app's router instead of the browser: a plain left click on a same-origin
// link to a page of the app. Modified clicks (new tab, new window, download)
// and links to /api/ are left to the browser.
export function shouldIntercept(click: Click, link: Link, origin: string): boolean {
  if (click.defaultPrevented || click.button !== 0) return false
  if (click.metaKey || click.ctrlKey || click.shiftKey || click.altKey) return false
  if (link.hasDownload || (link.target !== '' && link.target !== '_self')) return false
  let url: URL
  try {
    url = new URL(link.href)
  } catch {
    return false
  }
  if (url.origin !== origin) return false
  return url.pathname !== '/api' && !url.pathname.startsWith('/api/')
}
