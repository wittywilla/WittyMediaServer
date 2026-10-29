import { Link } from 'react-router-dom'
import { fmtRuntime, isDisc, posterUrl, type Item } from '../api'
import { IconPlay } from '../icons'

export const isGeneratedArt = (it: Item) => it.poster.startsWith('cache:thumb-')

export function ItemCard({ item }: { item: Item }) {
  const pct = item.progress && item.progress.durationSec > 0 && !item.progress.watched
    ? Math.min(100, (item.progress.positionSec / item.progress.durationSec) * 100) : 0
  const gen = isGeneratedArt(item)
  return (
    <Link to={`/item/${item.id}`} className="card" title={item.title}>
      <div className={'poster' + (gen ? ' generated' : '')}>
        {gen && <div className="blur" style={{ backgroundImage: `url(${posterUrl(item)})` }} />}
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <div className="ph">{item.title}</div>}
        {gen && <div className="cap">{item.title}</div>}
        {isDisc(item) && <span className="badge disc">DVD</span>}
        {item.progress?.watched && <span className="badge" style={{ left: 'auto', right: 8 }}>✓</span>}
        <div className="play"><span><IconPlay /></span></div>
        {pct > 0 && <div className="progress"><i style={{ width: pct + '%' }} /></div>}
      </div>
      <div className="title">{item.title}</div>
      <div className="sub">{[item.year, fmtRuntime(item.runtimeSec)].filter(Boolean).join(' · ')}</div>
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
