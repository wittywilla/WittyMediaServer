// WittyTV's themes, apart from the app's theme: what each one is, the colours it works out
// to, and how older saved looks become one. Nothing here touches the page, so it can be tested
// with any colour someone might pick (tvThemes.test.mjs). look.ts is what the screens use.

// A shape is how the guide is built: its proportions, corners, rules and type. tv.css and
// tvthemes.css draw each one. A theme is a shape with colours and a name; Custom is a shape and
// colours someone chose.
export type Shape = 'box' | 'modern' | 'receiver' | 'teletext' | 'listings' | 'cinema' | 'outline' | 'compact'
export type ThemeId = 'modern' | 'classic' | 'satellite' | 'teletext' | 'listings' | 'bigpicture' | 'outline' | 'compact' | 'custom'
export type Font = '' | 'system' | 'narrow' | 'mono' | 'serif'

// The colours that can be chosen. Everything else is worked out from them.
export type Slot = 'bg' | 'cell' | 'live' | 'hi' | 'accent' | 'text'
export type Colours = Partial<Record<Slot, string>>
export const SLOTS: { slot: Slot; name: string; hint: string }[] = [
  { slot: 'bg', name: 'Background', hint: 'Behind everything, and the panels are worked out from it' },
  { slot: 'cell', name: 'Programmes', hint: 'The cells in the grid' },
  { slot: 'live', name: 'On now', hint: 'Cells for what is on at this moment' },
  { slot: 'hi', name: 'Highlight', hint: 'What has the keyboard, and what is selected' },
  { slot: 'accent', name: 'Accent', hint: 'Channel numbers, the line at the current time, progress' },
  { slot: 'text', name: 'Text', hint: 'Words on the background' },
]

export type ShapeInfo = {
  name: string
  // Rows: phone, desktop and TV heights in pixels, before the viewer's row size.
  rows: [number, number, number]
  // How much of the page's width the preview starts at, and the side it starts on.
  preview: number
  side: 'right' | 'left'
  art: boolean // the programme's picture and title art at the top
  details: boolean // cells say time, year and genre, rather than the episode
  rowHighlight: boolean // the highlighted row's channel takes the highlight's colour
}

export const SHAPES: Record<Shape, ShapeInfo> = {
  box: { name: 'Cable box', rows: [56, 64, 72], preview: 0.34, side: 'right', art: false, details: false, rowHighlight: false },
  modern: { name: 'Modern', rows: [64, 76, 80], preview: 0.38, side: 'right', art: true, details: true, rowHighlight: true },
  receiver: { name: 'Receiver', rows: [56, 58, 70], preview: 0.34, side: 'right', art: false, details: false, rowHighlight: false },
  teletext: { name: 'Teletext', rows: [52, 50, 64], preview: 0.34, side: 'right', art: false, details: false, rowHighlight: false },
  listings: { name: 'Newspaper', rows: [60, 62, 72], preview: 0.34, side: 'right', art: false, details: false, rowHighlight: false },
  cinema: { name: 'Big picture', rows: [56, 54, 64], preview: 0.48, side: 'left', art: false, details: true, rowHighlight: true },
  outline: { name: 'Outline', rows: [60, 66, 76], preview: 0.36, side: 'right', art: true, details: true, rowHighlight: false },
  compact: { name: 'Compact', rows: [44, 40, 56], preview: 0.22, side: 'right', art: false, details: true, rowHighlight: false },
}

export type TvTheme = { id: ThemeId; name: string; blurb: string; shape: Shape; follows: boolean }

