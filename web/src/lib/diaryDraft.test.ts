import { describe, expect, it } from 'vitest'
import { draft, draftProfile, draftSession, forgetDraft, restoreDraft, switchDraft } from './diaryDraft.svelte'

describe('diary drafts per profile', () => {
  it("puts back failed text in its own profile, not the one in use", () => {
    switchDraft('a')
    expect(draftProfile()).toBe('a')
    switchDraft('b')
    restoreDraft('a', 'written in a', null)
    expect(draft.body).toBe('')
    switchDraft('a')
    expect(draft.body).toBe('written in a')
  })

  it('keeps both the failed text and a newer draft', () => {
    switchDraft('c')
    draft.body = 'newer'
    restoreDraft('c', 'older', null)
    expect(draft.body).toBe('older\n\nnewer')
    switchDraft('c2')
    draft.body = 'parked in c2'
    switchDraft('c')
    restoreDraft('c2', 'failed in c2', { projectID: 1, path: '/' })
    switchDraft('c2')
    expect(draft.body).toBe('failed in c2\n\nparked in c2')
    expect(draft.choice).toBeNull()
  })

  it('counts every switch, even back to the same profile', () => {
    switchDraft('x')
    const before = draftSession()
    switchDraft('y')
    switchDraft('x')
    expect(draftSession()).not.toBe(before)
  })

  it("forgets a deleted profile's draft", () => {
    switchDraft('d')
    draft.body = 'in d'
    switchDraft('e')
    forgetDraft('d')
    switchDraft('d')
    expect(draft.body).toBe('')
  })
})
