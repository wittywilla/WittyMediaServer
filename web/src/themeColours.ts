// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// The colour rules of the server's theme engine (docs/THEMES.md "The theme engine"), for the
// settings page that edits its overrides: which colours there are, what counts as a colour,
// and which text has to stay readable on what. Everything here is the server's own rule
// (internal/themes: NormalizeColor, CheckContrast; scripts/theme-contrast.py), written again
// so the page can say what a colour does while it is still being typed. The server has the
// last word: its answer after saving is what the page shows from then on.

const tk = <S extends string>(english: S): S => english // marks text translated where it's shown (no i18n import: node tests load this file)

export type Colours = Record<string, string>

// ---- The colours a theme has ----

export type TokenInfo = { name: string; label: string; hint: string }
export type TokenGroup = { id: string; title: string; tokens: TokenInfo[] }

// Every colour name the built-in themes give, in the order a person meets them. The server
// says which names it takes (the built-in themes' `colors`); a name it has that isn't here
// yet is listed under "Other" by tokenGroups, so nothing it accepts is out of reach.
export const TOKEN_GROUPS: TokenGroup[] = [
  { id: 'surfaces', title: tk('Surfaces'), tokens: [
    { name: 'bg', label: tk('Page'), hint: tk('The page background') },
    { name: 'bg2', label: tk('Panels'), hint: tk('Side menu, panels and dialogs') },
    { name: 'bg3', label: tk('Cards and fields'), hint: tk('Fields, cards and tiles') },
    { name: 'bg4', label: tk('Hovered cards'), hint: tk('A card or field under the pointer, chip outlines') },
  ] },
  { id: 'text', title: tk('Text'), tokens: [
    { name: 'fg', label: tk('Main text'), hint: tk('Titles and everything you read') },
    { name: 'fg2', label: tk('Secondary text'), hint: tk('Meta lines, overviews, inactive tabs') },
    { name: 'fg3', label: tk('Hints'), hint: tk('Hints, captions and section labels') },
    { name: 'onImage', label: tk('Text over pictures'), hint: tk('Words and icons on artwork') },
  ] },
  { id: 'accent', title: tk('Accent'), tokens: [
    { name: 'accent', label: tk('Accent'), hint: tk('Primary buttons, progress bars, selected items') },
    { name: 'accentHover', label: tk('Accent, hovered'), hint: tk('An accent button under the pointer or pressed') },
    { name: 'accentFg', label: tk('Text on the accent'), hint: tk('The words and icons on an accent button') },
    { name: 'accentText', label: tk('Links'), hint: tk('The accent as words: links and highlighted text') },
    { name: 'focus', label: tk('Focus ring'), hint: tk('The outline around what a keyboard or remote is on') },
    { name: 'brandAccent', label: tk('Name highlight'), hint: tk('The highlighted part of the server’s name') },
  ] },
  { id: 'status', title: tk('Status'), tokens: [
    { name: 'danger', label: tk('Danger'), hint: tk('Errors and delete buttons') },
    { name: 'ok', label: tk('Success'), hint: tk('Messages that something worked') },
    { name: 'warn', label: tk('Warning'), hint: tk('Warnings and the rating star') },
  ] },
  { id: 'lines', title: tk('Lines'), tokens: [
    { name: 'border', label: tk('Hairlines'), hint: tk('Lines around panels, lists and tiles') },
    { name: 'borderStrong', label: tk('Strong lines'), hint: tk('Lines that need to show more') },
    { name: 'controlLine', label: tk('Field outlines'), hint: tk('Outlines of fields and switches') },
  ] },
  { id: 'shades', title: tk('Shades'), tokens: [
    { name: 'hover', label: tk('Hover shade'), hint: tk('Laid over a thing under the pointer') },
    { name: 'pressed', label: tk('Pressed shade'), hint: tk('Laid over a thing being pressed') },
    { name: 'accentSoft', label: tk('Selected row'), hint: tk('The tint behind a selected row') },
    { name: 'accentLine', label: tk('Selected outline'), hint: tk('The line around a selected thing') },
    { name: 'okSoft', label: tk('Success tint'), hint: tk('Behind a success message') },
    { name: 'warnSoft', label: tk('Warning tint'), hint: tk('Behind a warning') },
    { name: 'dangerSoft', label: tk('Danger tint'), hint: tk('Behind an error') },
    { name: 'scrim', label: tk('Plate over pictures'), hint: tk('Behind words drawn on artwork') },
    { name: 'overlay', label: tk('Behind dialogs'), hint: tk('The dimmed page behind a dialog') },
  ] },
]

