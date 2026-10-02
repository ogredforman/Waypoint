import { getJSON, useProfile } from './api/client'
import type { Profile } from './api/types.gen'

// How long to wait before asking for the profiles again when the server
// didn't answer.
export const profileRetryDelay = 2000

// chooseProfile picks the profile the app works in and points the API client
// at it (ADR 0005). For now that is the first profile; choosing one is PLAN
// 1.5.9. It keeps asking until the server answers with at least one, calling
// onWaiting after each attempt that didn't.
export async function chooseProfile(
  fetchFn?: typeof fetch,
  retryDelay = profileRetryDelay,
  onWaiting: () => void = () => {},
): Promise<Profile> {
  for (;;) {
    try {
      const list = await getJSON<Profile[]>('/api/profiles', fetchFn)
      if (list.length > 0) {
        useProfile(list[0].id)
        return list[0]
      }
    } catch {
      // Not answering yet; try again below.
    }
    onWaiting()
    await new Promise((resolve) => setTimeout(resolve, retryDelay))
  }
}

// waitingMessage is shown in place of the app while the server can't be
// reached at startup: calm, and saying it keeps trying.
export const waitingMessage = "Can't reach the Waypoint server yet. Trying again…"
