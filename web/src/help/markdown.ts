// A small Markdown reader for the guides in docs/. It makes a tree that help/GuideBody.tsx
// draws as React elements, so nothing in a guide ever reaches the page as HTML. Only what the
// guides use is here: headings, paragraphs, lists, tables, code, quotes, rules and pictures. Raw
// HTML lines are dropped. A picture is a line of its own, ![words](images/name.webp), and an
// italic line straight after it is its caption. Only names in docs/images can be pictures.

export type Inline =
  | { t: 'text'; v: string }
  | { t: 'code'; v: string }
  | { t: 'strong' | 'em'; c: Inline[] }
  | { t: 'link'; href: string; c: Inline[] }

export type Align = 'left' | 'center' | 'right' | undefined

export type Block =
  | { t: 'heading'; level: number; id: string; c: Inline[] }
  | { t: 'p'; c: Inline[] }
  | { t: 'list'; ordered: boolean; start: number; items: Block[][] }
  | { t: 'table'; head: Inline[][]; align: Align[]; rows: Inline[][][] }
  | { t: 'code'; lang: string; v: string }
  | { t: 'quote'; c: Block[] }
  | { t: 'hr' }
  | { t: 'figure'; name: string; alt: string; caption: Inline[] }

const MAX_DEPTH = 12

// ---------------------------------------------------------------- inline

const PUNCT = /[\\`*_{}[\]()#+\-.!|<>~]/

function closing(s: string, from: number, mark: string): number {
  for (let i = from; i < s.length; i++) {
    if (s[i] === '\\') { i++; continue }
    if (s[i] === '`') {
      const end = s.indexOf('`', i + 1)
      if (end > 0) i = end
      continue
    }
    if (s.startsWith(mark, i) && s[i - 1] !== ' ') {
      if (mark.length === 1 && s[i + 1] === mark) { i++; continue }
      return i
    }
  }
  return -1
}

// [text](address): where the text ends, and the address. Brackets may nest in the text and
// parentheses in the address.
function linkAt(s: string, at: number): { text: string; href: string; end: number } | null {
  let depth = 0, i = at
  for (; i < s.length; i++) {
    if (s[i] === '\\') { i++; continue }
    if (s[i] === '`') { const e = s.indexOf('`', i + 1); if (e > 0) i = e; continue }
    if (s[i] === '[') depth++
    else if (s[i] === ']' && --depth === 0) break
  }
  if (i >= s.length || s[i + 1] !== '(') return null
  const text = s.slice(at + 1, i)
  let parens = 0, j = i + 1
  for (; j < s.length; j++) {
    if (s[j] === '(') parens++
    else if (s[j] === ')' && --parens === 0) break
  }
  if (j >= s.length) return null
  return { text, href: s.slice(i + 2, j).trim().split(/\s+/)[0] || '', end: j + 1 }
}

export function inline(s: string, depth = 0): Inline[] {
  const out: Inline[] = []
  let text = ''
  const flush = () => { if (text) { out.push({ t: 'text', v: text }); text = '' } }
  const wordChar = (c: string | undefined) => !!c && /[\p{L}\p{N}]/u.test(c)
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (c === '\\' && i + 1 < s.length && PUNCT.test(s[i + 1])) { text += s[++i]; continue }
    if (c === '`') {
      let n = 1
      while (s[i + n] === '`') n++
      const mark = '`'.repeat(n)
      const end = s.indexOf(mark, i + n)
      if (end > 0) {
        flush()
        out.push({ t: 'code', v: s.slice(i + n, end).replace(/^ (.*) $/, '$1') })
        i = end + n - 1
        continue
      }
      text += mark
      i += n - 1
      continue
    }
    if (depth < MAX_DEPTH && c === '[') {
      const l = linkAt(s, i)
      if (l) {
        flush()
        out.push({ t: 'link', href: l.href, c: inline(l.text, depth + 1) })
        i = l.end - 1
        continue
      }
    }
    if (depth < MAX_DEPTH && (c === '*' || c === '_')) {
      const two = s[i + 1] === c
      const mark = two ? c + c : c
      const after = s[i + mark.length]
      const edge = c === '*' || !wordChar(s[i - 1])
      if (edge && after && after !== ' ' && after !== c) {
        const end = closing(s, i + mark.length, mark)
        if (end > 0 && (c === '*' || !wordChar(s[end + mark.length]))) {
          flush()
          out.push({ t: two ? 'strong' : 'em', c: inline(s.slice(i + mark.length, end), depth + 1) })
          i = end + mark.length - 1
          continue
        }
      }
    }
    text += c
  }
  flush()
  return out
}

