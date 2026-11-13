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
// copy (/me/ui) keeps only these four, and the server keeps a file's others (docs/specs/wittyplay-file.md, Unknown fields).
export type Look = { picture?: PicturePick; case?: string; label?: LabelPick; cases?: Record<string, string>; [more: string]: unknown }
// The person's own choices, as kept in /me/ui: compact, games by this server's id.
export type PicturesPref = { default?: Look; consoles?: Record<string, Look>; games?: Record<string, Look> }
export const PLAY_PICTURES_KEY = 'wittyplayPictures'
// /me/ui holds 16 KB in all, so a person keeps their own look for at most this many games (the oldest choice goes first).
export const MAX_PREF_GAMES = 150

export const PICTURES: { id: PicturePick; label: string; note: string }[] = [
  { id: 'case', label: 'In its case or cartridge', note: 'Drawn whole in a cartridge, disc case or tape like the ones its console used' },
  { id: 'filled', label: 'Box art filled', note: 'The picture fills the tile; its edges may be cut off' },
  { id: 'fitted', label: 'Box art fitted', note: 'The whole picture, nothing cut off, over a soft blurred copy of itself' },
]
export const LABELS: { id: LabelPick; label: string; note: string }[] = [
  { id: 'boxart', label: 'Box art', note: 'The game’s cover on the label' },
  { id: 'logo', label: 'Game logo', note: 'The game’s logo, when it has one; the cover when not' },
  { id: 'title', label: 'Title', note: 'The game’s name in type' },
]

// ---- the cases: data, so a style is added or redrawn in one place (GameCase.tsx draws each "draw" id) ----

// How a kind of game medium can be shown: front on, at an angle, in its box; in its case, or the bare disc; or as a download (a
// download is a way to show any game, so every medium offers it).
export type Variant = 'front' | 'angled' | 'box' | 'case' | 'bare' | 'alt' | 'digital'
export const VARIANTS: { id: Variant; label: string }[] = [
  { id: 'front', label: 'Front on' }, { id: 'angled', label: 'At an angle' }, { id: 'box', label: 'In its box' },
  { id: 'case', label: 'In its case' }, { id: 'bare', label: 'The disc alone' }, { id: 'alt', label: 'Another way' },
  { id: 'digital', label: 'As a download' },
]

export type CaseStyle = string
// Every style WittyPlay draws, in the format's order: its words, the drawing it uses (gameCaseShapes.tsx SHAPES), the finish it is drawn
// in (tone) and whether it is that drawing turned at an angle. The words never name a maker or a product: they say the era and the shape.
export type StyleDef = { id: CaseStyle; label: string; note: string; draw: string; angled?: boolean; tone?: string }
const st = (id: string, label: string, note: string, draw = id, more: Partial<StyleDef> = {}): StyleDef => ({ id, label, note, draw, ...more })
const angled = (base: StyleDef): StyleDef => ({ ...base, id: base.id + '-angled', label: base.label + ', at an angle', note: 'Turned to show its side', angled: true })
// The finishes a drawing can come in: a colour of plastic, gold, see-through, wood grain.
const FINISH: Record<string, string> = { gold: 'gold', dark: 'black', clear: 'see-through', red: 'red', blue: 'blue', green: 'green', white: 'white', wood: 'wood-grain' }
const finish = (base: StyleDef, t: string): StyleDef => ({ ...base, id: base.id + '-' + t, label: base.label + ', ' + FINISH[t], note: 'The same shape in ' + FINISH[t], tone: t })
// A drawing in each of its finishes, each also turned at an angle when it has a side to show.
const finishes = (base: StyleDef, fins: string[], turn = true): StyleDef[] => fins.flatMap((t) => { const x = finish(base, t); return turn ? [x, angled(x)] : [x] })

