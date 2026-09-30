import { describe, expect, it } from 'vitest'
import {
  BIG_STEP,
  clampWidth,
  DEFAULT_WIDTH,
  loadPrefs,
  MAX_WIDTH,
  MIN_WIDTH,
  savePrefs,
  STEP,
  STORAGE_KEY,
  widthForDrag,
  widthForKey,
} from './diaryPanel'

function memoryStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial))
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  }
}

const defaults = { width: DEFAULT_WIDTH, open: true }

describe('clampWidth', () => {
  it('keeps widths within the limits', () => {
    expect(clampWidth(MIN_WIDTH - 50)).toBe(MIN_WIDTH)
    expect(clampWidth(MAX_WIDTH + 50)).toBe(MAX_WIDTH)
    expect(clampWidth(400.4)).toBe(400)
  })

  it('falls back to the default for non-finite numbers', () => {
    expect(clampWidth(NaN)).toBe(DEFAULT_WIDTH)
    expect(clampWidth(Infinity)).toBe(DEFAULT_WIDTH)
  })
})

describe('widthForDrag', () => {
  it('widens when the pointer moves left and narrows when it moves right', () => {
    expect(widthForDrag(400, 1000, 950)).toBe(450)
    expect(widthForDrag(400, 1000, 1030)).toBe(370)
  })

  it('stays within the limits', () => {
    expect(widthForDrag(400, 1000, 0)).toBe(MAX_WIDTH)
    expect(widthForDrag(400, 1000, 2000)).toBe(MIN_WIDTH)
  })
})

describe('widthForKey', () => {
  it('steps with the arrows, further with Shift', () => {
    expect(widthForKey(400, 'ArrowLeft', false)).toBe(400 + STEP)
    expect(widthForKey(400, 'ArrowRight', false)).toBe(400 - STEP)
    expect(widthForKey(400, 'ArrowLeft', true)).toBe(400 + BIG_STEP)
  })

  it('jumps to the limits with Home and End', () => {
    expect(widthForKey(400, 'Home', false)).toBe(MIN_WIDTH)
    expect(widthForKey(400, 'End', false)).toBe(MAX_WIDTH)
  })

  it('clamps at the limits', () => {
    expect(widthForKey(MAX_WIDTH, 'ArrowLeft', true)).toBe(MAX_WIDTH)
    expect(widthForKey(MIN_WIDTH, 'ArrowRight', true)).toBe(MIN_WIDTH)
  })

  it('ignores other keys', () => {
    expect(widthForKey(400, 'Enter', false)).toBeNull()
    expect(widthForKey(400, 'ArrowUp', false)).toBeNull()
  })
})

describe('loadPrefs and savePrefs', () => {
  it('round-trips', () => {
    const s = memoryStorage()
    savePrefs(s, { width: 500, open: false })
    expect(loadPrefs(s)).toEqual({ width: 500, open: false })
  })

  it('uses the defaults when nothing is stored or storage is unavailable', () => {
    expect(loadPrefs(memoryStorage())).toEqual(defaults)
    expect(loadPrefs(null)).toEqual(defaults)
  })

  it('recovers from corrupt or out-of-range values', () => {
    expect(loadPrefs(memoryStorage({ [STORAGE_KEY]: '{not json' }))).toEqual(defaults)
    expect(loadPrefs(memoryStorage({ [STORAGE_KEY]: '42' }))).toEqual(defaults)
    expect(loadPrefs(memoryStorage({ [STORAGE_KEY]: '{"width":"wide","open":"yes"}' }))).toEqual(defaults)
    expect(loadPrefs(memoryStorage({ [STORAGE_KEY]: '{"width":9999,"open":false}' }))).toEqual({ width: MAX_WIDTH, open: false })
  })

  it('does not throw when storage refuses writes', () => {
    const s = {
      getItem: () => null,
      setItem: () => {
        throw new Error('quota')
      },
    }
    expect(() => savePrefs(s, { width: 400, open: true })).not.toThrow()
  })
})