export function plain(c: Inline[]): string {
  return c.map((n) => (n.t === 'text' || n.t === 'code' ? n.v : plain(n.c))).join('')
}

// The way GitHub names a heading, so an address works in both places.
export function slug(text: string): string {
  return text.trim().toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').replace(/\s/g, '-')
}

// ---------------------------------------------------------------- blocks

const LIST_ITEM = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/
const FENCE = /^\s*(`{3,}|~{3,})\s*([^\s`]*)/
const HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/
const RULE = /^\s*(-\s*){3,}$|^\s*(\*\s*){3,}$|^\s*(_\s*){3,}$/
const TABLE_RULE = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/
const HTML = /^\s*<\/?[a-zA-Z][^>]*>/
const IMAGE = /^\s*!\[([^\]]*)\]\(([^)\s]+)\)\s*$/
const CAPTION = /^\s*\*(?!\*)(.+)\*\s*$/
const IMAGE_NAME = /^(?:\.\/)?images\/([A-Za-z0-9_-]+\.(?:webp|png|jpe?g))$/

function indentOf(line: string) {
  let n = 0
  for (const c of line) {
    if (c === ' ') n++
    else if (c === '\t') n += 4
    else break
  }
  return n
}

function cells(line: string): string[] {
  let s = line.trim()
  if (s.startsWith('|')) s = s.slice(1)
  const out: string[] = []
  let cur = '', inCode = false
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (c === '\\' && s[i + 1] === '|') { cur += '|'; i++; continue }
    if (c === '`') inCode = !inCode
    if (c === '|' && !inCode) { out.push(cur.trim()); cur = ''; continue }
    cur += c
  }
  if (cur.trim() !== '') out.push(cur.trim())
  return out
}

function startsBlock(line: string, next: string | undefined): boolean {
  return HEADING.test(line) || FENCE.test(line) || RULE.test(line) || /^\s*>/.test(line) || LIST_ITEM.test(line) || IMAGE.test(line)
    || (line.includes('|') && next !== undefined && TABLE_RULE.test(next) && next.includes('-'))
}

type Used = Map<string, number>