const CARTS: [StyleDef, string[]][] = [
  [st('cart-nes', '8-bit cartridge', 'A grey front-loading cartridge with ridges and a label window'), ['gold', 'dark', 'clear']],
  [st('cart-atari', 'Early cartridge', 'A short first-generation cartridge with a ribbed top and wide label'), ['wood', 'gold', 'clear']],
  [st('cart-super', '16-bit cartridge', 'Rounded at the top, with grip dimples and a narrow foot'), ['gold', 'dark', 'clear']],
  [st('cart-64', '64-bit cartridge', 'Wide and short, with a rounded, ridged top edge and a large label'), ['gold', 'red', 'blue', 'clear']],
  [st('cart-mega', 'Black 16-bit cartridge', 'Black, with a slanted top over a recessed label'), ['gold', 'red', 'clear']],
  [st('cart-pocket', 'Handheld cartridge', 'Small, with a notched corner, top ridges and an arrow'), ['red', 'blue', 'green', 'gold', 'clear']],
  [st('cart-advance', 'Wide handheld cartridge', 'Wide and short, with sloped shoulders'), ['clear', 'gold', 'blue']],
  [st('card-slim', 'Game card', 'A small, thin card with one corner cut'), ['red', 'blue', 'green', 'white']],
]
const CASES: [StyleDef, string[]][] = [
  [st('case-jewel', 'CD jewel case', 'A square clear case with a hinge down one side'), ['white', 'clear']],
  [st('case-keep', 'DVD case', 'A tall case with a clear sleeve'), ['white', 'green', 'blue', 'red']],
  [st('case-blu', 'Slim disc case', 'A shorter, slimmer case with a band along the top'), ['dark', 'white', 'green']],
  [st('case-mini', 'Mini disc case', 'A smaller, squarer disc case'), ['white', 'dark']],
  [st('case-clamshell', 'Plastic clamshell', 'A hinged plastic case with ribs and a latch'), ['white', 'clear']],
  [st('case-card', 'Card case', 'A small case for a game card'), ['dark', 'red', 'blue']],
  [st('case-umd', 'Small disc case', 'The tall, narrow case a small disc shell came in'), ['dark', 'blue']],
  [st('case-tape', 'Cassette case', 'A cassette in its clear box'), ['white', 'dark']],
]
const BOXES: StyleDef[] = [
  st('box-cart', 'Cartridge box', 'A tall cardboard box with a lid and a side'),
  st('box-cart-wide', 'Wide cartridge box', 'A wide cardboard box'),
  st('box-small', 'Small box', 'A small cardboard box for a handheld game'),
  st('box-big', 'Big box', 'A large, deep cardboard box, as computer games came in'),
  st('box-tall', 'Tall box', 'A narrow, tall cardboard box'),
  st('box-flip', 'Lid box', 'A cardboard box with a lid that opens along the top'),
]
const HU = st('card-hu', 'Thin card', 'A card no thicker than a credit card, contacts along one edge')
const UMD = st('umd-bare', 'Small disc shell', 'A small disc in its shell, with a window')
const FLOPPY = st('disk-floppy', '3.5-inch floppy disk', 'A hard disk with a metal shutter')
const F525 = st('disk-525', '5.25-inch floppy disk', 'A floppy disk in a square sleeve')
const TAPE = st('tape-cassette', 'Cassette tape', 'A tape with two reels')
const MARQUEE = st('arcade-marquee', 'Arcade marquee', 'A cabinet’s lit sign over its screen')
const CABINET = st('arcade-cabinet', 'Arcade cabinet', 'A whole upright cabinet, the game on its screen')
const TILE = st('digital-tile', 'Download', 'A game that came as a download, as a glossy app tile')
const KEEP = CASES[1][0], BLU = CASES[2][0]

