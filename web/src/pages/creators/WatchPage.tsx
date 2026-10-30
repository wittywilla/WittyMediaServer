import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtTime, type PlayInfo } from '../../api'
import { EmptyState, Spinner } from '../../components/common'
import ShareDialog from '../../components/ShareDialog'
import { SaveButton } from '../../components/SaveToCollection'
import { canPost, fmtCount, timeAgo, videoThumb, type Creator, type VideoCard, type VideoDetail } from '../../gallery'
import { IconEdit } from '../../icons'
import InlinePlayer from '../../player/InlinePlayer'
import { useApp } from '../../state'
import { Avatar, Comments, EditVideoDialog, LikeButtons, SubscribeButton, viewsOf } from './parts'
import '../../creators.css'

// The watch page: the video, what it is and who made it, comments, and what to
// watch next beside it.

const AUTONEXT = 'wms.channels.autoNext'

export default function WatchPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user } = useApp()
  const [params] = useSearchParams()
  const [d, setD] = useState<VideoDetail | null>(null)
  const [err, setErr] = useState('')
  const [creator, setCreator] = useState<Creator | null>(null)
  const [next, setNext] = useState<VideoCard[]>([])
  const [theatre, setTheatre] = useState(false)
  const [more, setMore] = useState(false)
  const [share, setShare] = useState(false)
  const [edit, setEdit] = useState(false)
  const [chapters, setChapters] = useState<NonNullable<PlayInfo['chapters']>>([])
  const [seekTo, setSeekTo] = useState<{ at: number } | undefined>()
  const [auto, setAuto] = useState(() => { try { return localStorage.getItem(AUTONEXT) !== '0' } catch { return true } })
  const load = () => api.get<VideoDetail>(`/videos/${id}`).then((v) => { setD(v); setErr('') }).catch((e) => setErr(e.message))
  useEffect(() => {
    setD(null); setMore(false); window.scrollTo(0, 0)
    load()
    api.get<VideoCard[]>(`/videos/${id}/related?limit=20`).then(setNext).catch(() => setNext([]))
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (d?.creatorId) api.get<{ creator: Creator }>(`/creators/${d.creatorId}`).then((r) => setCreator(r.creator)).catch(() => setCreator(null)) }, [d?.creatorId])
  if (err) return <EmptyState title="Video not found" text={err} />
  const startParam = params.get('t')
  const start = startParam ? Number(startParam) : d && !d.watched && d.positionSec > 10 ? d.positionSec : undefined
  const setAutoNext = (on: boolean) => { setAuto(on); try { localStorage.setItem(AUTONEXT, on ? '1' : '0') } catch { /* private window */ } }
  const views = d ? viewsOf(d) : 0
  const long = (d?.description || '').length > 260 || (d?.description || '').split('\n').length > 4
  return (
    <div className={'ch-watch' + (theatre ? ' theatre' : '')}>
      <div className="ch-watch-main">
        {d ? <InlinePlayer key={d.id} id={d.id} startAt={start} durationSec={d.durationSec} onEnded={() => { if (auto && next[0]) nav(`/watch/${next[0].id}`) }} className={d.vertical ? 'tall' : ''} onInfo={(i) => setChapters(i.chapters || [])} seekTo={seekTo} /> : <div className="player inline"><div className="center-spinner"><div className="spinner" /></div></div>}
        {d && (
          <>
            <h1 className="ch-title">{d.title}</h1>
            <div className="ch-row">
              {creator && (
                <div className="ch-owner">
                  <Link to={`/creator/${creator.id}`} className="ch-owner-link"><Avatar id={creator.id} name={creator.name} has={creator.hasAvatar} size={42} v={creator.updatedAt} />
                    <span><b>{creator.name}</b><span className="muted">{creator.videos} videos{creator.platform ? ` · ${creator.platform}` : ''}</span></span></Link>
                  <SubscribeButton c={creator} onChange={(on) => setCreator({ ...creator, subscribed: on })} />
                </div>
              )}
              <div className="ch-actions">
                <LikeButtons v={d} onChange={(r) => setD({ ...d, reaction: r, likes: d.likes + (r > 0 ? 1 : d.reaction > 0 ? -1 : 0) })} />
                {canPost(user) && <button className="btn sm" onClick={() => setShare(true)}>Share</button>}
                <SaveButton itemId={d.id} />
                <button className={'btn sm ghost ch-theatre'} aria-pressed={theatre} onClick={() => setTheatre(!theatre)}>{theatre ? 'Default view' : 'Wide view'}</button>
                {user?.isAdmin && <button className="btn sm ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
              </div>
            </div>
            <div className="ch-desc">
              <div className="ch-desc-stats">{[views ? `${fmtCount(views)} views` : '', d.uploadDate ? `${d.uploadDate} (${timeAgo(d.uploadDate)})` : '', d.platform].filter(Boolean).join(' · ')}</div>
              {d.description && <p className={more || !long ? '' : 'clamped'}>{d.description}</p>}
              {long && <button className="btn sm ghost" aria-expanded={more} onClick={() => setMore(!more)}>{more ? 'Show less' : 'Show more'}</button>}
              {d.tags.length > 0 && <div className="ch-tagline">{d.tags.map((t) => <Link key={t} to={`/library/${d.libraryId}?tag=${encodeURIComponent(t)}`}>#{t}</Link>)}</div>}
              {d.sourceUrl && <p className="muted ch-source">Source: <a href={d.sourceUrl} target="_blank" rel="noopener noreferrer nofollow">{d.sourceUrl}</a></p>}
              {d.custom && Object.keys(d.custom).length > 0 && (
                <dl className="ch-custom">{Object.entries(d.custom).map(([k, x]) => <div key={k}><dt>{k}</dt><dd>{String(x)}</dd></div>)}</dl>
              )}
            </div>
            {chapters.length > 1 && (
              <section className="ch-chapters" aria-label="Chapters">
                <h2>{d.durationSec >= 1200 ? 'Setlist' : 'Chapters'}</h2>
                <ol>{chapters.map((c, i) => <li key={i}><button onClick={() => setSeekTo({ at: c.start })}><span className="muted">{fmtTime(c.start)}</span> {c.title || `Part ${i + 1}`}</button></li>)}</ol>
              </section>
            )}
            <Comments key={d.id} itemId={d.id} count={d.comments} />
          </>
        )}
        {!d && !err && <Spinner />}
      </div>
      <aside className="ch-next" aria-label="Up next">
        <div className="ch-next-head">
          <h2>Up next</h2>
          <label className="share-check"><input type="checkbox" checked={auto} onChange={(e) => setAutoNext(e.target.checked)} /> Autoplay</label>
        </div>
        {next.map((v) => (
          <Link key={v.id} to={`/watch/${v.id}`} className="ch-next-item">
            <span className={'ch-next-thumb' + (v.vertical ? ' tall' : '')}><img src={videoThumb(v.id, 320, v.updatedAt)} alt="" loading="lazy" /></span>
            <span className="cr-text"><b>{v.title}</b><span className="muted">{v.creator}</span><span className="muted">{[viewsOf(v) ? `${fmtCount(viewsOf(v))} views` : '', timeAgo(v.uploadDate)].filter(Boolean).join(' · ')}</span></span>
          </Link>
        ))}
      </aside>
      {share && d && <ShareDialog kind="item" id={d.id} title={d.title} internalPath={`/watch/${d.id}`} onClose={() => setShare(false)} />}
      {edit && d && <EditVideoDialog v={d} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
    </div>
  )
}
