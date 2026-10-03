import {
  ApiError,
  createProfile as apiCreateProfile,
  deleteProfile as apiDeleteProfile,
  renameProfile as apiRenameProfile,
  listProfiles,
  useProfile,
} from './api/client'
import type { Profile } from './api/types.gen'
import { browserStorage, type Store } from './diaryPanel'
import { forgetDraft, switchDraft } from './diaryDraft.svelte'
import { current, navigate } from './navigation.svelte'
import { paths } from './router'
import { resetTimeline } from './timeline.svelte'

// The profiles and the one the app works in (ADR 0005). Every data request goes
// under the active profile; App.svelte remounts everything that shows a
// profile's data when it changes, so screens and the event stream start over
// with nothing from the old one. The browser remembers the active profile's ID
// in localStorage; it is checked against the list at startup, since a deleted
// profile's ID can come back as a new profile.

export const STORAGE_KEY = 'waypoint.profile'

// How long to wait before asking for the profiles again when the server
// didn't answer.
export const profileRetryDelay = 2000

export const profiles = $state<{ list: Profile[]; active: Profile | null; notice: string }>({
  list: [],
  active: null,
  // A message about the last profile change (e.g. a deletion), kept here
  // because the Settings screen showing it is remounted by the change.
  notice: '',
})

let storage: Store | null = null

// fallbackProfile is the profile to start in: the remembered one if it still
// exists, otherwise the first.
export function fallbackProfile(list: Profile[], remembered: string | null): Profile | null {
  return list.find((p) => p.id === remembered) ?? list[0] ?? null
}

function remember(id: string) {
  try {
    storage?.setItem(STORAGE_KEY, id)
  } catch {
    // Full or blocked storage only costs remembering the choice.
  }
}

function recalled(): string | null {
  try {
    return storage?.getItem(STORAGE_KEY) ?? null
  } catch {
    return null
  }
}

function activate(p: Profile) {
  useProfile(p.id)
  remember(p.id)
  switchDraft(p.id)
  profiles.active = p
}

// chooseProfile picks the profile the app starts in and points the API client
// at it. It keeps asking until the server answers with at least one, calling
// onWaiting after each attempt that didn't.
export async function chooseProfile(
  fetchFn?: typeof fetch,
  retryDelay = profileRetryDelay,
  onWaiting: () => void = () => {},
  store: Store | null = browserStorage(),
): Promise<Profile> {
  storage = store
  for (;;) {
    try {
      const list = await listProfiles(fetchFn)
      const pick = fallbackProfile(list, recalled())
      if (pick) {
        profiles.list = list
        activate(pick)
        return pick
      }
    } catch {
      // Not answering yet; try again below.
    }
    onWaiting()
    await new Promise((resolve) => setTimeout(resolve, retryDelay))
  }
}

// switchProfile makes another profile the active one. The diary timeline
// empties and the draft is put aside for the old profile; a project or task
// page belongs to the old profile, so the app goes to Now from there.
export function switchProfile(id: string): void {
  const next = profiles.list.find((p) => p.id === id)
  if (!next || profiles.active?.id === id) return
  resetTimeline()
  const route = current.route
  if (route.name === 'project' || route.name === 'task') navigate(paths.now)
  activate(next)
}

// createProfile adds a profile to the list without switching to it. It starts
// with the appearance (theme and fonts) of the profile in use.
export async function createProfile(name: string, fetchFn?: typeof fetch): Promise<Profile> {
  const p = await apiCreateProfile(name, profiles.active?.id, fetchFn)
  profiles.list = [...profiles.list, p]
  return p
}

export async function renameProfile(id: string, name: string, fetchFn?: typeof fetch): Promise<Profile> {
  const p = await apiRenameProfile(id, name, fetchFn)
  profiles.list = profiles.list.map((q) => (q.id === p.id ? p : q))
  if (profiles.active?.id === p.id) profiles.active = p
  return p
}

// successor is the profile the app switches to when id is deleted while active.
export function successor(list: Profile[], id: string): Profile | null {
  return list.find((p) => p.id !== id) ?? null
}

// removeProfile deletes a profile and everything in it. Deleting the active
// profile switches to another one first, so nothing is left pointing at it.
export async function removeProfile(id: string, fetchFn?: typeof fetch): Promise<void> {
  if (profiles.active?.id === id) {
    const next = successor(profiles.list, id)
    if (!next) throw new ApiError(409, 'last_profile', "Your only profile can't be deleted.")
    switchProfile(next.id)
  }
  try {
    await apiDeleteProfile(id, fetchFn)
  } catch (err) {
    // Already gone (e.g. deleted in another tab): the outcome the user wanted.
    if (!(err instanceof ApiError && err.status === 404)) throw err
  }
  profiles.list = profiles.list.filter((p) => p.id !== id)
  forgetDraft(id)
}

// deleteConfirmed says whether what the user typed matches the profile's name,
// ignoring spaces around it.
export function deleteConfirmed(typed: string, name: string): boolean {
  return typed.trim() === name
}

// nameProblem is the message for a failed create or rename: the server's
// problem with the name if it gave one.
export function nameProblem(err: unknown, fallback: string): string {
  if (!(err instanceof ApiError)) return fallback
  const p = err.problems.find((q) => q.field === 'name')
  if (err.code === 'invalid' && p) return p.problem.charAt(0).toUpperCase() + p.problem.slice(1) + '.'
  return err.message
}

// waitingMessage is shown in place of the app while the server can't be
// reached at startup: calm, and saying it keeps trying.
export const waitingMessage = "Can't reach the Waypoint server yet. Trying again…"