// The ids and their order are the format's: the first sixty-odd are the first version's, and nothing is ever renamed or removed.
const ALL: StyleDef[] = [
  ...CARTS.map(([d]) => d), HU, ...CARTS.map(([d]) => angled(d)),
  ...BOXES.slice(0, 4), CASES[4][0], CASES[5][0], CASES[0][0], KEEP, angled(KEEP), BLU, angled(BLU), CASES[3][0],
  st('disc-bare', 'Disc', 'The disc alone, label side up'),
  st('disc-gd', 'Disc, gold-edged', 'The disc alone, with a bright inner ring', 'disc-bare', { tone: 'gd' }),
  st('disc-mini', 'Mini disc', 'A small disc alone'),
  UMD, CASES[6][0], FLOPPY, F525, TAPE, CASES[7][0], MARQUEE, st('arcade-pcb', 'Arcade board', 'A circuit board with a label'), TILE,
  st('generic-box', 'Plain box', 'A plain box, for anything'),
  // Added in the redraw.
  ...CARTS.flatMap(([d, f]) => finishes(d, f)),
  ...CASES.flatMap(([d, f]) => [angled(d), ...finishes(d, f)]),
  ...BOXES.slice(4), ...BOXES.flatMap((b) => finishes(b, ['white', 'dark'], false)),
  ...finishes(HU, ['dark', 'gold', 'blue'], false),
  st('disc-bd', 'Disc, blue-ringed', 'The disc alone, with a blue-violet inner ring', 'disc-bare', { tone: 'bd' }),
  st('disc-dark', 'Disc, black-ringed', 'The disc alone, with a black inner ring', 'disc-bare', { tone: 'dark' }),
  st('disc-mini-dark', 'Mini disc, black-ringed', 'A small disc alone, with a black inner ring', 'disc-mini', { tone: 'dark' }),
  st('disc-sleeve', 'Disc in a sleeve', 'The disc half out of a paper sleeve'),
  st('disc-sleeve-gd', 'Disc in a sleeve, gold-edged', 'A disc with a bright inner ring half out of a sleeve', 'disc-sleeve', { tone: 'gd' }),
  angled(UMD), ...finishes(UMD, ['clear', 'blue'], false),
  ...finishes(FLOPPY, ['blue', 'red', 'green', 'white'], false), ...finishes(F525, ['blue', 'red', 'clear'], false),
  angled(TAPE), ...finishes(TAPE, ['white', 'dark', 'clear'], false),
  CABINET, angled(CABINET), ...finishes(CABINET, ['red', 'blue'], false), ...finishes(MARQUEE, ['red', 'blue', 'green'], false),
  st('digital-poster', 'Download, poster', 'A tall store poster with a download bar'),
  st('digital-banner', 'Download, banner', 'A wide banner with a play button'),
  st('digital-window', 'Download, window', 'The game in an app window with a progress bar'),
  ...finishes(TILE, ['red', 'blue', 'green', 'gold', 'dark'], false),
]
export const CASE_STYLES: StyleDef[] = ALL.filter((c, i) => ALL.findIndex((x) => x.id === c.id) === i)
const BY_ID = new Map(CASE_STYLES.map((c) => [c.id, c]))
export const styleDef = (id: string): StyleDef | undefined => BY_ID.get(id)
export const isCaseStyle = (v: unknown): v is CaseStyle => typeof v === 'string' && BY_ID.has(v)

