import { useCallback, useEffect, useRef, useState } from 'react'
import { IconBack, useHost } from '../wittyink'
import './comics.css'

// Comics and manga page by page: one page, a two-page spread or a vertical
// strip; left-to-right or right-to-left; fit to height or width.

type Prefs = { mode: 'single' | 'double' | 'strip'; dir: 'ltr' | 'rtl'; fit: 'height' | 'width' }
const DEF: Prefs = { mode: 'single', dir: 'ltr', fit: 'height' }
const key = (k: string) => `wms.reader.${k}`
const load = (k: string, reading: string): Prefs => {
  try {
    const v = localStorage.getItem(key(k))
    if (v) return { ...DEF, ...JSON.parse(v) }
  } catch { /* ignore */ }
  return { ...DEF, dir: reading === 'rtl' ? 'rtl' : 'ltr', mode: reading === 'webtoon' ? 'strip' : 'single' }
}

export default function ComicReader() {
  const host = useHost()
  const pages = host.comic?.pages || 0
  const close = host.close
  const [p, setP] = useState(() => load(host.key, host.reading || ''))
  const [page, setPage] = useState(Math.min(Math.floor(host.start), Math.max(0, pages - 1)))
  const [bar, setBar] = useState(true)
  const barTimer = useRef<any>(null)
  const poke = useCallback(() => { setBar(true); clearTimeout(barTimer.current); barTimer.current = setTimeout(() => setBar(false), 3000) }, [])
  useEffect(() => { poke(); return () => clearTimeout(barTimer.current) }, [poke])
  const step = p.mode === 'double' && page > 0 ? 2 : 1 // the cover is shown alone in spreads
  const src = (n: number) => host.comic!.pageUrl(n)
  const setPrefs = (v: Partial<Prefs>) => { const n = { ...p, ...v }; setP(n); try { localStorage.setItem(key(host.key), JSON.stringify(n)) } catch { /* ignore */ } }
  const go = useCallback((d: number) => setPage((x) => Math.max(0, Math.min(pages - 1, x + d))), [pages])
  const saved = useRef(-1)
  const save = (n: number) => { if (n !== saved.current) { saved.current = n; host.saveProgress(n, pages) } }
  useEffect(() => { if (p.mode !== 'strip') save(page) }, [page, p.mode]) // eslint-disable-line react-hooks/exhaustive-deps
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
    if (x > 0.33 && x < 0.67) { setBar((b) => !b); return }
    go((x >= 0.67 !== (p.dir === 'rtl') ? 1 : -1) * step)
  }
  // Swipes on touch screens.
  const touch = useRef<number | null>(null)
  const strip = useRef<HTMLDivElement | null>(null)
  const onStripScroll = () => {
    const imgs = Array.from(strip.current?.querySelectorAll('img') || [])
    const i = imgs.findIndex((im) => im.getBoundingClientRect().bottom > window.innerHeight / 2)
    if (i >= 0) { save(i); setPage(i) }
  }
  useEffect(() => { if (p.mode === 'strip') strip.current?.querySelectorAll('img')[page]?.scrollIntoView() }, [p.mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const shown = p.mode === 'double' && page > 0 ? [page, page + 1].filter((n) => n < pages) : [page]
  if (p.dir === 'rtl') shown.reverse()
  return (
    <div className={'reader comic fit-' + p.fit} onMouseMove={poke}>
      {p.mode === 'strip' ? (
        <div className="comic-strip" ref={strip} onScroll={onStripScroll} onClick={() => setBar((b) => !b)}>
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
      <div className={'reader-bar' + (bar ? '' : ' hidden')} onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={close} aria-label="Close the book"><IconBack /></button>
        <b className="grow">{host.book.title}</b>
        <div className="seg" role="radiogroup" aria-label="Layout">
          {([['single', 'Page'], ['double', 'Spread'], ['strip', 'Scroll']] as const).map(([v, l]) => <button key={v} role="radio" aria-checked={p.mode === v} className={p.mode === v ? 'active' : ''} onClick={() => setPrefs({ mode: v })}>{l}</button>)}
        </div>
        <button className="btn sm" onClick={() => setPrefs({ dir: p.dir === 'rtl' ? 'ltr' : 'rtl' })} title="Reading direction">{p.dir === 'rtl' ? 'Right to left' : 'Left to right'}</button>
        {p.mode !== 'strip' && <button className="btn sm ghost" onClick={() => setPrefs({ fit: p.fit === 'height' ? 'width' : 'height' })}>Fit {p.fit === 'height' ? 'width' : 'height'}</button>}
      </div>
      {p.mode !== 'strip' && (
        <div className={'reader-foot' + (bar ? '' : ' hidden')} onClick={(e) => e.stopPropagation()}>
          <input type="range" min={0} max={Math.max(0, pages - 1)} value={page} aria-label="Page" style={{ direction: p.dir }} onChange={(e) => setPage(Number(e.target.value))} />
          <span>{page + 1} / {pages}</span>
        </div>
      )}
    </div>
  )
}
