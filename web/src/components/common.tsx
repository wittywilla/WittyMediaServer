import { Link } from 'react-router-dom'
import { episodeLabel, fmtRuntime, discName, isBluray, isDisc, posterUrl, showPosterUrl, type Item, type Show } from '../api'
import { IconPlay } from '../icons'

export const isGeneratedArt = (it: Item) => it.poster.startsWith('cache:thumb-')

// Episodes render as wide stills (16:9) with the show name; `wide` is set
// automatically for episodes.
export function ItemCard({ item, wide = item.showId != null }: { item: Item; wide?: boolean }) {
  const pct = item.progress && item.progress.durationSec > 0 && !item.progress.watched
    ? Math.min(100, (item.progress.positionSec / item.progress.durationSec) * 100) : 0
  const gen = isGeneratedArt(item)
  return (
    <Link to={`/item/${item.id}`} className={'card' + (wide ? ' wide' : '')} title={item.title}>
      <div className={'poster' + (gen && !wide ? ' generated' : '')}>
        {gen && !wide && <div className="blur" style={{ backgroundImage: `url(${posterUrl(item)})` }} />}
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <div className="ph">{item.title}</div>}
        {gen && !wide && <div className="cap">{item.title}</div>}
        {isDisc(item) && <span className={'badge disc' + (isBluray(item) ? ' bluray' : '')}>{discName(item)}</span>}
        {item.progress?.watched && <span className="badge" style={{ left: 'auto', right: 8 }}>✓</span>}
        <div className="play"><span><IconPlay /></span></div>
        {pct > 0 && <div className="progress"><i style={{ width: pct + '%' }} /></div>}
      </div>
      <div className="title">{item.showTitle && wide ? item.showTitle : item.title}</div>
      <div className="sub">{item.showId != null
        ? [episodeLabel(item), wide ? item.title : fmtRuntime(item.runtimeSec)].filter(Boolean).join(' · ')
        : [item.year, fmtRuntime(item.runtimeSec)].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

export function ShowCard({ show }: { show: Show }) {
  return (
    <Link to={`/show/${show.id}`} className="card" title={show.title}>
      <div className="poster">
        <div className="ph">{show.title}</div>
        {/* The server falls back to an episode still when there is no poster yet. */}
        <img src={showPosterUrl(show)} alt="" loading="lazy" style={{ position: 'relative' }} onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} />
        {show.unwatched > 0 && show.unwatched < show.episodeCount && <span className="badge count">{show.unwatched}</span>}
        {show.unwatched === 0 && <span className="badge" style={{ left: 'auto', right: 8 }}>✓</span>}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{show.title}</div>
      <div className="sub">{[show.year, `${show.seasonCount} ${show.seasonCount === 1 ? 'season' : 'seasons'}`, `${show.episodeCount} episodes`].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

export function EmptyState({ title, text, action }: { title: string; text?: string; action?: React.ReactNode }) {
  return (
    <div className="empty">
      <h3>{title}</h3>
      {text && <p>{text}</p>}
      {action}
    </div>
  )
}

export function Spinner() { return <div style={{ display: 'grid', placeItems: 'center', padding: 60 }}><div className="spinner" /></div> }

// VersionTag is the release line shown at the bottom of every page, e.g.
// "WittyMediaServer 0.2.0 · ALPHA".
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

// Brand is the name in the top-left corner: the custom name from
// Settings -> Appearance, or WittyMediaServer.
export function Brand({ name }: { name: string }) {
  return name ? <span className="brand-name">{name}</span> : <span>Witty<b>Media</b>Server</span>
}
