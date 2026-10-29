import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, mediaUrl, type Item } from '../api'
import { IconBack, IconBook, IconSearch, IconSettings } from '../icons'
import { centerline, hits, InkBar, InkDefs, InkStroke, loadInk, MARKER_COLORS, outline, pointPressure, saveInk, type DrawTool, type InkPrefs, type Stroke } from '../ink'
import { toast } from '../state'

// The EPUB reader. Book view (two pages side by side), one page or
// scrolling; the reader's own typography and page colours; brightness and
// warmth; highlights in any colour with notes and named colour meanings;
// handwriting with pen pressure (pen, pencil, highlighter, eraser); bookmarks;
// footnotes in a pop-up; search in the book; definitions; read aloud;
// citations; and an honest "time left" from the reader's own speed.
// Annotations and the place in the book are kept per account on the server,
// so every device continues where you were.
//
// Book pages come from the server with scripts blocked (CSP) and are shown
// in a sandboxed frame without scripts; this component styles and reads the
// frame's document from the outside.

export type BookInfo = { format: string; reading: string; title: string; spine?: string[]; toc?: { title: string; href: string }[]; words?: number[] }
type Ann = { id: number; kind: 'bookmark' | 'highlight' | 'ink'; chapter: number; pos: number; quote: string; prefix: string; suffix: string; color: string; note: string; data?: string; created: number }
type Prefs = {
  layout: 'auto' | 'single' | 'book' | 'scroll'; font: number; family: 'publisher' | 'serif' | 'sans' | 'readable' | 'mono'
  line: number; margin: number; justify: boolean; theme: 'light' | 'sepia' | 'dark' | 'black'; brightness: number; warmth: number
  labels: Record<string, string> // what each highlight colour means to the reader
}
const DEF: Prefs = { layout: 'auto', font: 100, family: 'publisher', line: 1.55, margin: 2, justify: true, theme: 'light', brightness: 1, warmth: 0, labels: {} }
const PREFS_KEY = 'wms.ereader'
const THEMES: Record<Prefs['theme'], [string, string, string]> = {
  light: ['#ffffff', '#1d1d1f', '#6b6b70'], sepia: ['#f4ecd8', '#3b2f1e', '#7a6a50'], dark: ['#1c1c1e', '#e5e5e7', '#98989d'], black: ['#000000', '#d4d4d6', '#8e8e93'],
}
const FAMILIES: Record<Prefs['family'], string> = {
  publisher: '', serif: "Georgia, 'Iowan Old Style', 'Palatino Linotype', serif", sans: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif",
  readable: "Verdana, Tahoma, 'Trebuchet MS', sans-serif", mono: "ui-monospace, Consolas, 'Courier New', monospace",
}
const NAMED: Record<string, string> = { yellow: '#ffd60a', green: '#34c759', blue: '#0a84ff', pink: '#ff375f' }
const QUICK = ['yellow', 'green', 'blue', 'pink']
const hex = (c: string) => NAMED[c] || c || NAMED.yellow
// A highlight colour as a see-through fill.
function fill(c: string, a = 0.38) {
  const h = hex(c)
  return `rgba(${parseInt(h.slice(1, 3), 16)},${parseInt(h.slice(3, 5), 16)},${parseInt(h.slice(5, 7), 16)},${a})`
}
// Bumped when the server's book headers change, so cached chapters refresh.
const READER_V = 2

function loadPrefs(): Prefs {
  try { return { ...DEF, ...JSON.parse(localStorage.getItem(PREFS_KEY) || '{}') } } catch { return DEF }
}
const fmtMin = (m: number) => (m < 1 ? 'less than a minute' : m < 60 ? `${Math.round(m)} min` : `${Math.floor(m / 60)} h ${Math.round(m % 60)} min`)
// A link target inside the book, relative to the chapter it is in.
const resolve = (base: string, rel: string) => decodeURIComponent(new URL(rel, 'https://book/' + base).pathname.slice(1))

// ---------------------------------------------------------------- text anchors

// The text of a chapter as one string, with the text node behind each part,
// so a highlight's quote or an ink stroke's word can be found again.
type TextIdx = { text: string; nodes: { node: Text; start: number }[] }
function textIndex(doc: Document): TextIdx {
  const nodes: TextIdx['nodes'] = []
  let text = ''
  const walker = doc.createTreeWalker(doc.body || doc.documentElement, NodeFilter.SHOW_TEXT)
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    nodes.push({ node: n as Text, start: text.length })
    text += n.nodeValue || ''
  }
  return { text, nodes }
}

function findQuote(idx: TextIdx, quote: string, prefix: string): number {
  if (!quote) return -1
  let best = -1
  for (let i = idx.text.indexOf(quote); i >= 0; i = idx.text.indexOf(quote, i + 1)) {
    if (best < 0) best = i
    if (prefix && idx.text.slice(Math.max(0, i - prefix.length), i).endsWith(prefix.slice(-20))) return i
  }
  return best
}

// The text node holding character `at` (binary search over node starts).
function nodeAt(idx: TextIdx, at: number) {
  let lo = 0, hi = idx.nodes.length - 1
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1
    if (idx.nodes[mid].start <= at) lo = mid; else hi = mid - 1
  }
  return idx.nodes[lo]
}

// Where character `at` is on screen (the frame's coordinates).
function charRect(doc: Document, idx: TextIdx, at: number): { x: number; y: number } | null {
  if (!idx.nodes.length) return null
  const n = nodeAt(idx, at)
  const len = (n.node.nodeValue || '').length
  const o = Math.max(0, Math.min(len - 1, at - n.start))
  const r = doc.createRange()
  r.setStart(n.node, o)
  r.setEnd(n.node, Math.min(len, o + 1))
  const rect = r.getClientRects()[0] || r.getBoundingClientRect()
  return rect ? { x: rect.left, y: rect.top } : null
}

// The text offset under a point in the frame.
function offsetAt(doc: Document, idx: TextIdx, x: number, y: number): number {
  const d = doc as any
  let node: Node | null = null, off = 0
  if (d.caretPositionFromPoint) { const c = d.caretPositionFromPoint(x, y); if (c) { node = c.offsetNode; off = c.offset } }
  else if (d.caretRangeFromPoint) { const c = d.caretRangeFromPoint(x, y); if (c) { node = c.startContainer; off = c.startOffset } }
  const hit = idx.nodes.find((n) => n.node === node)
  return hit ? hit.start + off : -1
}

// Wraps text from start to end in <mark> elements (one per text node).
function wrap(doc: Document, idx: TextIdx, start: number, end: number, attrs: Record<string, string>): HTMLElement[] {
  const marks: HTMLElement[] = []
  for (const { node, start: ns } of idx.nodes) {
    const ne = ns + (node.nodeValue || '').length
    if (ne <= start || ns >= end) continue
    const r = doc.createRange()
    r.setStart(node, Math.max(start, ns) - ns)
    r.setEnd(node, Math.min(end, ne) - ns)
    const m = doc.createElement('mark')
    for (const [k, v] of Object.entries(attrs)) m.setAttribute(k, v)
    try { r.surroundContents(m); marks.push(m) } catch { /* range across elements: skip that part */ }
  }
  return marks
}
const unwrap = (m: Element) => m.replaceWith(...Array.from(m.childNodes))

// ---------------------------------------------------------------- reader

type Op = { kind: 'add' | 'del'; ann: Ann }