// Every kind of medium games came on, and the styles each can be shown in, the first being its usual one. variants are the named ways
// (and what the quick choices for a whole family use); more are the other finishes, boxes and cases that also suit it.
export type MediaId = string
export type MediaDef = { id: MediaId; label: string; family: 'cart' | 'disc' | 'other'; variants: Partial<Record<Variant, CaseStyle>>; more: CaseStyle[] }
const mk = (id: string, label: string, family: MediaDef['family'], variants: MediaDef['variants'], more: CaseStyle[] = []): MediaDef => ({ id, label, family, variants, more })
// The ways to show any game as a download, then plain boxes.
const DL = ['digital-poster', 'digital-banner', 'digital-window', 'digital-tile-red', 'digital-tile-blue', 'digital-tile-green', 'digital-tile-gold', 'digital-tile-dark']
const fins = (base: string, ...f: string[]) => f.flatMap((t) => [`${base}-${t}`, `${base}-${t}-angled`])
const caseSet = (base: string, ...f: string[]) => [base + '-angled', ...f.flatMap((t) => [`${base}-${t}`, `${base}-${t}-angled`])]
export const MEDIA: MediaDef[] = [
  mk('cart-8bit', '8-bit cartridges', 'cart', { front: 'cart-nes', angled: 'cart-nes-angled', box: 'box-cart', digital: 'digital-tile' },
    [...fins('cart-nes', 'gold', 'dark', 'clear'), 'box-tall', 'box-flip', 'box-cart-white', 'box-cart-dark', 'case-clamshell', ...DL]),
  mk('cart-early', 'Early cartridges', 'cart', { front: 'cart-atari', angled: 'cart-atari-angled', box: 'box-cart', digital: 'digital-tile' },
    [...fins('cart-atari', 'wood', 'gold', 'clear'), 'box-flip', 'box-tall', 'box-cart-dark', 'box-cart-white', ...DL]),
  mk('cart-16bit', '16-bit cartridges', 'cart', { front: 'cart-super', angled: 'cart-super-angled', box: 'box-cart-wide', digital: 'digital-tile' },
    [...fins('cart-super', 'gold', 'dark', 'clear'), 'box-cart', 'box-flip', 'box-cart-wide-white', 'box-cart-wide-dark', 'case-clamshell', ...DL]),
  mk('cart-mega', 'Black 16-bit cartridges', 'cart', { front: 'cart-mega', angled: 'cart-mega-angled', box: 'case-clamshell', alt: 'box-cart', digital: 'digital-tile' },
    [...fins('cart-mega', 'gold', 'red', 'clear'), 'case-clamshell-angled', 'case-clamshell-white', 'case-clamshell-clear', 'box-cart-wide', 'box-flip', 'box-cart-dark', ...DL]),
  mk('cart-64', '64-bit cartridges', 'cart', { front: 'cart-64', angled: 'cart-64-angled', box: 'box-cart-wide', digital: 'digital-tile' },
    [...fins('cart-64', 'gold', 'red', 'blue', 'clear'), 'box-cart', 'box-flip', 'box-cart-wide-white', 'box-cart-wide-dark', 'case-clamshell', ...DL]),
  mk('cart-pocket', 'Handheld cartridges', 'cart', { front: 'cart-pocket', angled: 'cart-pocket-angled', box: 'box-small', digital: 'digital-tile' },
    [...fins('cart-pocket', 'red', 'blue', 'green', 'gold', 'clear'), 'box-small-white', 'box-small-dark', 'box-flip', 'box-tall', ...DL]),
  mk('cart-advance', 'Wide handheld cartridges', 'cart', { front: 'cart-advance', angled: 'cart-advance-angled', box: 'box-small', digital: 'digital-tile' },
    [...fins('cart-advance', 'clear', 'gold', 'blue'), 'box-small-white', 'box-small-dark', 'box-flip', 'box-cart-wide', ...DL]),
  mk('card', 'Game cards', 'cart', { front: 'card-slim', angled: 'card-slim-angled', box: 'case-card', digital: 'digital-tile' },
    [...fins('card-slim', 'red', 'blue', 'green', 'white'), ...caseSet('case-card', 'dark', 'red', 'blue'), 'box-small', 'box-small-white', 'box-small-dark', 'box-tall', ...DL]),
  mk('hucard', 'Thin cards', 'cart', { front: 'card-hu', box: 'case-jewel', digital: 'digital-tile' },
    ['card-hu-dark', 'card-hu-gold', 'card-hu-blue', 'case-jewel-angled', 'case-jewel-white', 'case-jewel-clear', 'box-small', 'box-small-white', 'box-flip', ...DL]),
  mk('cd', 'CDs', 'disc', { case: 'case-jewel', bare: 'disc-bare', alt: 'case-keep', digital: 'digital-tile' },
    [...caseSet('case-jewel', 'white', 'clear'), 'disc-sleeve', 'disc-dark', 'case-keep-angled', 'case-keep-white', 'case-keep-blue', 'case-mini', 'box-flip', 'box-small', ...DL]),
  mk('gd', 'Gold-edged discs', 'disc', { case: 'case-jewel', bare: 'disc-gd', alt: 'case-keep', digital: 'digital-tile' },
    [...caseSet('case-jewel', 'white', 'clear'), 'disc-sleeve-gd', 'case-keep-angled', 'case-keep-white', 'case-keep-blue', 'case-keep-red', 'box-flip', 'box-small', ...DL]),
  mk('dvd', 'DVDs', 'disc', { case: 'case-keep', bare: 'disc-bare', angled: 'case-keep-angled', digital: 'digital-tile' },
    [...['white', 'green', 'blue', 'red'].flatMap((t) => [`case-keep-${t}`, `case-keep-${t}-angled`]), 'disc-sleeve', 'disc-dark', 'case-blu', 'case-jewel', 'box-flip', 'box-big', ...DL]),
  mk('bluray', 'High-definition discs', 'disc', { case: 'case-blu', bare: 'disc-bare', angled: 'case-blu-angled', digital: 'digital-tile' },
    [...['dark', 'white', 'green'].flatMap((t) => [`case-blu-${t}`, `case-blu-${t}-angled`]), 'disc-bd', 'disc-sleeve', 'case-keep', 'case-keep-angled', 'case-keep-blue', 'box-flip', ...DL]),
  mk('minidisc', 'Mini discs', 'disc', { case: 'case-mini', bare: 'disc-mini', digital: 'digital-tile' },
    [...caseSet('case-mini', 'white', 'dark'), 'disc-mini-dark', 'case-keep', 'box-small', 'box-flip', ...DL]),
  mk('umd', 'Small disc shells', 'disc', { bare: 'umd-bare', case: 'case-umd', digital: 'digital-tile' },
    ['umd-bare-angled', 'umd-bare-clear', 'umd-bare-blue', ...caseSet('case-umd', 'dark', 'blue'), 'box-small', 'box-tall', ...DL]),
  mk('floppy35', '3.5-inch floppy disks', 'other', { front: 'disk-floppy', box: 'box-big', digital: 'digital-tile' },
    ['disk-floppy-blue', 'disk-floppy-red', 'disk-floppy-green', 'disk-floppy-white', 'box-flip', 'box-cart', 'box-cart-white', 'box-big-white', 'box-big-dark', 'case-keep', ...DL]),
  mk('floppy525', '5.25-inch floppy disks', 'other', { front: 'disk-525', box: 'box-big', digital: 'digital-tile' },
    ['disk-525-blue', 'disk-525-red', 'disk-525-clear', 'box-flip', 'box-cart-wide', 'box-big-white', 'box-big-dark', 'box-small', ...DL]),
  mk('tape', 'Cassette tapes', 'other', { front: 'tape-cassette', case: 'case-tape', digital: 'digital-tile' },
    ['tape-cassette-angled', 'tape-cassette-white', 'tape-cassette-dark', 'tape-cassette-clear', ...caseSet('case-tape', 'white', 'dark'), 'box-flip', 'box-cart', 'box-cart-wide', 'box-big', ...DL]),
  mk('arcade', 'Arcade', 'other', { front: 'arcade-marquee', alt: 'arcade-pcb', digital: 'digital-tile' },
    ['arcade-cabinet', 'arcade-cabinet-angled', 'arcade-cabinet-red', 'arcade-cabinet-blue', 'arcade-marquee-red', 'arcade-marquee-blue', 'arcade-marquee-green', 'box-flip', 'box-tall', ...DL]),
  mk('digital', 'Downloads', 'other', { front: 'digital-tile', box: 'generic-box', digital: 'digital-poster' },
    ['digital-banner', 'digital-window', 'digital-tile-red', 'digital-tile-blue', 'digital-tile-green', 'digital-tile-gold', 'digital-tile-dark', 'box-big', 'box-big-white', 'box-big-dark', 'box-flip', 'case-keep', 'case-keep-angled', 'case-jewel', 'case-jewel-angled', 'disc-bare', 'disk-floppy', 'disk-525']),
  mk('other', 'Anything else', 'other', { box: 'generic-box', front: 'digital-tile', digital: 'digital-poster' },
    ['box-cart', 'box-cart-wide', 'box-small', 'box-big', 'box-tall', 'box-flip', 'case-keep', 'case-jewel', 'case-clamshell', 'disc-bare', 'cart-nes', 'tape-cassette', 'disk-floppy', ...DL.filter((d) => d !== 'digital-poster')]),
]
const MEDIA_BY_ID = new Map(MEDIA.map((x) => [x.id, x]))
export const mediaDef = (id: string): MediaDef => MEDIA_BY_ID.get(id) || MEDIA_BY_ID.get('other')!
// A medium's styles, its usual one first, then its other ways, then the finishes and everything else that suits it.
export const stylesOf = (media: string): CaseStyle[] => { const m = mediaDef(media); return [...new Set([...(Object.values(m.variants) as CaseStyle[]), ...m.more])].filter(isCaseStyle) }
export const usualStyle = (media: string): CaseStyle => stylesOf(media)[0]
// Which variants a family offers as a quick choice (Setup, and every game): cartridges front, angled, boxed or as a download; discs in a case, bare or as a download.
export const FAMILY_VARIANTS: Record<'cart' | 'disc', Variant[]> = { cart: ['front', 'angled', 'box', 'digital'], disc: ['case', 'bare', 'digital'] }

