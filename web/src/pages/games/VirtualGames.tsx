import { useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState, type ReactNode, type Ref } from 'react'
import { api, type Item } from '../../api'

// A grid of any number of games. Only the rows near the screen are on the page, and only
// the pages of results those rows need are asked for, so ten thousand games cost about
// the same as sixty. The page itself scrolls, as it does everywhere else: nothing here
// takes over the scroll bar, the browser's find-in-page aside.
//
// Scrolling doesn't touch React until it crosses a row, and reads the page's position
// once a frame at most.

const PAGE = 120
const OVERSCAN = 3 // rows kept beyond the screen, so that a remote always has one to move to
const SETTLE_MS = 90 // a fling across thousands of games asks for what it stops on

type Store = { query: string; items: Map<number, Item>; pages: Set<number>; total: number; scrollY: number }

// Going into a game and back finds the list as it was left: the last few stay in memory.
const stores: Store[] = []
function storeFor(query: string): Store {
  const found = stores.find((s) => s.query === query)
  if (found) return found
  const s: Store = { query, items: new Map(), pages: new Set(), total: -1, scrollY: 0 }
  stores.push(s)
  if (stores.length > 4) stores.shift()
  return s
}

export type VirtualHandle = { jumpTo: (offset: number) => void }

export function VirtualGames({ query, card, onTotal, label, empty, failed, version, handle, restore }: {
  query: string; card: (it: Item) => ReactNode; onTotal?: (n: number) => void; label: string; empty?: ReactNode; failed?: (retry: () => void) => ReactNode
  version?: number; handle?: Ref<VirtualHandle>; restore?: boolean
}) {
  const box = useRef<HTMLDivElement>(null)
  const storeRef = useRef<Store>(storeFor(query))
  if (storeRef.current.query !== query) storeRef.current = storeFor(query)
  const store = storeRef.current
  const [, redraw] = useState(0)
  const [error, setError] = useState(false)
  const [shape, setShape] = useState({ cols: 6, rowH: 340, gap: 0 })
  const [range, setRange] = useState({ first: 0, last: 8 })
  const shapeRef = useRef(shape)
  shapeRef.current = shape
  const hadFocus = useRef(false)
  const wantFocus = useRef<number | null>(null)

  // How many across, and how tall a row is: from the page's own sizes, so a theme or
  // couch mode can change them.
  const measure = useCallback(() => {
    const el = box.current
    if (!el) return
    const css = getComputedStyle(el)
    const min = parseFloat(css.getPropertyValue('--gm-cell')) || 150
    const gapX = parseFloat(css.columnGap) || 16, gapY = parseFloat(css.getPropertyValue('--gm-row-gap')) || 20
    const inner = el.clientWidth
    const cols = Math.max(window.innerWidth <= 520 ? 3 : 2, Math.floor((inner + gapX) / (min + gapX)))
    const cell = (inner - gapX * (cols - 1)) / cols
    const textH = parseFloat(css.getPropertyValue('--gm-text-h')) || 52
    const rowH = Math.round(cell * (4 / 3) + textH + gapY)
    setShape((s) => (s.cols === cols && s.rowH === rowH ? s : { cols, rowH, gap: gapY }))
  }, [])
  useLayoutEffect(() => {
    measure()
    const el = box.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [measure])

  // Which rows are near the screen.
  const look = useCallback(() => {
    const el = box.current
    if (!el) return
    const { cols, rowH } = shapeRef.current
    const top = el.getBoundingClientRect().top
    const rows = store.total < 0 ? 12 : Math.ceil(store.total / cols)
    const first = Math.max(0, Math.floor(-top / rowH) - OVERSCAN)
    const last = Math.min(Math.max(rows - 1, 0), Math.ceil((window.innerHeight - top) / rowH) + OVERSCAN)
    setRange((r) => (r.first === first && r.last === last ? r : { first, last }))
    store.scrollY = window.scrollY
  }, [store])
  useLayoutEffect(() => { look() }, [look, shape])
  useEffect(() => {
    let frame = 0
    const later = () => { if (!frame) frame = requestAnimationFrame(() => { frame = 0; look() }) }
    window.addEventListener('scroll', later, { passive: true })
    window.addEventListener('resize', later, { passive: true })
    return () => { window.removeEventListener('scroll', later); window.removeEventListener('resize', later); cancelAnimationFrame(frame) }
  }, [look])

  // Asks for the pages those rows are on.
  const seq = useRef(0)
  const fetchPage = useCallback((page: number, gen: number) => {
    const s = store
    s.pages.add(page)
    return api.get<{ items: Item[]; total: number }>(`/items?${query}&limit=${PAGE}&offset=${page * PAGE}`).then((r) => {
      r.items.forEach((it, i) => s.items.set(page * PAGE + i, it))
      if (s.total !== r.total) { s.total = r.total; onTotal?.(r.total) }
      setError(false)
      if (gen === seq.current) redraw((n) => n + 1)
    }).catch(() => {
      s.pages.delete(page)
      if (gen === seq.current && s.total < 0) setError(true)
    })
  }, [query, store, onTotal])
  useEffect(() => {
    const gen = ++seq.current
    if (store.total >= 0) onTotal?.(store.total)
    const t = window.setTimeout(() => {
      const from = range.first * shape.cols, to = (range.last + 1) * shape.cols
      for (let p = Math.floor(from / PAGE); p <= Math.floor(Math.max(from, to - 1) / PAGE); p++) {
        if (!store.pages.has(p)) void fetchPage(p, gen)
      }
    }, store.total < 0 ? 0 : SETTLE_MS)
    return () => window.clearTimeout(t)
  }, [range, shape.cols, store, fetchPage, onTotal])

  // After a scan or a lookup there's more to show: what's on screen is asked for again.
  const lastVersion = useRef(version)
  useEffect(() => {
    if (lastVersion.current === version) return
    lastVersion.current = version
    const gen = ++seq.current
    const from = Math.floor((range.first * shape.cols) / PAGE)
    store.pages.clear()
    void fetchPage(from, gen)
  }, [version, range.first, shape.cols, store, fetchPage])

  // Coming back to a list that's still in memory finds the same place.
  const restored = useRef(false)
  useLayoutEffect(() => {
    if (restored.current) return
    restored.current = true
    if (restore && store.total >= 0 && store.scrollY > 0) window.scrollTo(0, store.scrollY)
  }, [restore, store])

  useImperativeHandle(handle, () => ({
    jumpTo: (offset: number) => {
      const el = box.current
      if (!el) return
      const { cols, rowH } = shapeRef.current
      const row = Math.floor(offset / cols)
      wantFocus.current = row * cols
      const y = el.getBoundingClientRect().top + window.scrollY + row * rowH - parseFloat(getComputedStyle(el).getPropertyValue('--gm-jump-gap') || '96')
      window.scrollTo(0, Math.max(0, y))
      look()
    },
  }), [look])

  // Focus can't be lost by scrolling a focused card away: the nearest one takes over.
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const want = wantFocus.current
    if (want != null) {
      const target = el.querySelector<HTMLElement>(`[data-gm-at="${want}"] a, [data-gm-at="${want}"] button`)
      if (target) { wantFocus.current = null; target.focus({ preventScroll: true }) }
      return
    }
    if (document.activeElement === document.body && hadFocus.current) {
      const top = el.getBoundingClientRect().top
      const row = Math.max(0, Math.ceil(-top / shape.rowH))
      el.querySelector<HTMLElement>(`[data-gm-at="${row * shape.cols}"] a`)?.focus({ preventScroll: true })
    }
  })

  if (error && store.total < 0) return <>{failed?.(() => { setError(false); void fetchPage(0, ++seq.current) })}</>
  if (store.total === 0) return <>{empty}</>
  const total = Math.max(store.total, 0)
  const rows = store.total < 0 ? 4 : Math.ceil(total / shape.cols)
  const cells = []
  const lastCell = store.total < 0 ? 4 * shape.cols : Math.min(total, (range.last + 1) * shape.cols)
  for (let i = range.first * shape.cols; i < lastCell; i++) {
    const it = store.items.get(i)
    cells.push(
      <div key={it ? it.id : 'wait' + i} role="listitem" aria-posinset={i + 1} aria-setsize={store.total < 0 ? undefined : total} data-gm-at={i} className="gm-cell">
        {it ? card(it) : <div className="card gm-card-wait" aria-hidden="true"><div className="poster" /><div className="title" /><div className="sub" /></div>}
      </div>,
    )
  }
  return (
    <div ref={box} className="gm-vgrid" role="list" aria-label={label} style={{ height: rows * shape.rowH }}
      onFocusCapture={() => { hadFocus.current = true }}
      onBlurCapture={(e) => { if (e.relatedTarget && !e.currentTarget.contains(e.relatedTarget as Node)) hadFocus.current = false }}>
      <div className="gm-vgrid-rows" style={{ transform: `translateY(${range.first * shape.rowH}px)`, gridTemplateColumns: `repeat(${shape.cols}, minmax(0, 1fr))`, gridAutoRows: shape.rowH }}>
        {cells}
      </div>
    </div>
  )
}
