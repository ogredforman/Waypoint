import type {
  BriefingUpdatedEvent,
  ChangesetAppliedEvent,
  ChangesetReadyEvent,
  EntryProcessedEvent,
  RunFailedEvent,
} from './api/types.gen'

// Live updates from the server's /api/events stream (SPEC §10.5). The app keeps
// one connection open; screens subscribe to the events they show. Events carry
// IDs only, so a handler refetches what it needs. Events missed while the
// connection was down aren't replayed, so after a reconnect screens reload.

export interface EventMap {
  'entry.processed': EntryProcessedEvent
  'changeset.ready': ChangesetReadyEvent
  'run.failed': RunFailedEvent
  'briefing.updated': BriefingUpdatedEvent
  'changeset.applied': ChangesetAppliedEvent
}

export type EventName = keyof EventMap

const eventNames: EventName[] = ['entry.processed', 'changeset.ready', 'run.failed', 'briefing.updated', 'changeset.applied']

// retryDelay is how long to wait before reconnecting after the server refused
// the stream (e.g. while it restarts). EventSource retries dropped connections
// by itself, but not refused ones.
export const retryDelay = 5000

const closed = 2 // EventSource.CLOSED, which isn't defined outside the browser

// Handlers are stored untyped; on() ties each to its event's payload type.
const handlers = new Map<EventName, Set<(data: unknown) => void>>()
const reconnectHandlers = new Set<() => void>()
const disconnectHandlers = new Set<() => void>()

// on calls handler with the data of each event named name, until the returned
// function is called.
export function on<N extends EventName>(name: N, handler: (data: EventMap[N]) => void): () => void {
  let set = handlers.get(name)
  if (!set) handlers.set(name, (set = new Set()))
  const h = handler as (data: unknown) => void
  set.add(h)
  return () => set.delete(h)
}

// onReconnect calls handler each time the stream reopens after being down,
// until the returned function is called.
export function onReconnect(handler: () => void): () => void {
  reconnectHandlers.add(handler)
  return () => reconnectHandlers.delete(handler)
}

// onDisconnect calls handler each time an open stream drops (not again while it
// stays down), until the returned function is called.
export function onDisconnect(handler: () => void): () => void {
  disconnectHandlers.add(handler)
  return () => disconnectHandlers.delete(handler)
}

function dispatch(name: EventName, raw: string) {
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return
  }
  for (const h of handlers.get(name) ?? []) h(data)
}

// startEvents opens the stream and keeps it open until the returned function
// is called.
export function startEvents(open: (url: string) => EventSource = (url) => new EventSource(url)): () => void {
  let source: EventSource | null = null
  let retry: ReturnType<typeof setTimeout> | undefined
  let opened = false
  let up = false
  let stopped = false

  function connect() {
    const s = open('/api/events')
    source = s
    for (const name of eventNames) s.addEventListener(name, (e) => dispatch(name, (e as MessageEvent).data))
    s.addEventListener('open', () => {
      if (opened) for (const h of reconnectHandlers) h()
      opened = true
      up = true
    })
    s.addEventListener('error', () => {
      if (stopped) return
      if (up) for (const h of disconnectHandlers) h()
      up = false
      if (s.readyState !== closed) return
      retry = setTimeout(connect, retryDelay)
    })
  }

  connect()
  return () => {
    stopped = true
    clearTimeout(retry)
    source?.close()
  }
}
