import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, getPrefs, type Library, type PlayInfo } from '../../api'
import { EmptyState, Spinner } from '../../components/common'
import ShareDialog from '../../components/ShareDialog'
import { canReact, fmtCount, videoThumb, type Creator, type VideoCard, type VideoDetail } from '../../gallery'
import { IconClose, IconEdit, IconMute, IconPause, IconPlay, IconRefresh, IconVolume } from '../../icons'
import { setSpeed, SPEEDS } from '../../player/Controls'
import { useSource } from '../../player/hooks'
import { toast, useApp } from '../../state'
import { Avatar, Comments, EditVideoDialog, LoadMore, SubscribeButton, useVideoList, VideoTile, likesOf } from './parts'
import '../../creators.css'
import LegalNote from '../../components/LegalNote'

// WittyShorts: vertical videos, one at a time, with the next a swipe (or the
// Down button) away. The order of For You comes from the server, from what this
// profile did on this server only (docs/SHORTS.md). One video element plays;
// the others are a picture until they are on screen, so a low-end phone has as
// much to do as for a single video.

type Tab = 'foryou' | 'following' | 'liked' | 'creators'
const TABS: [Tab, string][] = [['foryou', 'For You'], ['following', 'Following'], ['liked', 'Liked'], ['creators', 'Creators']]

export default function ShortsLibrary({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const tab = (params.get('tab') || 'foryou') as Tab
  const startId = params.get('v') ? Number(params.get('v')) : null
  const creator = params.get('creator') ? Number(params.get('creator')) : null
  const set = (p: Record<string, string | null>, replace = false) => {
    const n = new URLSearchParams(params)
    for (const [k, v] of Object.entries(p)) v === null ? n.delete(k) : n.set(k, v)
    setParams(n, { replace })
  }
  return (
    <div className="sh-page">
      <h1 className="sr-only">{lib.name}</h1>
      <div className="sh-tabs" role="tablist" aria-label={lib.name}>
        {TABS.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k && !creator} className={tab === k && !creator ? 'on' : ''} onClick={() => set({ tab: k === 'foryou' ? null : k, v: null, creator: null })}>{l}</button>)}
      </div>
      {tab === 'foryou' && !creator && <Feed key={lib.id} lib={lib} startId={startId} onPosition={(id) => set({ v: String(id) }, true)} />}
      {creator && <CreatorFeed lib={lib} id={creator} startId={startId} onPosition={(id) => set({ v: String(id) }, true)} />}
      {tab === 'following' && !creator && <FollowingFeed lib={lib} startId={startId} onPosition={(id) => set({ v: String(id) }, true)} onCreators={() => set({ tab: 'creators' })} />}
      {tab === 'liked' && !creator && <GridOf lib={lib} query={`library=${lib.id}&liked=1&sort=latest`} empty="Videos you like appear here." />}
      {tab === 'creators' && !creator && <CreatorsGrid lib={lib} onOpen={(id) => set({ creator: String(id), tab: null, v: null })} />}
    </div>
  )
}

function GridOf({ lib, query, empty }: { lib: Library; query: string; empty: string }) {
  const { items, more, loadMore } = useVideoList(query)
  if (!items) return <Spinner />
  if (!items.length) return <EmptyState title="Nothing here yet" text={empty} />
  return (
    <div className="sh-scroll">
      <div className="cr-grid vertical">{items.map((v) => <VideoTile key={v.id} v={v} to={`/library/${lib.id}?v=${v.id}`} />)}</div>
      <LoadMore more={more} onMore={loadMore} />
    </div>
  )
}

function CreatorsGrid({ lib, onOpen }: { lib: Library; onOpen: (id: number) => void }) {
  const [list, setList] = useState<Creator[] | null>(null)
  useEffect(() => { api.get<Creator[]>(`/creators?library=${lib.id}`).then(setList).catch(() => setList([])) }, [lib.id])
  if (!list) return <Spinner />
  if (!list.length) return <EmptyState title="No creators yet" text="Each folder in this library counts as a creator, unless a sidecar file names one." />
  return (
    <div className="sh-scroll">
      <div className="cr-people">
        {list.map((c) => (
          <button key={c.id} className="cr-person" onClick={() => onOpen(c.id)}>
            <Avatar id={c.id} name={c.name} has={c.hasAvatar} size={84} v={c.updatedAt} />
            <b>{c.name}</b><span className="muted">{c.videos} {c.videos === 1 ? 'video' : 'videos'}{c.subscribed ? ' · following' : ''}</span>
          </button>
        ))}
      </div>
      <LegalNote kind="creators" />
    </div>
  )
}

