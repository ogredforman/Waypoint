import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { themeValues } from './api/enums.gen'

// Reads the themes from app.css, so the checks follow the CSS rather than a copy of it.
const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8')

// Each [data-theme='…'] block's colours (6-digit hex; the glows, with their own alpha, aren't text colours).
const themes: Record<string, Record<string, string>> = Object.fromEntries(
  [...css.matchAll(/\[data-theme='([\w-]+)'\]\s*\{([^}]*)\}/g)].map((m) => [
    m[1],
    Object.fromEntries([...m[2].matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})\s*;/g)].map((c) => [c[1], c[2]])),
  ]),
)

// The colours the utilities use (@theme inline: --color-x: var(--x)); every theme must set all of them.
const colourVars = [...css.matchAll(/--color-[\w-]+:\s*var\(--([\w-]+)\)/g)].map((m) => m[1])

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

// blend paints fg at the given opacity over bg in sRGB, close to what a `bg-plan/15` tint renders.
function blend(fg: string, bg: string, alpha: number): string {
  const channel = (hex: string, i: number) => parseInt(hex.slice(i, i + 2), 16)
  return (
    '#' +
    [1, 3, 5]
      .map((i) => Math.round(alpha * channel(fg, i) + (1 - alpha) * channel(bg, i)))
      .map((c) => c.toString(16).padStart(2, '0'))
      .join('')
  )
}

// hue is a colour's OKLCH hue in degrees, the hue people see.
function hue(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  const a = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s
  const bb = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
  return ((Math.atan2(bb, a) * 180) / Math.PI + 360) % 360
}

function hueGap(x: string, y: string): number {
  const d = Math.abs(hue(x) - hue(y))
  return Math.min(d, 360 - d)
}

const backgrounds = ['canvas', 'surface', 'raised']

// Text needs 4.5:1 (WCAG AA). ink-faint is left out: it's only for decoration and disabled text.
const textPairs: [string, string][] = [
  ...['ink', 'ink-muted', 'accent'].flatMap((fg) => backgrounds.map((bg): [string, string] => [fg, bg])),
  ['on-accent', 'accent-strong'],
  ['on-accent', 'accent-deep'],
  ['on-accent', 'danger'],
]

// Highlights tint the text's background at 15% (25% on hover); the text on them must stay readable.
const highlightPairs: [string, string, number][] = ['accent', 'plan'].flatMap((tint) =>
  backgrounds.flatMap((bg) => [0.15, 0.25].map((a): [string, string, number] => [tint, bg, a])),
)

describe('theme CSS', () => {
  it('computes contrast and hue the standard way', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21)
    expect(hue('#ff0000')).toBeCloseTo(29.2, 0)
  })

  it('defines exactly the themes the setting allows', () => {
    expect(Object.keys(themes)).toEqual([...themeValues])
  })

  it('makes Midnight the default', () => {
    expect(css).toMatch(/:root,\s*\[data-theme='midnight'\]/)
  })

  it('colours scrollbars from theme variables', () => {
    const colours = [...css.matchAll(/scrollbar-color:\s*([^;]+);/g)].map((m) => m[1].trim())
    expect(colours.length).toBeGreaterThan(0)
    for (const c of colours) expect(c).toMatch(/^var\(--[\w-]+\) transparent$/)
  })
})

describe.each(themeValues)('the %s theme', (name) => {
  const t = themes[name] ?? {}

  it('sets every colour', () => {
    expect(colourVars.length).toBeGreaterThan(10)
    for (const v of colourVars) expect(t[v], `--${v}`).toBeDefined()
  })

  it.each(textPairs)('%s on %s meets WCAG AA', (fg, bg) => {
    expect(contrast(t[fg], t[bg])).toBeGreaterThanOrEqual(4.5)
  })

  it.each(highlightPairs)('ink on a %s highlight over %s at %s meets WCAG AA', (tint, bg, alpha) => {
    expect(contrast(t.ink, blend(t[tint], t[bg], alpha))).toBeGreaterThanOrEqual(4.5)
  })

  it('keeps ink readable on a glowing card (raised into accent-deep/25)', () => {
    expect(contrast(t.ink, blend(t['accent-deep'], t.raised, 0.25))).toBeGreaterThanOrEqual(4.5)
  })

  it.each(['accent', 'accent-strong', 'accent-deep'])('keeps %s well away from the danger red', (accent) => {
    expect(hueGap(t[accent], t.danger)).toBeGreaterThanOrEqual(40)
  })
})
