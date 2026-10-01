import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

// Reads the colour tokens from app.css, so the check follows the theme rather than a copy of it.
const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8')
const tokens: Record<string, string> = Object.fromEntries(
  [...css.matchAll(/--color-([\w-]+):\s*(#[0-9a-fA-F]{6})\b/g)].map((m) => [m[1], m[2]]),
)

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

// Text needs 4.5:1 (WCAG AA). ink-faint is left out: it's only for decoration and disabled text.
const textPairs: [string, string][] = [
  ...['ink', 'ink-muted', 'accent'].flatMap((fg) => ['canvas', 'surface', 'raised'].map((bg): [string, string] => [fg, bg])),
  ['on-accent', 'accent-strong'],
  ['on-accent', 'accent-deep'],
]

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

// Highlights tint the text's background at 15% (25% on hover); the text on them must stay readable.
const highlightPairs: [string, string, number][] = ['accent', 'plan'].flatMap((tint) =>
  ['canvas', 'surface', 'raised'].flatMap((bg) => [0.15, 0.25].map((a): [string, string, number] => [tint, bg, a])),
)

describe('theme tokens', () => {
  it('computes contrast the WCAG way', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21)
  })

  it.each(textPairs)('%s on %s meets WCAG AA', (fg, bg) => {
    expect(tokens[fg], `--color-${fg}`).toBeDefined()
    expect(tokens[bg], `--color-${bg}`).toBeDefined()
    expect(contrast(tokens[fg], tokens[bg])).toBeGreaterThanOrEqual(4.5)
  })

  it('colours scrollbars from tokens', () => {
    const colours = [...css.matchAll(/scrollbar-color:\s*([^;]+);/g)].map((m) => m[1].trim())
    expect(colours.length).toBeGreaterThan(0)
    for (const c of colours) expect(c).toMatch(/^var\(--color-[\w-]+\) transparent$/)
  })

  it.each(highlightPairs)('ink on a %s highlight over %s at %s meets WCAG AA', (tint, bg, alpha) => {
    expect(tokens[tint], `--color-${tint}`).toBeDefined()
    expect(contrast(tokens.ink, blend(tokens[tint], tokens[bg], alpha))).toBeGreaterThanOrEqual(4.5)
  })
})
