import { matchRoute, shouldIntercept, type Route } from './router'

// The current route (current.route), kept in step with the browser's history.
// Screens use plain <a href> links; startRouting turns clicks on them into
// navigate calls.

let path = $state(window.location.pathname)
let search = $state(window.location.search)
const route = $derived(matchRoute(path))

// headingFocus is a request, made when the page changes, to move focus to the
// new screen's <h1> once it renders. from is what had focus when the page
// changed: if the user has moved focus since, the request is dropped.
let headingFocus: { from: Element | null } | null = null

// moveTo makes pathname the current page. Only a new pathname asks for the
// heading to be focused; the first load and query-string changes don't.
function moveTo(pathname: string): void {
  if (pathname === path) return
  headingFocus = { from: document.activeElement }
  path = pathname
}

export const current = {
  get path(): string {
    return path
  },
  get route(): Route {
    return route
  },
  // search is the query string, e.g. ?entry=N on /diary.
  get search(): string {
    return search
  },
}

// navigate moves to another page of the app without reloading it.
export function navigate(to: string, options: { replace?: boolean } = {}): void {
  const url = new URL(to, window.location.href)
  const target = url.pathname + url.search + url.hash
  if (target === window.location.pathname + window.location.search + window.location.hash) return
  if (options.replace) {
    history.replaceState(null, '', target)
  } else {
    history.pushState(null, '', target)
  }
  search = url.search
  moveTo(url.pathname)
  window.scrollTo(0, 0)
}

// startRouting follows back/forward and handles in-app link clicks. It
// returns a function that stops both.
export function startRouting(): () => void {
  const onPopState = () => {
    search = window.location.search
    moveTo(window.location.pathname)
  }
  const onClick = (e: MouseEvent) => {
    const a = e.target instanceof Element ? e.target.closest('a') : null
    if (!a || !a.hasAttribute('href')) return
    const link = { href: a.href, target: a.target, hasDownload: a.hasAttribute('download') }
    if (!shouldIntercept(e, link, window.location.origin)) return
    e.preventDefault()
    navigate(a.href)
  }
  window.addEventListener('popstate', onPopState)
  document.addEventListener('click', onClick)
  return () => {
    window.removeEventListener('popstate', onPopState)
    document.removeEventListener('click', onClick)
  }
}

// pageHeading is attached to each screen's <h1> ({@attach pageHeading}). It
// makes the heading focusable from script and, after a client-side page change,
// focuses it so screen readers announce the new page. Screens that load first
// show their <h1> when the data arrives, which is when it takes focus.
export function pageHeading(h: HTMLElement): void {
  h.tabIndex = -1
  const req = headingFocus
  if (req === null) return
  headingFocus = null
  const active = document.activeElement
  if (active !== null && active !== document.body && active !== req.from) return
  h.focus({ preventScroll: true })
}