function parseLines(lines: string[], used: Used, depth: number): Block[] {
  const out: Block[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (line.trim() === '') { i++; continue }

    const fence = FENCE.exec(line)
    if (fence) {
      const mark = fence[1], pad = indentOf(line), body: string[] = []
      i++
      while (i < lines.length && !lines[i].trim().startsWith(mark)) {
        body.push(lines[i].slice(Math.min(pad, indentOf(lines[i]))))
        i++
      }
      i++
      out.push({ t: 'code', lang: fence[2] || '', v: body.join('\n') })
      continue
    }

    const h = HEADING.exec(line)
    if (h) {
      const c = inline(h[2])
      const base = slug(plain(c))
      const n = used.get(base) ?? 0
      used.set(base, n + 1)
      out.push({ t: 'heading', level: h[1].length, id: n ? `${base}-${n}` : base, c })
      i++
      continue
    }

    if (RULE.test(line)) { out.push({ t: 'hr' }); i++; continue }

    const img = IMAGE.exec(line)
    if (img) {
      i++
      const name = IMAGE_NAME.exec(img[2])?.[1]
      // Anything but a picture of ours says what it is in words, and nothing is fetched.
      if (!name) { if (img[1]) out.push({ t: 'p', c: [{ t: 'text', v: img[1] }] }); continue }
      const under = CAPTION.exec(lines[i] ?? '')
      if (under) i++
      out.push({ t: 'figure', name, alt: img[1], caption: under ? inline(under[1]) : [] })
      continue
    }

    if (/^\s*>/.test(line)) {
      const body: string[] = []
      while (i < lines.length && lines[i].trim() !== '' && (/^\s*>/.test(lines[i]) || !startsBlock(lines[i], lines[i + 1]))) {
        body.push(lines[i++].replace(/^\s*> ?/, ''))
      }
      out.push({ t: 'quote', c: depth < MAX_DEPTH ? parseLines(body, used, depth + 1) : [{ t: 'p', c: inline(body.join(' ')) }] })
      continue
    }

    const item = LIST_ITEM.exec(line)
    if (item && depth < MAX_DEPTH) {
      const pad = item[1].length
      const ordered = /\d/.test(item[2])
      const start = ordered ? parseInt(item[2], 10) : 1
      const items: Block[][] = []
      while (i < lines.length) {
        const m = LIST_ITEM.exec(lines[i])
        if (!m || m[1].length < pad || m[1].length > pad + 1 || /\d/.test(m[2]) !== ordered) break
        const inner = m[0].length - m[3].length
        const body = [m[3]]
        i++
        // What belongs to this item: anything indented past its marker, blank lines included
        // when more of the item follows them.
        while (i < lines.length) {
          const l = lines[i]
          if (l.trim() === '') {
            let k = i + 1
            while (k < lines.length && lines[k].trim() === '') k++
            if (k < lines.length && indentOf(lines[k]) >= inner) { body.push(''); i++; continue }
            break
          }
          if (indentOf(l) >= inner) { body.push(l.slice(inner)); i++; continue }
          if (LIST_ITEM.test(l) || startsBlock(l, lines[i + 1])) break
          body.push(l.trim()) // a lazy continuation line
          i++
        }
        items.push(parseLines(body, used, depth + 1))
        // One blank line between items doesn't end the list.
        let k = i
        while (k < lines.length && lines[k].trim() === '') k++
        const again = k < lines.length ? LIST_ITEM.exec(lines[k]) : null
        if (again && again[1].length >= pad && again[1].length <= pad + 1 && /\d/.test(again[2]) === ordered) i = k
        else break
      }
      out.push({ t: 'list', ordered, start, items })
      continue
    }

    if (line.includes('|') && i + 1 < lines.length && TABLE_RULE.test(lines[i + 1]) && lines[i + 1].includes('-')) {
      const head = cells(line).map((c) => inline(c))
      const align: Align[] = cells(lines[i + 1]).map((c) => {
        const l = c.startsWith(':'), r = c.endsWith(':')
        return l && r ? 'center' : r ? 'right' : l ? 'left' : undefined
      })
      const rows: Inline[][][] = []
      i += 2
      while (i < lines.length && lines[i].trim() !== '' && lines[i].includes('|')) {
        const row = cells(lines[i]).map((c) => inline(c))
        while (row.length < head.length) row.push([])
        rows.push(row.slice(0, head.length))
        i++
      }
      out.push({ t: 'table', head, align, rows })
      continue
    }

    // A raw HTML line says nothing to a reader here: footers, and the odd centred header.
    if (HTML.test(line)) { i++; continue }

    const para: string[] = []
    while (i < lines.length && lines[i].trim() !== '' && !startsBlock(lines[i], lines[i + 1]) && !HTML.test(lines[i])) {
      para.push(lines[i++].trim())
    }
    if (para.length === 0) { i++; continue }
    out.push({ t: 'p', c: inline(para.join('\n')) })
  }
  return out
}

export function parse(md: string): Block[] {
  return parseLines(md.replace(/\r\n?/g, '\n').split('\n'), new Map(), 0)
}

// ---------------------------------------------------------------- addresses

// What a link in a guide leads to, given which guides are in the app. Anything that isn't a
// plain web address, a mail address or a guide comes back null, and is shown as plain text.
export type Target = { kind: 'guide'; file: string; hash: string } | { kind: 'here'; hash: string } | { kind: 'web'; href: string }

export function target(href: string): Target | null {
  if (href.startsWith('#')) return { kind: 'here', hash: href.slice(1) }
  if (/^(https?:\/\/|mailto:)/i.test(href)) return { kind: 'web', href }
  const m = /^(?:\.{0,2}\/)*(?:docs\/)?([A-Za-z0-9_-]+)\.md(?:#(.*))?$/.exec(href)
  return m ? { kind: 'guide', file: m[1], hash: m[2] ?? '' } : null
}
