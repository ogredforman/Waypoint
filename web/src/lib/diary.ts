import type { CreateEntryRequest, Project } from './api/types.gen'
import type { Route } from './router'
import { groupByStatus, pickable } from './projects'

// Key is the part of a KeyboardEvent that decides whether it submits an entry.
export interface Key {
  key: string
  ctrlKey: boolean
  metaKey: boolean
  isComposing?: boolean
}

// isSubmitShortcut reports whether a key press in the diary input submits it:
// Ctrl+Enter, or Cmd+Enter on a Mac. Plain Enter is a new line.
export function isSubmitShortcut(e: Key): boolean {
  return e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.isComposing
}

// screenProjectID is the project on screen for a route, if any: the default
// scope for a new entry.
export function screenProjectID(route: Route): number | null {
  return route.name === 'project' ? route.id : null
}

// ScopeChoice is a project the user picked for the next entry (null for none)
// and the page they picked it on. On any other page the pick no longer
// applies and the scope goes back to the project on screen.
export interface ScopeChoice {
  projectID: number | null
  path: string
}

// resolveScope is the project a new entry will be about.
export function resolveScope(choice: ScopeChoice | null, path: string, screenID: number | null): number | null {
  return choice && choice.path === path ? choice.projectID : screenID
}

// scopeOptions are the projects an entry can be scoped to, active first. Done
// and archived projects are left out unless one of them is the current scope.
export function scopeOptions(projects: Project[], current: number | null): Project[] {
  return groupByStatus(projects)
    .flatMap((g) => g.projects)
    .filter((p) => pickable(p, current))
}

// entryRequest builds the POST /api/entries body, or null when there's
// nothing to save. The body is kept as written apart from surrounding blank
// space; entries are never edited after this.
export function entryRequest(body: string, projectID: number | null): CreateEntryRequest | null {
  const text = body.trim()
  if (!text) return null
  const req: CreateEntryRequest = { body: text }
  if (projectID !== null) req.project_id = projectID
  return req
}
