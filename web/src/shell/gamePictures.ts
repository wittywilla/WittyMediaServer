// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How WittyPlay pictures each game (gamePictures.test.mjs): the box art fitted into its tile over a blurred copy of itself, the box art
// filling the tile, or the game drawn in a cartridge or case like the ones its console used (GameCase.tsx). Chosen for every game, per
// console and per game, by the person (their /me/ui, key PLAY_PICTURES_KEY) and by the server (an administrator's choices and .wittyplay
// files beside games, GET /api/games/pictures). The words are the .wittyplay format's (docs/specs/wittyplay-file.md), so the person's
// choices export as a .wittyplay file and import from one.

export type PicturePick = 'fitted' | 'filled' | 'case'
export type LabelPick = 'boxart' | 'logo' | 'title'
// A look at one level: every field optional, an empty one decided by the level above. A file may carry more fields; a person's own
// copy (/me/ui) keeps only these three, and the server keeps a file's others (docs/specs/wittyplay-file.md, Unknown fields).
export type Look = { picture?: PicturePick; case?: string; label?: LabelPick; [more: string]: unknown }
// The person's own choices, as kept in /me/ui: compact, games by this server's id.
export type PicturesPref = { default?: Look; consoles?: Record<string, Look>; games?: Record<string, Look> }
export const PLAY_PICTURES_KEY = 'wittyplayPictures'
// /me/ui holds 16 KB in all, so a person keeps their own look for at most this many games (the oldest choice goes first).
export const MAX_PREF_GAMES = 150

export const PICTURES: { id: PicturePick; label: string; note: string }[] = [
  { id: 'fitted', label: 'Box art fitted', note: 'The whole picture, nothing cut off, over a soft blurred copy of itself' },
  { id: 'filled', label: 'Box art filled', note: 'The picture fills the tile; its edges may be cut off' },
  { id: 'case', label: 'In its case or cartridge', note: 'Drawn in a cartridge, disc case or tape like the ones its console used' },
]
export const LABELS: { id: LabelPick; label: string; note: string }[] = [
  { id: 'boxart', label: 'Box art', note: 'The game’s cover on the label' },
  { id: 'logo', label: 'Game logo', note: 'The game’s logo, when it has one; the cover when not' },
  { id: 'title', label: 'Title', note: 'The game’s name in type' },
]

export type CaseStyle =
  | 'cart-nes' | 'cart-atari' | 'cart-super' | 'cart-64' | 'cart-mega' | 'cart-pocket' | 'cart-advance' | 'card-slim'
  | 'case-jewel' | 'case-keep' | 'case-mini' | 'case-umd' | 'disk-floppy' | 'tape-cassette' | 'arcade-marquee' | 'generic-box'

// Every case WittyPlay draws, in the format's order. The words never name a maker or a product: they say the era and the shape.
export const CASE_STYLES: { id: CaseStyle; label: string; note: string }[] = [
  { id: 'cart-nes', label: '8-bit cartridge', note: 'A grey front-loading cartridge with ridges and a label window' },
  { id: 'cart-atari', label: 'Early cartridge', note: 'A short first-generation cartridge with a wide label' },
  { id: 'cart-super', label: '16-bit cartridge', note: 'Rounded at the top, with grip ridges' },
  { id: 'cart-64', label: '64-bit cartridge', note: 'Tall, with a ridged, sloping top' },
  { id: 'cart-mega', label: 'Black 16-bit cartridge', note: 'Black, with a slanted label top' },
  { id: 'cart-pocket', label: 'Handheld cartridge', note: 'Small, with a notched corner' },
  { id: 'cart-advance', label: 'Wide handheld cartridge', note: 'Wide and short' },
  { id: 'card-slim', label: 'Game card', note: 'A thin card' },
  { id: 'case-jewel', label: 'CD jewel case', note: 'A square clear case with a hinge' },
  { id: 'case-keep', label: 'DVD case', note: 'A tall case with a clear sleeve' },
  { id: 'case-mini', label: 'Mini disc case', note: 'A smaller, squarer disc case' },
  { id: 'case-umd', label: 'Small disc shell', note: 'A small disc in a shell with a window' },
  { id: 'disk-floppy', label: 'Floppy disk', note: 'A 3.5-inch disk with a shutter' },
  { id: 'tape-cassette', label: 'Cassette tape', note: 'A tape with two reels' },
  { id: 'arcade-marquee', label: 'Arcade marquee', note: 'A cabinet’s lit sign' },
  { id: 'generic-box', label: 'Plain box', note: 'A plain box, for anything' },
]
export const isCaseStyle = (v: unknown): v is CaseStyle => CASE_STYLES.some((c) => c.id === v)

