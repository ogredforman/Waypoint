// Health is what the UI shows about the server: "ok" and "unavailable" come
// from /api/health itself; "unreachable" means the request didn't get an answer.
export type Health = 'ok' | 'unavailable' | 'unreachable'

type HealthResponse = { status?: unknown }

// fetchHealth asks the server whether it can reach its database. It never throws.
export async function fetchHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
  try {
    const res = await fetchFn('/api/health', { headers: { Accept: 'application/json' } })
    const body = (await res.json()) as HealthResponse
    if (body.status === 'ok' && res.ok) return 'ok'
    if (body.status === 'unavailable') return 'unavailable'
    return 'unreachable'
  } catch {
    return 'unreachable'
  }
}
