// Size and open state of the diary panel (ADR 0004). These are browser UI preferences kept in
// localStorage; they never go to the server.

export const MIN_WIDTH = 280
export const MAX_WIDTH = 640
export const DEFAULT_WIDTH = 380
export const STEP = 16
export const BIG_STEP = 64
export const STORAGE_KEY = 'waypoint.diaryPanel'

export interface PanelPrefs {
  width: number
  open: boolean
}

export const defaultPrefs: PanelPrefs = { width: DEFAULT_WIDTH, open: true }

export function clampWidth(width: number): number {
  if (!Number.isFinite(width)) return DEFAULT_WIDTH
  return Math.round(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, width)))
}

// The panel is on the right with its handle on its left edge, so moving the pointer left widens it.
export function widthForDrag(startWidth: number, startX: number, x: number): number {
  return clampWidth(startWidth + (startX - x))
}

// widthForKey returns the new width for a key pressed on the focused handle, or null if the key
// doesn't resize. The arrows move the handle, like dragging does.
export function widthForKey(width: number, key: string, shift: boolean): number | null {
  const step = shift ? BIG_STEP : STEP
  switch (key) {
    case 'ArrowLeft':
      return clampWidth(width + step)
    case 'ArrowRight':
      return clampWidth(width - step)
    case 'Home':
      return MIN_WIDTH
    case 'End':
      return MAX_WIDTH
    default:
      return null
  }
}

export type Store = Pick<Storage, 'getItem' | 'setItem'>

// browserStorage returns localStorage, or null where it's unavailable (blocked, some private modes).
export function browserStorage(): Store | null {
  try {
    return globalThis.localStorage ?? null
  } catch {
    return null
  }
}

export function loadPrefs(storage: Store | null): PanelPrefs {
  let raw: unknown
  try {
    raw = JSON.parse(storage?.getItem(STORAGE_KEY) ?? 'null')
  } catch {
    return { ...defaultPrefs }
  }
  if (typeof raw !== 'object' || raw === null) return { ...defaultPrefs }
  const { width, open } = raw as Record<string, unknown>
  return {
    width: typeof width === 'number' ? clampWidth(width) : DEFAULT_WIDTH,
    open: typeof open === 'boolean' ? open : defaultPrefs.open,
  }
}

export function savePrefs(storage: Store | null, prefs: PanelPrefs): void {
  try {
    storage?.setItem(STORAGE_KEY, JSON.stringify({ width: clampWidth(prefs.width), open: prefs.open }))
  } catch {
    // Full or blocked storage only costs the preference, not the app.
  }
}
