import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import * as pdfjs from 'pdfjs-dist/legacy/build/pdf.mjs'
import type { PDFDocumentProxy, PDFPageProxy, RenderTask } from 'pdfjs-dist/legacy/build/pdf.mjs'
import workerUrl from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url'
import { centerline, cite, colorName, Define, exportNotes, fill, HighlightEditor, hits, IconBack, IconBook, IconSearch, IconSettings, InkBar, InkDefs, InkStroke, loadInk, NotesPanel, outline, pointPressure, saveInk, SelectionBar, useAnnotations, useColorLabels, useHost, type Ann, type DrawTool, type InkPrefs, type Stroke } from '@wittywilla/wittyink'
import './pdf.css'

// WittyPDF: the PDF reader, for papers, textbooks and scans: pages drawn by pdf.js
// (loaded only when a PDF is opened) with selectable text, zoom, one or two
// pages across, outline and page thumbnails, search, and the same study
// tools as the EPUB reader: highlights with notes, handwriting with pen
// pressure, bookmarks, definitions and citations.
//
// Pages never reflow in a PDF, so highlights are kept as boxes on the page
// (fractions of its size) and ink in PDF points. Only pages near the screen
// are drawn, and canvases are capped in size, so long books stay light on
// low-end hardware.

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

type Size = { w: number; h: number } // a page in PDF points
type Box = [number, number, number, number, number] // page, x, y, w, h (fractions of the page)
type Prefs = { zoom: 'width' | 'page' | number; spread: boolean; theme: 'normal' | 'sepia' | 'dark' }
const DEF: Prefs = { zoom: 'width', spread: false, theme: 'normal' }
const PREFS_KEY = 'wms.pdfreader'
const GAP = 16
const MAX_PIXELS = 8e6 // per page canvas; keeps memory in check on phones and TVs
const ZOOMS = [0.5, 0.67, 0.8, 1, 1.25, 1.5, 2, 3, 4]
const FILTERS: Record<Prefs['theme'], string> = { normal: 'none', sepia: 'sepia(.35) brightness(.97)', dark: 'invert(.9) hue-rotate(180deg)' }

function loadPrefs(): Prefs {
  try { return { ...DEF, ...JSON.parse(localStorage.getItem(PREFS_KEY) || '{}') } } catch { return DEF }
}

// Joins the boxes of a selection that sit on the same line.
function mergeBoxes(boxes: Box[]): Box[] {
  const out: Box[] = []
  for (const b of [...boxes].sort((a, c) => a[0] - c[0] || a[2] - c[2] || a[1] - c[1])) {
    const last = out[out.length - 1]
    if (last && last[0] === b[0] && Math.abs(last[2] - b[2]) < Math.min(last[4], b[4]) * 0.5 && b[1] <= last[1] + last[3] + 0.01) {
      const x2 = Math.max(last[1] + last[3], b[1] + b[3]), y2 = Math.max(last[2] + last[4], b[2] + b[4])
      last[1] = Math.min(last[1], b[1]); last[2] = Math.min(last[2], b[2]); last[3] = x2 - last[1]; last[4] = y2 - last[2]
    } else out.push([...b] as Box)
  }
  return out.map((b) => b.map((v, i) => (i ? Math.round(v * 10000) / 10000 : v)) as Box)
}

// ---------------------------------------------------------------- one page

type PageProps = {
  doc: PDFDocumentProxy; index: number; size: Size; scale: number; draw: boolean; theme: Prefs['theme']
  boxes: { a: Ann; b: Box }[]; strokes: { a: Ann; s: Stroke }[]; dark: boolean; bookmarked: boolean; flash: string
  onSize: (i: number, s: Size) => void
}