// follows: its colours come from the app's theme, until someone overrides them here.
export const THEMES: TvTheme[] = [
  { id: 'modern', name: 'Modern', blurb: 'Clean and calm, with the programme\'s picture up top and a big preview.', shape: 'modern', follows: true },
  { id: 'classic', name: 'Classic', blurb: 'The chunky blue box of the early 2000s, bevelled and square.', shape: 'box', follows: false },
  { id: 'satellite', name: 'Satellite', blurb: 'A receiver\'s display: black and amber, thin rules, digits like an LED.', shape: 'receiver', follows: false },
  { id: 'teletext', name: 'Teletext', blurb: 'Blocky letters and eight flat colours on black, page number and all.', shape: 'teletext', follows: false },
  { id: 'listings', name: 'Newspaper', blurb: 'The listings page: ink on paper, serif type and hairline rules.', shape: 'listings', follows: false },
  { id: 'bigpicture', name: 'Big picture', blurb: 'The preview comes first and takes half the width, over a short grid.', shape: 'cinema', follows: true },
  { id: 'outline', name: 'Outline', blurb: 'Light and open: cells are outlines, and only the highlight is filled.', shape: 'outline', follows: true },
  { id: 'compact', name: 'Compact', blurb: 'Small type and tight rows, for seeing the most channels at once.', shape: 'compact', follows: true },
  { id: 'custom', name: 'Custom', blurb: 'Your own: choose the layout, type and colours, and it stays yours.', shape: 'modern', follows: true },
]

export const themeById = (id: string | undefined) => THEMES.find((t) => t.id === id)
export const isThemeId = (v: unknown): v is ThemeId => typeof v === 'string' && THEMES.some((t) => t.id === v)
export const isShape = (v: unknown): v is Shape => typeof v === 'string' && v in SHAPES

// ------------------------------------------------------------------ colours

const rgb = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
const toHex = (c: number[]) => '#' + c.map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')).join('')
export const isHex = (v: unknown): v is string => typeof v === 'string' && /^#[0-9a-fA-F]{6}$/.test(v)
export const mix = (a: string, b: string, t: number) => { const x = rgb(a), y = rgb(b); return toHex(x.map((v, i) => v + (y[i] - v) * t)) }
const lum = (hex: string) => { const f = (v: number) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4 }; const [r, g, b] = rgb(hex); return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
export const contrast = (a: string, b: string) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
export const readableOn = (bg: string) => (contrast(bg, '#ffffff') >= contrast(bg, '#000000') ? '#ffffff' : '#000000')

const AA = 4.5

// A colour moved towards black or white until it reads on all of these surfaces. When no
// colour can (a black surface and a white one), it reads on the first of them, which is
// what it mostly sits on.
function legible(colour: string, surfaces: string[], need = AA): string {
  const ok = (c: string) => surfaces.every((s) => contrast(c, s) >= need)
  for (const towards of [readableOn(surfaces[0]), readableOn(colour)]) {
    let c = colour
    for (let i = 0; i < 12 && !ok(c); i++) c = mix(c, towards, 0.15)
    if (ok(c)) return c
    if (ok(towards)) return towards
  }
  return readableOn(surfaces[0])
}

// A surface moved back towards one the text already reads on, until the text reads on it too.
function underText(surface: string, text: string, readsOn: string): string {
  let s = surface
  for (let i = 0; i < 12 && contrast(text, s) < AA; i++) s = mix(s, readsOn, 0.2)
  return contrast(text, s) >= AA ? s : readsOn
}

// The softer words on something: as soft as can still be read.
function softOn(text: string, surfaces: string[]): string {
  for (const t of [0.24, 0.16, 0.08]) {
    const c = mix(text, surfaces[0], t)
    if (surfaces.every((s) => contrast(c, s) >= AA)) return c
  }
  return text
}

export type Tokens = Record<string, string>

// Text for a cell and its gradient's other end: a cell can be any colour, and the page behind
// it another, so the words on it are worked out from the cell.
const onCells = (text: string, cell: string, second: string) => {
  const t = contrast(text, cell) >= AA ? text : readableOn(cell)
  const cell2 = underText(second, t, cell)
  return { text: t, cell2, soft: softOn(t, [cell, cell2]) }
}

