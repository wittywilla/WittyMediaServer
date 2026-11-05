// The theme maker: a whole theme from a few choices. Every colour is checked as it's
// made, so anything someone can pick here is readable (WCAG AA: 4.5:1 for text, 3:1
// for outlines). The same rules are in scripts/theme-contrast.py for the built-ins.

export type MakerInput = { name: string; scheme: 'dark' | 'light'; accent: string; tint: string; corners: 'square' | 'soft' | 'round' }

export type MadeTheme = { vars: Record<string, string>; radius: number; adjusted: boolean }

type RGB = [number, number, number]

export const isHex = (s: string) => /^#[0-9a-fA-F]{6}$/.test(s)
const toRgb = (h: string): RGB => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255) as RGB
const toHex = (c: RGB) => '#' + c.map((v) => Math.round(Math.max(0, Math.min(1, v)) * 255).toString(16).padStart(2, '0')).join('')

function luminance(c: RGB) {
  const [r, g, b] = c.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function contrast(a: string, b: string) {
  const la = luminance(toRgb(a)), lb = luminance(toRgb(b))
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

function toHsl([r, g, b]: RGB): RGB {
  const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min
  if (d === 0) return [0, 0, l]
  const s = d / (1 - Math.abs(2 * l - 1))
  const h = max === r ? ((g - b) / d + 6) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [h / 6, s, l]
}

function fromHsl([h, s, l]: RGB): RGB {
  const c = (1 - Math.abs(2 * l - 1)) * s, x = c * (1 - Math.abs((h * 6) % 2 - 1)), m = l - c / 2
  const i = Math.floor(h * 6) % 6
  const [r, g, b] = [[c, x, 0], [x, c, 0], [0, c, x], [0, x, c], [x, 0, c], [c, 0, x]][i]
  return [r + m, g + m, b + m]
}

const shade = (hue: number, sat: number, l: number) => toHex(fromHsl([hue, sat, l]))

// The colour, moved in lightness only as far as it takes to reach `need` against all of `on`.
function reach(color: string, on: string[], need: number, lighter: boolean) {
  const [h, s, start] = toHsl(toRgb(color))
  let l = start
  for (let i = 0; i < 220; i++) {
    const c = toHex(fromHsl([h, s, l]))
    if (on.every((x) => contrast(c, x) >= need)) return c
    l = Math.max(0, Math.min(1, l + (lighter ? 0.005 : -0.005)))
  }
  return lighter ? '#ffffff' : '#000000'
}

const CORNERS = { square: 2, soft: 10, round: 18 }
const INK = '#0e1013'

export function makeTheme(i: MakerInput): MadeTheme {
  const dark = i.scheme === 'dark'
  const accent0 = isHex(i.accent) ? i.accent : '#7955ff'
  const [hue, tintSat] = toHsl(toRgb(isHex(i.tint) ? i.tint : accent0))
  // Surfaces take the tint's hue, held back so the page stays calm.
  const sat = Math.min(tintSat, dark ? 0.32 : 0.28)
  const surface = (dk: number, lt: number) => shade(hue, sat, dark ? dk : lt)
  const bg = surface(0.07, 0.96), bg2 = surface(0.1, 0.995), bg3 = surface(0.14, 0.92), bg4 = surface(0.19, 0.86)
  const surfaces = [bg, bg2, bg3]
  const text = (dk: number, lt: number, s = 0.12) => reach(shade(hue, Math.min(sat, s), dark ? dk : lt), surfaces, 4.5, dark)
  // A label on the accent is white or ink, whichever reads better; if neither reaches
  // 4.5:1 the accent moves just enough.
  const label = contrast('#ffffff', accent0) >= contrast(INK, accent0) ? '#ffffff' : INK
  const accent = reach(accent0, [label], 4.5, label === INK)
  const [ah, as, al] = toHsl(toRgb(accent))
  const hover = toHex(fromHsl([ah, as, Math.max(0, Math.min(1, al + (label === INK ? 0.06 : -0.06)))]))
  const status = (c: string) => reach(c, surfaces, 4.5, dark)
  return {
    radius: CORNERS[i.corners],
    adjusted: accent.toLowerCase() !== accent0.toLowerCase(),
    vars: {
      bg, 'bg-2': bg2, 'bg-3': bg3, 'bg-4': bg4,
      fg: text(0.93, 0.1), 'fg-2': text(0.72, 0.3), 'fg-3': text(0.6, 0.38),
      accent, 'accent-2': reach(hover, [label], 4.5, label === INK), 'accent-fg': label, 'accent-text': reach(accent, surfaces, 4.5, dark),
      'control-line': reach(bg4, surfaces, 3, dark),
      danger: status(dark ? '#ff5c6c' : '#cf2537'), ok: status(dark ? '#3ad29f' : '#117b50'), warn: status(dark ? '#e0a030' : '#9b5d00'),
    },
  }
}

const rgbList = (h: string) => toRgb(h).map((v) => Math.round(v * 255)).join(' ')

// themeCss: the stylesheet that makes it real, for one device.
export function themeCss(i: MakerInput, t = makeTheme(i)) {
  const lines = Object.entries(t.vars).map(([k, v]) => `--${k}:${v};`).join('')
  return `:root{color-scheme:${i.scheme};${lines}--bg-rgb:${rgbList(t.vars.bg)};--accent-rgb:${rgbList(t.vars.accent)};--radius:${t.radius}px;}`
}

// themeDeclarations: a made theme's stylesheet (themeCss, one rule on :root) as its declarations, to
// set one by one on the page's root element. The server's content security policy (style-src 'self')
// refuses a <style> element made by the page, but not properties set through the CSSOM.
export function themeDeclarations(css: string): [string, string][] {
  const open = css.indexOf('{'), close = css.lastIndexOf('}')
  if (open < 0 || close < open) return []
  const out: [string, string][] = []
  for (const decl of css.slice(open + 1, close).split(';')) {
    const at = decl.indexOf(':')
    if (at < 0) continue
    const name = decl.slice(0, at).trim(), value = decl.slice(at + 1).trim()
    if (/^(--[\w-]+|color-scheme)$/.test(name) && value) out.push([name, value])
  }
  return out
}

// What the theme cards need to draw a made theme, in the names theme.json uses.
export function themeColors(t: MadeTheme): Record<string, string> {
  const v = t.vars
  return { bg: v.bg, bg2: v['bg-2'], bg3: v['bg-3'], bg4: v['bg-4'], fg: v.fg, fg2: v['fg-2'], fg3: v['fg-3'], accent: v.accent, accentHover: v['accent-2'], accentFg: v['accent-fg'], accentText: v['accent-text'] }
}
