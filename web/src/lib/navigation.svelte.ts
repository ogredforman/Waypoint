import { matchRoute, shouldIntercept, type Route } from './router'

// The current route (current.route), kept in step with the browser's history.
// Screens use plain <a href> links; startRouting turns clicks on them into
// navigate calls.

let path = $state(window.location.pathname)
const route = $derived(matchRoute(path))

export const current = {
  get path(): string {
    return path
  },
  get route(): Route {
    return route
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
  path = url.pathname
  window.scrollTo(0, 0)
}

// startRouting follows back/forward and handles in-app link clicks. It
// returns a function that stops both.
export function startRouting(): () => void {
  const onPopState = () => {
    path = window.location.pathname
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