function Feed({ lib, startId, onPosition }: { lib: Library; startId: number | null; onPosition: (id: number) => void }) {
  const seed = useRef<number | null>(null)
  const fetchPage = useCallback(async (offset: number) => {
    const r = await api.get<{ videos: VideoCard[]; more: boolean; seed: number }>(`/videos/feed?library=${lib.id}&limit=12&offset=${offset}${seed.current != null ? `&seed=${seed.current}` : ''}`)
    seed.current = r.seed
    return r
  }, [lib.id])
  return <Pager fetchPage={fetchPage} startId={startId} onPosition={onPosition} lib={lib} reshuffle={() => { seed.current = null }} />
}

function FollowingFeed({ lib, startId, onPosition, onCreators }: { lib: Library; startId: number | null; onPosition: (id: number) => void; onCreators: () => void }) {
  const fetchPage = useCallback((offset: number) => api.get<{ videos: VideoCard[]; more: boolean }>(`/videos?library=${lib.id}&subscribed=1&limit=12&offset=${offset}`), [lib.id])
  return <Pager fetchPage={fetchPage} startId={startId} onPosition={onPosition} lib={lib}
    empty={<EmptyState title="You aren't following anyone yet" text="Follow a creator from their video, or choose from the list." action={<button className="btn primary" onClick={onCreators}>See creators</button>} />} />
}

function CreatorFeed({ lib, id, startId, onPosition }: { lib: Library; id: number; startId: number | null; onPosition: (id: number) => void }) {
  const fetchPage = useCallback((offset: number) => api.get<{ videos: VideoCard[]; more: boolean }>(`/videos?library=${lib.id}&creator=${id}&limit=12&offset=${offset}`), [lib.id, id])
  return <Pager fetchPage={fetchPage} startId={startId} onPosition={onPosition} lib={lib} />
}

type Page = { videos: VideoCard[]; more: boolean }

