import { allGuides, isQuiet, loadGuide, slugOf, type Guide } from './guides'
import { parse, plain, type Block } from './markdown'

// Searching the guides, on this device only. The first search fetches every guide from this
// server; after that it's all in memory until the page goes away.

type Section = { guide: Guide; heading: string; id: string; text: string; low: string; lowHead: string; lowTitle: string }

export type Hit = { guide: Guide; heading: string; to: string; before: string; match: string; after: string }

let index: Promise<Section[]> | null = null

function blockText(b: Block): string {
  switch (b.t) {
    case 'heading':
    case 'p': return plain(b.c)
    case 'code': return b.v
    case 'list': return b.items.map((it) => it.map(blockText).join(' ')).join(' ')
    case 'quote': return b.c.map(blockText).join(' ')
    case 'table': return [b.head, ...b.rows].map((r) => r.map(plain).join(' ')).join(' ')
    case 'figure': return `${b.alt} ${plain(b.caption)}`
    case 'hr': return ''
  }
}

export function sections(guide: Guide, blocks: Block[]): Section[] {
  const out: Section[] = []
  let heading = guide.title, id = '', parts: string[] = []
  const push = () => {
    const text = parts.join(' ').replace(/\s+/g, ' ').trim()
    if (text || id) out.push({ guide, heading, id, text, low: text.toLowerCase(), lowHead: heading.toLowerCase(), lowTitle: guide.title.toLowerCase() })
  }
  for (const b of blocks) {
    if (b.t === 'heading' && b.level > 1 && b.level <= 3) {
      push()
      heading = plain(b.c); id = b.id; parts = []
    } else if (b.t !== 'heading' || b.level > 3) parts.push(blockText(b))
  }
  push()
  return out
}

function loadIndex(): Promise<Section[]> {
  if (!index) {
    index = Promise.all(allGuides.map((g) => loadGuide(g).then((md) => sections(g, parse(md)), () => [] as Section[]))).then((all) => all.flat())
    index.catch(() => { index = null })
  }
  return index
}

const SNIPPET = 90

// Every word has to be there. A word in a title counts for more than one in a heading, which
// counts for more than one in the text, and the guides for developers come after the rest.
export function rank(all: Section[], query: string, limit = 40): Hit[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean).slice(0, 8)
  if (words.length === 0) return []
  const scored: { s: Section; score: number }[] = []
  for (const s of all) {
    let score = 0
    let every = true
    for (const w of words) {
      const inTitle = s.lowTitle.includes(w), inHead = s.lowHead.includes(w), inText = s.low.includes(w)
      if (!inTitle && !inHead && !inText) { every = false; break }
      score += (inTitle ? 6 : 0) + (inHead ? 10 : 0) + (inText ? 2 : 0)
    }
    if (!every) continue
    if (s.low.includes(words.join(' '))) score += 6
    if (s.id === '') score += 2
    if (isQuiet(s.guide)) score /= 3
    scored.push({ s, score })
  }
  scored.sort((a, b) => b.score - a.score)
  return scored.slice(0, limit).map(({ s }) => {
    const first = words.map((w) => s.low.indexOf(w)).filter((i) => i >= 0).sort((a, b) => a - b)[0] ?? -1
    const word = first < 0 ? '' : words.find((w) => s.low.startsWith(w, first)) ?? ''
    const from = Math.max(0, first - SNIPPET)
    return {
      guide: s.guide,
      heading: s.heading,
      to: `/help/${slugOf(s.guide.file)}${s.id ? '#' + s.id : ''}`,
      before: first < 0 ? s.text.slice(0, SNIPPET * 2) : (from > 0 ? '…' : '') + s.text.slice(from, first),
      match: first < 0 ? '' : s.text.slice(first, first + word.length),
      after: first < 0 ? '' : s.text.slice(first + word.length, first + word.length + SNIPPET) + '…',
    }
  })
}

export async function search(query: string): Promise<Hit[]> {
  return rank(await loadIndex(), query)
}
