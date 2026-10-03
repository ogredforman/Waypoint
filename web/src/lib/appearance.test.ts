import { describe, expect, it } from 'vitest'
import { diaryFontValues, interfaceFontValues } from './api/enums.gen'
import { appearancePatch, applyAppearance, diaryFontChoices, fonts, interfaceFontChoices } from './appearance'

const saved = { interface_font: 'inter', diary_font: 'literata' } as const

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
    expect(appearancePatch(saved, { interface_font: 'manrope', diary_font: 'inter' })).toEqual({
      interface_font: 'manrope',
      diary_font: 'inter',
    })
  })
})

describe('applyAppearance', () => {
  it('sets the font tokens on the root element', () => {
    const root = { style: new Map<string, string>() }
    const el = { style: { setProperty: (k: string, v: string) => root.style.set(k, v) } } as unknown as HTMLElement
    applyAppearance({ interface_font: 'atkinson-hyperlegible-next', diary_font: 'ibm-plex-sans' }, el)
    expect(root.style.get('--font-sans')).toBe(fonts['atkinson-hyperlegible-next'].stack)
    expect(root.style.get('--font-diary')).toBe(fonts['ibm-plex-sans'].stack)
  })
})