// Each console's medium, by its id (the server's console ids, docs/specs/wittyplay-file.md "Media").
export const CONSOLE_MEDIA: Record<string, MediaId> = {
  nes: 'cart-8bit', fds: 'floppy35',
  a2600: 'cart-early', a5200: 'cart-early', a7800: 'cart-early',
  snes: 'cart-16bit',
  n64: 'cart-64', jaguar: 'cart-64',
  md: 'cart-mega', '32x': 'cart-mega', sms: 'cart-mega', neogeo: 'cart-mega',
  gb: 'cart-pocket', gbc: 'cart-pocket', gg: 'cart-pocket', ngp: 'cart-pocket', ws: 'cart-pocket', vb: 'cart-pocket',
  gba: 'cart-advance',
  nds: 'card', '3ds': 'card', switch: 'card', switch2: 'card',
  pce: 'hucard', lynx: 'hucard',
  ps1: 'cd', saturn: 'cd', segacd: 'cd', pcecd: 'cd', '3do': 'cd', pcfx: 'cd', neogeocd: 'cd',
  dc: 'gd', naomi: 'arcade',
  ps2: 'dvd', xbox: 'dvd', wii: 'dvd', xbox360: 'dvd',
  ps3: 'bluray', wiiu: 'bluray',
  gc: 'minidisc',
  psp: 'umd',
  amiga: 'floppy35', atarist: 'floppy35', pc98: 'floppy35', x68000: 'floppy35', mac: 'floppy35', dos: 'floppy35',
  apple2: 'floppy525', bbc: 'floppy525',
  c64: 'tape', zx: 'tape', zxs: 'tape', cpc: 'tape', msx: 'tape',
  arcade: 'arcade', mame: 'arcade', fbneo: 'arcade', cps1: 'arcade', cps2: 'arcade', cps3: 'arcade',
  pc: 'digital', rig: 'digital',
}
// A console's medium: its own, or, for one not listed, what the server says of how its games came (its shelf case and media).
export function mediaFor(consoleId: string, shelfKind?: string, media?: string): MediaId {
  const own = CONSOLE_MEDIA[consoleId]
  if (own) return own
  if (shelfKind === 'jewel') return 'cd'
  if (shelfKind === 'keep') return 'dvd'
  if (media === 'pocket') return 'cart-pocket'
  if (media === 'cart') return 'cart-8bit'
  if (media === 'disc') return 'dvd'
  return 'other'
}
// A console's usual case.
export const caseFor = (consoleId: string, shelfKind?: string, media?: string): CaseStyle => usualStyle(mediaFor(consoleId, shelfKind, media))

