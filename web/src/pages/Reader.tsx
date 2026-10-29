import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import { IconBack } from '../icons'

// The reader: comics and manga page by page (single, two-page spread or a
// vertical strip; left-to-right or right-to-left), EPUB books paged or
// scrolling with font and theme choices, PDFs in the browser's viewer.
// Progress is saved to the server, so reading continues on any device.

type BookInfo = { format: string; reading: string; title: string; pages?: number; spine?: string[]; toc?: { title: string; href: string }[] }
type Prefs = { mode: 'single' | 'double' | 'strip'; dir: 'ltr' | 'rtl'; fit: 'height' | 'width'; font: number; theme: 'light' | 'sepia' | 'dark'; flow: 'paged' | 'scroll' }
const DEF: Prefs = { mode: 'single', dir: 'ltr', fit: 'height', font: 100, theme: 'light', flow: 'paged' }
const key = (id: number) => `wms.reader.${id}`
const load = (id: number, reading: string): Prefs => {
  try {
    const v = localStorage.getItem(key(id))
    if (v) return { ...DEF, ...JSON.parse(v) }
  } catch { /* ignore */ }
  return { ...DEF, dir: reading === 'rtl' ? 'rtl' : 'ltr', mode: reading === 'webtoon' ? 'strip' : 'single' }
}

export default function ReaderPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [it, setIt] = useState<Item | null>(null)
  const [info, setInfo] = useState<BookInfo | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message))
    api.get<BookInfo>(`/items/${id}/book`).then(setInfo).catch((e) => setErr(e.message))
  }, [id])
  const close = () => (window.history.state?.idx > 0 ? nav(-1) : nav(`/item/${id}`))
  if (err) return <div className="reader center"><div className="error">{err}<br /><button className="btn" onClick={close}>Back</button></div></div>
  if (!it || !info) return <div className="reader center"><div className="spinner" /></div>
  if (info.format === 'pdf') {
    return (
      <div className="reader">
        <div className="reader-bar"><button className="ibtn" onClick={close} aria-label="Close the book"><IconBack /></button><b>{it.title}</b></div>
        <iframe className="reader-pdf" title={it.title} src={mediaUrl(`/api/items/${it.id}/file`)} />
      </div>
    )
  }
  return info.format === 'cbz' ? <ComicReader it={it} pages={info.pages || 0} close={close} /> : <EpubReader it={it} info={info} close={close} />
}

function useSave(it: Item, total: number) {
  const last = useRef(-1)
  return useCallback((pos: number) => {
    if (Math.abs(pos - last.current) < 0.01) return
    last.current = pos
    api.post(`/items/${it.id}/progress`, { positionSec: pos, durationSec: total }).catch(() => {})
  }, [it.id, total])
}

function useBarAutoHide() {
  const [bar, setBar] = useState(true)
  const t = useRef<any>(null)
  const poke = useCallback(() => { setBar(true); clearTimeout(t.current); t.current = setTimeout(() => setBar(false), 3000) }, [])
  useEffect(() => { poke(); return () => clearTimeout(t.current) }, [poke])
  return { bar, poke, toggle: () => setBar((b) => !b) }
}

// ---------------------------------------------------------------- comics

