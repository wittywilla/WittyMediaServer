// How the server's name is coloured: WittyMediaServer is drawn as Witty, Media
// in the theme colour, then Server, and a name of the owner's own the same way.
// The same rule is in internal/brandname on the server, and both are tested
// against internal/brandname/testdata/cases.json. Offsets are characters (code
// points), as the server counts them, not UTF-16 units.

export const DEFAULT_NAME = 'WittyMediaServer'
const MAX_RUNES = 80

export type BrandPart = { text: string; accent?: boolean }

// The brandColouring setting: 'auto' (also when empty or unknown), 'one' for
// the whole name in one colour, or 'pick:' and the coloured ranges as
// start-end pairs, end not included: 'pick:5-10,16-22'.
export type Colouring = 'auto' | 'one' | `pick:${string}`

const space = /^\p{White_Space}$/u
const upper = /^\p{Lu}$/u
const lower = /^\p{Ll}$/u
const digit = /^\p{Nd}$/u
const letter = /^\p{L}$/u
const mark = /^\p{M}$/u
const dash = /^\p{Pd}$/u

// joiner: the marks that join two words into one, where a capital after them
// starts the next word. Apostrophes and full stops don't (O'Brien, J.R.).
const joiner = (c: string) => dash.test(c) || '_/&+|·:'.includes(c)

function breaksBefore(r: string[], start: number, i: number): boolean {
  const c = r[i]
  // Accents written as separate marks belong to the letter before them.
  let p = -1
  for (let j = i - 1; j >= start; j--) if (!mark.test(r[j])) { p = j; break }
  if (p < 0 || mark.test(c)) return false
  const prev = r[p]
  if (upper.test(c)) {
    if (lower.test(prev)) return true
    // The last capital of a run starts the next word when two small letters
    // follow: TVRoom, 4KMovies, not TVs or DVDs.
    if (upper.test(prev) || digit.test(prev)) return i + 2 < r.length && lower.test(r[i + 1]) && lower.test(r[i + 2])
    if (joiner(prev)) {
      for (let j = start; j < p; j++) if (letter.test(r[j]) || digit.test(r[j])) return true
    }
    return false
  }
  return digit.test(c) && lower.test(prev)
}

// words finds the words of a name as [start, end) character offsets: split at
// spaces, and inside joined words at capitals (MyHome), before the last capital
// of a run that starts a word (TVRoom, not TVs), between small letters and
// digits (Movies4K) and before a capital after a dash and the like
// (Witty-Media). Runs of capitals and digits stay together (TV, 4K), and
// scripts without capitals split at spaces only.
export function words(name: string): [number, number][] {
  const r = Array.from(name)
  const out: [number, number][] = []
  let start = -1
  r.forEach((c, i) => {
    if (space.test(c)) {
      if (start >= 0) { out.push([start, i]); start = -1 }
      return
    }
    if (start < 0) { start = i; return }
    if (breaksBefore(r, start, i)) { out.push([start, i]); start = i }
  })
  if (start >= 0) out.push([start, r.length])
  return out
}

// parsePick reads 'pick:5-10,16-22', or null when it isn't valid.
export function parsePick(v: string): [number, number][] | null {
  if (!v.startsWith('pick:')) return null
  const body = v.slice(5)
  if (body === '') return []
  const out: [number, number][] = []
  let end = 0
  for (const s of body.split(',')) {
    const m = /^(\d{1,2})-(\d{1,2})$/.exec(s)
    if (!m) return null
    const x = Number(m[1]), y = Number(m[2])
    if (x < end || y <= x || y > MAX_RUNES) return null
    out.push([x, y])
    end = y
  }
  return out
}

export const validColouring = (v: string) => v === '' || v === 'auto' || v === 'one' || parsePick(v) !== null

// accentMask: for each character of the name, whether it is in the theme colour.
export function accentMask(name: string, colouring: string): boolean[] {
  const n = Array.from(name).length
  const mask = new Array<boolean>(n).fill(false)
  if (colouring === 'one') return mask
  const picked = parsePick(colouring)
  if (picked) {
    for (const [a, b] of picked) for (let i = a; i < b && i < n; i++) mask[i] = true
    return mask
  }
  words(name).forEach(([a, b], k) => { if (k % 2 === 1) for (let i = a; i < b; i++) mask[i] = true })
  return mask
}