// The whole set of tokens from the colours that can be chosen. Every pairing the guide draws
// has AA contrast, whatever came in.
export function deriveTokens(c: Required<Colours>, base?: Tokens): Tokens {
  const bg = c.bg
  const text = legible(c.text, [bg])
  const lift = (t: number) => underText(mix(bg, text, t), text, bg)
  const fromBase = (k: string, t: number) => (base && base['--ctv-bg'] === bg ? underText(base[k], text, bg) : lift(t))
  const panel = fromBase('--ctv-panel', 0.06), panel2 = fromBase('--ctv-panel-2', 0.12), head = fromBase('--ctv-head', 0.14), ch = fromBase('--ctv-ch', 0.06), dim = fromBase('--ctv-dim', 0.06)
  const page = [bg, panel, panel2, head, ch, dim]
  const accent = legible(c.accent, page)
  const hi = legible(c.hi, [bg], 3)
  const cell = c.cell, live = c.live
  const onCell = onCells(text, cell, base && base['--ctv-cell'] === cell ? base['--ctv-cell-2'] : mix(cell, bg, 0.18))
  const onLive = onCells(text, live, base && base['--ctv-live'] === live ? base['--ctv-live-2'] : mix(live, bg, 0.18))
  const hiText = readableOn(hi)
  const key = (k: string, fallback: string) => (base && base['--ctv-accent'] === accent && base[k] ? base[k] : fallback)
  return {
    '--ctv-bg': bg, '--ctv-panel': panel, '--ctv-panel-2': panel2, '--ctv-head': head, '--ctv-ch': ch, '--ctv-dim': dim,
    '--ctv-line': legible(fromBase('--ctv-line', 0.34), [bg], 3),
    '--ctv-text': text, '--ctv-soft': softOn(text, page),
    '--ctv-cell': cell, '--ctv-cell-2': onCell.cell2, '--ctv-cell-text': onCell.text, '--ctv-cell-soft': onCell.soft,
    '--ctv-live': live, '--ctv-live-2': onLive.cell2, '--ctv-live-text': onLive.text, '--ctv-live-soft': onLive.soft,
    '--ctv-hi': hi, '--ctv-hi-text': hiText, '--ctv-accent': accent, '--ctv-accent-text': readableOn(accent),
    '--ctv-key-a': key('--ctv-key-a', accent), '--ctv-key-b': key('--ctv-key-b', accent), '--ctv-key-c': key('--ctv-key-c', accent), '--ctv-key-text': readableOn(key('--ctv-key-a', accent)),
    '--ctv-err': legible(base?.['--ctv-err'] && base['--ctv-bg'] === bg ? base['--ctv-err'] : accent, page),
    '--ctv-lite': base?.['--ctv-lite'] && base['--ctv-bg'] === bg ? base['--ctv-lite'] : readableOn(bg) === '#ffffff' ? 'rgb(255 255 255 / .16)' : 'rgb(0 0 0 / .1)',
    '--ctv-shade': readableOn(hi) === '#ffffff' ? 'rgb(255 255 255 / .2)' : 'rgb(0 0 0 / .14)',
  }
}