// A choice made of fields this version knows (a value it doesn't is left to the level above).
const PICTURE_IDS = new Set(PICTURES.map((p) => p.id as string))
const LABEL_IDS = new Set(LABELS.map((p) => p.id as string))
const STYLE_RE = /^[a-z0-9][a-z0-9-]{0,39}$/
export function cleanLook(v: unknown): Look {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return {}
  const o = v as Record<string, unknown>
  const out: Look = {}
  if (typeof o.picture === 'string' && PICTURE_IDS.has(o.picture)) out.picture = o.picture as PicturePick
  if (typeof o.case === 'string' && STYLE_RE.test(o.case)) out.case = o.case
  if (typeof o.label === 'string' && LABEL_IDS.has(o.label)) out.label = o.label as LabelPick
  if (o.cases && typeof o.cases === 'object' && !Array.isArray(o.cases)) {
    const c = Object.fromEntries(Object.entries(o.cases as Record<string, unknown>).filter(([k, v]) => STYLE_RE.test(k) && typeof v === 'string' && STYLE_RE.test(v))) as Record<string, string>
    if (Object.keys(c).length) out.cases = c
  }
  return out
}
export const emptyLook = (l: Look | undefined) => !l || (!l.picture && !l.case && !l.label && !(l.cases && Object.keys(l.cases).length))

// One medium's style set at a level (null takes it away), and every medium of a family set to one variant at once.
export function withMediaStyle(l: Look, media: MediaId, style: CaseStyle | null): Look {
  const cases = { ...(l.cases || {}) }
  if (style) cases[media] = style; else delete cases[media]
  const out: Look = { ...l, cases }
  if (!Object.keys(cases).length) delete out.cases
  return out
}
export function withFamilyVariant(l: Look, family: 'cart' | 'disc', variant: Variant | null): Look {
  let out = l
  for (const md of MEDIA.filter((x) => x.family === family)) out = withMediaStyle(out, md.id, variant ? md.variants[variant] || null : null)
  return out
}
// The variant a whole family is set to at a level, when every medium of it agrees.
export function familyVariant(l: Look, family: 'cart' | 'disc'): Variant | '' {
  for (const v of FAMILY_VARIANTS[family]) {
    if (MEDIA.filter((x) => x.family === family).every((md) => !md.variants[v] ? !l.cases?.[md.id] : l.cases?.[md.id] === md.variants[v])) {
      if (MEDIA.some((md) => md.family === family && l.cases?.[md.id])) return v
    }
  }
  return ''
}