const Page = memo(function Page({ doc, index, size, scale, draw, theme, boxes, strokes, dark, bookmarked, flash, onSize }: PageProps) {
  const canvas = useRef<HTMLCanvasElement | null>(null)
  const text = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (!draw) {
      // Off screen: give the memory back.
      if (canvas.current) { canvas.current.width = 0; canvas.current.height = 0 }
      if (text.current) text.current.replaceChildren()
      return
    }
    let task: RenderTask | null = null
    let layer: pdfjs.TextLayer | null = null
    let gone = false
    doc.getPage(index + 1).then(async (page: PDFPageProxy) => {
      if (gone) return
      const base = page.getViewport({ scale: 1 })
      if (Math.abs(base.width - size.w) > 0.5 || Math.abs(base.height - size.h) > 0.5) onSize(index, { w: base.width, h: base.height })
      const vp = page.getViewport({ scale })
      const ratio = Math.min(window.devicePixelRatio || 1, 2, Math.sqrt(MAX_PIXELS / (vp.width * vp.height)))
      const c = canvas.current!
      c.width = Math.floor(vp.width * ratio)
      c.height = Math.floor(vp.height * ratio)
      task = page.render({ canvasContext: c.getContext('2d', { alpha: false })!, viewport: vp, transform: ratio !== 1 ? [ratio, 0, 0, ratio, 0, 0] : undefined })
      try { await task.promise } catch { return } // cancelled
      if (gone || !text.current) return
      text.current.replaceChildren()
      layer = new pdfjs.TextLayer({ textContentSource: page.streamTextContent(), container: text.current, viewport: vp })
      try { await layer.render() } catch { /* cancelled */ }
    }).catch(() => {})
    return () => { gone = true; task?.cancel(); layer?.cancel() }
  }, [doc, index, scale, draw]) // eslint-disable-line react-hooks/exhaustive-deps

  // A search hit: briefly mark the text pieces that contain it.
  useEffect(() => {
    if (!flash || !text.current) return
    const want = flash.toLowerCase()
    const t = setTimeout(() => {
      const spans = Array.from(text.current?.querySelectorAll('span') || []).filter((s) => s.textContent?.toLowerCase().includes(want))
      spans.forEach((s) => s.classList.add('pdf-flash'))
      spans[0]?.scrollIntoView({ block: 'center' })
      setTimeout(() => spans.forEach((s) => s.classList.remove('pdf-flash')), 2000)
    }, 350)
    return () => clearTimeout(t)
  }, [flash])

  return (
    <div className="pdf-page" data-page={index} style={{ width: size.w * scale, height: size.h * scale, ['--scale-factor' as any]: scale }}>
      <canvas ref={canvas} style={{ filter: FILTERS[theme] }} aria-hidden="true" />
      <div className="pdf-marks" aria-hidden="true">
        {boxes.map(({ a, b }, i) => (
          <div key={a.id + ':' + i} data-ann={a.id} className={a.note ? 'noted' : ''}
            style={{ left: `${b[1] * 100}%`, top: `${b[2] * 100}%`, width: `${b[3] * 100}%`, height: `${b[4] * 100}%`, background: fill(a.color, dark ? 0.45 : 0.4) }} />
        ))}
      </div>
      <div ref={text} className="textLayer" />
      <svg className="pdf-ink" viewBox={`0 0 ${size.w} ${size.h}`} preserveAspectRatio="none" aria-hidden="true">
        {strokes.map(({ a, s }) => <InkStroke key={a.id} s={s} color={a.color} blend={!dark} grain="ink-grain-pdf" />)}
      </svg>
      {bookmarked && <div className="rd-ribbon" aria-label="Bookmarked page" />}
    </div>
  )
})

// A small picture of a page, drawn when it scrolls into the thumbnail list.
function Thumb({ doc, index, current, go }: { doc: PDFDocumentProxy; index: number; current: boolean; go: () => void }) {
  const c = useRef<HTMLCanvasElement | null>(null)
  useEffect(() => {
    const el = c.current
    if (!el) return
    let task: RenderTask | null = null
    const io = new IntersectionObserver(([e]) => {
      if (!e.isIntersecting || task) return
      io.disconnect()
      doc.getPage(index + 1).then((p) => {
        const vp = p.getViewport({ scale: 140 / p.getViewport({ scale: 1 }).width })
        el.width = vp.width; el.height = vp.height
        task = p.render({ canvasContext: el.getContext('2d')!, viewport: vp })
        task.promise.catch(() => {})
      })
    })
    io.observe(el)
    return () => { io.disconnect(); task?.cancel() }
  }, [doc, index])
  return (
    <button className={'pdf-thumb' + (current ? ' on' : '')} onClick={go} aria-label={`Page ${index + 1}`} aria-current={current ? 'page' : undefined}>
      <canvas ref={c} width={140} height={180} /><span>{index + 1}</span>
    </button>
  )
}

type OutlineItem = { title: string; dest: any; depth: number }

// ---------------------------------------------------------------- reader