// The colours of the themes that have their own. These are the choices someone starts from
// when overriding one, and deriveTokens keeps what was tuned by hand for what isn't changed.
const OWN: Partial<Record<ThemeId, Tokens>> = {
  classic: {
    '--ctv-bg': '#06164a', '--ctv-panel': '#0a2270', '--ctv-panel-2': '#123391', '--ctv-head': '#16389e', '--ctv-cell': '#1a43b8', '--ctv-cell-2': '#143594', '--ctv-live': '#0f6a7f', '--ctv-live-2': '#0b5466', '--ctv-ch': '#0c2a84', '--ctv-dim': '#0d2a7c', '--ctv-line': '#6f94f2',
    '--ctv-text': '#ffffff', '--ctv-soft': '#d6e2ff', '--ctv-hi': '#ffd60a', '--ctv-accent': '#ffd60a', '--ctv-key-a': '#ffd60a', '--ctv-key-b': '#4ce06a', '--ctv-key-c': '#7fc7ff', '--ctv-err': '#ffe08a', '--ctv-lite': 'rgb(255 255 255 / .16)',
  },
  satellite: {
    '--ctv-bg': '#07090d', '--ctv-panel': '#10131a', '--ctv-panel-2': '#22262f', '--ctv-head': '#14171e', '--ctv-cell': '#14171e', '--ctv-cell-2': '#14171e', '--ctv-live': '#2e2300', '--ctv-live-2': '#2e2300', '--ctv-ch': '#10131a', '--ctv-dim': '#0c0e13', '--ctv-line': '#6a7181',
    '--ctv-text': '#ffffff', '--ctv-soft': '#dcdfe6', '--ctv-hi': '#ffb300', '--ctv-accent': '#ffb300', '--ctv-key-a': '#ffb300', '--ctv-key-b': '#5fd47d', '--ctv-key-c': '#8cc6ff', '--ctv-err': '#ffd27a', '--ctv-lite': 'rgb(255 179 0 / .18)',
  },
  teletext: {
    '--ctv-bg': '#000000', '--ctv-panel': '#000000', '--ctv-panel-2': '#0000d0', '--ctv-head': '#0000d0', '--ctv-cell': '#000000', '--ctv-cell-2': '#000000', '--ctv-live': '#003c00', '--ctv-live-2': '#003c00', '--ctv-ch': '#000000', '--ctv-dim': '#000000', '--ctv-line': '#00d8d8',
    '--ctv-text': '#ffffff', '--ctv-soft': '#00ffff', '--ctv-hi': '#00ffff', '--ctv-accent': '#ffff00', '--ctv-key-a': '#ff5a5a', '--ctv-key-b': '#00ff00', '--ctv-key-c': '#ffff00', '--ctv-err': '#ff9d9d', '--ctv-lite': 'rgb(0 255 0 / .22)',
  },
  listings: {
    '--ctv-bg': '#f5f0e4', '--ctv-panel': '#f5f0e4', '--ctv-panel-2': '#eae3d2', '--ctv-head': '#f5f0e4', '--ctv-cell': '#f5f0e4', '--ctv-cell-2': '#f5f0e4', '--ctv-live': '#ebdfb8', '--ctv-live-2': '#ebdfb8', '--ctv-ch': '#f5f0e4', '--ctv-dim': '#efe9db', '--ctv-line': '#6b6252',
    '--ctv-text': '#1b1813', '--ctv-soft': '#4d463a', '--ctv-hi': '#1b1813', '--ctv-accent': '#9c1c12', '--ctv-key-a': '#1b1813', '--ctv-key-b': '#1b1813', '--ctv-key-c': '#1b1813', '--ctv-err': '#9c1c12', '--ctv-lite': 'rgb(156 28 18 / .12)',
  },
}

// The tokens a theme draws with while it follows the app's theme: only the app's own colours,
// so a change of app theme is a change here too, with no work.
function followingTokens(hi: 'fg' | 'accent'): Tokens {
  const hiBg = hi === 'fg' ? 'var(--fg)' : 'var(--accent)', hiFg = hi === 'fg' ? 'var(--bg)' : 'var(--accent-fg)'
  return {
    '--ctv-bg': 'var(--bg)', '--ctv-panel': 'var(--bg-2)', '--ctv-panel-2': 'var(--bg-3)', '--ctv-head': 'var(--bg-2)', '--ctv-cell': 'var(--bg-3)', '--ctv-cell-2': 'var(--bg-3)', '--ctv-live': 'var(--bg-4)', '--ctv-live-2': 'var(--bg-4)', '--ctv-ch': 'var(--bg-2)', '--ctv-dim': 'var(--bg-2)', '--ctv-line': 'var(--border-strong, var(--bg-4))',
    '--ctv-text': 'var(--fg)', '--ctv-soft': 'var(--fg-2)', '--ctv-cell-text': 'var(--fg)', '--ctv-cell-soft': 'var(--fg-2)', '--ctv-live-text': 'var(--fg)', '--ctv-live-soft': 'var(--fg-2)',
    '--ctv-hi': hiBg, '--ctv-hi-text': hiFg, '--ctv-accent': 'var(--accent-text)', '--ctv-accent-text': 'var(--bg)',
    '--ctv-key-a': hiBg, '--ctv-key-b': hiBg, '--ctv-key-c': hiBg, '--ctv-key-text': hiFg, '--ctv-err': 'var(--danger)',
    '--ctv-lite': 'rgb(var(--accent-rgb) / .24)', '--ctv-shade': 'rgb(var(--bg-rgb) / .24)',
  }
}