export type Resolved = { picture: PicturePick; case: CaseStyle; label: LabelPick; caseChosen: boolean; media: MediaId }

// The look for one game: each field from the first level that sets it, most particular first; then in its case (the medium's usual one) and
// the box art. At each level a case for this game itself ("case") counts before one for its medium ("cases"). A style this version
// doesn't draw (a newer program's) is passed over.
export function resolveLook(levels: (Look | undefined)[], media: MediaId): Resolved {
  const pick = <K extends 'picture' | 'label'>(k: K) => {
    for (const l of levels) { const v = cleanLook(l)[k]; if (v) return v }
    return undefined
  }
  let c: string | undefined
  for (const l of levels) {
    const x = cleanLook(l)
    if (x.case === 'auto') break
    const v = x.case && isCaseStyle(x.case) ? x.case : x.cases?.[media] && isCaseStyle(x.cases[media]) ? x.cases[media] : undefined
    if (v) { c = v; break }
  }
  return { picture: (pick('picture') as PicturePick) || 'case', case: c || usualStyle(media), label: (pick('label') as LabelPick) || 'boxart', caseChosen: !!c, media }
}

export type ServerPictures = { server?: { default?: Look; consoles?: Record<string, Look> }; games?: Record<string, Look> }

// The order the levels count in (docs/specs/wittyplay-file.md, "Precedence"): the person's choice for the game, the server's for the game,
// the person's for the console, the server's for the console, the person's default, the server's default.
export function lookFor(pref: PicturesPref | undefined, server: ServerPictures | undefined, itemId: string, consoleId: string, media: MediaId): Resolved {
  return resolveLook([
    pref?.games?.[itemId], server?.games?.[itemId],
    pref?.consoles?.[consoleId], server?.server?.consoles?.[consoleId],
    pref?.default, server?.server?.default,
  ], media)
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

// The picture at the top of WittyPlay's game panel (beside the games): the game's own art as it is, or the game in its case or
// cartridge as chosen for it. Per profile in /me/ui.
export type PanelArt = 'art' | 'case'
export const PLAY_PANEL_ART_KEY = 'wittyplayPanelArt'
export const PANEL_ARTS: { id: PanelArt; label: string; note: string }[] = [
  { id: 'art', label: 'The game’s art, as it is', note: 'Its box art whole, nothing drawn round it' },
  { id: 'case', label: 'In its case or cartridge', note: 'The style chosen for the game, its console or every game' },
]
export const panelArtOf = (v: unknown): PanelArt => (v === 'case' ? 'case' : 'art')

// How games look outside WittyPlay (Home, the app's shelves and search): as WittyPlay draws them (in their case or cartridge, as chosen),
// or just their box art. Per profile in /me/ui.
export type MainArt = 'follow' | 'art'
export const PLAY_MAIN_ART_KEY = 'wittyplayMainArt'
export const MAIN_ARTS: { id: MainArt; label: string; note: string }[] = [
  { id: 'follow', label: 'As WittyPlay shows them', note: 'In their case or cartridge, or however you chose here' },
  { id: 'art', label: 'Just their box art', note: 'The cover alone, like a film’s poster' },
]
export const mainArtOf = (v: unknown): MainArt => (v === 'art' ? 'art' : 'follow')

// How big games are drawn in WittyPlay's grids and rows: smaller to see more at once, larger to see each better. Per profile in /me/ui.
export type TileSize = 'small' | 'medium' | 'large'
export const PLAY_TILE_SIZE_KEY = 'wittyplayTileSize'
export const TILE_SIZES: { id: TileSize; label: string }[] = [{ id: 'small', label: 'Small' }, { id: 'medium', label: 'Medium' }, { id: 'large', label: 'Large' }]
export const tileSizeOf = (v: unknown): TileSize => (v === 'small' || v === 'large' ? v : 'medium')
