import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { centerline, cite, colorName, Define, exportNotes, Field, fill, HighlightEditor, hits, IconBack, IconBookmark, IconClose, IconContents, IconHandwriting, IconNext, IconNotes, IconPrev, IconSearch, IconSettings, IconSpeaker, IconStop, InkBar, InkDefs, InkStroke, loadInk, NotesPanel, outline, pointPressure, Range, saveInk, SelectionBar, Sheet, useAnnotations, useColorLabels, useHost, type Ann, type DrawTool, type InkPrefs, type Stroke } from '@wittywilla/wittyink'
import './ebook.css'

// WittyEbook: the EPUB reader. Book view (two pages side by side), one page or
// scrolling; the reader's own typography and page colours; brightness and
// warmth; highlights in any colour with notes and named colour meanings;
// handwriting with pen pressure (pen, pencil, highlighter, eraser); bookmarks;
// footnotes in a pop-up; search in the book; definitions; read aloud;
// citations; and an honest "time left" from the reader's own speed.
// Annotations and the place in the book are kept per account on the server,
// so every device continues where you were.
//
// Book pages come from the host with scripts blocked (CSP) and are shown
// in a sandboxed frame without scripts; this component styles and reads the
// frame's document from the outside.

type Prefs = {
  layout: 'auto' | 'single' | 'book' | 'scroll'; font: number; family: 'publisher' | 'serif' | 'sans' | 'readable' | 'mono'
  line: number; margin: number; justify: boolean; theme: 'light' | 'sepia' | 'dark' | 'black'; brightness: number; warmth: number
}
const DEF: Prefs = { layout: 'auto', font: 100, family: 'publisher', line: 1.55, margin: 2, justify: true, theme: 'light', brightness: 1, warmth: 0 }
const PREFS_KEY = 'wms.ereader'
const THEMES: Record<Prefs['theme'], [string, string, string]> = {
  light: ['#ffffff', '#1d1d1f', '#6b6b70'], sepia: ['#f4ecd8', '#3b2f1e', '#7a6a50'], dark: ['#1c1c1e', '#e5e5e7', '#98989d'], black: ['#000000', '#d4d4d6', '#8e8e93'],
}
const FAMILIES: Record<Prefs['family'], string> = {
  publisher: '', serif: "Georgia, 'Iowan Old Style', 'Palatino Linotype', serif", sans: "system-ui, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif",
  readable: "Verdana, Tahoma, 'Trebuchet MS', sans-serif", mono: "ui-monospace, Consolas, 'Courier New', monospace",
}

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