// Each console's own case, by its id (the server's console ids, docs/specs/wittyplay-file.md "Case styles").
export const CONSOLE_CASE: Record<string, CaseStyle> = {
  nes: 'cart-nes', fds: 'disk-floppy',
  a2600: 'cart-atari', a5200: 'cart-atari', a7800: 'cart-atari',
  snes: 'cart-super',
  n64: 'cart-64', jaguar: 'cart-64',
  md: 'cart-mega', '32x': 'cart-mega', sms: 'cart-mega', neogeo: 'cart-mega',
  gb: 'cart-pocket', gbc: 'cart-pocket', gg: 'cart-pocket', ngp: 'cart-pocket', ws: 'cart-pocket', vb: 'cart-pocket',
  gba: 'cart-advance',
  nds: 'card-slim', '3ds': 'card-slim', switch: 'card-slim', switch2: 'card-slim', pce: 'card-slim', lynx: 'card-slim',
  ps1: 'case-jewel', saturn: 'case-jewel', segacd: 'case-jewel', dc: 'case-jewel', pcecd: 'case-jewel', '3do': 'case-jewel', pcfx: 'case-jewel', neogeocd: 'case-jewel',
  ps2: 'case-keep', xbox: 'case-keep', wii: 'case-keep', wiiu: 'case-keep', ps3: 'case-keep', xbox360: 'case-keep',
  gc: 'case-mini',
  psp: 'case-umd',
  amiga: 'disk-floppy', dos: 'disk-floppy', atarist: 'disk-floppy', pc98: 'disk-floppy', x68000: 'disk-floppy', mac: 'disk-floppy',
  c64: 'tape-cassette', zx: 'tape-cassette', zxs: 'tape-cassette', cpc: 'tape-cassette', msx: 'tape-cassette', bbc: 'tape-cassette',
  arcade: 'arcade-marquee', mame: 'arcade-marquee', fbneo: 'arcade-marquee', cps1: 'arcade-marquee', cps2: 'arcade-marquee', cps3: 'arcade-marquee', naomi: 'arcade-marquee',
}

// A console's case: its own, or, for one not listed, what the server says of how its games came (its shelf case and media).
export function caseFor(consoleId: string, shelfKind?: string, media?: string): CaseStyle {
  const own = CONSOLE_CASE[consoleId]
  if (own) return own
  if (shelfKind === 'jewel') return 'case-jewel'
  if (shelfKind === 'keep') return 'case-keep'
  if (media === 'pocket') return 'cart-pocket'
  if (media === 'cart') return 'cart-nes'
  if (media === 'disc') return 'case-keep'
  return 'generic-box'
}

// A choice made of fields this version knows (a value it doesn't is left to the level above; the rest of the object is kept as it was).
const PICTURE_IDS = new Set(PICTURES.map((p) => p.id as string))
const LABEL_IDS = new Set(LABELS.map((p) => p.id as string))
export function cleanLook(v: unknown): Look {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return {}
  const o = v as Record<string, unknown>
  const out: Look = {}
  if (typeof o.picture === 'string' && PICTURE_IDS.has(o.picture)) out.picture = o.picture as PicturePick
  if (typeof o.case === 'string' && /^[a-z0-9][a-z0-9-]{0,39}$/.test(o.case)) out.case = o.case
  if (typeof o.label === 'string' && LABEL_IDS.has(o.label)) out.label = o.label as LabelPick
  return out
}
export const emptyLook = (l: Look | undefined) => !l || (!l.picture && !l.case && !l.label)