const KNOWN = new Map(TOKEN_GROUPS.flatMap((g) => g.tokens.map((t) => [t.name, t] as const)))

// A token's plain name for a sentence ("Links", "Cards and fields"); its own name when it's new.
export const tokenLabel = (name: string) => KNOWN.get(name)?.label || name

// tokenGroups: the groups to show for the names the server takes. Names it doesn't take are
// left out; names this page has no words for yet come last, under "Other".
export function tokenGroups(serverTokens: Iterable<string>): TokenGroup[] {
  const have = new Set(serverTokens)
  const out = TOKEN_GROUPS.map((g) => ({ ...g, tokens: g.tokens.filter((t) => have.has(t.name)) })).filter((g) => g.tokens.length > 0)
  const other = [...have].filter((n) => !KNOWN.has(n) && /^[a-z][A-Za-z0-9]*$/.test(n)).sort()
  if (other.length) out.push({ id: 'other', title: tk('Other'), tokens: other.map((name) => ({ name, label: name, hint: tk('A colour this theme has') })) })
  return out
}

// ---- What counts as a colour (themes.NormalizeColor) ----

const HEX = /^#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/
const RGB = /^rgba?\(\s*([0-9.]+)[\s,]+([0-9.]+)[\s,]+([0-9.]+)(?:[\s,/]+([0-9.]+%?))?\s*\)$/
const hex2 = (n: number) => n.toString(16).toUpperCase().padStart(2, '0')
// A plain decimal number, as Go's ParseFloat reads the digits and dots the pattern lets through.
const num = (s: string) => (/^(\d+\.?\d*|\.\d+)$/.test(s) ? Number(s) : NaN)

// What the server says when a value isn't one, so the field says the same before and after saving.
export const NOT_A_COLOUR = tk('not a colour (#rrggbb, #rrggbbaa or rgb()/rgba())')

// normalizeColour turns the forms the server takes (#rgb, #rgba, #rrggbb, #rrggbbaa, rgb(),
// rgba(), and the words transparent, black and white) into #RRGGBB or #RRGGBBAA, the way the
// server stores and answers them. null: the server would refuse it.
export function normalizeColour(value: string): string | null {
  const v = value.trim()
  switch (v.toLowerCase()) {
    case 'transparent': return '#00000000'
    case 'black': return '#000000'
    case 'white': return '#FFFFFF'
  }
  if (HEX.test(v)) {
    const h = v.slice(1).toUpperCase()
    return '#' + (h.length <= 4 ? [...h].map((c) => c + c).join('') : h)
  }
  const m = RGB.exec(v)
  if (!m) return null
  let out = '#'
  for (let i = 1; i <= 3; i++) {
    const f = num(m[i])
    if (Number.isNaN(f) || f > 255) return null
    out += hex2(Math.round(f))
  }
  if (m[4]) {
    const pct = m[4].endsWith('%')
    let a = num(pct ? m[4].slice(0, -1) : m[4])
    if (Number.isNaN(a)) return null
    if (pct) a /= 100
    out += hex2(Math.round(Math.min(Math.max(a, 0), 1) * 255))
  }
  return out
}

// pickerValue: a colour as the browser's colour picker wants it, #rrggbb. A picker has no
// see-through, so that part is left off here and put back by withPicked.
export function pickerValue(colour: string, fallback = '#000000'): string {
  const n = normalizeColour(colour)
  return (n ? n.slice(0, 7) : fallback).toLowerCase()
}

// withPicked: what the picker chose, keeping how see-through the colour was before.
export function withPicked(picked: string, before: string): string {
  const n = normalizeColour(before)
  const alpha = n && n.length === 9 && n.slice(7) !== 'FF' ? n.slice(7) : ''
  return (pickerValue(picked) + alpha).toUpperCase()
}

// ---- Contrast (themes.CheckContrast, scripts/theme-contrast.py) ----

// WCAG AA for normal text.
export const CONTRAST_AA = 4.5