// What the app's theme gives, as plain colours. Only needed to work something out from them.
export type AppColours = { bg: string; bg2: string; bg3: string; bg4: string; fg: string; fg2: string; accent: string; accentText: string; accentFg: string; border: string }
export const FALLBACK_APP: AppColours = { bg: '#0f1115', bg2: '#16181d', bg3: '#23262d', bg4: '#30343c', fg: '#ffffff', fg2: '#c9ccd3', accent: '#7955ff', accentText: '#b39dff', accentFg: '#ffffff', border: '#4a4f5a' }

const followingHi = (id: ThemeId): 'fg' | 'accent' => (id === 'modern' || id === 'bigpicture' ? 'fg' : 'accent')

// A following theme's tokens, as plain colours.
function followingAsColours(id: ThemeId, app: AppColours): Tokens {
  const hi = followingHi(id) === 'fg' ? app.fg : app.accent
  return {
    '--ctv-bg': app.bg, '--ctv-panel': app.bg2, '--ctv-panel-2': app.bg3, '--ctv-head': app.bg2, '--ctv-cell': app.bg3, '--ctv-cell-2': app.bg3, '--ctv-live': app.bg4, '--ctv-live-2': app.bg4, '--ctv-ch': app.bg2, '--ctv-dim': app.bg2, '--ctv-line': app.border,
    '--ctv-text': app.fg, '--ctv-soft': app.fg2, '--ctv-hi': hi, '--ctv-accent': app.accentText,
  }
}

const hasColours = (c?: Colours) => !!c && Object.values(c).some(isHex)

// What a theme's colours are before anything is overridden: for showing in the colour pickers.
export function baseColours(id: ThemeId, app: AppColours = FALLBACK_APP): Required<Colours> {
  const t = OWN[id] ?? followingAsColours(id, app)
  return { bg: t['--ctv-bg'], cell: t['--ctv-cell'], live: t['--ctv-live'], hi: t['--ctv-hi'], accent: t['--ctv-accent'], text: t['--ctv-text'] }
}

// themeTokens: every colour token for a theme, with its overrides.
export function themeTokens(id: ThemeId, overrides?: Colours, app: AppColours = FALLBACK_APP): Tokens {
  const own = OWN[id]
  if (!hasColours(overrides)) {
    if (!own) return followingTokens(followingHi(id))
    return deriveTokens(baseColours(id, app), own)
  }
  const base = own ?? followingAsColours(id, app)
  const start = baseColours(id, app)
  const set = Object.fromEntries(Object.entries(overrides!).filter(([, v]) => isHex(v))) as Colours
  // A new background with no word on the text: the text goes with it, and so do cells
  // that were worked out for the old one.
  const text = set.text ?? (set.bg ? readableOn(set.bg) : start.text)
  const bg = set.bg ?? start.bg
  const cell = set.cell ?? (set.bg ? mix(bg, readableOn(bg), 0.14) : start.cell)
  const accent = set.accent ?? start.accent
  const live = set.live ?? (set.bg || set.accent || set.cell ? mix(cell, accent, 0.22) : start.live)
  return deriveTokens({ bg, cell, live, hi: set.hi ?? start.hi, accent, text }, base)
}

// The pairings the guide draws, to hold it to them: text colour, then what it sits on.
export const PAIRS: [string, string, number][] = [
  ...['--ctv-bg', '--ctv-panel', '--ctv-panel-2', '--ctv-head', '--ctv-ch', '--ctv-dim'].flatMap((s): [string, string, number][] => [['--ctv-text', s, AA], ['--ctv-soft', s, AA], ['--ctv-accent', s, AA]]),
  ['--ctv-cell-text', '--ctv-cell', AA], ['--ctv-cell-text', '--ctv-cell-2', AA], ['--ctv-cell-soft', '--ctv-cell', AA], ['--ctv-cell-soft', '--ctv-cell-2', AA],
  ['--ctv-live-text', '--ctv-live', AA], ['--ctv-live-text', '--ctv-live-2', AA], ['--ctv-live-soft', '--ctv-live', AA], ['--ctv-live-soft', '--ctv-live-2', AA],
  ['--ctv-hi-text', '--ctv-hi', AA], ['--ctv-accent-text', '--ctv-accent', AA], ['--ctv-key-text', '--ctv-key-a', AA],
  ['--ctv-hi', '--ctv-bg', 3], ['--ctv-line', '--ctv-bg', 3],
]

