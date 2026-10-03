import { getSettings } from './api/client'
import {
  diaryFontValues,
  interfaceFontValues,
  themeValues,
  type DiaryFont,
  type InterfaceFont,
  type Theme,
} from './api/enums.gen'
import type { Settings, UpdateSettingsRequest } from './api/types.gen'
import { browserStorage, type Store } from './diaryPanel'

// The colour themes, by setting value. Their colours are the [data-theme]
// blocks in app.css; a theme the generated enum adds without an entry here
// fails to compile.
export const themes: Record<Theme, { label: string; mode: 'Dark' | 'Light'; description: string }> = {
  midnight: { label: 'Midnight', mode: 'Dark', description: 'Deep blue-black, purple accent' },
  forest: { label: 'Forest', mode: 'Dark', description: 'Deep green, teal accent' },
  paper: { label: 'Paper', mode: 'Light', description: 'Warm off-white, indigo accent' },
  sage: { label: 'Sage', mode: 'Light', description: 'Soft green' },
  dawn: { label: 'Dawn', mode: 'Light', description: 'Cream, amber accent' },
}

export const themeChoices = themeValues.map((value) => ({ value, ...themes[value] }))

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

// Appearance is the part of the settings that changes how the app looks. A
// new profile starts with the appearance of the one in use (createProfile).
export type Appearance = Pick<Settings, 'theme' | 'interface_font' | 'diary_font'>

export function appearanceForm(s: Settings): Appearance {
  return { theme: s.theme, interface_font: s.interface_font, diary_font: s.diary_font }
}

// appearancePatch is the PATCH body for the appearance fields that changed,
// or null when none did.
export function appearancePatch(saved: Appearance, form: Appearance): UpdateSettingsRequest | null {
  const patch: UpdateSettingsRequest = {}
  if (form.theme !== saved.theme) patch.theme = form.theme
  if (form.interface_font !== saved.interface_font) patch.interface_font = form.interface_font
  if (form.diary_font !== saved.diary_font) patch.diary_font = form.diary_font
  return Object.keys(patch).length > 0 ? patch : null
}

// applyAppearance puts a profile's appearance on the page: the theme as
// <html data-theme>, and the font tokens that app.css declares overridden on
// <html>.
export function applyAppearance(a: Appearance, root: HTMLElement = document.documentElement): void {
  root.dataset.theme = a.theme
  root.style.setProperty('--font-sans', fonts[a.interface_font].stack)
  root.style.setProperty('--font-diary', fonts[a.diary_font].stack)
}

// THEME_KEY remembers the theme last shown in this browser, so the next page
// load starts in it instead of flashing Midnight until the settings arrive.
export const THEME_KEY = 'waypoint.theme'

function show(a: Appearance, store: Store | null): void {
  applyAppearance(a)
  try {
    store?.setItem(THEME_KEY, a.theme)
  } catch {
    // Not remembered; the next load shows Midnight for a moment.
  }
}

// restoreTheme applies the remembered theme, if there is a valid one. main.ts
// calls it before anything is shown.
export function restoreTheme(store: Store | null = browserStorage(), root: HTMLElement = document.documentElement): void {
  let theme: string | null = null
  try {
    theme = store?.getItem(THEME_KEY) ?? null
  } catch {
    return
  }
  if (theme && (themeValues as readonly string[]).includes(theme)) root.dataset.theme = theme
}

let loads = 0

// setAppearance applies a just-saved appearance, so a load still in flight
// can't put the old one back.
export function setAppearance(a: Appearance, store: Store | null = browserStorage()): void {
  loads++
  show(a, store)
}

// loadAppearance applies the active profile's appearance. If the settings
// can't be read, the page keeps what it has (the defaults at first). Only the
// latest call applies, so a quick profile switch can't end on the old look.
export async function loadAppearance(fetchFn?: typeof fetch, store: Store | null = browserStorage()): Promise<void> {
  const mine = ++loads
  try {
    const s = await getSettings(fetchFn)
    if (mine === loads) show(s, store)
  } catch {
    // Keep the current look; the Settings screen reports load problems.
  }
}
