import { useEffect, useRef, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import LogsLink from '../components/LogsLink'
import { IconFilm, IconInfo, IconLeft, IconPlay, IconRight } from '../icons'

// What the content sits on: Card, the poster card, Shelf (a titled row of them),
// and the three states a page can be in besides ready: loading, empty and failed.

export function Card({ title, desc, actions, children, className, id, flush }: { title?: ReactNode; desc?: ReactNode; actions?: ReactNode; children?: ReactNode; className?: string; id?: string; flush?: boolean }) {
  return (
    <section className={'ui-card' + (flush ? ' flush' : '') + (className ? ' ' + className : '')} id={id}>
      {(title || actions) && (
        <header className="ui-card-head">
          <div>{title && <h2>{title}</h2>}{desc && <p>{desc}</p>}</div>
          {actions && <div className="ui-card-actions">{actions}</div>}
        </header>
      )}
      {children}
    </section>
  )
}

export type PosterShape = 'poster' | 'wide' | 'square'

// A piece of media as a picture and a caption. Poster cards are the only place
// with the moving tilt (ui/tilt.ts), and it finds them by their class.
export function PosterCard({ to, onClick, title, sub, name, art, shape = 'poster', badges, progress, playIcon = true, posterClass, posterStyle }: {
  to?: string; onClick?: () => void; title: ReactNode; sub?: ReactNode; name?: string; art: ReactNode; shape?: PosterShape; badges?: ReactNode; progress?: number
  playIcon?: boolean; posterClass?: string; posterStyle?: CSSProperties
}) {
  const body = (
    <>
      <div className={'poster' + (posterClass ? ' ' + posterClass : '')} style={posterStyle}>
        {art}
        {badges}
        {playIcon && <div className="play"><span><IconPlay /></span></div>}
        {!!progress && progress > 0 && <div className="progress"><i style={{ width: Math.min(100, progress) + '%' }} /></div>}
      </div>
      <div className="title">{title}</div>
      {sub ? <div className="sub">{sub}</div> : null}
    </>
  )
  const cls = 'card' + (shape === 'poster' ? '' : ' ' + shape)
  return to
    ? <Link to={to} className={cls} title={name}>{body}</Link>
    : <button type="button" className={cls + ' ui-card-button'} title={name} onClick={onClick}>{body}</button>
}

// CardRow: a sideways-scrolling row of cards. Mouse users get arrows at its ends
// (touch and keyboard scroll it themselves); they update on the DOM directly, so
// scrolling never re-renders the cards.
export function CardRow({ children }: { children: ReactNode }) {
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
      <div className="row" ref={row} data-nav-group>{children}</div>
      <button ref={next} type="button" className="row-arrow next" tabIndex={-1} aria-hidden="true" onClick={() => by(1)} hidden><IconRight /></button>
    </div>
  )
}

// Shelf: a heading, with an optional link to all of it, over a row of cards.
export function Shelf({ title, to, toLabel = 'See all', children }: { title: ReactNode; to?: string; toLabel?: string; children: ReactNode }) {
  return (
    <section className="hub">
      <div className="hub-head"><h2>{title}</h2>{to && <Link to={to}>{toLabel}<IconRight /></Link>}</div>
      <CardRow>{children}</CardRow>
    </section>
  )
}

// ---------------------------------------------------------------- page states

export function Spinner() { return <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div> }

// Grey shapes where something will land. Shown only if loading takes a moment.
export function Skeleton({ w, h, round, lines }: { w?: number | string; h?: number | string; round?: boolean; lines?: number }) {
  if (lines) return <div className="ui-skel-lines" aria-hidden="true">{Array.from({ length: lines }, (_, i) => <i key={i} className="ui-skel" style={i === lines - 1 ? { width: '62%' } : undefined} />)}</div>
  return <i className={'ui-skel' + (round ? ' round' : '')} aria-hidden="true" style={{ width: w, height: h }} />
}

// SkeletonGrid: grey shapes where a grid of cards will land.
export function SkeletonGrid({ square, count = 12 }: { square?: boolean; count?: number }) {
  return <div className={(square ? 'album-grid' : 'grid') + ' skeleton-grid' + (square ? ' square' : '')} aria-busy="true" aria-label="Loading">{Array.from({ length: count }, (_, i) => <div key={i} />)}</div>
}

export function EmptyState({ title, text, action, icon = <IconFilm /> }: { title: string; text?: ReactNode; action?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-ic" aria-hidden="true">{icon}</span>
      <h3>{title}</h3>
      {text && <p>{text}</p>}
      {action}
    </div>
  )
}

// A page that couldn't load, which is not the same as having nothing to show.
export function LoadError({ what, onRetry }: { what: string; onRetry?: () => void }) {
  return (
    <div className="empty" role="alert">
      <span className="empty-ic" aria-hidden="true"><IconInfo /></span>
      <h3>{what} couldn’t be loaded</h3>
      <p>The server didn’t answer properly. This is usually brief, and doesn’t mean anything has gone.</p>
      {onRetry && <button type="button" className="btn" onClick={onRetry}>Try again</button>}
      <LogsLink />
    </div>
  )
}

// A message that stays on the page until it no longer applies.
export function Notice({ tone = 'info', title, children, action }: { tone?: 'info' | 'ok' | 'warn' | 'danger'; title?: ReactNode; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className={'ui-notice ' + tone} role={tone === 'danger' ? 'alert' : 'status'}>
      <div className="ui-notice-text">{title && <strong>{title}</strong>}{children}</div>
      {action}
    </div>
  )
}

// ---------------------------------------------------------------- progress

// A bar. Without a value it says "working on it" without saying how far along.
export function Progress({ value, max = 100, label, tone, showValue }: { value?: number; max?: number; label: string; tone?: 'ok' | 'warn' | 'danger'; showValue?: boolean }) {
  const pct = value === undefined ? undefined : Math.max(0, Math.min(100, (value / (max || 1)) * 100))
  return (
    <div className={'ui-progress' + (tone ? ' ' + tone : '') + (pct === undefined ? ' busy' : '')}>
      <div className="ui-progress-track" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={pct === undefined ? undefined : max} aria-valuenow={value}>
        <i style={pct === undefined ? undefined : { transform: `scaleX(${pct / 100})` }} />
      </div>
      {showValue && pct !== undefined && <span className="ui-progress-value">{Math.round(pct)}%</span>}
    </div>
  )
}

// A ring, for small places: a button in the top bar, a card's corner.
export function ProgressRing({ value, max = 100, label, size = 24 }: { value?: number; max?: number; label: string; size?: number }) {
  const pct = value === undefined ? undefined : Math.max(0, Math.min(1, value / (max || 1)))
  const r = 10, c = 2 * Math.PI * r
  return (
    <svg className={'ui-ring' + (pct === undefined ? ' busy' : '')} viewBox="0 0 24 24" width={size} height={size} role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={pct === undefined ? undefined : max} aria-valuenow={value}>
      <circle cx="12" cy="12" r={r} className="track" />
      <circle cx="12" cy="12" r={r} className="fill" strokeDasharray={c} strokeDashoffset={pct === undefined ? c * 0.7 : c * (1 - pct)} />
    </svg>
  )
}