export default function PdfReader() {
  const host = useHost()
  const { book: it, close, toast } = host
  const [doc, setDoc] = useState<PDFDocumentProxy | null>(null)
  const [err, setErr] = useState('')
  const [sizes, setSizes] = useState<Size[]>([])
  const [p, setP] = useState(loadPrefs)
  const [width, setWidth] = useState(window.innerWidth)
  const [current, setCurrent] = useState(0)
  const [near, setNear] = useState<Set<number>>(new Set())
  const [panel, setPanel] = useState<null | 'contents' | 'notes' | 'search' | 'display'>(null)
  const [tab, setTab] = useState<'outline' | 'pages'>('outline')
  const [outlineItems, setOutline] = useState<OutlineItem[] | null>(null)
  const [sel, setSel] = useState<{ x: number; y: number; quote: string; boxes: Box[] } | null>(null)
  const [edit, setEdit] = useState<{ ann: Ann; x: number; y: number } | null>(null)
  const [define, setDefine] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [found, setFound] = useState<{ page: number; snippet: string }[] | null>(null)
  const [flash, setFlash] = useState<{ page: number; q: string } | null>(null)
  const [bar, setBar] = useState(true)
  const [speaking, setSpeaking] = useState(false)
  const [ink, setInkState] = useState(loadInk)
  const [inkMode, setInkMode] = useState(false)
  const A = useAnnotations()
  const anns = A.anns
  const [labels, setLabel] = useColorLabels()
  const scroller = useRef<HTMLDivElement | null>(null)
  const live = useRef<SVGPathElement | null>(null)
  const drawing = useRef<null | { id: number; tool: DrawTool | 'eraser'; page: number; pts: number[]; gone: Set<number> }>(null)
  const pan = useRef<null | { id: number; x: number; y: number; left: number; top: number }>(null)
  const texts = useRef<Map<number, string>>(new Map())
  const keep = useRef<{ page: number; frac: number } | null>(null)
  const barTimer = useRef<any>(null)
  const saved = useRef(-1)
  const lastDraw = useRef(0)

  const setPrefs = (v: Partial<Prefs>) => setP((cur) => { const n = { ...cur, ...v }; try { localStorage.setItem(PREFS_KEY, JSON.stringify(n)) } catch { /* ignore */ } return n })
  const setInk = (v: Partial<InkPrefs>) => setInkState((cur) => { const n = { ...cur, ...v }; saveInk(n); return n })
  const dark = p.theme === 'dark'
  const pokeBar = () => { setBar(true); clearTimeout(barTimer.current); barTimer.current = setTimeout(() => setBar(false), 3500) }
  useEffect(() => { pokeBar(); return () => clearTimeout(barTimer.current) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Open the document. Scripts in PDFs never run (pdf.js has no scripting
  // here) and fonts are not compiled with eval.
  useEffect(() => {
    const task = pdfjs.getDocument({ ...host.pdf, isEvalSupported: false, enableXfa: false, disableAutoFetch: true, rangeChunkSize: 1 << 18 })
    task.promise.then(async (d) => {
      const first = (await d.getPage(1)).getViewport({ scale: 1 })
      setSizes(Array.from({ length: d.numPages }, () => ({ w: first.width, h: first.height })))
      const start = host.start
      keep.current = { page: Math.min(d.numPages - 1, Math.floor(start)), frac: start % 1 }
      setDoc(d)
    }).catch((e) => setErr(e?.message || 'This PDF cannot be opened'))
    return () => { task.destroy() }
  }, [host.key]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    // A new width changes the fitted zoom: keep the place across it.
    const r = () => {
      const w = scroller.current?.clientWidth || window.innerWidth
      setWidth((old) => { if (old !== w && !keep.current && scroller.current) keep.current = where(); return w })
    }
    r()
    window.addEventListener('resize', r)
    return () => window.removeEventListener('resize', r)
  }, [doc])

  // ---------------------------------------------------------- layout
  const n = sizes.length
  const cols = p.spread && n > 1 ? 2 : 1
  const maxW = useMemo(() => sizes.reduce((m, s) => Math.max(m, s.w), 1), [sizes])
  const maxH = useMemo(() => sizes.reduce((m, s) => Math.max(m, s.h), 1), [sizes])
  const scale = useMemo(() => {
    const avail = Math.max(200, width - 32 - (cols - 1) * GAP)
    if (p.zoom === 'width') return Math.min(4, avail / cols / maxW)
    if (p.zoom === 'page') return Math.min(avail / cols / maxW, (window.innerHeight - 40) / maxH)
    return p.zoom
  }, [p.zoom, width, cols, maxW, maxH])
  const pageEl = (i: number) => scroller.current?.querySelector(`[data-page="${i}"]`) as HTMLElement | null

  // Where the reader is: the page at the top of the window and how far down it.
  const where = () => {
    const sc = scroller.current
    if (!sc || !n) return { page: 0, frac: 0 }
    const top = sc.scrollTop + 8
    let lo = 0, hi = n - 1
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1
      if ((pageEl(mid)?.offsetTop ?? 0) <= top) lo = mid; else hi = mid - 1
    }
    const el = pageEl(lo)
    return { page: lo, frac: el ? Math.max(0, Math.min(0.999, (top - el.offsetTop) / el.offsetHeight)) : 0 }
  }
  const goTo = (page: number, frac = 0, smooth = false) => {
    const el = pageEl(Math.max(0, Math.min(n - 1, page)))
    if (el && scroller.current) scroller.current.scrollTo({ top: el.offsetTop + frac * el.offsetHeight - 8, behavior: smooth ? 'smooth' : 'auto' })
  }
  // Keep the place across zoom and layout changes.
  useLayoutEffect(() => {
    if (keep.current && n) { const k = keep.current; keep.current = null; goTo(k.page, k.frac) }
    onScroll()
  }, [scale, cols, n]) // eslint-disable-line react-hooks/exhaustive-deps
  const setZoom = (z: Prefs['zoom']) => { keep.current = where(); setPrefs({ zoom: z }) }
  // The next step up or down from the zoom now (fitted zooms fall between steps).
  const zoomBy = (dir: 1 | -1) => {
    const next = dir > 0 ? ZOOMS.find((z) => z > scale + 0.01) : [...ZOOMS].reverse().find((z) => z < scale - 0.01)
    if (next) setZoom(next)
  }

  const onScroll = () => {
    const sc = scroller.current
    if (!sc || !n) return
    const w = where()
    setCurrent(w.page)
    // Draw pages within a screen and a half of the window.
    const lo = sc.scrollTop - sc.clientHeight * 1.5, hi = sc.scrollTop + sc.clientHeight * 2.5
    const s = new Set<number>()
    for (let i = Math.max(0, w.page - 6); i < Math.min(n, w.page + 12); i++) {
      const el = pageEl(i)
      if (el && el.offsetTop + el.offsetHeight > lo && el.offsetTop < hi) s.add(i)
    }
    setNear((old) => (old.size === s.size && [...s].every((i) => old.has(i)) ? old : s))
    const pos = w.page + w.frac
    if (Math.abs(pos - saved.current) > 0.05) {
      saved.current = pos
      clearTimeout((onScroll as any).t)
      ;(onScroll as any).t = setTimeout(() => host.saveProgress(pos, n), 800)
    }
  }
  const onSize = useCallback((i: number, s: Size) => setSizes((cur) => { const c = [...cur]; c[i] = s; return c }), [])

  // Ctrl + wheel zooms the document, not the whole app.
  useEffect(() => {
    const sc = scroller.current
    if (!sc) return
    const wheel = (e: WheelEvent) => { if (e.ctrlKey) { e.preventDefault(); zoomBy(e.deltaY < 0 ? 1 : -1) } }
    sc.addEventListener('wheel', wheel, { passive: false })
    return () => sc.removeEventListener('wheel', wheel)
  })

  // ---------------------------------------------------------- page text
  const pageText = async (i: number): Promise<string> => {
    const c = texts.current.get(i)
    if (c !== undefined || !doc) return c || ''
    const content = await (await doc.getPage(i + 1)).getTextContent()
    const t = content.items.map((x: any) => (x.str || '') + (x.hasEOL ? '\n' : '')).join('').replace(/[ \t]+/g, ' ')
    texts.current.set(i, t)
    return t
  }
  const search = async (q: string) => {
    setQuery(q); setPanel('search'); setFound(null)
    const want = q.trim().toLowerCase()
    if (want.length < 2 || !doc) { setFound([]); return }
    const out: { page: number; snippet: string }[] = []
    for (let i = 0; i < n && out.length < 300; i++) {
      const t = await pageText(i)
      const low = t.toLowerCase()
      for (let at = low.indexOf(want); at >= 0 && out.length < 300; at = low.indexOf(want, at + want.length)) {
        out.push({ page: i, snippet: '…' + t.slice(Math.max(0, at - 50), at + want.length + 50).replace(/\s+/g, ' ') + '…' })
      }
      if (i % 20 === 19) setFound([...out]) // results appear while a long book is searched
    }
    setFound(out)
  }

  // ---------------------------------------------------------- outline
  useEffect(() => {
    if (!doc) return
    doc.getOutline().then((o) => {
      const flat: OutlineItem[] = []
      const walk = (items: any[] | null, depth: number) => items?.forEach((x) => { flat.push({ title: x.title, dest: x.dest, depth }); walk(x.items, depth + 1) })
      walk(o, 0)
      setOutline(flat)
    }).catch(() => setOutline([]))
  }, [doc])
  const goDest = async (dest: any) => {
    if (!doc) return
    try {
      const d = typeof dest === 'string' ? await doc.getDestination(dest) : dest
      if (!d) return
      const idx = typeof d[0] === 'number' ? d[0] : await doc.getPageIndex(d[0])
      // [ref, /XYZ, left, top, zoom]: top is in points from the page bottom.
      const top = d[1]?.name === 'XYZ' && typeof d[3] === 'number' ? 1 - d[3] / (sizes[idx]?.h || 1) : 0
      goTo(idx, Math.max(0, top))
    } catch { /* broken link in the PDF */ }
  }

  // ---------------------------------------------------------- selection
  const onSelect = () => {
    if (inkMode || drawing.current) return
    const s = window.getSelection()
    if (!s || s.isCollapsed || !s.rangeCount || !scroller.current?.contains(s.anchorNode)) { setSel(null); return }
    const quote = s.toString()
    if (!quote.trim()) { setSel(null); return }
    const range = s.getRangeAt(0)
    const boxes: Box[] = []
    for (const r of Array.from(range.getClientRects())) {
      if (r.width < 1 || r.height < 1) continue
      const el = (document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2) as HTMLElement | null)?.closest('[data-page]') as HTMLElement | null
      if (!el) continue
      const pr = el.getBoundingClientRect()
      boxes.push([Number(el.dataset.page), (r.left - pr.left) / pr.width, (r.top - pr.top) / pr.height, r.width / pr.width, r.height / pr.height])
    }
    if (!boxes.length) { setSel(null); return }
    const b = range.getBoundingClientRect()
    setSel({ x: b.left + b.width / 2, y: b.top, quote, boxes: mergeBoxes(boxes) })
    setEdit(null)
  }
  const addHighlight = async (color: string) => {
    if (!sel) return null
    const page = sel.boxes[0][0]
    const a = await A.add({ kind: 'highlight', chapter: page, pos: page + sel.boxes[0][2], quote: sel.quote, color: colorName(color), data: JSON.stringify(sel.boxes) })
    window.getSelection()?.removeAllRanges()
    setSel(null)
    return a
  }
  const deleteAnn = (a: Ann, undoable = true) => { setEdit(null); return A.remove(a, undoable) }

  // Highlight boxes and ink strokes per page.
  const byPage = useMemo(() => {
    const boxes = new Map<number, { a: Ann; b: Box }[]>()
    const strokes = new Map<number, { a: Ann; s: Stroke }[]>()
    for (const a of anns) {
      if (!a.data) continue
      try {
        if (a.kind === 'highlight') for (const b of JSON.parse(a.data) as Box[]) { const l = boxes.get(b[0]) || []; l.push({ a, b }); boxes.set(b[0], l) }
        else if (a.kind === 'ink') { const l = strokes.get(a.chapter) || []; l.push({ a, s: JSON.parse(a.data) }); strokes.set(a.chapter, l) }
      } catch { /* damaged entry */ }
    }
    return { boxes, strokes }
  }, [anns])
  const bookmarks = useMemo(() => new Map(anns.filter((a) => a.kind === 'bookmark').map((a) => [a.chapter, a])), [anns])
  const toggleBookmark = async () => {
    const b = bookmarks.get(current)
    if (b) { await deleteAnn(b, false); return }
    const t = (await pageText(current)).trim().slice(0, 80)
    if (await A.create({ kind: 'bookmark', chapter: current, pos: current, quote: t || `Page ${current + 1}`, color: '' })) toast('Bookmarked')
  }

  // Clicking a highlight opens it (the text layer sits on top, so find it by position).
  const onClick = (e: React.MouseEvent) => {
    if (inkMode || Date.now() - lastDraw.current < 400 || window.getSelection()?.toString()) return
    const el = (e.target as HTMLElement).closest('[data-page]') as HTMLElement | null
    if (!el) { setEdit(null); return }
    const r = el.getBoundingClientRect()
    const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height
    const hit = byPage.boxes.get(Number(el.dataset.page))?.find(({ b }) => x >= b[1] && x <= b[1] + b[3] && y >= b[2] && y <= b[2] + b[4])
    if (hit) setEdit({ ann: hit.a, x: e.clientX, y: e.clientY + 12 })
    else { setEdit(null); if (bar) setBar(false); else pokeBar() }
  }

  // ---------------------------------------------------------- ink
  // Strokes are kept in PDF points on their page, so they scale with zoom.
  const pageAt = (e: PointerEvent | React.PointerEvent) => {
    const el = (document.elementFromPoint(e.clientX, e.clientY) as HTMLElement | null)?.closest('[data-page]') as HTMLElement | null
    if (!el) return null
    const i = Number(el.dataset.page)
    const r = el.getBoundingClientRect()
    const k = r.width / (sizes[i]?.w || 1) // screen pixels per point
    return { i, r, k }
  }
  const onPointerDown = (e: React.PointerEvent) => {
    const pen = e.pointerType === 'pen'
    if (pen && !ink.penOnly && !localStorage.getItem('wms.ink.pen')) {
      try { localStorage.setItem('wms.ink.pen', '1') } catch { /* ignore */ }
      setInk({ penOnly: true })
    }
    // In ink mode with palm rejection, a finger moves the page instead.
    if (inkMode && ink.penOnly && e.pointerType === 'touch' && scroller.current) {
      pan.current = { id: e.pointerId, x: e.clientX, y: e.clientY, left: scroller.current.scrollLeft, top: scroller.current.scrollTop }
      return
    }
    const active = inkMode || (pen && ink.autoPen)
    if (!active || drawing.current) return
    // Outside ink mode a pen always writes; its eraser end or button erases.
    const eraser = (inkMode && ink.tool === 'eraser') || (pen && ((e.buttons & 32) !== 0 || (e.buttons & 2) !== 0))
    if (e.button !== 0 && !eraser) return
    const at = pageAt(e)
    if (!at) return
    e.preventDefault()
    setSel(null); setEdit(null)
    window.getSelection()?.removeAllRanges()
    try { (e.target as Element).setPointerCapture(e.pointerId) } catch { /* ignore */ }
    const tool: DrawTool | 'eraser' = eraser ? 'eraser' : ink.tool === 'eraser' ? 'pen' : ink.tool
    drawing.current = { id: e.pointerId, tool, page: at.i, pts: [], gone: new Set() }
    const lp = live.current
    if (lp && tool !== 'eraser') {
      const marker = tool === 'marker'
      lp.setAttribute('fill', marker ? 'none' : ink.color[tool])
      lp.setAttribute('stroke', marker ? ink.color[tool] : 'none')
      lp.setAttribute('stroke-width', String(ink.size[tool]))
      lp.setAttribute('opacity', String(ink.opacity[tool]))
      lp.style.mixBlendMode = marker && !dark ? 'multiply' : ''
    }
    addPoint(e.nativeEvent)
  }
  const addPoint = (e: PointerEvent) => {
    const dr = drawing.current!
    if (dr.tool === 'eraser') {
      const at = pageAt(e)
      if (!at) return
      const x = (e.clientX - at.r.left) / at.k, y = (e.clientY - at.r.top) / at.k
      for (const { a, s } of byPage.strokes.get(at.i) || []) {
        if (!dr.gone.has(a.id) && hits(s.pts, s.w, x, y, 6 / at.k)) { dr.gone.add(a.id); deleteAnn(a) }
      }
      return
    }
    const m = dr.pts.length
    if (m && Math.hypot(e.clientX - dr.pts[m - 3], e.clientY - dr.pts[m - 2]) < 0.75) return
    dr.pts.push(e.clientX, e.clientY, pointPressure(e, dr.tool, ink.pressure))
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const pn = pan.current
    if (pn && e.pointerId === pn.id && scroller.current) {
      scroller.current.scrollLeft = pn.left - (e.clientX - pn.x)
      scroller.current.scrollTop = pn.top - (e.clientY - pn.y)
      return
    }
    const dr = drawing.current
    if (!dr || e.pointerId !== dr.id) return
    e.preventDefault()
    const all = e.nativeEvent.getCoalescedEvents?.() || []
    for (const c of all.length ? all : [e.nativeEvent]) addPoint(c)
    if (dr.tool !== 'eraser' && live.current) live.current.setAttribute('d', dr.tool === 'marker' ? centerline(dr.pts) : outline(dr.pts, ink.size[dr.tool], true))
  }
  const onPointerUp = async (e: React.PointerEvent) => {
    if (pan.current?.id === e.pointerId) { pan.current = null; return }
    const dr = drawing.current
    if (!dr || e.pointerId !== dr.id) return
    drawing.current = null
    lastDraw.current = Date.now()
    if (dr.tool === 'eraser' || !dr.pts.length) return
    const el = pageEl(dr.page)
    if (!el) { live.current?.setAttribute('d', ''); return }
    const r = el.getBoundingClientRect()
    const k = r.width / (sizes[dr.page]?.w || 1)
    const pts = dr.pts.map((v, i) => (i % 3 === 2 ? Math.round(v * 100) / 100 : Math.round(((v - (i % 3 === 0 ? r.left : r.top)) / k) * 100) / 100))
    const s: Stroke = { t: dr.tool, w: Math.round((ink.size[dr.tool] / k) * 100) / 100, o: ink.opacity[dr.tool], a: 0, pts }
    await A.add({ kind: 'ink', chapter: dr.page, pos: dr.page + Math.max(0, pts[1] / (sizes[dr.page]?.h || 1)), quote: `Page ${dr.page + 1}`, color: ink.color[dr.tool], data: JSON.stringify(s) })
    live.current?.setAttribute('d', '')
  }

  // ---------------------------------------------------------- read aloud
  const speech = useRef({ stop: true })
  const readAloud = async (fromQuote?: string) => {
    const synth = window.speechSynthesis
    if (!synth) { toast('This browser cannot read aloud'); return }
    if (speaking) { speech.current.stop = true; synth.cancel(); setSpeaking(false); return }
    const token = { stop: false }
    speech.current = token
    setSpeaking(true)
    for (let i = current; i < n && !token.stop; i++) {
      let t = await pageText(i)
      if (fromQuote && i === current) { const at = t.indexOf(fromQuote.slice(0, 30)); if (at > 0) t = t.slice(at) }
      if (i !== current) goTo(i)
      // Sentences, so long pages are spoken in pieces the engine can handle.
      for (const part of t.replace(/-\n/g, '').replace(/\n/g, ' ').match(/[^.!?]+[.!?]*\s*/g) || []) {
        if (token.stop) break
        await new Promise<void>((done) => { const u = new SpeechSynthesisUtterance(part); u.onend = () => done(); u.onerror = () => done(); synth.speak(u) })
      }
    }
    setSpeaking(false)
  }
  useEffect(() => () => { speech.current.stop = true; window.speechSynthesis?.cancel() }, [])

  // ---------------------------------------------------------- keys
  const onKey = (e: KeyboardEvent) => {
    if ((e.target as HTMLElement)?.closest?.('input, textarea, select')) return
    const mod = e.ctrlKey || e.metaKey
    const sc = scroller.current
    if (mod && e.key.toLowerCase() === 'z') { e.preventDefault(); if (e.shiftKey) A.redo(); else A.undo(); return }
    if (mod && e.key.toLowerCase() === 'y') { e.preventDefault(); A.redo(); return }
    if (mod && (e.key === '=' || e.key === '+')) { e.preventDefault(); zoomBy(1); return }
    if (mod && e.key === '-') { e.preventDefault(); zoomBy(-1); return }
    if (mod && e.key === '0') { e.preventDefault(); setZoom('width'); return }
    if (mod) return
    if (e.key === 'Escape' || e.key === 'GoBack') {
      if (panel || sel || edit || define) { setPanel(null); setSel(null); setEdit(null); setDefine(null) } else if (inkMode) setInkMode(false); else close()
      return
    }
    if (!sc) return
    if (e.key === 'PageDown' || e.key === ' ') { e.preventDefault(); sc.scrollBy({ top: sc.clientHeight * 0.9 }) }
    else if (e.key === 'PageUp') { e.preventDefault(); sc.scrollBy({ top: -sc.clientHeight * 0.9 }) }
    else if (e.key === 'ArrowRight' && p.zoom !== 'width') { e.preventDefault(); goTo(current + cols) }
    else if (e.key === 'ArrowLeft' && p.zoom !== 'width') { e.preventDefault(); goTo(current - cols) }
    else if (e.key === 'Home') goTo(0)
    else if (e.key === 'End') goTo(n - 1)
    else if (e.key === '+' || e.key === '=') zoomBy(1)
    else if (e.key === '-') zoomBy(-1)
    else if (e.key === 'b') toggleBookmark()
    else if (e.key === 'p') setInkMode(!inkMode)
    else if (e.key === 'e' && inkMode) setInk({ tool: 'eraser' })
    else if (e.key === '/') { e.preventDefault(); setPanel('search') }
  }
  const keyRef = useRef(onKey)
  keyRef.current = onKey
  useEffect(() => { const k = (e: KeyboardEvent) => keyRef.current(e); window.addEventListener('keydown', k); return () => window.removeEventListener('keydown', k) }, [])

  if (err) return <div className="reader center"><div className="error">{err}<br /><button className="btn" onClick={close}>Back</button></div></div>
  const place = (c: number) => `Page ${c + 1}`
  const flashFor = (i: number) => (flash?.page === i ? flash.q : '')
  return (
    <div className={'reader pdf2' + (inkMode ? ' inking' : '') + (dark ? ' theme-dark' : '')} onMouseMove={(e) => { if (e.clientY < 70 || e.clientY > window.innerHeight - 70) pokeBar() }}>
      <div ref={scroller} className={'pdf-scroll' + (inkMode ? ' pdf-inking' : '')} onScroll={onScroll} onClick={onClick}
        onMouseUp={() => setTimeout(onSelect, 0)} onTouchEnd={() => setTimeout(onSelect, 250)}
        onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}>
        {!doc && <div className="spinner" />}
        {doc && (
          <div className={'pdf-pages' + (cols === 2 ? ' spread' : '')} style={{ gap: GAP }}>
            {sizes.map((s, i) => (
              <Page key={i} doc={doc} index={i} size={s} scale={scale} draw={near.has(i)} theme={p.theme} dark={dark}
                boxes={byPage.boxes.get(i) || NONE} strokes={byPage.strokes.get(i) || NONE} bookmarked={bookmarks.has(i)} flash={flashFor(i)} onSize={onSize} />
            ))}
          </div>
        )}
      </div>
      <svg className="pdf-live" aria-hidden="true"><InkDefs id="ink-grain-pdf" freq={0.7} /><path ref={live} strokeLinecap="round" strokeLinejoin="round" /></svg>

      <div className={'reader-bar' + (bar || panel || inkMode ? '' : ' hidden')}>
        <button className="ibtn" onClick={close} aria-label="Close the document"><IconBack /></button>
        <div className="grow rd-titles"><b>{it.title}</b>{it.author && <span className="muted">{it.author}</span>}</div>
        <div className="pdf-zoom" role="group" aria-label="Zoom">
          <button className="ibtn sm" onClick={() => zoomBy(-1)} aria-label="Zoom out" title="Zoom out (−)">−</button>
          <button className="btn sm ghost" onClick={() => setZoom(p.zoom === 'width' ? 'page' : 'width')} title="Fit width or whole page">{p.zoom === 'width' ? 'Fit width' : p.zoom === 'page' ? 'Fit page' : `${Math.round(scale * 100)}%`}</button>
          <button className="ibtn sm" onClick={() => zoomBy(1)} aria-label="Zoom in" title="Zoom in (+)">+</button>
        </div>
        <button className="ibtn" onClick={() => setPanel(panel === 'contents' ? null : 'contents')} aria-label="Contents and pages" title="Contents and pages"><IconBook /></button>
        <button className="ibtn" onClick={() => setPanel(panel === 'search' ? null : 'search')} aria-label="Search in the document" title="Search (/)"><IconSearch /></button>
        <button className="ibtn" onClick={() => setPanel(panel === 'notes' ? null : 'notes')} aria-label="Notes and highlights" title="Notes and highlights">☰</button>
        <button className={'ibtn' + (inkMode ? ' on' : '')} onClick={() => setInkMode(!inkMode)} aria-pressed={inkMode} aria-label="Handwriting" title="Pen, pencil and highlighter (p)">✎</button>
        <button className={'ibtn' + (bookmarks.has(current) ? ' on' : '')} onClick={toggleBookmark} aria-pressed={bookmarks.has(current)} aria-label="Bookmark this page" title="Bookmark (b)">🔖</button>
        <button className={'ibtn' + (speaking ? ' on' : '')} onClick={() => readAloud()} aria-pressed={speaking} aria-label="Read aloud" title="Read aloud">🔊</button>
        <button className="ibtn" onClick={() => setPanel(panel === 'display' ? null : 'display')} aria-label="Display settings" title="Display"><IconSettings /></button>
      </div>

      {inkMode ? (
        <InkBar p={ink} set={setInk} undo={A.undo} redo={A.redo} canUndo={A.canUndo} canRedo={A.canRedo} done={() => setInkMode(false)} />
      ) : (
        <div className={'reader-foot rd-foot' + (bar || panel ? '' : ' hidden')}>
          <button className="ibtn sm" onClick={() => goTo(current - cols)} aria-label="Previous page">‹</button>
          <input type="range" min={0} max={Math.max(0, n - 1)} value={current} aria-label="Page" aria-valuetext={`Page ${current + 1} of ${n}`} onChange={(e) => goTo(Number(e.target.value))} />
          <button className="ibtn sm" onClick={() => goTo(current + cols)} aria-label="Next page">›</button>
          <form onSubmit={(e) => { e.preventDefault(); const v = Number((e.currentTarget.elements.namedItem('pg') as HTMLInputElement).value); if (v >= 1 && v <= n) goTo(v - 1) }}>
            <input name="pg" className="pdf-pg" key={current} defaultValue={current + 1} inputMode="numeric" aria-label="Go to page" /> <span>of {n}</span>
          </form>
        </div>
      )}

      {panel && (
        <aside className="rd-panel" role="dialog" aria-label={panel}>
          {panel === 'contents' && (
            <>
              <div className="seg">{(['outline', 'pages'] as const).map((v) => <button key={v} className={tab === v ? 'active' : ''} onClick={() => setTab(v)}>{v === 'outline' ? 'Contents' : 'Pages'}</button>)}</div>
              {tab === 'outline' && (outlineItems === null ? <div className="spinner" /> : outlineItems.length === 0 ? <p className="muted">This document has no contents list. Use Pages to find your way.</p> :
                outlineItems.map((o, i) => <button key={i} style={{ paddingLeft: 10 + o.depth * 16 }} onClick={() => { goDest(o.dest); setPanel(null) }}>{o.title}</button>))}
              {tab === 'pages' && doc && <div className="pdf-thumbs">{sizes.map((_, i) => <Thumb key={i} doc={doc} index={i} current={i === current} go={() => { goTo(i); setPanel(null) }} />)}</div>}
            </>
          )}
          {panel === 'notes' && (
            <NotesPanel anns={anns} labels={labels} setLabel={setLabel} place={place} remove={deleteAnn} onExport={() => exportNotes(it, anns, labels, place)}
              open={(a) => { goTo(a.chapter, Math.max(0, a.pos - a.chapter - 0.05)); setPanel(null) }} />
          )}
          {panel === 'search' && (
            <>
              <h3>Search</h3>
              <form onSubmit={(e) => { e.preventDefault(); search(query) }}>
                <input className="input" autoFocus value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search in this document" aria-label="Search in this document" />
              </form>
              {found === null && query && <div className="spinner" />}
              {found && <p className="muted">{found.length === 0 ? 'No matches' : `${found.length}${found.length >= 300 ? '+' : ''} matches`}</p>}
              {found?.map((h, i) => (
                <button key={i} className="ep-hit" onClick={() => { goTo(h.page); setFlash({ page: h.page, q: query.trim() }); setPanel(null) }}>
                  <span className="muted">{place(h.page)}</span> {h.snippet}
                </button>
              ))}
            </>
          )}
          {panel === 'display' && (
            <div className="ep-display">
              <h3>Display</h3>
              <label>Zoom
                <div className="seg">
                  <button className={p.zoom === 'width' ? 'active' : ''} onClick={() => setZoom('width')}>Fit width</button>
                  <button className={p.zoom === 'page' ? 'active' : ''} onClick={() => setZoom('page')}>Fit page</button>
                  {[1, 1.5, 2].map((z) => <button key={z} className={p.zoom === z ? 'active' : ''} onClick={() => setZoom(z)}>{z * 100}%</button>)}
                </div>
              </label>
              <label>Pages across
                <div className="seg">
                  <button className={!p.spread ? 'active' : ''} onClick={() => { keep.current = where(); setPrefs({ spread: false }) }}>One</button>
                  <button className={p.spread ? 'active' : ''} onClick={() => { keep.current = where(); setPrefs({ spread: true }) }}>Two</button>
                </div>
              </label>
              <label>Page colour
                <div className="seg">{([['normal', 'As printed'], ['sepia', 'Sepia'], ['dark', 'Dark']] as const).map(([v, l]) => <button key={v} className={p.theme === v ? 'active' : ''} onClick={() => setPrefs({ theme: v })}>{l}</button>)}</div>
              </label>
              <p className="muted small">Dark turns pages to light text on dark; pictures change colour too. Ctrl + scroll or + and − zoom.</p>
            </div>
          )}
          <button className="btn ghost ep-close" onClick={() => setPanel(null)}>Close</button>
        </aside>
      )}

      {sel && (
        <SelectionBar x={sel.x} y={sel.y} quote={sel.quote} labels={labels} onHighlight={(c) => addHighlight(c)} onClose={() => setSel(null)}
          onNote={async () => { const s = sel; const a = await addHighlight('yellow'); if (a) setEdit({ ann: a, x: s.x, y: s.y + 20 }) }}
          onDefine={host.define ? setDefine : undefined} onCite={() => navigator.clipboard?.writeText(cite(it, sel.quote, `p. ${sel.boxes[0][0] + 1}`))} onSearch={search}
          onRead={() => readAloud(sel.quote.trim())} />
      )}
      {edit && (
        <HighlightEditor ann={edit.ann} x={edit.x} y={edit.y} labels={labels} onDone={() => setEdit(null)} onRemove={() => deleteAnn(edit.ann)}
          onColor={(c) => { A.update(edit.ann, { color: c }); setEdit({ ...edit, ann: { ...edit.ann, color: c } }) }} onNote={(note) => A.update(edit.ann, { note })}
          onCite={() => navigator.clipboard?.writeText(cite(it, edit.ann.quote, `p. ${edit.ann.chapter + 1}`))} />
      )}
      {define && <Define word={define} onClose={() => setDefine(null)} />}
    </div>
  )
}

const NONE: never[] = []
