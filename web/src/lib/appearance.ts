import { getSettings } from './api/client'
import { diaryFontValues, interfaceFontValues, type DiaryFont, type InterfaceFont } from './api/enums.gen'
import type { Settings, UpdateSettingsRequest } from './api/types.gen'

// The fonts bundled with the app (fonts.ts), by setting value. Every diary
// font is listed, and interface fonts are a subset, so a value the generated
// enums add without an entry here fails to compile.
export const fonts: Record<DiaryFont, { label: string; stack: string }> = {
  literata: { label: 'Literata', stack: "'Literata Variable', ui-serif, Georgia, serif" },
  inter: { label: 'Inter', stack: "'Inter Variable', ui-sans-serif, system-ui, sans-serif" },
  geist: { label: 'Geist', stack: "'Geist Variable', ui-sans-serif, system-ui, sans-serif" },
  manrope: { label: 'Manrope', stack: "'Manrope Variable', ui-sans-serif, system-ui, sans-serif" },
  'ibm-plex-sans': { label: 'IBM Plex Sans', stack: "'IBM Plex Sans Variable', ui-sans-serif, system-ui, sans-serif" },
  'atkinson-hyperlegible-next': {
    label: 'Atkinson Hyperlegible Next',
    stack: "'Atkinson Hyperlegible Next Variable', ui-sans-serif, system-ui, sans-serif",
  },
}

export interface FontChoice<T extends DiaryFont> {
  value: T
  label: string
  stack: string
}

export const interfaceFontChoices: FontChoice<InterfaceFont>[] = interfaceFontValues.map((value) => ({ value, ...fonts[value] }))
export const diaryFontChoices: FontChoice<DiaryFont>[] = diaryFontValues.map((value) => ({ value, ...fonts[value] }))

// Appearance is the part of the settings that changes how the app looks.
// (1.5.11 adds the theme here.)
export type Appearance = Pick<Settings, 'interface_font' | 'diary_font'>

export function appearanceForm(s: Settings): Appearance {
  return { interface_font: s.interface_font, diary_font: s.diary_font }
}

// appearancePatch is the PATCH body for the appearance fields that changed,
// or null when none did.
export function appearancePatch(saved: Appearance, form: Appearance): UpdateSettingsRequest | null {
  const patch: UpdateSettingsRequest = {}
  if (form.interface_font !== saved.interface_font) patch.interface_font = form.interface_font
  if (form.diary_font !== saved.diary_font) patch.diary_font = form.diary_font
  return Object.keys(patch).length > 0 ? patch : null
}

// applyAppearance puts a profile's appearance on the page: the font tokens
// that app.css declares are overridden on <html>.
export function applyAppearance(a: Appearance, root: HTMLElement = document.documentElement): void {
  root.style.setProperty('--font-sans', fonts[a.interface_font].stack)
  root.style.setProperty('--font-diary', fonts[a.diary_font].stack)
}

let loads = 0

// setAppearance applies a just-saved appearance, so a load still in flight
// can't put the old one back.
export function setAppearance(a: Appearance): void {
  loads++
  applyAppearance(a)
}

// loadAppearance applies the active profile's appearance. If the settings
// can't be read, the page keeps what it has (the defaults at first). Only the
// latest call applies, so a quick profile switch can't end on the old look.
export async function loadAppearance(fetchFn?: typeof fetch): Promise<void> {
  const mine = ++loads
  try {
    const s = await getSettings(fetchFn)
    if (mine === loads) applyAppearance(s)
  } catch {
    // Keep the current fonts; the Settings screen reports load problems.
  }
}