// ------------------------------------------------------------------ type

export const FONTS: { v: Font; name: string }[] = [
  { v: '', name: 'As the layout has it' }, { v: 'system', name: 'Plain' }, { v: 'narrow', name: 'Narrow' }, { v: 'mono', name: 'Typewriter' }, { v: 'serif', name: 'Newspaper' },
]
export const FONT_STACKS: Record<Exclude<Font, ''>, string> = {
  system: 'var(--font)',
  narrow: '"Arial Narrow", "Helvetica Neue", Arial, "Liberation Sans Narrow", system-ui, sans-serif',
  mono: 'var(--font-mono)',
  serif: 'Georgia, "Times New Roman", "Liberation Serif", serif',
}

// ------------------------------------------------------------------ what's saved

// What a viewer has chosen. Anything left out hasn't been chosen, and comes from elsewhere.
export type Saved = {
  theme?: ThemeId
  custom?: { shape: Shape; font: Font }
  themeColours?: Partial<Record<ThemeId, Colours>> // overrides, per theme
  density?: 'compact' | 'normal' | 'large'
  hours?: 0 | 1.5 | 2 | 3 | 4
  nowLine?: boolean
  progress?: boolean
  subtitles?: boolean
  logos?: boolean
  art?: boolean
  preview?: 'highlight' | 'last' | 'off'
  previewSide?: 'right' | 'left'
  previewW?: number
  previewSound?: boolean
}

export type OwnerLook = { theme?: string; layout?: string; skin?: string }

const LEGACY_COLOURS: Record<string, Colours> = {
  midnight: { bg: '#000000', cell: '#2a2a2a', live: '#3a3a3a', hi: '#ffffff', accent: '#c9a2ff' },
  cable: { bg: '#06164a', cell: '#1a43b8', live: '#0f6a7f', hi: '#ffd60a', accent: '#ffd60a' },
  satellite: { bg: '#07090d', cell: '#2b3039', live: '#4d3700', hi: '#ffb300', accent: '#ffb300' },
}

// The two earlier choices, a layout and a set of colours, as one theme. Every pair has an exact
// home, so nobody's look changes in moving over: the pairs that were built into a theme become
// it, and the rest become that theme's overrides or Custom.
export function fromLegacy(layout: string | undefined, colours: string | undefined, customBg?: unknown, customAccent?: unknown): Pick<Saved, 'theme' | 'custom' | 'themeColours'> {
  const own: Colours = isHex(customBg) && isHex(customAccent) ? { bg: customBg, accent: customAccent, hi: customAccent } : {}
  if (layout === 'streaming') {
    if (colours === 'theme') return { theme: 'modern' }
    return { theme: 'modern', themeColours: { modern: colours === 'custom' ? own : LEGACY_COLOURS[colours || 'midnight'] || LEGACY_COLOURS.midnight } }
  }
  if (colours === 'satellite') return { theme: 'satellite' }
  if (colours === 'midnight') return { theme: 'custom', custom: { shape: 'box', font: '' }, themeColours: { custom: LEGACY_COLOURS.midnight } }
  if (colours === 'theme') return { theme: 'custom', custom: { shape: 'box', font: 'system' } }
  if (colours === 'custom') return { theme: 'custom', custom: { shape: 'box', font: '' }, themeColours: { custom: own } }
  return { theme: 'classic' }
}

// Reads what was saved in any earlier form. A look saved before there were themes held a layout,
// colours or both. What it left out came from the owner's starting look at the time, as now.
export function upgradeSaved(raw: Record<string, unknown>, owner: OwnerLook = {}): Saved {
  const out = { ...raw } as Saved & Record<string, unknown>
  if (typeof raw.previewPos === 'string' && !raw.previewSide) out.previewSide = raw.previewPos === 'tl' || raw.previewPos === 'bl' ? 'left' : 'right'
  if (typeof raw.previewSize === 'string' && !raw.previewW) {
    const w = ({ s: 220, m: 320, l: 460 } as Record<string, number>)[raw.previewSize]
    if (w) out.previewW = w
  }
  if (!isThemeId(raw.theme) && (typeof raw.layout === 'string' || typeof raw.colours === 'string')) {
    Object.assign(out, fromLegacy(typeof raw.layout === 'string' ? raw.layout : owner.layout, typeof raw.colours === 'string' ? raw.colours : owner.skin, raw.customBg, raw.customAccent))
  }
  for (const k of ['previewPos', 'previewSize', 'previewX', 'previewY', 'layout', 'colours', 'customBg', 'customAccent']) delete out[k]
  if (!isThemeId(out.theme)) delete out.theme
  if (out.custom && !isShape(out.custom.shape)) delete out.custom
  return out
}

