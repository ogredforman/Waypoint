// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { diaryFontValues, interfaceFontValues, themeValues } from './api/enums.gen'
import {
  THEME_KEY,
  appearancePatch,
  applyAppearance,
  diaryFontChoices,
  fonts,
  interfaceFontChoices,
  restoreTheme,
  setAppearance,
  themeChoices,
} from './appearance'

const saved = { theme: 'midnight', interface_font: 'inter', diary_font: 'literata' } as const

function memoryStorage(initial: Record<string, string> = {}) {
  const m = new Map(Object.entries(initial))
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) }
}

// A stand-in for <html>: the theme attribute and the font tokens.
function fakeRoot() {
  const style = new Map<string, string>()
  const el = {
    dataset: {} as DOMStringMap,
    style: { setProperty: (k: string, v: string) => style.set(k, v) },
  } as unknown as HTMLElement
  return { el, style }
}

describe('theme choices', () => {
  it('offer every theme the API allows, in its order, dark ones first', () => {
    expect(themeChoices.map((c) => c.value)).toEqual([...themeValues])
    expect(themeChoices.map((c) => c.mode)).toEqual(['Dark', 'Dark', 'Light', 'Light', 'Light'])
  })
})

describe('font choices', () => {
  it('offer every font the API allows, in its order', () => {
    expect(interfaceFontChoices.map((c) => c.value)).toEqual([...interfaceFontValues])
    expect(diaryFontChoices.map((c) => c.value)).toEqual([...diaryFontValues])
  })

  it('name a bundled face first and fall back by kind', () => {
    for (const [value, f] of Object.entries(fonts)) {
      expect(f.stack, value).toMatch(/^'[\w ]+ Variable', /)
      expect(f.stack, value).toMatch(value === 'literata' ? /serif$/ : /sans-serif$/)
    }
  })
})

describe('appearancePatch', () => {
  it('is null when nothing changed', () => {
    expect(appearancePatch(saved, { ...saved })).toBeNull()
  })

  it('holds only the changed fields', () => {
    expect(appearancePatch(saved, { ...saved, diary_font: 'geist' })).toEqual({ diary_font: 'geist' })
    expect(appearancePatch(saved, { ...saved, theme: 'sage' })).toEqual({ theme: 'sage' })
    expect(appearancePatch(saved, { theme: 'midnight', interface_font: 'manrope', diary_font: 'inter' })).toEqual({
      interface_font: 'manrope',
      diary_font: 'inter',
    })
  })
})

describe('applyAppearance', () => {
  it('sets the theme and the font tokens on the root element', () => {
    const { el, style } = fakeRoot()
    applyAppearance({ theme: 'forest', interface_font: 'atkinson-hyperlegible-next', diary_font: 'ibm-plex-sans' }, el)
    expect(el.dataset.theme).toBe('forest')
    expect(style.get('--font-sans')).toBe(fonts['atkinson-hyperlegible-next'].stack)
    expect(style.get('--font-diary')).toBe(fonts['ibm-plex-sans'].stack)
  })
})

describe('the remembered theme', () => {
  it('is the theme last applied, and the next load starts in it', () => {
    const store = memoryStorage()
    setAppearance({ ...saved, theme: 'dawn' }, store)
    expect(store.getItem(THEME_KEY)).toBe('dawn')
    const { el } = fakeRoot()
    restoreTheme(store, el)
    expect(el.dataset.theme).toBe('dawn')
  })

  it('is ignored when missing or not a theme', () => {
    for (const store of [memoryStorage(), memoryStorage({ [THEME_KEY]: 'solarized' }), null]) {
      const { el } = fakeRoot()
      restoreTheme(store, el)
      expect(el.dataset.theme).toBeUndefined()
    }
  })
})
