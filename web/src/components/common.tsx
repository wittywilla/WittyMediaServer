import { useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { api, episodeLabel, fmtRuntime, discName, isBluray, isDisc, posterUrl, serverBase, showPosterUrl, type Item, type Show } from '../api'
import { toast, useApp } from '../state'
import { IconCheck, IconFilm, IconLeft, IconPlay, IconRefresh, IconRight } from '../icons'

// The way back up: the library (or show) a page belongs to.
export function BackLink({ to, label }: { to: string; label: string }) {
  return <Link to={to} className="back-link"><IconLeft size={18} /><span>{label}</span></Link>
}

export const isGeneratedArt = (it: Item) => it.poster.startsWith('cache:thumb-')

// A colour of its own for something without a real poster: worked out from its
// title, so it never changes.
export const artHue = (title: string) => {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  return h
}

// Artwork for something without a picture: a colour of its own, worked out from
// the title. The title itself is printed under the card, so it isn't repeated on
// the art; label is a short word for the art alone, such as a console's name.
export function Placeholder({ title, label }: { title: string; label?: string }) {
  const h = artHue(title)
  return (
    <div className="ph art-ph" style={{ background: `linear-gradient(155deg, hsl(${h} 32% 32%), hsl(${(h + 40) % 360} 38% 15%))` }} aria-hidden="true">
      {label && <b>{label}</b>}
    </div>
  )
}

// ScanButton: look through a library's folders again, for administrators.
export function ScanButton({ lib, onStarted }: { lib: { id: number; name: string }; onStarted?: () => void }) {
  const { user } = useApp()
  if (!user?.isAdmin) return null
  const scan = async () => {
    try { await api.post(`/libraries/${lib.id}/scan`); toast(`Scanning ${lib.name}`); onStarted?.() } catch (e: any) { toast(e.message) }
  }
  return <button className="btn ghost" onClick={scan}><IconRefresh /> Scan</button>
}

// Episodes render as wide stills (16:9) with the show name; `wide` is set
// automatically for episodes.
export function ItemCard({ item, wide = item.showId != null }: { item: Item; wide?: boolean }) {
  const pct = item.progress && item.progress.durationSec > 0 && !item.progress.watched
    ? Math.min(100, (item.progress.positionSec / item.progress.durationSec) * 100) : 0
  const gen = isGeneratedArt(item)
  const audio = item.kind === 'audio'
  // Books count pages or chapters in runtimeSec, so their length isn't a time.
  const length = item.kind === 'book' || audio ? '' : fmtRuntime(item.runtimeSec)
  return (
    <Link to={`/item/${item.id}`} className={'card' + (wide ? ' wide' : '') + (audio ? ' square' : '')} title={item.title}>
      <div className={'poster' + (gen && !wide ? ' generated' : '')} style={gen && !wide ? { '--art-hue': artHue(item.title) } as React.CSSProperties : undefined}>
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <Placeholder title={item.title} />}
        {isDisc(item) && <span className={'badge disc' + (isBluray(item) ? ' bluray' : '')}>{discName(item)}</span>}
        {item.progress?.watched && <SeenBadge />}
        <div className="play"><span><IconPlay /></span></div>
        {pct > 0 && <div className="progress"><i style={{ width: pct + '%' }} /></div>}
      </div>
      <div className="title">{item.showTitle && wide ? item.showTitle : item.title}</div>
      <div className="sub">{item.showId != null
        ? [episodeLabel(item), wide ? item.title : fmtRuntime(item.runtimeSec)].filter(Boolean).join(' · ')
        : audio ? item.albumArtist || item.artist || ''
        : item.kind === 'book' ? item.artist || item.year || ''
        : [item.year, length].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

// SeenBadge: the mark on a finished title.
export function SeenBadge() {
  return <span className="badge seen" role="img" aria-label="Watched"><IconCheck /></span>
}

export function ShowCard({ show }: { show: Show }) {
  return (
    <Link to={`/show/${show.id}`} className="card" title={show.title}>
      <div className="poster">
        <Placeholder title={show.title} />
        {/* The server falls back to an episode still when there is no poster yet. */}
        <img src={showPosterUrl(show)} alt="" loading="lazy" style={{ position: 'relative' }} onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} />
        {show.unwatched > 0 && show.unwatched < show.episodeCount && <span className="badge count">{show.unwatched}</span>}
        {show.unwatched === 0 && <SeenBadge />}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{show.title}</div>
      <div className="sub" title={`${show.episodeCount} episodes`}>{[show.year, `${show.seasonCount} ${show.seasonCount === 1 ? 'season' : 'seasons'}`].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

// CardRow: a sideways-scrolling row of cards. Mouse users get arrows at its ends
// (touch and keyboard scroll it themselves); they update on the DOM directly, so
// scrolling never re-renders the cards.
export function CardRow({ children }: { children: React.ReactNode }) {
  const row = useRef<HTMLDivElement | null>(null)
  const prev = useRef<HTMLButtonElement | null>(null)
  const next = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    const el = row.current
    if (!el) return
    let frame = 0
    const update = () => {
      frame = 0
      if (prev.current) prev.current.hidden = el.scrollLeft < 8
      if (next.current) next.current.hidden = el.scrollLeft + el.clientWidth >= el.scrollWidth - 8
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(update) }
    update()
    el.addEventListener('scroll', later, { passive: true })
    const size = new ResizeObserver(later)
    size.observe(el)
    return () => { el.removeEventListener('scroll', later); size.disconnect(); cancelAnimationFrame(frame) }
  }, [])
  const by = (dir: number) => { const el = row.current; if (el) el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' }) }
  return (
    <div className="row-wrap">
      <button ref={prev} type="button" className="row-arrow prev" tabIndex={-1} aria-hidden="true" onClick={() => by(-1)} hidden><IconLeft /></button>
      <div className="row" ref={row}>{children}</div>
      <button ref={next} type="button" className="row-arrow next" tabIndex={-1} aria-hidden="true" onClick={() => by(1)} hidden><IconRight /></button>
    </div>
  )
}

export function EmptyState({ title, text, action, icon = <IconFilm /> }: { title: string; text?: string; action?: React.ReactNode; icon?: React.ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-ic" aria-hidden="true">{icon}</span>
      <h3>{title}</h3>
      {text && <p>{text}</p>}
      {action}
    </div>
  )
}

export function Spinner() { return <div className="loading-box"><div className="spinner" /></div> }

// SkeletonGrid: grey shapes where a grid of cards will land.
export function SkeletonGrid({ square, count = 12 }: { square?: boolean; count?: number }) {
  return <div className={(square ? 'album-grid' : 'grid') + ' skeleton-grid' + (square ? ' square' : '')} aria-busy="true" aria-label="Loading">{Array.from({ length: count }, (_, i) => <div key={i} />)}</div>
}

// VersionTag is the release line shown at the bottom of every page, e.g.
// "WittyMediaServer 0.4.0 · ALPHA".
export function VersionTag({ version }: { version: string }) {
  const m = /^v?(\d+\.\d+\.\d+)(?:-([a-z]+)[.\d]*)?/i.exec(version || '')
  const num = m ? m[1] : version || 'dev'
  const stage = m?.[2] || (version === 'dev' ? 'dev' : '')
  return (
    <span className="version-tag" title={`WittyMediaServer ${version}`}>
      WittyMediaServer {num}{stage && <b className={'stage ' + stage.toLowerCase()}>{stage.toUpperCase()}</b>}
    </span>
  )
}

// logoUrl: the server's custom logo, or the app icon.
export const logoUrl = (version: string) => (version ? `${serverBase()}/api/branding/logo?v=${encodeURIComponent(version)}` : '/icon.svg')

// BrandMark is the picture beside the name: the custom logo or the app icon.
// src overrides it (a logo being previewed before it is saved).
export function BrandMark({ src }: { src?: string }) {
  const { brandLogo } = useApp()
  return <img className="brand-mark" src={src || logoUrl(brandLogo)} alt="" />
}

// Brand is the name in the top-left corner: the custom name from
// Settings -> Appearance, or WittyMediaServer.
export function Brand({ name }: { name: string }) {
  return name ? <span className="brand-name">{name}</span> : <span>Witty<b>Media</b>Server</span>
}

// UpdateDot: admins see that a newer version can be installed (Settings,
// About, Updates).
export function UpdateDot() {
  const { update, user } = useApp()
  if (!update || !user?.isAdmin) return null
  return <i className="upd-dot" role="img" aria-label={`Version ${update} is available`} title={`Version ${update} is available: Settings, About`} />
}