// brandParts splits the name into runs drawn plain or in the theme colour. They
// join back into the name, and neighbours never share the same colour.
export function brandParts(name: string, colouring: string): BrandPart[] {
  return partsFromMask(Array.from(name), accentMask(name, colouring))
}

export function partsFromMask(r: string[], mask: boolean[]): BrandPart[] {
  const out: BrandPart[] = []
  let start = 0
  for (let i = 1; i <= r.length; i++) {
    if (i === r.length || mask[i] !== mask[start]) {
      out.push(mask[start] ? { text: r.slice(start, i).join(''), accent: true } : { text: r.slice(start, i).join('') })
      start = i
    }
  }
  return out
}

// pickFromMask writes a mask as the setting: the coloured runs, spaces at their
// ends left out (they show no colour either way).
export function pickFromMask(r: string[], mask: boolean[]): Colouring {
  const ranges: string[] = []
  for (let i = 0; i < r.length; i++) {
    if (!mask[i] || space.test(r[i])) continue
    let j = i
    while (j < r.length && mask[j]) j++
    let e = j
    while (e > i && space.test(r[e - 1])) e--
    ranges.push(`${i}-${e}`)
    i = j - 1
  }
  return `pick:${ranges.join(',')}`
}

// Choosing the words by hand: the name is split at cuts (character offsets
// inside a run of letters, as well as at spaces), and each piece is coloured
// or not. Every piece is all one colour.

// cutsFor: where the words split, automatically and where the colouring changes.
export function cutsFor(name: string, mask: boolean[]): Set<number> {
  const r = Array.from(name)
  const cuts = new Set<number>()
  for (const [a] of words(name)) if (a > 0 && !space.test(r[a - 1])) cuts.add(a)
  for (let i = 1; i < r.length; i++) if (mask[i] !== mask[i - 1] && !space.test(r[i]) && !space.test(r[i - 1])) cuts.add(i)
  return cuts
}

// pieces: the runs of non-space characters, split at cuts, as [start, end).
export function pieces(r: string[], cuts: Set<number>): [number, number][] {
  const out: [number, number][] = []
  let start = -1
  for (let i = 0; i <= r.length; i++) {
    const end = i === r.length || space.test(r[i])
    if (start >= 0 && (end || cuts.has(i))) { out.push([start, i]); start = -1 }
    if (!end && start < 0) start = i
  }
  return out
}

// letters: the name as people see its characters (an accent or an emoji made
// of several characters stays whole), each with the offset it starts at.
const seg = typeof Intl !== 'undefined' && 'Segmenter' in Intl ? new Intl.Segmenter(undefined, { granularity: 'grapheme' }) : null
export function letters(name: string): [number, string][] {
  const units = seg ? Array.from(seg.segment(name), (s) => s.segment) : Array.from(name)
  let at = 0
  return units.map((u) => { const l: [number, string] = [at, u]; at += Array.from(u).length; return l })
}

// gaps: where a piece may be split, between two letters.
export function gaps(name: string): Set<number> {
  const r = Array.from(name)
  return new Set(letters(name).map(([at]) => at).filter((at) => at > 0 && !space.test(r[at - 1]) && !space.test(r[at])))
}

export const isSpace = (c: string) => space.test(c)

// split cuts a piece at i, and the part after the cut changes colour, so the
// split shows. join takes the cut away, and the joined piece keeps the
// colour of its first part.
export function split(r: string[], cuts: Set<number>, mask: boolean[], i: number): [Set<number>, boolean[]] {
  const piece = pieces(r, cuts).find(([a, b]) => a < i && i < b)
  if (!piece) return [cuts, mask]
  const m = mask.slice()
  for (let j = i; j < piece[1]; j++) m[j] = !mask[j]
  return [new Set(cuts).add(i), m]
}

export function join(r: string[], cuts: Set<number>, mask: boolean[], i: number): [Set<number>, boolean[]] {
  const c = new Set(cuts)
  if (!c.delete(i)) return [cuts, mask]
  const piece = pieces(r, c).find(([a, b]) => a < i && i < b)
  const m = mask.slice()
  if (piece) for (let j = piece[0]; j < piece[1]; j++) m[j] = mask[piece[0]]
  return [c, m]
}
