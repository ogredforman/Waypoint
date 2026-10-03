import type {
  CreateProfileRequest,
  ErrorBody,
  FieldProblem,
  Profile,
  Project,
  Settings,
  UpdateSettingsRequest,
} from './types.gen'

// ApiError is a failed API request. code is the server's error code (SPEC
// §10.5), or "unreachable" when no API answered at all.
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly problems: FieldProblem[]

  constructor(status: number, code: string, message: string, problems: FieldProblem[] = []) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.problems = problems
  }
}

const unreachableMessage = "Can't reach the Waypoint server."

// The profile every data path is under (ADR 0005), set at startup by
// lib/profile.svelte.ts. While none is set, paths go out as given.
let profileID: string | null = null

export function useProfile(id: string | null) {
  profileID = id
}

// Paths that aren't a profile's: the profile list itself and the health check.
const serverWide = /^\/api\/(profiles|health)(\/|\?|$)/

// apiPath puts an /api/... path under the active profile, as
// /api/profiles/{id}/....
export function apiPath(path: string): string {
  if (profileID === null || !path.startsWith('/api/') || serverWide.test(path)) return path
  return `/api/profiles/${encodeURIComponent(profileID)}${path.slice('/api'.length)}`
}

// request sends a JSON request to the API and returns the decoded response.
// A 204 resolves to undefined; anything else but a 2xx with a JSON body
// throws an ApiError.
export async function request<T>(method: string, path: string, body?: unknown, fetchFn: typeof fetch = fetch): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const init: RequestInit = { method, headers }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }

  let res: Response
  try {
    res = await fetchFn(apiPath(path), init)
  } catch {
    throw new ApiError(0, 'unreachable', unreachableMessage)
  }

  if (res.status === 204) return undefined as T

  let decoded: unknown
  try {
    decoded = await res.json()
  } catch {
    // Not JSON: when the Go server is down, the Vite proxy answers instead.
    throw new ApiError(res.status, 'unreachable', unreachableMessage)
  }
  if (res.ok) return decoded as T

  const err = (decoded as Partial<ErrorBody>).error
  if (err && typeof err.code === 'string' && typeof err.message === 'string') {
    throw new ApiError(res.status, err.code, err.message, err.problems ?? [])
  }
  throw new ApiError(res.status, 'unreachable', unreachableMessage)
}

export function getJSON<T>(path: string, fetchFn?: typeof fetch): Promise<T> {
  return request<T>('GET', path, undefined, fetchFn)
}

export function postJSON<T>(path: string, body: unknown, fetchFn?: typeof fetch): Promise<T> {
  return request<T>('POST', path, body, fetchFn)
}

export function patchJSON<T>(path: string, body: unknown, fetchFn?: typeof fetch): Promise<T> {
  return request<T>('PATCH', path, body, fetchFn)
}

export function deleteJSON(path: string, fetchFn?: typeof fetch): Promise<void> {
  return request<void>('DELETE', path, undefined, fetchFn)
}

// Named calls. New code calls these rather than building /api/ paths itself.

export function listProjects(fetchFn?: typeof fetch): Promise<Project[]> {
  return getJSON<Project[]>('/api/projects', fetchFn)
}

export function getSettings(fetchFn?: typeof fetch): Promise<Settings> {
  return getJSON<Settings>('/api/settings', fetchFn)
}

// updateSettings changes the settings present in req and answers with all of them.
export function updateSettings(req: UpdateSettingsRequest, fetchFn?: typeof fetch): Promise<Settings> {
  return patchJSON<Settings>('/api/settings', req, fetchFn)
}

// Profiles (ADR 0005). These paths are server-wide, never under a profile.

export function listProfiles(fetchFn?: typeof fetch): Promise<Profile[]> {
  return getJSON<Profile[]>('/api/profiles', fetchFn)
}

// createProfile makes a profile; with appearanceFrom (a profile ID) it starts
// with that profile's theme and fonts.
export function createProfile(name: string, appearanceFrom?: string, fetchFn?: typeof fetch): Promise<Profile> {
  const body: CreateProfileRequest = { name }
  if (appearanceFrom) body.appearance_from = appearanceFrom
  return postJSON<Profile>('/api/profiles', body, fetchFn)
}

export function renameProfile(id: string, name: string, fetchFn?: typeof fetch): Promise<Profile> {
  return patchJSON<Profile>(`/api/profiles/${encodeURIComponent(id)}`, { name }, fetchFn)
}

// deleteProfile removes the profile and everything in it, for good.
export function deleteProfile(id: string, fetchFn?: typeof fetch): Promise<void> {
  return deleteJSON(`/api/profiles/${encodeURIComponent(id)}`, fetchFn)
}