// What a theme says for clients that only know the two earlier looks. Kept beside the theme in
// the account's saved look, and the guide API's layout and skin come from the same table.
export function legacyOf(id: ThemeId, shape: Shape): { layout: 'classic' | 'streaming'; colours: 'cable' | 'satellite' | 'midnight' | 'theme' } {
  if (id === 'satellite') return { layout: 'classic', colours: 'satellite' }
  if (id === 'classic' || id === 'teletext') return { layout: 'classic', colours: 'cable' }
  if (shape === 'box') return { layout: 'classic', colours: 'theme' }
  return { layout: 'streaming', colours: 'theme' }
}

// ------------------------------------------------------------------ which wins

export type Suggestion = { theme?: string; layout?: string; colours?: string }
export type ThemeChoice = { theme: ThemeId; source: 'you' | 'app' | 'owner'; custom?: Saved['custom']; colours?: Partial<Record<ThemeId, Colours>> }

// A viewer's own choice wins. Then what the app's theme suggests, then the owner's starting
// look. This is the one place that says so; the Look panel and docs/TV.md say it in words.
export function chooseTheme(saved: Saved, suggestion: Suggestion, owner: OwnerLook): ThemeChoice {
  if (saved.theme) return { theme: saved.theme, source: 'you' }
  if (isThemeId(suggestion.theme) && suggestion.theme !== 'custom') return { theme: suggestion.theme, source: 'app' }
  if (suggestion.layout || suggestion.colours) {
    const l = fromLegacy(suggestion.layout || owner.layout, suggestion.colours || owner.skin)
    return { theme: l.theme!, source: 'app', custom: l.custom, colours: l.themeColours }
  }
  if (isThemeId(owner.theme) && owner.theme !== 'custom') return { theme: owner.theme, source: 'owner' }
  if (owner.layout || owner.skin) {
    const l = fromLegacy(owner.layout, owner.skin)
    return { theme: l.theme!, source: 'owner', custom: l.custom, colours: l.themeColours }
  }
  return { theme: 'classic', source: 'owner' }
}

// ------------------------------------------------------------------ the preview's size

export const PREVIEW_MIN = 180

// What the page can spare: never so much that the words beside it or the guide below have no
// room. A phone gives it the whole width, on a line of its own.
export function previewLimit(pageW: number, pageH: number, narrow: boolean, gutter = 16): number {
  const byWidth = narrow ? pageW - gutter * 2 : pageW * 0.6
  const byHeight = (pageH * (narrow ? 0.34 : 0.46) - 44) * (16 / 9)
  return Math.round(Math.max(PREVIEW_MIN, Math.min(byWidth, byHeight)))
}

// How big it starts: a share of the window that suits the theme, held to what fits. This is
// what someone gets until they choose a size, and the window can change it, so it reads the
// same on a laptop and across the room.
export function previewAuto(shape: Shape, pageW: number, pageH: number, narrow: boolean, gutter = 16): number {
  const max = previewLimit(pageW, pageH, narrow, gutter)
  if (narrow) return shape === 'compact' ? Math.round(Math.min(max, pageW * 0.6)) : max
  return Math.round(Math.min(max, Math.max(PREVIEW_MIN, pageW * SHAPES[shape].preview)))
}

export const rowHeight = (shape: Shape, width: number, density: 'compact' | 'normal' | 'large') => {
  const [phone, desk, tv] = SHAPES[shape].rows
  return Math.round((width < 560 ? phone : width >= 1800 ? tv : desk) * { compact: 0.8, normal: 1, large: 1.25 }[density])
}