export default function EpubReader() {
  const host = useHost()
  const { book: it, close, toast } = host
  const info = host.epub!.info
  const spine = info.spine
  const words = info.words.length === spine.length ? info.words : spine.map(() => 0)
  const totalWords = words.reduce((a, b) => a + b, 0) || 1
  const [p, setP] = useState(loadPrefs)
  const start = host.start
  const [chapter, setChapter] = useState(Math.min(Math.floor(start), Math.max(0, spine.length - 1)))
  const [page, setPage] = useState(0)
  const [pages, setPages] = useState(1)
  const pending = useRef<{ frac?: number; quote?: { quote: string; prefix: string }; anchor?: string } | null>({ frac: start % 1 })
  const A = useAnnotations()
  const anns = A.anns
  const [labels, setLabel] = useColorLabels()
  const [panel, setPanel] = useState<null | 'contents' | 'notes' | 'search' | 'display'>(null)
  const [sel, setSel] = useState<{ x: number; y: number; y2?: number; quote: string; prefix: string; suffix: string } | null>(null)
  const [edit, setEdit] = useState<{ ann: Ann; x: number; y: number } | null>(null)
  const [define, setDefine] = useState<string | null>(null)
  const [foot, setFoot] = useState<{ text: string; chapter: number; anchor: string } | null>(null)
  const [query, setQuery] = useState('')
  const [hitsFound, setHits] = useState<{ chapter: number; snippet: string; quote: string; prefix: string }[] | null>(null)
  const [bar, setBar] = useState(true)
  const [speaking, setSpeaking] = useState(false)
  const [ink, setInkState] = useState(loadInk)
  const [inkMode, setInkMode] = useState(false)
  const [tick, setTick] = useState(0) // the layout moved: place ink again
  const frame = useRef<HTMLIFrameElement | null>(null)
  const live = useRef<SVGPathElement | null>(null)
  const drawing = useRef<null | { id: number; tool: DrawTool | 'eraser'; pts: number[]; gone: Set<number> }>(null)
  const lastDraw = useRef(0)
  const turnTimes = useRef<number[]>([])
  const barTimer = useRef<any>(null)
  const savePos = useRef(-1)

  const setPrefs = (v: Partial<Prefs>) => setP((cur) => { const n = { ...cur, ...v }; try { localStorage.setItem(PREFS_KEY, JSON.stringify(n)) } catch { /* ignore */ } return n })
  const setInk = (v: Partial<InkPrefs>) => setInkState((cur) => { const n = { ...cur, ...v }; saveInk(n); return n })
  // Two pages need room: "Auto" shows them on wide landscape screens, and
  // even "Two pages" shows one on a phone.
  const wide = typeof window !== 'undefined' && window.innerWidth >= 1000 && window.innerWidth > window.innerHeight
  const roomy = typeof window !== 'undefined' && window.innerWidth >= 700
  const cols = (p.layout === 'book' && roomy) || (p.layout === 'auto' && wide) ? 2 : 1
  const paged = p.layout !== 'scroll'
  const dark = p.theme === 'dark' || p.theme === 'black'
  const pokeBar = () => { setBar(true); clearTimeout(barTimer.current); barTimer.current = setTimeout(() => setBar(false), 3500) }
  useEffect(() => { pokeBar(); return () => clearTimeout(barTimer.current) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  // A mouse near the top or bottom edge brings the bars back.
  const edgeHover = (y: number) => { if (y < 72 || y > window.innerHeight - 96) pokeBar() }


  const save = useCallback((pos: number) => {
    if (Math.abs(pos - savePos.current) < 0.002) return
    savePos.current = pos
    host.saveProgress(pos, spine.length)
  }, [host, spine.length])

  // ---------------------------------------------------------- layout
  const geometry = () => {
    // The frame's exact width: rounding it would make the columns drift
    // from the pages a little more on every turn.
    const r = frame.current!.getBoundingClientRect()
    const W = r.width, H = Math.floor(r.height)
    const gap = Math.max(32, Math.round(W * 0.04))
    return { W, H, gap, step: W + gap }
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
    const { H, gap } = geometry()
    let st = d.getElementById('wms-style') as HTMLStyleElement | null
    if (!st) { st = d.createElement('style'); st.id = 'wms-style'; (d.head || d.documentElement).appendChild(st) }
    const fam = FAMILIES[p.family]
    st.textContent = `
      html{background:${bg}!important;color:${fg}!important;font-size:${p.font}%!important;-webkit-text-size-adjust:none;touch-action:${inkMode ? 'none' : paged ? 'pinch-zoom' : 'auto'}}
      ${inkMode ? 'html{cursor:crosshair;-webkit-user-select:none;user-select:none}' : ''}
      body{margin:0!important;background:transparent!important;color:inherit!important;line-height:${p.line}!important;${paged ? 'padding:0!important' : 'padding:28px 0 120px!important'}}
      ${fam ? `body,p,div,span,li,td,blockquote,h1,h2,h3,h4,h5,h6{font-family:${fam}!important}` : ''}
      p,li{text-align:${p.justify ? 'justify' : 'start'}!important;hyphens:${p.justify ? 'auto' : 'manual'}}
      a{color:inherit!important} img,svg,video{max-width:100%!important;height:auto!important;max-height:${H - 80}px!important;object-fit:contain}
      ::selection{background:rgba(10,132,255,.3)}
      mark[data-ann]{background:var(--c)!important;color:inherit!important;border-radius:2px;cursor:pointer}
      mark[data-ann][data-note]{text-decoration:underline dotted ${muted};text-underline-offset:3px}
      mark.wms-flash{background:rgba(255,159,10,.55)!important;color:inherit!important}
      ${paged ? `html{height:${H}px!important;column-count:${cols}!important;column-width:auto!important;column-gap:${gap}px!important;column-fill:auto!important;overflow:hidden!important}
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
      if (labels[a.color]) attrs.title = labels[a.color]
      wrap(d, idx, at, at + a.quote.length, attrs)
    }
    setTick((t) => t + 1)
  }, [anns, chapter, labels])

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
    for (const [x, y] of (paged ? [[6, 8], [24, 30], [6, 60]] : [[6, 44], [24, 70], [6, 110]])) {
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
    on(d, 'mousemove', 'hover', { passive: true })
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
  useEffect(() => { if (inkMode) { setSel(null); setEdit(null); doc()?.getSelection()?.removeAllRanges() } }, [inkMode]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (paged && loaded.current) save(chapter + textFrac()) }, [chapter, page, pages, paged, save]) // eslint-disable-line react-hooks/exhaustive-deps

  // ---------------------------------------------------------- navigation
  const goChapter = (c: number, to: { frac?: number; quote?: { quote: string; prefix: string }; anchor?: string } = {}) => {
    if (c < 0 || c >= spine.length) return
    pending.current = to
    if (c === chapter) place()
    else { loaded.current = false; setChapter(c) }
  }
  const turn = (dir: 1 | -1) => {
    if (!loaded.current) return // the next chapter is still loading (a held arrow key must not race through the book)
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
    if (mod && e.key.toLowerCase() === 'z') { e.preventDefault(); if (e.shiftKey) A.redo(); else A.undo(); return }
    if (mod && e.key.toLowerCase() === 'y') { e.preventDefault(); A.redo(); return }
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
    fetch(host.epub!.chapterUrl(spine[c])).then((r) => r.text()).then((t) => {
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
    setSel({ x: o.left + r.left + r.width / 2, y: o.top + r.top, y2: o.top + r.bottom, quote, prefix: idx.text.slice(Math.max(0, at - 40), at), suffix: idx.text.slice(at + quote.length, at + quote.length + 40) })
    setEdit(null)
  }

  // ---------------------------------------------------------- annotations
  const posNow = () => chapter + textFrac()
  const deleteAnn = (a: Ann, undoable = true) => { setEdit(null); return A.remove(a, undoable) }
  const addHighlight = async (color: string, range = sel) => {
    if (!range) return null
    const a = await A.add({ kind: 'highlight', chapter, pos: posNow(), quote: range.quote, prefix: range.prefix, suffix: range.suffix, color: colorName(color) })
    doc()?.getSelection()?.removeAllRanges()
    setSel(null)
    return a
  }

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
    if (await A.create({ kind: 'bookmark', chapter, pos: posNow(), quote: (d ? textIndex(d).text : '').trim().slice(0, 80), color: '' })) toast('Bookmarked')
  }

  const chapterName = (c: number) => info.toc?.find((t) => t.href.split('#')[0] === spine[c])?.title.trim() || `Chapter ${c + 1}`

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
    // Outside ink mode a pen always writes; its eraser end or button erases.
    const eraser = (inkMode && ink.tool === 'eraser') || (pen && ((e.buttons & 32) !== 0 || (e.buttons & 2) !== 0))
    if (e.button !== 0 && !eraser) return
    e.preventDefault()
    setSel(null); setEdit(null); setFoot(null)
    doc()?.getSelection()?.removeAllRanges()
    try { (e.target as Element).setPointerCapture?.(e.pointerId) } catch { /* ignore */ }
    const tool: DrawTool | 'eraser' = eraser ? 'eraser' : ink.tool === 'eraser' ? 'pen' : ink.tool
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
    await A.add({ kind: 'ink', chapter, pos: posNow(), quote: idx.text.slice(at, at + 60).trim(), color: ink.color[tool], data: JSON.stringify(s) })
    live.current?.setAttribute('d', '')
  }

  // ---------------------------------------------------------- tools
  const search = async (q: string) => {
    setQuery(q); setPanel('search'); setHits(null)
    if (q.trim().length < 2) { setHits([]); return }
    try { setHits(await host.epub!.search(q.trim())) } catch { setHits([]) }
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
    hover: (e: MouseEvent) => { if (!drawing.current) edgeHover((frame.current?.getBoundingClientRect().top || 0) + e.clientY) },
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

  const [bg, fg] = THEMES[p.theme]
  const chapterTitle = info.toc?.find((t) => t.href.split('#')[0] === spine[chapter])?.title.trim()
  const margin = `${2 + p.margin * 2.5}%`
  const toc = info.toc?.length ? info.toc : spine.map((s, i) => ({ title: `Section ${i + 1}`, href: s }))
  const panelTitle = { contents: 'Contents', notes: 'Notes & highlights', search: 'Search', display: 'Text and display' }
  return (
    <div className={'reader epub2 theme-' + p.theme + (paged ? ' paged' : '') + (inkMode ? ' inking' : '')} style={{ background: bg, color: fg }} onMouseMove={(e) => edgeHover(e.clientY)}>
      <div className="epub-page" style={{ left: margin, right: margin }}>
        <iframe ref={frame} key={chapter + p.layout} className="epub-frame" title={it.title} sandbox="allow-same-origin"
          src={host.epub!.chapterUrl(spine[chapter])} onLoad={onLoad} />
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
      {bookmarkHere && <div className="rd-ribbon" aria-label="This page is bookmarked" />}

      <div className={'reader-bar' + (bar || panel || inkMode ? '' : ' hidden')}>
        <button className="ibtn" onClick={close} aria-label="Close the book" title="Close (Esc)"><IconBack /></button>
        <div className="rd-titles"><b>{it.title}</b>{chapterTitle && <span>{chapterTitle}</span>}</div>
        <button className={'ibtn' + (panel === 'contents' ? ' on' : '')} onClick={() => setPanel(panel === 'contents' ? null : 'contents')} aria-label="Contents" title="Contents"><IconContents /></button>
        <button className={'ibtn' + (panel === 'search' ? ' on' : '')} onClick={() => setPanel(panel === 'search' ? null : 'search')} aria-label="Search in the book" title="Search (/)"><IconSearch /></button>
        <button className={'ibtn' + (panel === 'notes' ? ' on' : '')} onClick={() => setPanel(panel === 'notes' ? null : 'notes')} aria-label="Notes and highlights" title="Notes and highlights"><IconNotes /></button>
        <span className="rd-gap rd-wide" />
        <button className={'ibtn' + (inkMode ? ' on' : '')} onClick={() => setInkMode(!inkMode)} aria-pressed={inkMode} aria-label="Handwriting" title="Pen, pencil and highlighter (p)"><IconHandwriting /></button>
        <button className={'ibtn' + (bookmarkHere ? ' on' : '')} onClick={toggleBookmark} aria-pressed={!!bookmarkHere} aria-label="Bookmark this page" title="Bookmark (b)"><IconBookmark on={!!bookmarkHere} /></button>
        <button className={'ibtn rd-wide' + (speaking ? ' on' : '')} onClick={() => readAloud()} aria-pressed={speaking} aria-label={speaking ? 'Stop reading aloud' : 'Read aloud'} title={speaking ? 'Stop reading' : 'Read aloud'}>{speaking ? <IconStop /> : <IconSpeaker />}</button>
        <button className={'ibtn' + (panel === 'display' ? ' on' : '')} onClick={() => setPanel(panel === 'display' ? null : 'display')} aria-label="Text and display" title="Text and display"><IconSettings /></button>
      </div>

      {inkMode ? (
        <InkBar p={ink} set={setInk} undo={A.undo} redo={A.redo} canUndo={A.canUndo} canRedo={A.canRedo} done={() => setInkMode(false)}
          extra={<><span className="ink-sep" /><button className="ink-tool" onClick={() => turn(-1)} aria-label="Previous page" title="Previous page"><IconPrev /></button><button className="ink-tool" onClick={() => turn(1)} aria-label="Next page" title="Next page"><IconNext /></button></>} />
      ) : (
        <div className={'reader-foot' + (bar || panel ? '' : ' hidden')}>
          <button className="ibtn" onClick={() => turn(-1)} aria-label="Previous page" title="Previous page"><IconPrev /></button>
          <Range min={0} max={1000} value={Math.round((doneWords / totalWords) * 1000)} label="Position in the book" valueText={`${pct}%`} onChange={seekBook} />
          <button className="ibtn" onClick={() => turn(1)} aria-label="Next page" title="Next page"><IconNext /></button>
          <div className="rd-foot-info">
            <b>{pct}%</b>
            {paged && pages > 1 && <><span className="rd-dot">·</span><span>Page {page + 1} of {pages} in chapter</span></>}
            <span className="rd-dot">·</span><span>{fmtMin(leftChapter)} left in chapter</span>
            <span className="rd-dot rd-wide">·</span><span className="rd-wide">{fmtMin(leftBook)} in book</span>
          </div>
        </div>
      )}
      {!bar && !panel && !inkMode && paged && <div className="epub-mini-progress" aria-hidden="true">{pct}%</div>}

      {panel && (
        <Sheet title={panelTitle[panel]} onClose={() => setPanel(null)}
          actions={panel === 'notes' && anns.length > 0 ? <button className="btn sm ghost" onClick={() => exportNotes(it, anns, labels, chapterName)} title="Save as Markdown">Export</button> : undefined}>
          {panel === 'contents' && toc.map((t, i) => {
            const [file, anchor] = t.href.split('#')
            const c = spine.findIndex((s) => s === file)
            // Only the first entry of the current chapter is marked, so a
            // chapter with sections shows one place.
            const on = c === chapter && toc.findIndex((u) => u.href.split('#')[0] === file) === i
            return <button key={i} disabled={c < 0} className={'rd-list-item' + (on ? ' on' : '')} aria-current={on ? 'true' : undefined} onClick={() => { goChapter(c, { anchor }); setPanel(null) }}>{t.title}</button>
          })}
          {panel === 'notes' && (
            <NotesPanel anns={anns} labels={labels} setLabel={setLabel} place={chapterName} remove={deleteAnn}
              open={(a) => { if (a.kind === 'highlight') goChapter(a.chapter, { quote: { quote: a.quote, prefix: a.prefix } }); else goChapter(a.chapter, { frac: a.pos - a.chapter }); setPanel(null) }} />
          )}
          {panel === 'search' && (
            <>
              <form onSubmit={(e) => { e.preventDefault(); search(query) }}>
                <input className="input" type="search" autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search in this book" aria-label="Search in this book" />
              </form>
              {hitsFound === null && query && <div className="spinner" />}
              {hitsFound && <div className="rd-count">{hitsFound.length === 0 ? 'No matches' : `${hitsFound.length}${hitsFound.length >= 200 ? '+' : ''} matches`}</div>}
              {!query && <p className="rd-empty">Type a word or phrase and press Enter.</p>}
              {hitsFound?.map((h, i) => (
                <button key={i} className="ep-hit" onClick={() => { goChapter(h.chapter, { quote: { quote: h.quote, prefix: h.prefix } }); setPanel(null) }}>
                  <span className="ep-where">{chapterName(h.chapter)}</span><Snippet text={h.snippet} q={h.quote} />
                </button>
              ))}
            </>
          )}
          {panel === 'display' && (
            <div className="ep-display">
              <Field label="Layout">
                <div className="seg">{([['auto', 'Auto'], ['single', 'One page'], ['book', 'Book view'], ['scroll', 'Scroll']] as const).map(([v, l]) => <button key={v} className={p.layout === v ? 'active' : ''} aria-pressed={p.layout === v} onClick={() => setPrefs({ layout: v })}>{l}</button>)}</div>
              </Field>
              <Field label="Page">
                <div className="ep-themes">{(Object.keys(THEMES) as Prefs['theme'][]).map((t) => (
                  <button key={t} className={p.theme === t ? 'on' : ''} style={{ background: THEMES[t][0], color: THEMES[t][1] }} onClick={() => setPrefs({ theme: t })} aria-pressed={p.theme === t} aria-label={t}><span>Aa<small>{t}</small></span></button>
                ))}</div>
              </Field>
              <Field label="Font">
                <select className="select" value={p.family} onChange={(e) => setPrefs({ family: e.target.value as Prefs['family'] })} aria-label="Font">
                  <option value="publisher">Publisher's font</option><option value="serif">Serif</option><option value="sans">Sans-serif</option><option value="readable">Extra readable</option><option value="mono">Monospace</option>
                </select>
              </Field>
              <Field label="Text size" value={`${p.font}%`}>
                <div className="ep-steps"><button className="btn sm" onClick={() => setPrefs({ font: Math.max(60, p.font - 10) })} aria-label="Smaller text"><span style={{ fontSize: 12 }}>A</span></button>
                  <Range min={60} max={250} step={5} value={p.font} onChange={(v) => setPrefs({ font: v })} label="Text size" />
                  <button className="btn sm" onClick={() => setPrefs({ font: Math.min(250, p.font + 10) })} aria-label="Larger text"><span style={{ fontSize: 18 }}>A</span></button></div>
              </Field>
              <Field label="Line spacing" value={p.line.toFixed(2)}><Range min={1.1} max={2.2} step={0.05} value={p.line} onChange={(v) => setPrefs({ line: v })} label="Line spacing" /></Field>
              <Field label="Margins" value={['None', 'Narrow', 'Small', 'Medium', 'Wide', 'Wider', 'Widest'][p.margin]}><Range min={0} max={6} step={1} value={p.margin} onChange={(v) => setPrefs({ margin: v })} label="Margins" /></Field>
              <Field label="Brightness" value={`${Math.round(p.brightness * 100)}%`}><Range min={0.35} max={1} step={0.05} value={p.brightness} onChange={(v) => setPrefs({ brightness: v })} label="Brightness" /></Field>
              <Field label="Warmth (night light)" value={p.warmth ? `${Math.round((p.warmth / 0.45) * 100)}%` : 'Off'}><Range min={0} max={0.45} step={0.05} value={p.warmth} onChange={(v) => setPrefs({ warmth: v })} label="Warmth" /></Field>
              <Field label="Paragraphs">
                <label className="check-row"><input type="checkbox" checked={p.justify} onChange={(e) => setPrefs({ justify: e.target.checked })} /> Justify and hyphenate</label>
              </Field>
              <button className="btn sm ghost rd-reset" onClick={() => setPrefs(DEF)}>Reset to defaults</button>
            </div>
          )}
        </Sheet>
      )}

      {sel && (
        <SelectionBar x={sel.x} y={sel.y} y2={sel.y2} quote={sel.quote} labels={labels} onHighlight={(c) => addHighlight(c)} onClose={() => setSel(null)}
          onNote={async () => { const s = sel; const a = await addHighlight('yellow'); if (a) setEdit({ ann: a, x: s.x, y: s.y2 ?? s.y + 20 }) }}
          onDefine={host.define ? setDefine : undefined} onCite={() => navigator.clipboard?.writeText(cite(it, sel.quote, chapterName(chapter)))} onSearch={search}
          onRead={() => readAloud(sel.quote.trim().slice(0, 40))} />
      )}

      {edit && (
        <HighlightEditor ann={edit.ann} x={edit.x} y={edit.y} labels={labels} onDone={() => setEdit(null)} onRemove={() => deleteAnn(edit.ann)}
          onColor={(c) => { A.update(edit.ann, { color: c }); setEdit({ ...edit, ann: { ...edit.ann, color: c } }) }} onNote={(note) => A.update(edit.ann, { note })}
          onCite={() => navigator.clipboard?.writeText(cite(it, edit.ann.quote, chapterName(edit.ann.chapter)))} />
      )}

      {foot && (
        <div className="ep-define" role="dialog" aria-label="Footnote">
          <div className="ep-head"><h3>Note</h3><button className="ibtn sm" onClick={() => setFoot(null)} aria-label="Close"><IconClose /></button></div>
          <p className="ep-foot-text">{foot.text}</p>
          <button className="btn sm ghost" onClick={() => { goChapter(foot.chapter, { anchor: foot.anchor }); setFoot(null) }}>Go to the note</button>
        </div>
      )}

      {define && <Define word={define} onClose={() => setDefine(null)} />}
    </div>
  )
}

// A search result with the words found marked.
function Snippet({ text, q }: { text: string; q: string }) {
  const i = q ? text.toLowerCase().indexOf(q.toLowerCase()) : -1
  if (i < 0) return <>{text}</>
  return <>{text.slice(0, i)}<mark>{text.slice(i, i + q.length)}</mark>{text.slice(i + q.length)}</>
}