function ComicReader({ it, pages, close }: { it: Item; pages: number; close: () => void }) {
  const [p, setP] = useState(() => load(it.id, it.reading || ''))
  const start = it.progress && !it.progress.watched ? Math.floor(it.progress.positionSec) : 0
  const [page, setPage] = useState(Math.min(start, Math.max(0, pages - 1)))
  const save = useSave(it, pages)
  const ui = useBarAutoHide()
  const step = p.mode === 'double' && page > 0 ? 2 : 1 // the cover is shown alone in spreads
  const src = (n: number) => mediaUrl(`/api/items/${it.id}/page/${n}`)
  const setPrefs = (v: Partial<Prefs>) => { const n = { ...p, ...v }; setP(n); try { localStorage.setItem(key(it.id), JSON.stringify(n)) } catch { /* ignore */ } }
  const go = useCallback((d: number) => setPage((x) => Math.max(0, Math.min(pages - 1, x + d))), [pages])
  useEffect(() => { if (p.mode !== 'strip') save(page) }, [page, p.mode, save])
  useEffect(() => { for (let i = 1; i <= 3; i++) if (page + i < pages) new Image().src = src(page + i) }, [page]) // eslint-disable-line react-hooks/exhaustive-deps
  // Keys: arrows follow the reading direction; space and PageDown go on.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const fwd = p.dir === 'rtl' ? 'ArrowLeft' : 'ArrowRight', back = p.dir === 'rtl' ? 'ArrowRight' : 'ArrowLeft'
      if (e.key === 'Escape' || e.key === 'GoBack') close()
      else if (p.mode === 'strip') return
      else if (e.key === fwd || e.key === ' ' || e.key === 'PageDown') { e.preventDefault(); go(step) }
      else if (e.key === back || e.key === 'PageUp') { e.preventDefault(); go(-step) }
      else if (e.key === 'Home') setPage(0)
      else if (e.key === 'End') setPage(pages - 1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [p, step, go, close, pages])
  // Tap zones: the outer thirds turn the page, the middle shows the bar.
  const tap = (e: React.MouseEvent) => {
    const x = e.clientX / window.innerWidth
    if (x > 0.33 && x < 0.67) { ui.toggle(); return }
    const right = x >= 0.67
    go((right !== (p.dir === 'rtl') ? 1 : -1) * step)
  }
  // Swipes on touch screens.
  const touch = useRef<number | null>(null)
  const strip = useRef<HTMLDivElement | null>(null)
  const onStripScroll = () => {
    const el = strip.current
    if (!el) return
    const imgs = Array.from(el.querySelectorAll('img'))
    const i = imgs.findIndex((im) => im.getBoundingClientRect().bottom > window.innerHeight / 2)
    if (i >= 0) { save(i); setPage(i) }
  }
  useEffect(() => { if (p.mode === 'strip') strip.current?.querySelectorAll('img')[page]?.scrollIntoView() }, [p.mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const shown = p.mode === 'double' && page > 0 ? [page, page + 1].filter((n) => n < pages) : [page]
  if (p.dir === 'rtl') shown.reverse()
  return (
    <div className={'reader comic fit-' + p.fit} onMouseMove={ui.poke}>
      {p.mode === 'strip' ? (
        <div className="comic-strip" ref={strip} onScroll={onStripScroll} onClick={ui.toggle}>
          {Array.from({ length: pages }, (_, n) => <img key={n} src={src(n)} alt={`Page ${n + 1}`} loading={Math.abs(n - page) < 3 ? 'eager' : 'lazy'} />)}
        </div>
      ) : (
        <div className="comic-stage" onClick={tap} onTouchStart={(e) => { touch.current = e.touches[0].clientX }}
          onTouchEnd={(e) => {
            if (touch.current === null) return
            const dx = e.changedTouches[0].clientX - touch.current
            touch.current = null
            if (Math.abs(dx) > 50) go((dx < 0 !== (p.dir === 'rtl') ? 1 : -1) * step)
          }}>
          {shown.map((n) => <img key={n} src={src(n)} alt={`Page ${n + 1} of ${pages}`} draggable={false} />)}
        </div>
      )}
      <div className={'reader-bar' + (ui.bar ? '' : ' hidden')} onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={close} aria-label="Close the book"><IconBack /></button>
        <b className="grow">{it.title}</b>
        <div className="seg" role="radiogroup" aria-label="Layout">
          {([['single', 'Page'], ['double', 'Spread'], ['strip', 'Scroll']] as const).map(([v, l]) => <button key={v} role="radio" aria-checked={p.mode === v} className={p.mode === v ? 'active' : ''} onClick={() => setPrefs({ mode: v })}>{l}</button>)}
        </div>
        <button className="btn sm" onClick={() => setPrefs({ dir: p.dir === 'rtl' ? 'ltr' : 'rtl' })} title="Reading direction">{p.dir === 'rtl' ? 'Right to left' : 'Left to right'}</button>
        {p.mode !== 'strip' && <button className="btn sm ghost" onClick={() => setPrefs({ fit: p.fit === 'height' ? 'width' : 'height' })}>Fit {p.fit === 'height' ? 'width' : 'height'}</button>}
      </div>
      {p.mode !== 'strip' && (
        <div className={'reader-foot' + (ui.bar ? '' : ' hidden')} onClick={(e) => e.stopPropagation()}>
          <input type="range" min={0} max={Math.max(0, pages - 1)} value={page} aria-label="Page" style={{ direction: p.dir }} onChange={(e) => setPage(Number(e.target.value))} />
          <span>{page + 1} / {pages}</span>
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- EPUB

// Bumped when the server's book headers change, so cached chapters refresh.
const READER_V = 2

const THEMES = { light: ['#ffffff', '#1d1d1f'], sepia: ['#f4ecd8', '#3b2f1e'], dark: ['#121212', '#e6e6e6'] }

function EpubReader({ it, info, close }: { it: Item; info: BookInfo; close: () => void }) {
  const spine = info.spine || []
  const [p, setP] = useState(() => load(it.id, ''))
  const startPos = it.progress && !it.progress.watched ? it.progress.positionSec : 0
  const [chapter, setChapter] = useState(Math.min(Math.floor(startPos), Math.max(0, spine.length - 1)))
  const [frac, setFrac] = useState(startPos % 1)
  const [toc, setToc] = useState(false)
  const frame = useRef<HTMLIFrameElement | null>(null)
  const pageRef = useRef({ page: 0, pages: 1 })
  const save = useSave(it, spine.length)
  const ui = useBarAutoHide()
  const setPrefs = (v: Partial<Prefs>) => { const n = { ...p, ...v }; setP(n); try { localStorage.setItem(key(it.id), JSON.stringify(n)) } catch { /* ignore */ } }

  // Style the chapter (the frame is same-origin but script-free: sandbox
  // without allow-scripts, and the server's CSP forbids scripts too).
  const layout = useCallback((restoreFrac?: number) => {
    const doc = frame.current?.contentDocument
    const win = frame.current?.contentWindow
    if (!doc || !win || !doc.documentElement) return
    const [bg, fg] = THEMES[p.theme]
    let st = doc.getElementById('wms-style') as HTMLStyleElement | null
    if (!st) { st = doc.createElement('style'); st.id = 'wms-style'; (doc.head || doc.documentElement).appendChild(st) }
    const w = frame.current!.clientWidth, h = frame.current!.clientHeight
    const paged = p.flow === 'paged'
    st.textContent = `html{background:${bg}!important;color:${fg}!important;font-size:${p.font}%!important}
      body{margin:0!important;padding:${paged ? '24px 0' : '24px 6vw 80px'}!important;background:transparent!important;color:inherit!important;line-height:1.6;font-family:Georgia,'Iowan Old Style',serif}
      a{color:inherit} img,svg{max-width:100%!important;height:auto!important;max-height:${h - 60}px} p{text-align:justify;hyphens:auto}
      ${paged ? `html{height:${h}px;column-width:${w}px;column-gap:0;column-fill:auto;overflow:hidden}
      body{padding:24px ${Math.round(w * 0.06)}px!important;box-sizing:border-box;height:${h}px}` : ''}`
    if (paged) {
      const pages = Math.max(1, Math.round(doc.documentElement.scrollWidth / w))
      pageRef.current.pages = pages
      const pg = restoreFrac !== undefined ? Math.min(pages - 1, Math.round(restoreFrac * pages)) : Math.min(pageRef.current.page, pages - 1)
      pageRef.current.page = pg
      doc.documentElement.scrollLeft = pg * w
    } else if (restoreFrac !== undefined) {
      win.scrollTo(0, restoreFrac * doc.documentElement.scrollHeight)
    }
  }, [p.theme, p.font, p.flow])

  useEffect(() => { layout() }, [layout])
  useEffect(() => { const r = () => layout(); window.addEventListener('resize', r); return () => window.removeEventListener('resize', r) }, [layout])

  const turn = (d: number) => {
    const doc = frame.current?.contentDocument
    if (!doc) return
    const w = frame.current!.clientWidth
    if (p.flow === 'paged') {
      const { page, pages } = pageRef.current
      if (page + d >= 0 && page + d < pages) {
        pageRef.current.page = page + d
        doc.documentElement.scrollLeft = (page + d) * w
        const f = (page + d) / pages
        setFrac(f); save(chapter + f)
        return
      }
    } else {
      const win = frame.current!.contentWindow!
      const atEnd = win.scrollY + win.innerHeight >= doc.documentElement.scrollHeight - 4
      if (!(d > 0 && atEnd) && !(d < 0 && win.scrollY <= 0)) { win.scrollBy(0, d * win.innerHeight * 0.9); return }
    }
    const next = chapter + d
    if (next < 0 || next >= spine.length) return
    setFrac(d < 0 ? 0.999 : 0)
    setChapter(next)
    save(next + (d < 0 ? 0.99 : 0))
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' || e.key === 'GoBack') close()
      else if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') { e.preventDefault(); turn(1) }
      else if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); turn(-1) }
    }
    window.addEventListener('keydown', onKey)
    // Keys pressed while the chapter has focus.
    const doc = frame.current?.contentDocument
    doc?.addEventListener('keydown', onKey)
    return () => { window.removeEventListener('keydown', onKey); doc?.removeEventListener('keydown', onKey) }
  })

  const onLoad = () => {
    pageRef.current.page = 0
    layout(frac)
    const doc = frame.current?.contentDocument
    // Taps on the chapter: outer thirds turn pages, the middle shows the bar.
    doc?.addEventListener('click', (e) => {
      if ((e.target as HTMLElement).closest('a')) return
      const x = e.clientX / (frame.current?.clientWidth || 1)
      if (x < 0.3) turn(-1); else if (x > 0.7) turn(1); else ui.toggle()
    })
    if (p.flow === 'scroll') {
      frame.current?.contentWindow?.addEventListener('scroll', () => {
        const d = frame.current!.contentDocument!.documentElement
        const f = d.scrollTop / Math.max(1, d.scrollHeight - d.clientHeight)
        save(chapter + Math.min(0.99, f))
      }, { passive: true })
    }
  }
  const [bg] = THEMES[p.theme]
  const pct = Math.round(((chapter + frac) / Math.max(1, spine.length)) * 100)
  return (
    <div className={'reader epub theme-' + p.theme} style={{ background: bg }} onMouseMove={ui.poke}>
      <iframe ref={frame} key={chapter + p.flow} className="epub-frame" title={it.title} sandbox="allow-same-origin"
        src={mediaUrl(`/api/items/${it.id}/epub/${spine[chapter]}?v=${READER_V}`)} onLoad={onLoad} />
      <div className={'reader-bar' + (ui.bar ? '' : ' hidden')}>
        <button className="ibtn" onClick={close} aria-label="Close the book"><IconBack /></button>
        <b className="grow">{it.title}</b>
        {(info.toc?.length || 0) > 0 && <button className="btn sm" onClick={() => setToc(!toc)} aria-expanded={toc}>Contents</button>}
        <button className="btn sm" onClick={() => setPrefs({ font: Math.max(70, p.font - 10) })} aria-label="Smaller text">A−</button>
        <button className="btn sm" onClick={() => setPrefs({ font: Math.min(200, p.font + 10) })} aria-label="Larger text">A+</button>
        <div className="seg" role="radiogroup" aria-label="Page colour">
          {(['light', 'sepia', 'dark'] as const).map((t) => <button key={t} role="radio" aria-checked={p.theme === t} className={p.theme === t ? 'active' : ''} onClick={() => setPrefs({ theme: t })}>{t[0].toUpperCase() + t.slice(1)}</button>)}
        </div>
        <button className="btn sm ghost" onClick={() => setPrefs({ flow: p.flow === 'paged' ? 'scroll' : 'paged' })}>{p.flow === 'paged' ? 'Scroll' : 'Pages'}</button>
      </div>
      {toc && (
        <nav className="epub-toc" aria-label="Contents">
          {info.toc!.map((t, i) => {
            const target = spine.findIndex((s) => s === t.href.split('#')[0])
            return <button key={i} disabled={target < 0} className={target === chapter ? 'on' : ''} onClick={() => { setChapter(target); setFrac(0); save(target); setToc(false) }}>{t.title}</button>
          })}
        </nav>
      )}
      <div className={'reader-foot' + (ui.bar ? '' : ' hidden')}>
        <button className="btn sm ghost" onClick={() => turn(-1)} aria-label="Previous page">‹</button>
        <span>Chapter {chapter + 1} of {spine.length} · {pct}%</span>
        <button className="btn sm ghost" onClick={() => turn(1)} aria-label="Next page">›</button>
      </div>
    </div>
  )
}