export type Resolved = { picture: PicturePick; case: CaseStyle; label: LabelPick; caseChosen: boolean }

// The look for one game: each field from the first level that sets it, most particular first; then fitted, the console's case and the
// box art. A case style this version doesn't draw (a newer program's) is the console's own.
export function resolveLook(levels: (Look | undefined)[], consoleCase: CaseStyle): Resolved {
  const pick = <K extends 'picture' | 'case' | 'label'>(k: K) => {
    for (const l of levels) { const v = cleanLook(l)[k]; if (v) return v }
    return undefined
  }
  const c = pick('case')
  const known = c && c !== 'auto' && isCaseStyle(c)
  return { picture: (pick('picture') as PicturePick) || 'fitted', case: known ? (c as CaseStyle) : consoleCase, label: (pick('label') as LabelPick) || 'boxart', caseChosen: !!known }
}

export type ServerPictures = { server?: { default?: Look; consoles?: Record<string, Look> }; games?: Record<string, Look> }

// The order the levels count in (docs/specs/wittyplay-file.md, "Precedence"): the person's choice for the game, the server's for the game,
// the person's for the console, the server's for the console, the person's default, the server's default.
export function lookFor(pref: PicturesPref | undefined, server: ServerPictures | undefined, itemId: string, consoleId: string, consoleCase: CaseStyle): Resolved {
  return resolveLook([
    pref?.games?.[itemId], server?.games?.[itemId],
    pref?.consoles?.[consoleId], server?.server?.consoles?.[consoleId],
    pref?.default, server?.server?.default,
  ], consoleCase)
}

// Read what /me/ui holds, whatever it is.
export function picturesPref(v: unknown): PicturesPref {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return {}
  const o = v as Record<string, unknown>
  const map = (x: unknown) => {
    const out: Record<string, Look> = {}
    if (x && typeof x === 'object' && !Array.isArray(x)) for (const [k, l] of Object.entries(x as Record<string, unknown>)) { const c = cleanLook(l); if (!emptyLook(c)) out[k] = c }
    return out
  }
  return { default: cleanLook(o.default), consoles: map(o.consoles), games: map(o.games) }
}

// One level changed: null or an empty look takes it away. Games keep their order of choosing, so the oldest goes when there are too many.
export function withLook(pref: PicturesPref, level: { console?: string; game?: string }, look: Look | null): PicturesPref {
  const l = look ? cleanLook(look) : {}
  if (!level.console && !level.game) return { ...pref, default: l }
  const key = level.game ? 'games' : 'consoles'
  const id = (level.game || level.console) as string
  const was = { ...(pref[key] || {}) }
  delete was[id]
  if (!emptyLook(l)) was[id] = l
  let entries = Object.entries(was)
  if (key === 'games' && entries.length > MAX_PREF_GAMES) entries = entries.slice(entries.length - MAX_PREF_GAMES)
  return { ...pref, [key]: Object.fromEntries(entries) }
}

// ---- the .wittyplay file ----
export const WITTYPLAY_EXT = '.wittyplay'
export const WITTYPLAY_TYPE = 'application/vnd.wittyplay+json'
export const WITTYPLAY_MAX = 64 * 1024
export type WittyplayGame = Look & { match: { item?: number; file?: string; title?: string; console?: string; [more: string]: unknown } }
export type WittyplayDoc = { wittyplay: number; name?: string; description?: string; generator?: string; default?: Look; consoles?: Record<string, Look>; games?: WittyplayGame[]; [more: string]: unknown }

