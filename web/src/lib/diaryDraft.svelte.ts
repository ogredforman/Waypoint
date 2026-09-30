import type { ScopeChoice } from './diary'

// The entry being written, shared by every copy of the diary input so it
// survives the panel or overlay closing (ADR 0004). choice is the project the
// user picked for it, if they changed the default.
export const draft = $state<{ body: string; choice: ScopeChoice | null }>({ body: '', choice: null })

export function clearDraft(): void {
  draft.body = ''
  draft.choice = null
}
