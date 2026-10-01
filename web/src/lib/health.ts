import type { Health as HealthResponse } from './api/types.gen'

// Health is what the UI shows about the server: "ok" and "unavailable" come
// from /api/health itself; "unreachable" means the request didn't get an answer.
export type Health = 'ok' | 'unavailable' | 'unreachable'

// fetchHealth asks the server whether it can reach its database. It never throws.
export async function fetchHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
  try {
    const res = await fetchFn('/api/health', { headers: { Accept: 'application/json' } })
    // Partial: when the Go server is down, the Vite proxy answers instead.
    const body = (await res.json()) as Partial<HealthResponse>
    if (body.status === 'ok' && res.ok) return 'ok'
    if (body.status === 'unavailable') return 'unavailable'
    return 'unreachable'
  } catch {
    return 'unreachable'
  }
}

// healthRetryDelay is how often watchHealth asks again while the server isn't healthy.
export const healthRetryDelay = 5000

export interface HealthWatch {
  // check asks the server now (e.g. when the event stream drops or comes back).
  check: () => void
  stop: () => void
}

// watchHealth asks once now, then whenever check is called, and every retryEvery
// ms while the answer isn't "ok", so a problem clears on its own. While healthy
// it doesn't poll: the app's event stream notices a lost server first.
// onChange gets every answer; only the latest check's answer is reported.
export function watchHealth(
  onChange: (h: Health) => void,
  fetchFn?: typeof fetch,
  retryEvery = healthRetryDelay,
): HealthWatch {
  let stopped = false
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  async function check() {
    clearTimeout(timer)
    const n = ++seq
    const h = await fetchHealth(fetchFn)
    if (stopped || n !== seq) return
    onChange(h)
    if (h !== 'ok') timer = setTimeout(check, retryEvery)
  }

  void check()
  return {
    check: () => void check(),
    stop: () => {
      stopped = true
      clearTimeout(timer)
    },
  }
}