// The person's choices as a file. Games carry this server's id, and their title and console when known, so another server finds them.
export function prefToDoc(pref: PicturesPref, opts: { name?: string; generator?: string; games?: Record<string, { title?: string; console?: string; file?: string }> } = {}): WittyplayDoc {
  const doc: WittyplayDoc = { wittyplay: 1 }
  if (opts.name) doc.name = opts.name.slice(0, 200)
  if (opts.generator) doc.generator = opts.generator.slice(0, 200)
  if (!emptyLook(pref.default)) doc.default = cleanLook(pref.default)
  const consoles = Object.entries(pref.consoles || {}).filter(([id, l]) => /^[a-z0-9][a-z0-9_-]{0,31}$/.test(id) && !emptyLook(l))
  if (consoles.length) doc.consoles = Object.fromEntries(consoles.map(([id, l]) => [id, cleanLook(l)]))
  const games = Object.entries(pref.games || {}).filter(([, l]) => !emptyLook(l)).map(([id, l]) => {
    const known = opts.games?.[id] || {}
    const match: WittyplayGame['match'] = {}
    if (/^\d+$/.test(id)) match.item = Number(id)
    if (known.file && !/[/\\]/.test(known.file)) match.file = known.file.slice(0, 200)
    if (known.title) match.title = known.title.slice(0, 200)
    if (known.console && /^[a-z0-9][a-z0-9_-]{0,31}$/.test(known.console)) match.console = known.console
    return { match, ...cleanLook(l) }
  }).filter((g) => g.match.item || g.match.file || g.match.title)
  if (games.length) doc.games = games
  return doc
}

// A file's text read as a .wittyplay document, with the checks a person should hear about at once (the server checks it all again).
export function readDoc(text: string): { doc: WittyplayDoc } | { error: string } {
  if (new TextEncoder().encode(text).length > WITTYPLAY_MAX) return { error: 'That file is too big: a .wittyplay file is at most 64 KB.' }
  let v: unknown
  try { v = JSON.parse(text.replace(/^﻿/, '')) } catch { return { error: 'That isn’t a .wittyplay file: it isn’t JSON.' } }
  if (!v || typeof v !== 'object' || Array.isArray(v)) return { error: 'That isn’t a .wittyplay file.' }
  const n = (v as Record<string, unknown>).wittyplay
  if (typeof n !== 'number' || !Number.isInteger(n) || n < 1) return { error: 'That isn’t a .wittyplay file: it doesn’t start with "wittyplay": 1.' }
  if (n > 1) return { error: 'That file was made for a newer version of WittyPlay.' }
  return { doc: v as WittyplayDoc }
}

// A file's choices added to the person's own: its default and consoles replace theirs where it sets them, and each of its games goes to
// the games here it matched (matches: for each game in the file, this server's ids, from POST /api/games/pictures/check).
export function importDoc(pref: PicturesPref, doc: WittyplayDoc, matches: number[][]): { pref: PicturesPref; games: number; unmatched: number } {
  let next: PicturesPref = { ...pref }
  if (!emptyLook(doc.default)) next = withLook(next, {}, { ...cleanLook(pref.default), ...cleanLook(doc.default) })
  for (const [id, l] of Object.entries(doc.consoles || {})) if (!emptyLook(cleanLook(l))) next = withLook(next, { console: id }, { ...cleanLook(pref.consoles?.[id]), ...cleanLook(l) })
  let games = 0, unmatched = 0
  ;(doc.games || []).forEach((g, i) => {
    const ids = matches[i] || []
    if (!ids.length) { unmatched++; return }
    for (const id of ids) { next = withLook(next, { game: String(id) }, { ...cleanLook(next.games?.[String(id)]), ...cleanLook(g) }); games++ }
  })
  return { pref: next, games, unmatched }
}

// A file name for an export: letters, digits and spaces of the name given, and the extension.
export const exportName = (name: string) => (name.replace(/[^\p{L}\p{N} _-]+/gu, '').trim().slice(0, 60) || 'game-pictures') + WITTYPLAY_EXT
