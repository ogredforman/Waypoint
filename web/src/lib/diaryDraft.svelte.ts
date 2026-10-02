import type { ScopeChoice } from './diary'

// The entry being written, shared by every copy of the diary input so it
// survives the panel or overlay closing (ADR 0004). choice is the project the
// user picked for it, if they changed the default.
export const draft = $state<{ body: string; choice: ScopeChoice | null }>({ body: '', choice: null })

export function clearDraft(): void {
  draft.body = ''
  draft.choice = null
}

// Drafts of the profiles not in use, by profile ID. A draft belongs to the
// profile it was written in: switching profiles puts it aside, and switching
// back brings it back. They live only as long as the page.
const parked = new Map<string, { body: string; choice: ScopeChoice | null }>()
let owner: string | null = null
// Counts switches, so a save that started before a switch away and back can
// still tell that the timeline it was meant for has been replaced.
let session = 0

// draftProfile is the profile the current draft belongs to.
export function draftProfile(): string | null {
  return owner
}

// draftSession changes every time the profile does.
export function draftSession(): number {
  return session
}

// switchDraft puts the current draft aside for its profile and brings back
// profile to's, or an empty one.
export function switchDraft(to: string): void {
  if (owner === to) return
  if (owner !== null && (draft.body !== '' || draft.choice !== null)) {
    parked.set(owner, { body: draft.body, choice: draft.choice })
  }
  const next = parked.get(to)
  parked.delete(to)
  draft.body = next?.body ?? ''
  draft.choice = next?.choice ?? null
  owner = to
  session++
}

// restoreDraft puts back text whose save failed, in the profile it was written
// in. If that profile has a newer draft by now, the failed text goes above it,
// so neither is lost.
export function restoreDraft(profile: string | null, body: string, choice: ScopeChoice | null): void {
  const merge = (newer: { body: string; choice: ScopeChoice | null }) => ({
    body: newer.body.trim() === '' ? body : `${body}\n\n${newer.body}`,
    choice: newer.body.trim() === '' ? choice : newer.choice,
  })
  if (profile === owner) {
    const next = merge(draft)
    draft.body = next.body
    draft.choice = next.choice
  } else if (profile !== null) {
    parked.set(profile, merge(parked.get(profile) ?? { body: '', choice: null }))
  }
}

// forgetDraft drops a deleted profile's draft.
export function forgetDraft(profile: string): void {
  parked.delete(profile)
}