export default function EpubReader({ it, info, close }: { it: Item; info: BookInfo; close: () => void }) {
  const spine = info.spine || []
  const words = info.words || spine.map(() => 0)
  const totalWords = words.reduce((a, b) => a + b, 0) || 1
  const [p, setP] = useState(loadPrefs)
  const start = it.progress && !it.progress.watched ? it.progress.positionSec : 0
  const [chapter, setChapter] = useState(Math.min(Math.floor(start), Math.max(0, spine.length - 1)))
  const [page, setPage] = useState(0)
  const [pages, setPages] = useState(1)
  const pending = useRef<{ frac?: number; quote?: { quote: string; prefix: string }; anchor?: string } | null>({ frac: start % 1 })
  const [anns, setAnns] = useState<Ann[]>([])
  const [panel, setPanel] = useState<null | 'contents' | 'notes' | 'search' | 'display'>(null)
  const [sel, setSel] = useState<{ x: number; y: number; quote: string; prefix: string; suffix: string; more?: boolean } | null>(null)
  const [edit, setEdit] = useState<{ ann: Ann; x: number; y: number } | null>(null)
  const [define, setDefine] = useState<{ word: string; result: any[] | null; error?: string } | null>(null)
  const [foot, setFoot] = useState<{ text: string; chapter: number; anchor: string } | null>(null)
  const [query, setQuery] = useState('')
  const [hitsFound, setHits] = useState<{ chapter: number; snippet: string; quote: string; prefix: string }[] | null>(null)
  const [filter, setFilter] = useState<{ kind: string; color: string; text: string }>({ kind: 'all', color: '', text: '' })
  const [bar, setBar] = useState(true)
  const [speaking, setSpeaking] = useState(false)
  const [ink, setInkState] = useState(loadInk)
  const [inkMode, setInkMode] = useState(false)
  const [tick, setTick] = useState(0) // the layout moved: place ink again
  const [, setHistory] = useState(0) // undo/redo stacks changed
  const undoStack = useRef<Op[]>([])
  const redoStack = useRef<Op[]>([])
  const frame = useRef<HTMLIFrameElement | null>(null)
  const live = useRef<SVGPathElement | null>(null)
  const drawing = useRef<null | { id: number; tool: DrawTool | 'eraser'; pts: number[]; gone: Set<number> }>(null)
  const lastDraw = useRef(0)
  const turnTimes = useRef<number[]>([])
  const barTimer = useRef<any>(null)
  const savePos = useRef(-1)

  const setPrefs = (v: Partial<Prefs>) => setP((cur) => { const n = { ...cur, ...v }; try { localStorage.setItem(PREFS_KEY, JSON.stringify(n)) } catch { /* ignore */ } return n })
  const setInk = (v: Partial<InkPrefs>) => setInkState((cur) => { const n = { ...cur, ...v }; saveInk(n); return n })
  const wide = typeof window !== 'undefined' && window.innerWidth >= 1000 && window.innerWidth > window.innerHeight
  const cols = p.layout === 'book' || (p.layout === 'auto' && wide) ? 2 : 1
  const paged = p.layout !== 'scroll'
  const dark = p.theme === 'dark' || p.theme === 'black'
  const pokeBar = () => { setBar(true); clearTimeout(barTimer.current); barTimer.current = setTimeout(() => setBar(false), 3500) }
  useEffect(() => { pokeBar(); return () => clearTimeout(barTimer.current) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { api.get<Ann[]>(`/items/${it.id}/annotations`).then(setAnns).catch(() => {}) }, [it.id])

  const save = useCallback((pos: number) => {
    if (Math.abs(pos - savePos.current) < 0.002) return
    savePos.current = pos
    api.post(`/items/${it.id}/progress`, { positionSec: pos, durationSec: spine.length }).catch(() => {})
  }, [it.id, spine.length])

  // ---------------------------------------------------------- layout
  const geometry = () => {
    const fr = frame.current!
    const W = fr.clientWidth, H = fr.clientHeight
    const gap = Math.max(32, Math.round(W * 0.04))
    return { W, H, gap, step: W + gap, cw: (W - (cols - 1) * gap) / cols }
  }
  // The chapter's document once it has loaded (before that the frame holds
  // an empty page, and laying that out would save a wrong position).
  const loaded = useRef(false)
  const doc = () => (loaded.current && frame.current?.contentDocument) || null
  const em = (d: Document) => parseFloat(d.defaultView?.getComputedStyle(d.body).fontSize || '16') || 16

  const style = () => {
    const d = doc()
    if (!d?.documentElement || !frame.current) return
    const [bg, fg, muted] = THEMES[p.theme]
    const { H, gap, cw } = geometry()
    let st = d.getElementById('wms-style') as HTMLStyleElement | null
    if (!st) { st = d.createElement('style'); st.id = 'wms-style'; (d.head || d.documentElement).appendChild(st) }
    const fam = FAMILIES[p.family]
    st.textContent = `
      html{background:${bg}!important;color:${fg}!important;font-size:${p.font}%!important;-webkit-text-size-adjust:none;touch-action:${inkMode ? 'none' : paged ? 'pinch-zoom' : 'auto'}}
      ${inkMode ? 'html{cursor:crosshair;-webkit-user-select:none;user-select:none}' : ''}
      body{margin:0!important;background:transparent!important;color:inherit!important;line-height:${p.line}!important;${paged ? 'padding:28px 0 36px!important' : 'padding:28px 0 120px!important'}}
      ${fam ? `body,p,div,span,li,td,blockquote,h1,h2,h3,h4,h5,h6{font-family:${fam}!important}` : ''}
      p,li{text-align:${p.justify ? 'justify' : 'start'}!important;hyphens:${p.justify ? 'auto' : 'manual'}}
      a{color:inherit!important} img,svg,video{max-width:100%!important;height:auto!important;max-height:${H - 80}px!important;object-fit:contain}
      ::selection{background:rgba(10,132,255,.3)}
      mark[data-ann]{background:var(--c)!important;color:inherit!important;border-radius:2px;cursor:pointer}
      mark[data-ann][data-note]{text-decoration:underline dotted ${muted};text-underline-offset:3px}
      mark.wms-flash{background:rgba(255,159,10,.55)!important;color:inherit!important}
      ${paged ? `html{height:${H}px!important;column-width:${cw}px!important;column-gap:${gap}px!important;column-fill:auto!important;overflow:hidden!important}
      body{height:${H}px!important;box-sizing:border-box}` : 'html{overflow-y:auto!important}'}`
  }

  const measure = () => {
    const d = doc()
    if (!d || !paged) return 1
    const { step, gap } = geometry()
    const n = Math.max(1, Math.ceil((d.documentElement.scrollWidth + gap) / step - 0.01))
    setPages(n)
    return n
  }
  const show = (pg: number, animate = 0) => {
    const d = doc()
    if (!d) return
    if (paged) {
      d.documentElement.scrollLeft = pg * geometry().step
      if (animate && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        frame.current?.parentElement?.animate([{ opacity: 0.55, transform: `translateX(${animate * 18}px)` }, { opacity: 1, transform: 'none' }], { duration: 170, easing: 'ease-out' })
      }
    }
    setPage(pg)
    setTick((t) => t + 1)
  }
  // Keeps the same place after the text reflows (settings or window size).
  const relayout = () => {
    const d = doc()
    if (!d?.documentElement) return
    const frac = page / Math.max(1, pages)
    style()
    const n = measure()
    if (paged) show(Math.min(n - 1, Math.round(frac * n)))
    else setTick((t) => t + 1)
  }

  // Marks for this chapter's highlights.
  const paint = useCallback(() => {
    const d = doc()
    if (!d?.body) return
    d.querySelectorAll('mark[data-ann]').forEach(unwrap)
    d.body.normalize()
    for (const a of anns.filter((x) => x.kind === 'highlight' && x.chapter === chapter)) {
      const idx = textIndex(d)
      const at = findQuote(idx, a.quote, a.prefix)
      if (at < 0) continue
      const attrs: Record<string, string> = { 'data-ann': String(a.id), style: `--c:${fill(a.color)}` }
      if (a.note) attrs['data-note'] = '1'
      if (p.labels[a.color]) attrs.title = p.labels[a.color]
      wrap(d, idx, at, at + a.quote.length, attrs)
    }
    setTick((t) => t + 1)
  }, [anns, chapter, p.labels])

  // Page of an element in the current chapter.
  const pageOf = (el: Element) => {
    const d = doc()!
    if (!paged) return 0
    return Math.max(0, Math.floor((el.getBoundingClientRect().left + d.documentElement.scrollLeft + 2) / geometry().step))
  }
  const reveal = (el: Element) => { if (paged) show(pageOf(el)); else el.scrollIntoView({ block: 'center' }) }
  const goToQuote = (quote: string, prefix: string) => {
    const d = doc()
    if (!d) return
    const idx = textIndex(d)
    const at = findQuote(idx, quote, prefix)
    if (at < 0) return
    const marks = wrap(d, idx, at, at + quote.length, { class: 'wms-flash' })
    if (!marks.length) return
    reveal(marks[0])
    setTimeout(() => marks.forEach(unwrap), 1800)
  }

  // Goes to the pending place (a fraction, a quote or an element id).
  const place = () => {
    const d = doc()
    if (!d) return
    const n = measure()
    const to = pending.current
    pending.current = null
    const target = to?.anchor ? d.getElementById(to.anchor) : null
    if (to?.quote) { if (paged) show(0); requestAnimationFrame(() => goToQuote(to.quote!.quote, to.quote!.prefix)) }
    else if (target) { if (paged) show(0); reveal(target) }
    else if (to?.frac) {
      // The fraction is of the chapter's text, so the same words open on
      // any screen or font size.
      if (paged) show(0)
      const idx = textIndex(d)
      const at = charRect(d, idx, Math.floor(to.frac * idx.text.length))
      if (paged) show(at ? Math.min(n - 1, Math.max(0, Math.floor((at.x + 1) / geometry().step))) : Math.min(n - 1, Math.floor(to.frac * n)))
      else { d.defaultView?.scrollTo(0, at ? at.y - 40 : to.frac * d.documentElement.scrollHeight); setTick((t) => t + 1) }
    } else show(0)
  }

  // Where the reader is, as a fraction of the chapter's text: the first
  // character on the page (or at the top of the window when scrolling).
  const textFrac = (): number => {
    const d = doc()
    if (!d) return 0
    const idx = textIndex(d)
    const len = idx.text.length || 1
    for (const [x, y] of [[6, 44], [24, 70], [6, 110]]) {
      const o = offsetAt(d, idx, x, y)
      if (o >= 0) return Math.min(0.9999, o / len)
    }
    return paged ? page / Math.max(1, pages) : 0
  }

  // Input in the chapter goes through H, so listeners added once per chapter
  // always reach this render's handlers (and its state).
  const H = useRef<Record<string, ((e: any) => void) | undefined>>({})
  const onLoad = () => {
    loaded.current = true
    const d = doc()
    if (!d) return
    style()
    paint()
    place()
    const on = (target: EventTarget, type: string, name: string, opts?: AddEventListenerOptions) => target.addEventListener(type, (e) => H.current[name]?.(e), opts)
    on(d, 'keydown', 'key')
    on(d, 'mouseup', 'select')
    on(d, 'click', 'click')
    on(d, 'pointerdown', 'down')
    on(d, 'pointermove', 'move')
    on(d, 'pointerup', 'up')
    on(d, 'pointercancel', 'up')
    on(d, 'touchstart', 'touchstart', { passive: true })
    on(d, 'touchend', 'touchend')
    if (d.defaultView) on(d.defaultView, 'scroll', 'scroll', { passive: true })
  }

  useEffect(() => { relayout() }, [p.layout, p.font, p.family, p.line, p.margin, p.justify, p.theme, cols, inkMode]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    let raf = 0
    const r = () => { cancelAnimationFrame(raf); raf = requestAnimationFrame(relayout) }
    window.addEventListener('resize', r)
    return () => { window.removeEventListener('resize', r); cancelAnimationFrame(raf) }
  })
  useEffect(() => { paint() }, [paint])
  useEffect(() => { if (paged && loaded.current) save(chapter + textFrac()) }, [chapter, page, pages, paged, save]) // eslint-disable-line react-hooks/exhaustive-deps

  // ---------------------------------------------------------- navigation
  const goChapter = (c: number, to: { frac?: number; quote?: { quote: string; prefix: string }; anchor?: string } = {}) => {
    if (c < 0 || c >= spine.length) return
    pending.current = to
    if (c === chapter) place()
    else { loaded.current = false; setChapter(c) }
  }
  const turn = (dir: 1 | -1) => {
    setSel(null); setEdit(null); setFoot(null)
    turnTimes.current = [...turnTimes.current.slice(-20), Date.now()]
    if (paged) {
      if (page + dir >= 0 && page + dir < pages) { show(page + dir, -dir); return }
    } else {
      const w = doc()?.defaultView
      const el = doc()?.documentElement
      if (w && el && !(dir > 0 && w.scrollY + w.innerHeight >= el.scrollHeight - 4) && !(dir < 0 && w.scrollY <= 0)) { w.scrollBy({ top: dir * w.innerHeight * 0.9, behavior: 'smooth' }); return }
    }
    goChapter(chapter + dir, { frac: dir < 0 ? 0.9999 : 0 })
  }

  const onKey = (e: KeyboardEvent) => {
    if ((e.target as HTMLElement)?.closest?.('input, textarea, select')) return
    const mod = e.ctrlKey || e.metaKey
    if (mod && e.key.toLowerCase() === 'z') { e.preventDefault(); if (e.shiftKey) redo(); else undo(); return }
    if (mod && e.key.toLowerCase() === 'y') { e.preventDefault(); redo(); return }
    if (mod) return
    if (e.key === 'Escape' || e.key === 'GoBack') {
      if (panel || sel || edit || define || foot) { setPanel(null); setSel(null); setEdit(null); setDefine(null); setFoot(null) } else if (inkMode) setInkMode(false); else close()
      return
    }
    if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') { e.preventDefault(); turn(1) }
    else if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); turn(-1) }
    else if (e.key === 'b') toggleBookmark()
    else if (e.key === 'p') setInkMode(!inkMode)
    else if (e.key === 'e' && inkMode) setInk({ tool: 'eraser' })
    else if (e.key === '/') { e.preventDefault(); setPanel('search') }
  }
  useEffect(() => { const k = (e: KeyboardEvent) => H.current.key?.(e); window.addEventListener('keydown', k); return () => window.removeEventListener('keydown', k) }, [])

  // Links: footnotes open in a pop-up, links in the book go to that place,
  // and web links open in a new tab (the frame itself never navigates).
  const followLink = (a: HTMLAnchorElement) => {
    const href = a.getAttribute('href') || ''
    if (/^[a-z][a-z0-9+.-]*:/i.test(href)) {
      if (/^(https?|mailto):/i.test(href)) window.open(href, '_blank', 'noopener,noreferrer')
      return
    }
    const [file, anchor = ''] = href.split('#')
    const target = file ? resolve(spine[chapter], file) : spine[chapter]
    const c = spine.indexOf(target)
    if (c < 0) return
    const isNote = /noteref/.test(a.getAttribute('epub:type') || a.getAttribute('role') || '') || /^\[?[\divx*†‡§]{1,4}\]?$/i.test((a.textContent || '').trim())
    if (!isNote || !anchor) { goChapter(c, { anchor }); return }
    const pick = (d: Document) => {
      const el = d.getElementById(anchor)
      const box = el && (el.closest('aside, li, p, div, section') || el)
      return (box?.textContent || '').replace(/\s+/g, ' ').trim()
    }
    const showNote = (text: string) => { if (text) setFoot({ text: text.slice(0, 2000), chapter: c, anchor }); else goChapter(c, { anchor }) }
    if (c === chapter) { showNote(pick(doc()!)); return }
    fetch(mediaUrl(`/api/items/${it.id}/epub/${spine[c]}?v=${READER_V}`)).then((r) => r.text()).then((t) => {
      let d = new DOMParser().parseFromString(t, 'application/xhtml+xml')
      if (d.querySelector('parsererror')) d = new DOMParser().parseFromString(t, 'text/html')
      showNote(pick(d))
    }).catch(() => goChapter(c, { anchor }))
  }

  const onClickDoc = (e: MouseEvent) => {
    const d = doc()!
    const target = e.target as HTMLElement
    const link = target.closest('a[href]') as HTMLAnchorElement | null
    if (link) e.preventDefault()
    if (inkMode || Date.now() - lastDraw.current < 400) return
    if (link) { followLink(link); return }
    const mark = target.closest('mark[data-ann]')
    if (mark) {
      const a = anns.find((x) => String(x.id) === mark.getAttribute('data-ann'))
      if (a) { const r = mark.getBoundingClientRect(); const o = frame.current!.getBoundingClientRect(); setEdit({ ann: a, x: o.left + r.left + r.width / 2, y: o.top + r.bottom }); return }
    }
    if (d.getSelection()?.toString()) return
    setFoot(null)
    const x = e.clientX / frame.current!.clientWidth
    if (x < 0.25) turn(-1)
    else if (x > 0.75) turn(1)
    else { setSel(null); setEdit(null); if (bar) setBar(false); else pokeBar() }
  }

  const onSelect = () => {
    const d = doc()
    const s = d?.getSelection()
    if (!d || !s || s.isCollapsed || !s.rangeCount || inkMode) { setSel(null); return }
    const range = s.getRangeAt(0)
    const quote = range.toString()
    if (!quote.trim()) { setSel(null); return }
    // Context around the quote, from the same text the marks are built on.
    const idx = textIndex(d)
    const pre = d.createRange()
    pre.setStart(d.body, 0)
    pre.setEnd(range.startContainer, range.startOffset)
    const at = pre.toString().length
    const r = range.getBoundingClientRect()
    const o = frame.current!.getBoundingClientRect()
    setSel({ x: o.left + r.left + r.width / 2, y: o.top + r.top, quote, prefix: idx.text.slice(Math.max(0, at - 40), at), suffix: idx.text.slice(at + quote.length, at + quote.length + 40) })
    setEdit(null)
  }

  // ---------------------------------------------------------- annotations
  const posNow = () => chapter + textFrac()
  const create = async (a: Partial<Ann>): Promise<Ann | null> => {
    try {
      const n = await api.post<Ann>(`/items/${it.id}/annotations`, { prefix: '', suffix: '', note: '', ...a, id: 0 })
      setAnns((x) => [...x, n].sort((m, k) => m.pos - k.pos))
      return n
    } catch (e: any) { toast(e.message); return null }
  }
  const record = (op: Op) => { undoStack.current = [...undoStack.current.slice(-199), op]; redoStack.current = []; setHistory((h) => h + 1) }
  const addHighlight = async (color: string, range = sel) => {
    if (!range) return null
    color = Object.keys(NAMED).find((k) => NAMED[k] === color.toLowerCase()) || color // one name per colour, for labels and filters
    const a = await create({ kind: 'highlight', chapter, pos: posNow(), quote: range.quote, prefix: range.prefix, suffix: range.suffix, color })
    if (a) record({ kind: 'add', ann: a })
    doc()?.getSelection()?.removeAllRanges()
    setSel(null)
    return a
  }
  const updateAnn = async (a: Ann, patch: Partial<Ann>) => {
    try { await api.put<Ann>(`/annotations/${a.id}`, { ...a, ...patch }); setAnns((x) => x.map((y) => (y.id === a.id ? { ...y, ...patch } : y))) } catch (e: any) { toast(e.message) }
  }
  const deleteAnn = async (a: Ann, undoable = true) => {
    setAnns((x) => x.filter((y) => y.id !== a.id))
    setEdit(null)
    try { await api.del(`/annotations/${a.id}`); if (undoable) record({ kind: 'del', ann: a }) } catch (e: any) { toast(e.message); setAnns((x) => [...x, a].sort((m, k) => m.pos - k.pos)) }
  }
  // Undo and redo for highlights and ink. A re-created annotation gets a new
  // id; the op keeps the object, so later steps follow it.
  const apply = async (op: Op, reverse: boolean) => {
    if ((op.kind === 'add') !== reverse) { const n = await create(op.ann); if (n) op.ann = n }
    else await deleteAnn(op.ann, false)
  }
  const undo = async () => { const op = undoStack.current.pop(); if (!op) return; await apply(op, true); redoStack.current.push(op); setHistory((h) => h + 1) }
  const redo = async () => { const op = redoStack.current.pop(); if (!op) return; await apply(op, false); undoStack.current.push(op); setHistory((h) => h + 1) }

  // A bookmark is on this page when its place in the text is.
  const bookmarkHere = useMemo(() => {
    const d = doc()
    const marks = anns.filter((a) => a.kind === 'bookmark' && a.chapter === chapter)
    if (!d || !marks.length || !frame.current) return undefined
    const idx = textIndex(d)
    const W = frame.current.clientWidth, Hh = frame.current.clientHeight
    return marks.find((a) => {
      const r = charRect(d, idx, Math.floor((a.pos - chapter) * idx.text.length))
      return !!r && r.x >= -2 && r.x < W && (paged || (r.y >= 0 && r.y < Hh))
    })
  }, [anns, chapter, tick]) // eslint-disable-line react-hooks/exhaustive-deps
  const toggleBookmark = async () => {
    if (bookmarkHere) { await deleteAnn(bookmarkHere, false); return }
    const d = doc()
    if (await create({ kind: 'bookmark', chapter, pos: posNow(), quote: (d ? textIndex(d).text : '').trim().slice(0, 80), color: '' })) toast('Bookmarked')
  }

  const chapterName = (c: number) => info.toc?.find((t) => t.href.split('#')[0] === spine[c])?.title.trim() || `Chapter ${c + 1}`
  const cite = (quote: string, c: number) => `“${quote.trim().replace(/\s+/g, ' ')}” (${[it.artist, `${it.title}${it.year ? ` (${it.year})` : ''}`, chapterName(c)].filter(Boolean).join(', ')})`
  const exportNotes = () => {
    const lines = [`# ${it.title}`, it.artist ? `*${it.artist}*` : '', '']
    let last = -1
    for (const a of anns.filter((x) => x.kind !== 'ink')) {
      if (a.chapter !== last) { lines.push(`## ${chapterName(a.chapter)}`, ''); last = a.chapter }
      if (a.kind === 'bookmark') lines.push(`- Bookmark: ${a.quote}…`)
      else lines.push(`> ${a.quote.replace(/\s+/g, ' ').trim()}`, '', `— ${p.labels[a.color] ? `**${p.labels[a.color]}** · ` : ''}${Math.round((a.pos - a.chapter) * 100)}% into the chapter`)
      if (a.note) lines.push('', a.note)
      lines.push('')
    }
    const strokes = anns.filter((x) => x.kind === 'ink').length
    if (strokes) lines.push(`*${strokes} handwritten strokes are kept in the reader.*`)
    const url = URL.createObjectURL(new Blob([lines.join('\n')], { type: 'text/markdown' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `${it.title} - notes.md`
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  // ---------------------------------------------------------- ink
  // A stroke is kept relative to the character it starts by, in ems, so it
  // stays with that text when the font, size or screen changes. Its outline
  // is worked out once (InkStroke is memoised); turning a page only moves it.
  const placed = useMemo(() => {
    const d = doc()
    const strokes = anns.filter((a) => a.kind === 'ink' && a.chapter === chapter)
    if (!d?.body || !strokes.length || !frame.current) return []
    const idx = textIndex(d)
    const k = em(d)
    const W = frame.current.clientWidth, Hh = frame.current.clientHeight
    const out: { a: Ann; s: Stroke; x: number; y: number; em: number }[] = []
    for (const a of strokes) {
      let s: Stroke
      try { s = JSON.parse(a.data || '') } catch { continue }
      const at = charRect(d, idx, s.a)
      if (!at || at.x < -W || at.x > 2 * W || at.y < -2 * Hh || at.y > 3 * Hh) continue
      out.push({ a, s, x: at.x, y: at.y, em: k })
    }
    return out
  }, [anns, chapter, tick]) // eslint-disable-line react-hooks/exhaustive-deps

  const onDown = (e: PointerEvent) => {
    const pen = e.pointerType === 'pen'
    if (pen && !ink.penOnly && !localStorage.getItem('wms.ink.pen')) {
      // The first pen: fingers stop drawing, so a resting palm leaves no marks.
      try { localStorage.setItem('wms.ink.pen', '1') } catch { /* ignore */ }
      setInk({ penOnly: true })
    }
    const active = inkMode ? !(ink.penOnly && e.pointerType === 'touch') : pen && ink.autoPen
    if (!active || drawing.current) return
    const eraser = ink.tool === 'eraser' || (pen && ((e.buttons & 32) !== 0 || (e.buttons & 2) !== 0))
    if (e.button !== 0 && !eraser) return
    e.preventDefault()
    setSel(null); setEdit(null); setFoot(null)
    doc()?.getSelection()?.removeAllRanges()
    try { (e.target as Element).setPointerCapture?.(e.pointerId) } catch { /* ignore */ }
    const tool: DrawTool | 'eraser' = eraser ? 'eraser' : (ink.tool as DrawTool)
    drawing.current = { id: e.pointerId, tool, pts: [], gone: new Set() }
    const lp = live.current
    if (lp && tool !== 'eraser') {
      const marker = tool === 'marker'
      lp.setAttribute('fill', marker ? 'none' : ink.color[tool])
      lp.setAttribute('stroke', marker ? ink.color[tool] : 'none')
      lp.setAttribute('stroke-width', String(ink.size[tool]))
      lp.setAttribute('opacity', String(ink.opacity[tool]))
      lp.style.mixBlendMode = marker && !dark ? 'multiply' : ''
    }
    addPoint(e)
  }
  const addPoint = (e: PointerEvent) => {
    const dr = drawing.current!
    if (dr.tool === 'eraser') {
      for (const o of placed) {
        if (!dr.gone.has(o.a.id) && hits(o.s.pts, o.s.w, (e.clientX - o.x) / o.em, (e.clientY - o.y) / o.em, 6 / o.em)) { dr.gone.add(o.a.id); deleteAnn(o.a) }
      }
      return
    }
    const n = dr.pts.length
    if (n && Math.hypot(e.clientX - dr.pts[n - 3], e.clientY - dr.pts[n - 2]) < 0.75) return
    dr.pts.push(e.clientX, e.clientY, pointPressure(e, dr.tool, ink.pressure))
  }
  const onMove = (e: PointerEvent) => {
    const dr = drawing.current
    if (!dr || e.pointerId !== dr.id) return
    e.preventDefault()
    const all = e.getCoalescedEvents?.() || []
    for (const c of all.length ? all : [e]) addPoint(c)
    if (dr.tool !== 'eraser' && live.current) live.current.setAttribute('d', dr.tool === 'marker' ? centerline(dr.pts) : outline(dr.pts, ink.size[dr.tool], true))
  }
  const onUp = async (e: PointerEvent) => {
    const dr = drawing.current
    if (!dr || e.pointerId !== dr.id) return
    drawing.current = null
    lastDraw.current = Date.now()
    const d = doc()
    if (dr.tool === 'eraser' || !d || !dr.pts.length) return
    const tool = dr.tool
    const idx = textIndex(d)
    const xs = dr.pts.filter((_, i) => i % 3 === 0), ys = dr.pts.filter((_, i) => i % 3 === 1)
    const k = em(d)
    // A highlighter line across text becomes a text highlight.
    if (tool === 'marker' && ink.snap && Math.max(...ys) - Math.min(...ys) < k * 1.6 && Math.max(...xs) - Math.min(...xs) > k) {
      const y = (Math.max(...ys) + Math.min(...ys)) / 2
      let a = offsetAt(d, idx, Math.min(...xs), y), b = offsetAt(d, idx, Math.max(...xs), y)
      if (a > b) [a, b] = [b, a]
      // Whole words, as a reader would mark them.
      const inWord = (c: string | undefined) => !!c && /[\p{L}\p{N}'’-]/u.test(c)
      if (a >= 0) { while (a > 0 && inWord(idx.text[a - 1])) a--; while (b < idx.text.length && inWord(idx.text[b])) b++ }
      const quote = a >= 0 && b > a ? idx.text.slice(a, b) : ''
      if (quote.trim()) {
        live.current?.setAttribute('d', '')
        await addHighlight(ink.color.marker, { x: 0, y: 0, quote, prefix: idx.text.slice(Math.max(0, a - 40), a), suffix: idx.text.slice(b, b + 40) })
        return
      }
    }
    let at = offsetAt(d, idx, dr.pts[0], dr.pts[1])
    if (at < 0) at = 0
    const origin = charRect(d, idx, at) || { x: 0, y: 0 }
    const pts = dr.pts.map((v, i) => (i % 3 === 2 ? Math.round(v * 100) / 100 : Math.round(((v - (i % 3 === 0 ? origin.x : origin.y)) / k) * 1000) / 1000))
    const s: Stroke = { t: tool, w: Math.round((ink.size[tool] / k) * 1000) / 1000, o: ink.opacity[tool], a: at, pts }
    const n = await create({ kind: 'ink', chapter, pos: posNow(), quote: idx.text.slice(at, at + 60).trim(), color: ink.color[tool], data: JSON.stringify(s) })
    live.current?.setAttribute('d', '')
    if (n) record({ kind: 'add', ann: n })
  }

  // ---------------------------------------------------------- tools
  const lookUp = (word: string) => {
    setSel(null)
    setDefine({ word, result: null })
    api.get<any[]>(`/dictionary?word=${encodeURIComponent(word)}`).then((r) => setDefine({ word, result: r })).catch((e) => setDefine({ word, result: [], error: e.message }))
  }
  const search = async (q: string) => {
    setQuery(q); setPanel('search'); setHits(null)
    if (q.trim().length < 2) { setHits([]); return }
    try { setHits(await api.get(`/items/${it.id}/search?q=${encodeURIComponent(q.trim())}`)) } catch { setHits([]) }
  }

  // Read aloud: paragraph by paragraph from the current page, turning pages
  // and chapters as it goes.
  const speech = useRef<{ stop: boolean }>({ stop: true })
  const readAloud = (fromQuote?: string) => {
    const synth = window.speechSynthesis
    if (!synth) { toast('This browser cannot read aloud'); return }
    if (speaking) { speech.current.stop = true; synth.cancel(); setSpeaking(false); return }
    const token = { stop: false }
    speech.current = token
    setSpeaking(true)
    const run = () => {
      const d = doc()
      if (!d || token.stop) return
      const blocks = Array.from(d.body.querySelectorAll('p, h1, h2, h3, h4, li, blockquote')).filter((b) => b.textContent?.trim())
      let i = blocks.findIndex((b) => (fromQuote ? b.textContent!.includes(fromQuote) : !paged || pageOf(b) >= page))
      fromQuote = undefined
      if (i < 0) i = 0
      const next = () => {
        if (token.stop) return
        if (i >= blocks.length) {
          if (H.current.nextChapter) { H.current.nextChapter(null); setTimeout(run, 700) } else setSpeaking(false)
          return
        }
        const b = blocks[i++]
        if (paged) H.current.show?.(b)
        const u = new SpeechSynthesisUtterance(b.textContent || '')
        u.lang = d.documentElement.lang || 'en'
        u.onend = next
        u.onerror = () => setSpeaking(false)
        synth.speak(u)
      }
      next()
    }
    run()
  }
  useEffect(() => () => { speech.current.stop = true; window.speechSynthesis?.cancel() }, [])

  // Touch swipes turn pages (unless a finger is drawing).
  const touchX = useRef<number | null>(null)
  H.current = {
    key: onKey, select: onSelect, click: onClickDoc, down: onDown, move: onMove, up: onUp,
    show: (b: Element) => reveal(b),
    nextChapter: chapter + 1 < spine.length ? () => goChapter(chapter + 1) : undefined,
    touchstart: (e: TouchEvent) => { touchX.current = drawing.current ? null : e.touches[0].clientX },
    touchend: (e: TouchEvent) => {
      if (touchX.current === null || doc()?.getSelection()?.toString()) { setTimeout(onSelect, 50); return }
      const dx = e.changedTouches[0].clientX - touchX.current
      touchX.current = null
      if (Math.abs(dx) > 50 && paged && !drawing.current) turn(dx < 0 ? 1 : -1)
      else setTimeout(onSelect, 50)
    },
    scroll: () => {
      if (paged) return
      const el = doc()!.documentElement
      save(chapter + (el.scrollTop + el.clientHeight >= el.scrollHeight - 4 ? 0.9999 : textFrac()))
      setTick((t) => t + 1)
    },
  }

  // ---------------------------------------------------------- time left
  const wpm = useMemo(() => {
    const t = turnTimes.current
    if (t.length < 4 || !paged) return 250
    const gaps = t.slice(1).map((x, i) => (x - t[i]) / 1000).filter((g) => g > 4 && g < 600)
    if (gaps.length < 3) return 250
    const secPerPage = gaps.reduce((a, b) => a + b, 0) / gaps.length
    const wordsPerPage = (words[chapter] || 250) / Math.max(1, pages)
    return Math.min(800, Math.max(80, (wordsPerPage / secPerPage) * 60))
  }, [page, chapter]) // eslint-disable-line react-hooks/exhaustive-deps
  const fracInChapter = paged ? page / Math.max(1, pages) : 0
  const leftChapter = ((words[chapter] || 0) * (1 - fracInChapter)) / wpm
  const leftBook = leftChapter + words.slice(chapter + 1).reduce((a, b) => a + b, 0) / wpm
  const doneWords = words.slice(0, chapter).reduce((a, b) => a + b, 0) + (words[chapter] || 0) * fracInChapter
  const pct = Math.round((doneWords / totalWords) * 100)
  const seekBook = (v: number) => {
    let acc = 0
    const target = (v / 1000) * totalWords
    for (let c = 0; c < spine.length; c++) {
      if (acc + words[c] >= target || c === spine.length - 1) { goChapter(c, { frac: Math.min(0.999, (target - acc) / Math.max(1, words[c])) }); return }
      acc += words[c]
    }
  }

  // ---------------------------------------------------------- notes panel
  const usedColors = [...new Set(anns.filter((a) => a.kind === 'highlight').map((a) => a.color))]
  const shown = anns.filter((a) => {
    if (a.kind === 'ink') return false // listed per chapter below
    if (filter.kind === 'highlight' && a.kind !== 'highlight') return false
    if (filter.kind === 'note' && !a.note) return false
    if (filter.kind === 'bookmark' && a.kind !== 'bookmark') return false
    if (filter.color && a.color !== filter.color) return false
    if (filter.text && !`${a.quote} ${a.note} ${p.labels[a.color] || ''}`.toLowerCase().includes(filter.text.toLowerCase())) return false
    return true
  })
  const inkChapters = [...new Set(anns.filter((a) => a.kind === 'ink').map((a) => a.chapter))]
  const palette = (current: string, onPick: (c: string) => void) => (
    <div className="ep-palette">
      {[...QUICK, ...MARKER_COLORS.filter((c) => !Object.values(NAMED).includes(c))].map((c) => (
        <button key={c} className={'ep-dot' + (current === c ? ' on' : '')} style={{ background: fill(c, 0.9) }} title={p.labels[c] || c} aria-label={p.labels[c] || c} onClick={() => onPick(c)} />
      ))}
      <input type="color" aria-label="Any colour" value={hex(current)} onChange={(e) => onPick(e.target.value)} />
    </div>
  )

  const [bg, fg] = THEMES[p.theme]
  const chapterTitle = info.toc?.find((t) => t.href.split('#')[0] === spine[chapter])?.title.trim()
  const margin = `${2 + p.margin * 2.5}%`
  return (
    <div className={'reader epub2 theme-' + p.theme + (inkMode ? ' inking' : '')} style={{ background: bg, color: fg }} onMouseMove={(e) => { if (e.clientY < 70 || e.clientY > window.innerHeight - 70) pokeBar() }}>
      <div className="epub-page" style={{ left: margin, right: margin }}>
        <iframe ref={frame} key={chapter + p.layout} className="epub-frame" title={it.title} sandbox="allow-same-origin"
          src={mediaUrl(`/api/items/${it.id}/epub/${spine[chapter]}?v=${READER_V}`)} onLoad={onLoad} />
        <svg className="ink-layer" aria-hidden="true">
          <InkDefs />
          {placed.map(({ a, s, x, y, em: k }) => (
            <g key={a.id} transform={`translate(${x} ${y}) scale(${k})`}><InkStroke s={s} color={a.color} blend={!dark} /></g>
          ))}
          <path ref={live} strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </div>
      {cols === 2 && paged && <div className="epub-spine" aria-hidden="true" />}
      {/* Brightness and warmth are layers over the page (nothing in the book changes). */}
      <div className="epub-light" style={{ background: `rgba(0,0,0,${1 - p.brightness})` }} aria-hidden="true" />
      <div className="epub-light warm" style={{ background: `rgba(255,140,0,${p.warmth})` }} aria-hidden="true" />
      {bookmarkHere && <div className="epub-ribbon" aria-label="This page is bookmarked" />}

      <div className={'reader-bar' + (bar || panel || inkMode ? '' : ' hidden')}>
        <button className="ibtn" onClick={close} aria-label="Close the book"><IconBack /></button>
        <div className="grow epub-titles"><b>{it.title}</b>{chapterTitle && <span className="muted">{chapterTitle}</span>}</div>
        <button className="ibtn" onClick={() => setPanel(panel === 'contents' ? null : 'contents')} aria-label="Contents" title="Contents"><IconBook /></button>
        <button className="ibtn" onClick={() => setPanel(panel === 'search' ? null : 'search')} aria-label="Search in the book" title="Search (/)"><IconSearch /></button>
        <button className="ibtn" onClick={() => setPanel(panel === 'notes' ? null : 'notes')} aria-label="Notes and highlights" title="Notes and highlights">☰</button>
        <button className={'ibtn' + (inkMode ? ' on' : '')} onClick={() => setInkMode(!inkMode)} aria-pressed={inkMode} aria-label="Handwriting" title="Pen, pencil and highlighter (p)">✎</button>
        <button className={'ibtn' + (bookmarkHere ? ' on' : '')} onClick={toggleBookmark} aria-pressed={!!bookmarkHere} aria-label="Bookmark this page" title="Bookmark (b)">🔖</button>
        <button className={'ibtn' + (speaking ? ' on' : '')} onClick={() => readAloud()} aria-pressed={speaking} aria-label="Read aloud" title="Read aloud">🔊</button>
        <button className="ibtn" onClick={() => setPanel(panel === 'display' ? null : 'display')} aria-label="Display settings" title="Text and display"><IconSettings /></button>
      </div>

      {inkMode ? (
        <InkBar p={ink} set={setInk} undo={undo} redo={redo} canUndo={undoStack.current.length > 0} canRedo={redoStack.current.length > 0} done={() => setInkMode(false)}
          extra={<><button className="ink-tool" onClick={() => turn(-1)} aria-label="Previous page">‹</button><button className="ink-tool" onClick={() => turn(1)} aria-label="Next page">›</button></>} />
      ) : (
        <div className={'reader-foot epub-foot' + (bar || panel ? '' : ' hidden')}>
          <button className="ibtn sm" onClick={() => turn(-1)} aria-label="Previous page">‹</button>
          <input type="range" min={0} max={1000} value={Math.round((doneWords / totalWords) * 1000)} aria-label="Position in the book" aria-valuetext={`${pct}%`} onChange={(e) => seekBook(Number(e.target.value))} />
          <button className="ibtn sm" onClick={() => turn(1)} aria-label="Next page">›</button>
          <span>{pct}% · {fmtMin(leftChapter)} left in chapter · {fmtMin(leftBook)} in book</span>
        </div>
      )}
      {!bar && !panel && !inkMode && paged && <div className="epub-mini-progress" aria-hidden="true">{pct}%</div>}

      {panel && (
        <aside className="epub-panel" role="dialog" aria-label={panel}>
          {panel === 'contents' && (
            <>
              <h3>Contents</h3>
              {(info.toc?.length ? info.toc : spine.map((s, i) => ({ title: `Section ${i + 1}`, href: s }))).map((t, i) => {
                const [file, anchor] = t.href.split('#')
                const c = spine.findIndex((s) => s === file)
                return <button key={i} disabled={c < 0} className={c === chapter ? 'on' : ''} onClick={() => { goChapter(c, { anchor }); setPanel(null) }}>{t.title}</button>
              })}
            </>
          )}
          {panel === 'notes' && (
            <>
              <div className="ep-head"><h3>Notes & highlights</h3>{anns.length > 0 && <button className="btn sm ghost" onClick={exportNotes} title="Save as Markdown">Export</button>}</div>
              <div className="seg ep-kinds">{([['all', 'All'], ['highlight', 'Highlights'], ['note', 'Notes'], ['bookmark', 'Bookmarks'], ['ink', 'Handwriting']] as const).map(([v, l]) => (
                <button key={v} className={filter.kind === v ? 'active' : ''} onClick={() => setFilter({ ...filter, kind: v })}>{l}</button>
              ))}</div>
              <input className="input" value={filter.text} onChange={(e) => setFilter({ ...filter, text: e.target.value })} placeholder="Filter notes" aria-label="Filter notes" />
              {usedColors.length > 0 && (
                <details className="ep-labels">
                  <summary>Colours{filter.color ? ' (filtered)' : ''}</summary>
                  <p className="muted small">Name what each colour means to you, like “Definition” or “Question”. Pick a colour to show only that one.</p>
                  {usedColors.map((c) => (
                    <div key={c} className="ep-label">
                      <button className={'ep-dot' + (filter.color === c ? ' on' : '')} style={{ background: fill(c, 0.9) }} aria-pressed={filter.color === c} aria-label={`Only ${p.labels[c] || c}`} onClick={() => setFilter({ ...filter, color: filter.color === c ? '' : c })} />
                      <input className="input" value={p.labels[c] || ''} placeholder="Name this colour" aria-label={`Meaning of ${c}`} onChange={(e) => setPrefs({ labels: { ...p.labels, [c]: e.target.value } })} />
                    </div>
                  ))}
                </details>
              )}
              {anns.length === 0 && <p className="muted">Select text to highlight it, add a note or look a word up. Draw with ✎ or any pen. Bookmark a page with 🔖.</p>}
              {filter.kind !== 'ink' && shown.map((a) => (
                <div key={a.id} className="ep-ann">
                  <button onClick={() => { if (a.kind === 'highlight') goChapter(a.chapter, { quote: { quote: a.quote, prefix: a.prefix } }); else goChapter(a.chapter, { frac: a.pos - a.chapter }); setPanel(null) }}>
                    <span className="ep-kind" style={{ background: a.kind === 'highlight' ? fill(a.color, 0.8) : 'transparent' }}>{a.kind === 'bookmark' ? '🔖' : ''}</span>
                    <span><span className="muted">{chapterName(a.chapter)}{p.labels[a.color] ? ` · ${p.labels[a.color]}` : ''}</span><br />{a.kind === 'bookmark' ? (a.quote || 'Bookmark') + '…' : `“${a.quote.slice(0, 220)}”`}{a.note && <em className="ep-note">{a.note}</em>}</span>
                  </button>
                  <button className="ibtn sm" onClick={() => deleteAnn(a)} aria-label="Delete">✕</button>
                </div>
              ))}
              {(filter.kind === 'ink' || filter.kind === 'all') && !filter.color && !filter.text && inkChapters.map((c) => {
                const strokes = anns.filter((a) => a.kind === 'ink' && a.chapter === c)
                return (
                  <div key={c} className="ep-ann">
                    <button onClick={() => { goChapter(c, { frac: strokes[0].pos - c }); setPanel(null) }}>
                      <span className="ep-kind">✎</span>
                      <span><span className="muted">{chapterName(c)}</span><br />{strokes.length} handwritten {strokes.length === 1 ? 'stroke' : 'strokes'}</span>
                    </button>
                  </div>
                )
              })}
            </>
          )}
          {panel === 'search' && (
            <>
              <h3>Search</h3>
              <form onSubmit={(e) => { e.preventDefault(); search(query) }}>
                <input className="input" autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search in this book" aria-label="Search in this book" />
              </form>
              {hitsFound === null && query && <div className="spinner" />}
              {hitsFound && <p className="muted">{hitsFound.length === 0 ? 'No matches' : `${hitsFound.length}${hitsFound.length >= 200 ? '+' : ''} matches`}</p>}
              {hitsFound?.map((h, i) => (
                <button key={i} className="ep-hit" onClick={() => { goChapter(h.chapter, { quote: { quote: h.quote, prefix: h.prefix } }); setPanel(null) }}>
                  <span className="muted">{chapterName(h.chapter)}</span> {h.snippet}
                </button>
              ))}
            </>
          )}
          {panel === 'display' && (
            <div className="ep-display">
              <h3>Text and display</h3>
              <label>Layout
                <div className="seg">{([['auto', 'Auto'], ['single', 'One page'], ['book', 'Book view'], ['scroll', 'Scroll']] as const).map(([v, l]) => <button key={v} className={p.layout === v ? 'active' : ''} onClick={() => setPrefs({ layout: v })}>{l}</button>)}</div>
              </label>
              <label>Font
                <select className="select" value={p.family} onChange={(e) => setPrefs({ family: e.target.value as Prefs['family'] })}>
                  <option value="publisher">Publisher's font</option><option value="serif">Serif</option><option value="sans">Sans-serif</option><option value="readable">Extra readable</option><option value="mono">Monospace</option>
                </select>
              </label>
              <label>Size <span className="muted">{p.font}%</span>
                <div className="ep-steps"><button className="btn sm" onClick={() => setPrefs({ font: Math.max(60, p.font - 10) })} aria-label="Smaller text">A−</button>
                  <input type="range" min={60} max={250} step={5} value={p.font} onChange={(e) => setPrefs({ font: Number(e.target.value) })} aria-label="Text size" />
                  <button className="btn sm" onClick={() => setPrefs({ font: Math.min(250, p.font + 10) })} aria-label="Larger text">A+</button></div>
              </label>
              <label>Line spacing <input type="range" min={1.1} max={2.2} step={0.05} value={p.line} onChange={(e) => setPrefs({ line: Number(e.target.value) })} /></label>
              <label>Margins <input type="range" min={0} max={6} step={1} value={p.margin} onChange={(e) => setPrefs({ margin: Number(e.target.value) })} /></label>
              <label className="check-row"><input type="checkbox" checked={p.justify} onChange={(e) => setPrefs({ justify: e.target.checked })} /> Justify and hyphenate</label>
              <label>Page
                <div className="ep-themes">{(Object.keys(THEMES) as Prefs['theme'][]).map((t) => (
                  <button key={t} className={p.theme === t ? 'on' : ''} style={{ background: THEMES[t][0], color: THEMES[t][1] }} onClick={() => setPrefs({ theme: t })} aria-label={t}>Aa</button>
                ))}</div>
              </label>
              <label>Brightness <input type="range" min={0.35} max={1} step={0.05} value={p.brightness} onChange={(e) => setPrefs({ brightness: Number(e.target.value) })} /></label>
              <label>Warmth <span className="muted">(night light)</span><input type="range" min={0} max={0.45} step={0.05} value={p.warmth} onChange={(e) => setPrefs({ warmth: Number(e.target.value) })} /></label>
              <button className="btn sm ghost" onClick={() => setPrefs({ ...DEF, labels: p.labels })}>Reset</button>
            </div>
          )}
          <button className="btn ghost ep-close" onClick={() => setPanel(null)}>Close</button>
        </aside>
      )}

      {sel && (
        <div className="ep-pop" style={{ left: Math.min(Math.max(190, sel.x), window.innerWidth - 190), top: Math.max(60, sel.y - 58) }} role="toolbar" aria-label="Selected text">
          {QUICK.map((c) => <button key={c} className="ep-dot" style={{ background: fill(c, 0.9) }} onClick={() => addHighlight(c)} title={p.labels[c] || c} aria-label={`Highlight ${p.labels[c] || c}`} />)}
          <button className="ep-dot more" onClick={() => setSel({ ...sel, more: !sel.more })} aria-expanded={!!sel.more} aria-label="More colours">+</button>
          <button onClick={async () => { const s = sel; const a = await addHighlight('yellow'); if (a) setEdit({ ann: a, x: s.x, y: s.y + 20 }) }}>Note</button>
          {sel.quote.trim().split(/\s+/).length <= 3 && <button onClick={() => lookUp(sel.quote.trim())}>Define</button>}
          <button onClick={() => { navigator.clipboard?.writeText(sel.quote); setSel(null); toast('Copied') }}>Copy</button>
          <button onClick={() => { navigator.clipboard?.writeText(cite(sel.quote, chapter)); setSel(null); toast('Copied with citation') }} title="Copy with author, title and chapter">Cite</button>
          <button onClick={() => { search(sel.quote.trim().slice(0, 80)); setSel(null) }}>Search</button>
          <button onClick={() => { const q = sel.quote.trim().slice(0, 40); setSel(null); readAloud(q) }}>Read aloud</button>
          {sel.more && <div className="ep-pop-more">{palette('', (c) => addHighlight(c))}</div>}
        </div>
      )}

      {edit && (
        <div className="ep-pop ep-edit" style={{ left: Math.min(Math.max(180, edit.x), window.innerWidth - 180), top: Math.max(60, Math.min(edit.y + 8, window.innerHeight - 320)) }} role="dialog" aria-label="Highlight">
          {palette(edit.ann.color, (c) => { updateAnn(edit.ann, { color: c }); setEdit({ ...edit, ann: { ...edit.ann, color: c } }) })}
          {p.labels[edit.ann.color] && <div className="small">{p.labels[edit.ann.color]}</div>}
          <textarea className="input" rows={4} placeholder="Add a note…" defaultValue={edit.ann.note} autoFocus
            onBlur={(e) => { if (e.target.value !== edit.ann.note) updateAnn(edit.ann, { note: e.target.value }) }} aria-label="Note" />
          <div className="ep-colors">
            <button className="btn sm ghost" onClick={() => { navigator.clipboard?.writeText(cite(edit.ann.quote, edit.ann.chapter)); toast('Copied with citation') }}>Cite</button>
            <button className="btn sm danger" onClick={() => deleteAnn(edit.ann)}>Remove</button>
            <span className="grow" />
            <button className="btn sm primary" onClick={() => setEdit(null)}>Done</button>
          </div>
        </div>
      )}

      {foot && (
        <div className="ep-define" role="dialog" aria-label="Footnote">
          <div className="ep-head"><h3>Note</h3><button className="ibtn" onClick={() => setFoot(null)} aria-label="Close">✕</button></div>
          <p className="ep-foot-text">{foot.text}</p>
          <button className="btn sm ghost" onClick={() => { goChapter(foot.chapter, { anchor: foot.anchor }); setFoot(null) }}>Go to the note</button>
        </div>
      )}

      {define && (
        <div className="ep-define" role="dialog" aria-label={`Definition of ${define.word}`}>
          <div className="ep-head"><h3>{define.word}</h3><button className="ibtn" onClick={() => setDefine(null)} aria-label="Close">✕</button></div>
          {define.result === null && <div className="spinner" />}
          {define.result?.length === 0 && <p className="muted">{define.error || 'No definition found.'}</p>}
          {define.result?.slice(0, 2).map((e: any, i: number) => (
            <div key={i}>
              {e.phonetic && <div className="muted">{e.phonetic}</div>}
              {e.meanings?.map((m: any, j: number) => (
                <div key={j} className="ep-sense"><i>{m.partOfSpeech}</i>
                  <ol>{m.definitions.map((d: any, k: number) => <li key={k}>{d.definition}{d.example && <div className="muted">“{d.example}”</div>}</li>)}</ol>
                </div>
              ))}
            </div>
          ))}
          <a className="muted" href={`https://en.wiktionary.org/wiki/${encodeURIComponent(define.word)}`} target="_blank" rel="noreferrer">More on Wiktionary ↗</a>
        </div>
      )}
    </div>
  )
}