// Pager: the feed itself. startId puts a video first (a shared link, a tile).
function Pager({ fetchPage, startId, onPosition, lib, reshuffle, empty }: {
  fetchPage: (offset: number) => Promise<Page>; startId: number | null; onPosition: (id: number) => void; lib: Library; reshuffle?: () => void; empty?: React.ReactNode
}) {
  const [items, setItems] = useState<VideoCard[] | null>(null)
  const [active, setActive] = useState(0)
  const [more, setMore] = useState(false)
  const [muted, setMuted] = useState(() => { try { return localStorage.getItem('wms.shorts.muted') === '1' } catch { return false } })
  const [rate, setRate] = useState(1)
  const scroller = useRef<HTMLDivElement | null>(null)
  const offset = useRef(0)
  const busy = useRef(false)
  const seen = useRef(new Set<number>())
  const addPage = useCallback(async (reset = false) => {
    if (busy.current) return
    busy.current = true
    try {
      if (reset) { offset.current = 0; seen.current = new Set() }
      const r = await fetchPage(offset.current)
      offset.current += r.videos.length
      const fresh = r.videos.filter((v) => !seen.current.has(v.id))
      fresh.forEach((v) => seen.current.add(v.id))
      setItems((x) => (reset || !x ? fresh : [...x, ...fresh]))
      setMore(r.more)
    } catch { setItems((x) => x || []) } finally { busy.current = false }
  }, [fetchPage])
  useEffect(() => {
    (async () => {
      await addPage(true)
      if (startId) {
        // A link to one video: it plays first, then the feed follows.
        try {
          const d = await api.get<VideoDetail>(`/videos/${startId}`)
          seen.current.add(d.id)
          setItems((x) => (x ? [d, ...x.filter((v) => v.id !== d.id)] : [d]))
        } catch { /* it may be gone; the feed still plays */ }
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addPage])
  useEffect(() => { if (items && more && active >= items.length - 4) addPage() }, [active, items, more, addPage])
  useEffect(() => { if (items?.[active]) onPosition(items[active].id) }, [active, items?.[active]?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  // The slide filling the screen is the active one.
  useEffect(() => {
    const root = scroller.current
    if (!root || !items) return
    const io = new IntersectionObserver((es) => {
      for (const e of es) if (e.isIntersecting) setActive(Number((e.target as HTMLElement).dataset.i))
    }, { root, threshold: 0.6 })
    root.querySelectorAll('[data-i]').forEach((el) => io.observe(el))
    return () => io.disconnect()
  }, [items?.length]) // eslint-disable-line react-hooks/exhaustive-deps
  const go = (i: number) => {
    if (!items || i < 0 || i >= items.length) return
    scroller.current?.querySelector<HTMLElement>(`[data-i="${i}"]`)?.scrollIntoView({ behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
  }
  const toggleMute = () => setMuted((m) => { try { localStorage.setItem('wms.shorts.muted', m ? '0' : '1') } catch { /* private window */ } return !m })
  if (!items) return <Spinner />
  if (!items.length) return <>{empty || <EmptyState title="No videos yet" text="Put video files in this library's folders and scan it. Sidecar files can add creators, tags and where a video came from." />}</>
  return (
    <div className="sh-feed" ref={scroller}>
      {items.map((v, i) => (
        <Slide key={v.id} v={v} i={i} active={i === active} near={Math.abs(i - active) <= 1} lib={lib} muted={muted} toggleMute={toggleMute} rate={rate} setRate={setRate}
          onNext={() => go(i + 1)} onPrev={() => go(i - 1)}
          onChange={(c) => setItems((x) => x && x.map((y) => (y.id === v.id ? { ...y, ...c } : y)))} />
      ))}
      {!more && (
        <div className="sh-end" data-i={items.length}>
          <p>You've reached the end.</p>
          {reshuffle && <button className="btn primary" onClick={() => { reshuffle(); setItems(null); setActive(0); addPage(true) }}><IconRefresh /> Start again</button>}
        </div>
      )}
    </div>
  )
}

function Slide({ v, i, active, near, lib, muted, toggleMute, rate, setRate, onNext, onPrev, onChange }: {
  v: VideoCard; i: number; active: boolean; near: boolean; lib: Library; muted: boolean; toggleMute: () => void; rate: number; setRate: (r: number) => void
  onNext: () => void; onPrev: () => void; onChange: (c: Partial<VideoCard>) => void
}) {
  const { user } = useApp()
  const [sheet, setSheet] = useState<'comments' | 'share' | 'edit' | 'about' | null>(null)
  const [detail, setDetail] = useState<VideoDetail | null>(null)
  const root = useRef<HTMLDivElement | null>(null)
  useEffect(() => { if (active) root.current?.focus({ preventScroll: true }) }, [active])
  useEffect(() => { if ((sheet === 'edit' || sheet === 'about') && !detail) api.get<VideoDetail>(`/videos/${v.id}`).then(setDetail).catch(() => {}) }, [sheet, detail, v.id])
  const like = async () => {
    if (!canReact(user)) return
    const next = v.reaction > 0 ? 0 : 1
    onChange({ reaction: next, likes: v.likes + (next > 0 ? 1 : v.reaction > 0 ? -1 : 0) })
    try { await api.put(`/items/${v.id}/reaction`, { value: next }) } catch { toast("Couldn't save that") }
  }
  // On a remote, focus on the slide means Up and Down change video; Right
  // reaches the buttons. The app's own focus movement handles them from there.
  const onKey = (e: React.KeyboardEvent) => {
    const onSlide = e.target === root.current
    if (e.key === 'ArrowDown' && onSlide) { e.preventDefault(); onNext() }
    else if (e.key === 'ArrowUp' && onSlide) { e.preventDefault(); onPrev() }
    else if (e.key === 'ArrowRight' && onSlide) { e.preventDefault(); root.current?.querySelector<HTMLElement>('.sh-rail button, .sh-rail a')?.focus() }
    else if (e.key === 'ArrowLeft' && !onSlide && (e.target as HTMLElement).closest('.sh-rail')) { e.preventDefault(); root.current?.focus() }
    else if (e.key === 'l' || e.key === 'L') like()
    else if (e.key === 'c' || e.key === 'C') setSheet('comments')
    else if (e.key === 'm' || e.key === 'M') toggleMute()
  }
  const liked = v.reaction > 0
  return (
    <div className={'sh-slide' + (v.vertical ? '' : ' wide')} data-i={i} ref={root} tabIndex={active ? 0 : -1} onKeyDown={onKey}
      aria-label={`${v.title}, by ${v.creator}. Up and Down change video, Right reaches the buttons.`} role="group">
      <div className="sh-stage">
        {near && !active && <img className="sh-poster" src={videoThumb(v.id, 640, v.updatedAt)} alt="" />}
        {!near && <div className="sh-poster blank" />}
        {active && <ActiveVideo v={v} muted={muted} toggleMute={toggleMute} rate={rate} onDone={onNext} />}
        <div className="sh-caption">
          <Link to={`/library/${lib.id}?creator=${v.creatorId}`} className="sh-who" tabIndex={active ? 0 : -1}>
            <Avatar id={v.creatorId} name={v.creator} has={v.hasAvatar} size={36} /><b>{v.creator}</b>
          </Link>
          <p>{v.title}</p>
          {v.tags.length > 0 && <div className="sh-tags">{v.tags.slice(0, 4).map((t) => <span key={t}>#{t}</span>)}</div>}
          <div className="sh-why">{[v.platform, v.reason].filter(Boolean).join(' · ')}</div>
        </div>
      </div>
      {active && (
        <div className="sh-rail" role="toolbar" aria-label="Video actions" aria-orientation="vertical">
          <button className={'sh-act' + (liked ? ' on' : '')} aria-pressed={liked} disabled={!canReact(user)} onClick={like} title="Like (L)"><i>♥</i><span>{fmtCount(likesOf(v)) }</span></button>
          <button className="sh-act" onClick={() => setSheet('comments')} title="Comments (C)"><i>💬</i><span>{fmtCount(v.comments)}</span></button>
          <button className="sh-act" onClick={() => setSheet('share')}><i>↗</i><span>Share</span></button>
          <SpeedAct rate={rate} setRate={setRate} />
          <button className="sh-act" onClick={() => setSheet('about')}><i>ⓘ</i><span>About</span></button>
          {user?.isAdmin && <button className="sh-act" onClick={() => setSheet('edit')}><i><IconEdit size={22} /></i><span>Edit</span></button>}
        </div>
      )}
      {sheet === 'comments' && (
        <div className="modal-backdrop" onClick={() => setSheet(null)}>
          <div className="modal sh-sheet" role="dialog" aria-modal="true" aria-label="Comments" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn sh-sheet-x" onClick={() => setSheet(null)} aria-label="Close"><IconClose /></button>
            <Comments itemId={v.id} count={v.comments} />
          </div>
        </div>
      )}
      {sheet === 'about' && (
        <div className="modal-backdrop" onClick={() => setSheet(null)}>
          <div className="modal sh-sheet" role="dialog" aria-modal="true" aria-labelledby="sh-about" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn sh-sheet-x" onClick={() => setSheet(null)} aria-label="Close"><IconClose /></button>
            <h2 id="sh-about">{v.title}</h2>
            {detail && <About d={detail} lib={lib} onCreator={(on) => { setDetail({ ...detail }); onChange({}); void on }} />}
          </div>
        </div>
      )}
      {sheet === 'share' && <ShareDialog kind="item" id={v.id} title={v.title} internalPath={`/library/${lib.id}?v=${v.id}`} onClose={() => setSheet(null)} />}
      {sheet === 'edit' && detail && <EditVideoDialog v={detail} onClose={() => setSheet(null)} onSaved={() => { setSheet(null); setDetail(null); api.get<VideoCard>(`/videos/${v.id}`).then((d) => onChange(d)).catch(() => {}) }} />}
    </div>
  )
}

function About({ d, lib, onCreator }: { d: VideoDetail; lib: Library; onCreator: (on: boolean) => void }) {
  const [creator, setCreator] = useState<Creator | null>(null)
  useEffect(() => { api.get<{ creator: Creator }>(`/creators/${d.creatorId}`).then((r) => setCreator(r.creator)).catch(() => {}) }, [d.creatorId])
  const custom = useMemo(() => Object.entries(d.custom || {}), [d.custom])
  return (
    <div className="sh-about">
      {creator && (
        <div className="sh-about-who">
          <Link to={`/library/${lib.id}?creator=${creator.id}`}><Avatar id={creator.id} name={creator.name} has={creator.hasAvatar} size={44} /></Link>
          <span><b>{creator.name}</b><br /><span className="muted">{creator.videos} videos</span></span>
          <SubscribeButton c={creator} onChange={(on) => { setCreator({ ...creator, subscribed: on }); onCreator(on) }} />
        </div>
      )}
      {d.description && <p className="sh-desc">{d.description}</p>}
      <dl>
        {d.uploadDate && <><dt>Date</dt><dd>{d.uploadDate}</dd></>}
        {d.platform && <><dt>Platform of origin</dt><dd>{d.platform}</dd></>}
        {d.sourceUrl && <><dt>Source</dt><dd><a href={d.sourceUrl} target="_blank" rel="noopener noreferrer nofollow">{d.sourceUrl}</a></dd></>}
        {d.origViews > 0 && <><dt>Views at the source</dt><dd>{d.origViews.toLocaleString()}</dd></>}
        {d.views > 0 && <><dt>Plays here</dt><dd>{d.views.toLocaleString()}</dd></>}
        {custom.map(([k, x]) => <div key={k} style={{ display: 'contents' }}><dt>{k}</dt><dd>{String(x)}</dd></div>)}
      </dl>
      <LegalNote kind="creators" />
    </div>
  )
}

function SpeedAct({ rate, setRate }: { rate: number; setRate: (r: number) => void }) {
  const [maxSpeed, setMaxSpeed] = useState(4)
  useEffect(() => { getPrefs().then((p) => setMaxSpeed(p.maxSpeed || 4)).catch(() => {}) }, [])
  const options = SPEEDS.filter((s) => s >= 0.5 && s <= maxSpeed)
  return (
    <label className="sh-act sh-speed" title="Playback speed">
      <i>{rate}×</i><span>Speed</span>
      <select value={rate} aria-label="Playback speed" onChange={(e) => setRate(Number(e.target.value))}>
        {options.map((s) => <option key={s} value={s}>{s === 1 ? 'Normal' : `${s}×`}</option>)}
      </select>
    </label>
  )
}

function ActiveVideo({ v, muted, toggleMute, rate, onDone }: { v: VideoCard; muted: boolean; toggleMute: () => void; rate: number; onDone: () => void }) {
  const video = useRef<HTMLVideoElement | null>(null)
  const [info, setInfo] = useState<PlayInfo | null>(null)
  const [err, setErr] = useState('')
  const [paused, setPaused] = useState(false)
  const [time, setTime] = useState(0)
  const [blocked, setBlocked] = useState(false)
  const start = useRef(Date.now())
  const watched = useRef(0)
  const last = useRef(0)
  const ended = useRef(false)
  useEffect(() => { api.get<PlayInfo>(`/items/${v.id}/play`).then(setInfo).catch((e) => setErr(e.message)) }, [v.id])
  useSource(video, info?.url || null, {
    onReady: () => {
      const el = video.current
      if (!el) return
      setSpeed(el, rate)
      el.play().catch(() => { // the browser wants a gesture before sound
        el.muted = true
        el.play().catch(() => setBlocked(true))
      })
    },
    onError: (m) => setErr(m),
  })
  useEffect(() => { if (video.current) setSpeed(video.current, rate) }, [rate])
  useEffect(() => { if (video.current) video.current.muted = muted }, [muted])
  // What happened to this video goes to the server when it leaves the screen:
  // the only thing For You has to go on.
  useEffect(() => () => {
    const dur = info?.durationSec || v.durationSec
    const secs = watched.current
    if (secs < 0.5 && Date.now() - start.current < 500) return
    const completed = ended.current || (dur > 0 && secs >= dur * 0.9)
    const skipped = !completed && (secs < 3 || (dur > 0 && secs < dur * 0.2))
    api.post(`/items/${v.id}/view`, { seconds: secs, completed, skipped }).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [v.id, info])
  const toggle = () => { const el = video.current; if (el) { setBlocked(false); el.paused ? el.play().catch(() => {}) : el.pause() } }
  const onKey = (e: React.KeyboardEvent) => { if (e.key === ' ' || e.key === 'k' || e.key === 'Enter') { e.preventDefault(); toggle() } }
  const dur = info?.durationSec || v.durationSec
  return (
    <div className="sh-video" onClick={toggle} onKeyDown={onKey}>
      <video ref={video} playsInline loop={false} muted={muted} poster={videoThumb(v.id, 640, v.updatedAt)}
        onPlay={() => setPaused(false)} onPause={() => setPaused(true)}
        onEnded={() => {
          ended.current = true
          // Short ones repeat once, as a viewer may not have caught it; longer ones move on.
          if (dur > 0 && dur <= 15 && video.current && !video.current.dataset.again) { video.current.dataset.again = '1'; video.current.currentTime = 0; video.current.play().catch(() => {}); return }
          onDone()
        }}
        onTimeUpdate={(e) => {
          const t = e.currentTarget.currentTime, d = t - last.current
          if (d > 0 && d < 5) watched.current += d
          last.current = t
          setTime(t)
        }} />
      {err && <div className="overlay-msg"><div className="box">{err}</div></div>}
      {(paused || blocked) && !err && <span className="sh-paused" aria-hidden="true"><IconPlay size={46} /></span>}
      <button className="sh-mute" onClick={(e) => { e.stopPropagation(); toggleMute() }} aria-pressed={muted} aria-label={muted ? 'Sound on' : 'Sound off'} title="Sound (M)">{muted ? <IconMute /> : <IconVolume />}</button>
      <button className="sh-play" onClick={(e) => { e.stopPropagation(); toggle() }} aria-label={paused ? 'Play' : 'Pause'}>{paused ? <IconPlay /> : <IconPause />}</button>
      <div className="sh-bar" aria-hidden="true"><i style={{ width: dur > 0 ? `${Math.min(100, (time / dur) * 100)}%` : 0 }} /></div>
    </div>
  )
}
