import { on, onReconnect } from './events'

// liveReload keeps a screen current (SPEC §10.5): it calls reload after each
// applied changeset and after the event stream reconnects, until the returned
// function is called. With projectId, only changesets that touched that
// project count; a projectId that returns undefined (e.g. not loaded yet), or
// none at all, means every changeset counts. Ideas aren't in project_ids, so
// idea lists leave projectId out.
export function liveReload(reload: () => void, projectId?: () => number | undefined): () => void {
  const offs = [
    on('changeset.applied', (e) => {
      const id = projectId?.()
      if (id === undefined || e.project_ids.includes(id)) reload()
    }),
    onReconnect(reload),
  ]
  return () => offs.forEach((off) => off())
}

// latest drops answers that a newer request replaced. Call the function it
// returns before each request; the result says, after the request, whether it
// is still the newest: `const current = loads(); const x = await get(); if (!current()) return`.
export function latest(): () => () => boolean {
  let seq = 0
  return () => {
    const mine = ++seq
    return () => mine === seq
  }
}