// The text colours and the surfaces each has to be readable on: the pairs the built-in
// themes are held to.
export const CONTRAST_PAIRS: [text: string, surface: string][] = [
  ...['fg', 'fg2', 'fg3', 'accentText', 'danger', 'ok', 'warn'].flatMap((t) => ['bg', 'bg2', 'bg3'].map((s) => [t, s] as [string, string])),
  ['accentFg', 'accent'],
]

// A colour as three 0..1 channels; null when it isn't one or is at all see-through (what
// shows through isn't known here, so such a pair isn't judged).
function opaque(colour: string | undefined): [number, number, number] | null {
  const n = colour === undefined ? null : normalizeColour(colour)
  if (!n || (n.length === 9 && n.slice(7) !== 'FF')) return null
  return [1, 3, 5].map((i) => parseInt(n.slice(i, i + 2), 16) / 255) as [number, number, number]
}

function luminance(c: [number, number, number]) {
  const [r, g, b] = c.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

// contrastRatio is the WCAG contrast ratio of two opaque colours, 1 to 21. null when either
// isn't an opaque colour.
export function contrastRatio(a: string | undefined, b: string | undefined): number | null {
  const ca = opaque(a), cb = opaque(b)
  if (!ca || !cb) return null
  const la = luminance(ca), lb = luminance(cb)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

export type ContrastIssue = { text: string; surface: string; ratio: number; need: number }

// checkContrast lists the pairs in colours below AA, with the ratio to two places as the
// server gives it. With `only`, a pair counts only when one of its two colours is in it:
// the colours an override changed.
export function checkContrast(colours: Colours, only?: Colours | null): ContrastIssue[] {
  const out: ContrastIssue[] = []
  for (const [text, surface] of CONTRAST_PAIRS) {
    if (only && !(text in only) && !(surface in only)) continue
    const r = contrastRatio(colours[text], colours[surface])
    if (r !== null && r < CONTRAST_AA) out.push({ text, surface, ratio: Math.round(r * 100) / 100, need: CONTRAST_AA })
  }
  return out
}

// ---- Overrides ----

// overlay: base with over on top, as the server layers a theme, the global overrides and a
// module's.
export const overlay = (base: Colours, over: Colours | null | undefined): Colours => ({ ...base, ...(over || {}) })

// normalizedColours: a set of colours as the server answers them; what isn't a colour is dropped.
export function normalizedColours(colours: Colours | null | undefined): Colours {
  const out: Colours = {}
  for (const [k, v] of Object.entries(colours || {})) { const n = normalizeColour(v); if (n) out[k] = n }
  return out
}

// What is being typed, by colour name. An empty or missing entry means the theme's own colour.
export type Draft = Record<string, string>

// draftTokens: what a draft would send. `tokens` holds every entry that is a colour,
// normalised; `bad` names the ones that aren't, which keep the save from going out.
export function draftTokens(draft: Draft): { tokens: Colours; bad: string[] } {
  const tokens: Colours = {}, bad: string[] = []
  for (const [name, value] of Object.entries(draft)) {
    if (!value.trim()) continue
    const n = normalizeColour(value)
    if (n) tokens[name] = n; else bad.push(name)
  }
  return { tokens, bad: bad.sort() }
}

// sameColours: two sets of overrides that would look the same.
export function sameColours(a: Colours, b: Colours): boolean {
  const ka = Object.keys(a), kb = Object.keys(b)
  return ka.length === kb.length && ka.every((k) => k in b && a[k].toUpperCase() === b[k].toUpperCase())
}

// tokenErrors reads the server's refusal of a set of overrides ("accent: not a colour (…);
// nope: not a colour token themes have. Known tokens: …") into its words for each colour, to
// show at that colour's field. Anything that isn't about one of `names` is in `other`, whole.
export function tokenErrors(message: string, names: Iterable<string>): { fields: Record<string, string>; other: string } {
  const known = new Set(names)
  const fields: Record<string, string> = {}
  const cut = message.indexOf('. Known tokens:')
  const body = cut >= 0 ? message.slice(0, cut) : message
  const rest: string[] = []
  for (const part of body.split('; ')) {
    const at = part.indexOf(': ')
    const name = at > 0 ? part.slice(0, at) : ''
    if (name && known.has(name)) fields[name] = part.slice(at + 2); else if (part.trim()) rest.push(part)
  }
  return { fields, other: rest.length ? (Object.keys(fields).length ? rest.join('; ') : message) : '' }
}
