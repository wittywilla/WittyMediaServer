import { useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, type Library } from '../../api'
import { BackLink, EmptyState, Spinner } from '../../components/common'
import { creatorArt, fmtCount, videoThumb, type Creator, type VideoCard } from '../../gallery'
import { IconEdit, IconRefresh, IconSearch } from '../../icons'
import { useApp, toast } from '../../state'
import { Avatar, LoadMore, SubscribeButton, useVideoList, VideoTile } from './parts'
import '../../creators.css'
import LegalNote from '../../components/LegalNote'

// WittyChannels: the owner's archive of creator videos, laid out the way people
// expect from a video site: a home of recommendations, subscriptions, a page per
// channel. Everything comes from files the owner already has (docs/CREATOR-VIDEOS.md).

const TABS: [string, string][] = [['home', 'Home'], ['latest', 'Latest'], ['subs', 'Subscriptions'], ['channels', 'Channels'], ['history', 'History'], ['liked', 'Liked']]
// Music videos and concerts use the same pages: an artist is a channel, and
// recordings of 20 minutes or more are concerts.
const MUSIC_TABS: [string, string][] = [['home', 'Home'], ['latest', 'Videos'], ['concerts', 'Concerts'], ['subs', 'Following'], ['channels', 'Artists'], ['history', 'History'], ['liked', 'Liked']]

export default function ChannelsLibrary({ lib }: { lib: Library }) {
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') || 'home'
  const search = params.get('q') || ''
  const tag = params.get('tag') || ''
  const [text, setText] = useState(search)
  const [tags, setTags] = useState<string[]>([])
  const [scanning, setScanning] = useState(false)
  useEffect(() => { api.get<string[]>(`/videos/tags?library=${lib.id}`).then(setTags).catch(() => {}) }, [lib.id])
  const set = (p: Record<string, string | null>) => {
    const n = new URLSearchParams(params)
    for (const [k, v] of Object.entries(p)) v === null || v === '' ? n.delete(k) : n.set(k, v)
    setParams(n)
  }
  const query = useMemo(() => {
    const u = new URLSearchParams({ library: String(lib.id) })
    if (search) u.set('search', search)
    if (tag) u.set('tag', tag)
    if (tab === 'subs') u.set('subscribed', '1')
    if (tab === 'liked') u.set('liked', '1')
    if (tab === 'history') u.set('history', '1')
    if (tab === 'concerts') u.set('length', 'long')
    return u.toString()
  }, [lib.id, search, tag, tab])
  const rescan = async () => { await api.post(`/libraries/${lib.id}/scan`); setScanning(true); toast('Scan started'); setTimeout(() => setScanning(false), 3000) }
  const browsing = search || tag
  const music = lib.type === 'musicvideos'
  return (
    <div className="ch-page">
      <div className="toolbar ch-bar">
        <h1>{lib.name}</h1>
        <div className="seg ch-tabs" role="tablist" aria-label="View">
          {(music ? MUSIC_TABS : TABS).map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k && !browsing} className={tab === k && !browsing ? 'active' : ''} onClick={() => { set({ tab: k === 'home' ? null : k, q: null, tag: null }); setText('') }}>{l}</button>)}
        </div>
        <div style={{ flex: 1 }} />
        <form className="ch-search" role="search" onSubmit={(e) => { e.preventDefault(); set({ q: text.trim(), tag: null }) }}>
          <input className="input" type="search" placeholder="Search this library" aria-label="Search this library" value={text} onChange={(e) => setText(e.target.value)} />
          <button className="btn" aria-label="Search"><IconSearch /></button>
        </form>
        {user?.isAdmin && <button className="btn sm ghost" disabled={scanning} onClick={rescan}><IconRefresh /> Scan</button>}
      </div>
      {tags.length > 0 && tab !== 'channels' && (
        <div className="ch-tags" role="group" aria-label="Topics">
          <button className={'ch-tag' + (!tag ? ' on' : '')} onClick={() => set({ tag: null })}>All</button>
          {tags.map((t) => <button key={t} className={'ch-tag' + (tag === t ? ' on' : '')} aria-pressed={tag === t} onClick={() => set({ tag: t })}>{t}</button>)}
        </div>
      )}
      {browsing ? <Grid query={query} empty={`Nothing matched ${search ? `“${search}”` : tag}.`} />
        : tab === 'home' ? <HomeFeed lib={lib} />
        : tab === 'channels' ? <ChannelsTab lib={lib} />
        : tab === 'concerts' ? <Grid query={query} empty="Recordings of 20 minutes or more appear here, with their chapters as the setlist." />
        : tab === 'latest' ? <Grid query={query} empty="No videos yet. Put them in this library's folders and scan it." />
        : tab === 'subs' ? <Grid query={query} empty="Subscribe to a channel and its videos appear here." />
        : tab === 'history' ? <Grid query={query} empty="What you watch appears here. Only you can see it." />
        : <Grid query={query} empty="Videos you like appear here." />}
      <LegalNote kind="creators" />
    </div>
  )
}

function Grid({ query, empty, showCreator = true }: { query: string; empty: string; showCreator?: boolean }) {
  const { items, more, loadMore } = useVideoList(query)
  if (!items) return <Spinner />
  if (!items.length) return <EmptyState title="Nothing here yet" text={empty} />
  return (
    <>
      <div className="cr-grid">{items.map((v) => <VideoTile key={v.id} v={v} to={`/watch/${v.id}`} showCreator={showCreator} />)}</div>
      <LoadMore more={more} onMore={loadMore} />
    </>
  )
}

// Home: For You from the server's local ranking, with vertical videos in a row.
function HomeFeed({ lib }: { lib: Library }) {
  const [items, setItems] = useState<VideoCard[] | null>(null)
  const [vertical, setVertical] = useState<VideoCard[]>([])
  const [seed, setSeed] = useState<number | null>(null)
  const [more, setMore] = useState(false)
  useEffect(() => {
    api.get<{ videos: VideoCard[]; more: boolean; seed: number }>(`/videos/feed?library=${lib.id}&limit=24${lib.type === 'musicvideos' ? '' : '&shape=wide'}`).then((r) => { setItems(r.videos); setMore(r.more); setSeed(r.seed) }).catch(() => setItems([]))
    if (lib.type !== 'musicvideos') api.get<{ videos: VideoCard[] }>(`/videos?library=${lib.id}&shape=vertical&limit=12`).then((r) => setVertical(r.videos)).catch(() => {})
  }, [lib.id])
  const loadMore = async () => {
    if (!items) return
    const r = await api.get<{ videos: VideoCard[]; more: boolean }>(`/videos/feed?library=${lib.id}&limit=24${lib.type === 'musicvideos' ? '' : '&shape=wide'}&seed=${seed}&offset=${items.length}`)
    const have = new Set(items.map((v) => v.id))
    setItems([...items, ...r.videos.filter((v) => !have.has(v.id))]); setMore(r.more)
  }
  if (!items) return <Spinner />
  if (!items.length && !vertical.length) return <EmptyState title="No videos yet" text="One folder per channel works best. Sidecar files add titles, dates and descriptions; pictures beside a video become its thumbnail." />
  const half = Math.min(8, items.length)
  return (
    <>
      <div className="cr-grid">{items.slice(0, half).map((v) => <VideoTile key={v.id} v={v} to={`/watch/${v.id}`} />)}</div>
      {vertical.length > 0 && (
        <section className="ch-shelf" aria-label="Vertical videos">
          <h2>Vertical videos</h2>
          <div className="ch-shelf-row">{vertical.map((v) => <VideoTile key={v.id} v={v} to={`/watch/${v.id}`} showCreator={false} />)}</div>
        </section>
      )}
      <div className="cr-grid">{items.slice(half).map((v) => <VideoTile key={v.id} v={v} to={`/watch/${v.id}`} />)}</div>
      <LoadMore more={more} onMore={loadMore} />
    </>
  )
}

function ChannelsTab({ lib }: { lib: Library }) {
  const [list, setList] = useState<Creator[] | null>(null)
  useEffect(() => { api.get<Creator[]>(`/creators?library=${lib.id}`).then(setList).catch(() => setList([])) }, [lib.id])
  if (!list) return <Spinner />
  if (!list.length) return <EmptyState title="No channels yet" text="Each folder in this library counts as a channel, unless a sidecar file names one." />
  return (
    <div className="cr-people">
      {list.map((c) => (
        <Link key={c.id} to={`/creator/${c.id}`} className="cr-person">
          <Avatar id={c.id} name={c.name} has={c.hasAvatar} size={96} v={c.updatedAt} />
          <b>{c.name}</b><span className="muted">{c.videos} {c.videos === 1 ? 'video' : 'videos'}{c.subscribed ? ' · subscribed' : ''}</span>
        </Link>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------- a channel's page

type Playlist = { name: string; count: number; coverId: number }

export function CreatorPage() {
  const { id } = useParams()
  const { user, libraries } = useApp()
  const [c, setC] = useState<Creator | null>(null)
  const [lists, setLists] = useState<Playlist[]>([])
  const [err, setErr] = useState('')
  const [params, setParams] = useSearchParams()
  const [editing, setEditing] = useState(false)
  const tab = params.get('tab') || 'videos'
  const playlist = params.get('playlist')
  const sort = params.get('sort') || 'latest'
  const load = () => api.get<{ creator: Creator; playlists: Playlist[] }>(`/creators/${id}`).then((r) => { setC(r.creator); setLists(r.playlists) }).catch((e) => setErr(e.message))
  useEffect(() => { load() }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  const set = (p: Record<string, string | null>) => { const n = new URLSearchParams(params); for (const [k, v] of Object.entries(p)) v === null ? n.delete(k) : n.set(k, v); setParams(n, { replace: true }) }
  if (err) return <EmptyState title="Channel not found" text={err} />
  if (!c) return <Spinner />
  const lib = libraries.find((l) => l.id === c.libraryId)
  const music = lib?.type === 'musicvideos'
  const q = `creator=${c.id}&sort=${sort}${tab === 'shorts' ? '&shape=vertical' : tab === 'videos' && !music ? '&shape=wide' : ''}${playlist !== null ? `&playlist=${encodeURIComponent(playlist)}` : ''}`
  return (
    <div className="ch-page">
      {lib && <BackLink to={`/library/${lib.id}?tab=channels`} label={lib.name} />}
      {c.hasBanner && <div className="ch-banner" style={{ backgroundImage: `url(${creatorArt(c.id, 'banner', c.updatedAt)})` }} />}
      <header className="ch-head">
        <Avatar id={c.id} name={c.name} has={c.hasAvatar} size={88} v={c.updatedAt} />
        <div className="ch-head-text">
          <h1>{c.name}</h1>
          <p className="muted">{[c.handle, `${fmtCount(c.videos)} ${c.videos === 1 ? 'video' : 'videos'}`, c.subscribers ? `${c.subscribers} ${c.subscribers === 1 ? 'subscriber' : 'subscribers'} here` : '', c.platform].filter(Boolean).join(' · ')}</p>
        </div>
        <SubscribeButton c={c} onChange={(on) => setC({ ...c, subscribed: on })} />
        {user?.isAdmin && <button className="btn sm ghost" onClick={() => setEditing(true)}><IconEdit /> Edit</button>}
      </header>
      <div className="seg ch-tabs" role="tablist" aria-label="This channel">
        {(music ? [['videos', 'Videos'], ['playlists', 'Albums'], ['about', 'About']] : [['videos', 'Videos'], ['shorts', 'Vertical'], ['playlists', 'Playlists'], ['about', 'About']]).map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => set({ tab: k === 'videos' ? null : k, playlist: null })}>{l}</button>)}
      </div>
      {(tab === 'videos' || tab === 'shorts') && (
        <>
          <div className="ch-sort">
            <label>Sort <select className="select sm" value={sort} onChange={(e) => set({ sort: e.target.value })}><option value="latest">Newest</option><option value="popular">Popular</option><option value="oldest">Oldest</option><option value="title">By title</option></select></label>
          </div>
          <Grid query={q} empty="Nothing here." showCreator={false} />
        </>
      )}
      {tab === 'playlists' && (playlist !== null
        ? <><div className="ch-sort"><button className="btn sm ghost" onClick={() => set({ playlist: null })}>‹ Playlists</button> <b>{playlist}</b></div><Grid query={`creator=${c.id}&sort=oldest&playlist=${encodeURIComponent(playlist)}`} empty="Nothing here." showCreator={false} /></>
        : lists.length === 0 ? <EmptyState title="No playlists" text="Subfolders of a channel's folder become its playlists, and so do playlists named in sidecar files." />
        : <div className="cr-grid">{lists.map((p) => (
            <button key={p.name} className="cr-tile" onClick={() => set({ playlist: p.name })}>
              <span className="cr-thumb"><img src={videoThumb(p.coverId, 480)} alt="" loading="lazy" /><span className="cr-dur">{p.count} videos</span></span>
              <span className="cr-meta"><span className="cr-text"><b>{p.name}</b></span></span>
            </button>
          ))}</div>)}
      {tab === 'about' && (
        <div className="ch-about">
          {c.description ? <p>{c.description}</p> : <p className="muted">Nothing written about this channel yet.</p>}
          {c.url && <p><a href={c.url} target="_blank" rel="noopener noreferrer nofollow">{c.url}</a></p>}
        </div>
      )}
      <LegalNote kind="creators" />
      {editing && <CreatorDialog c={c} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); load() }} />}
    </div>
  )
}

function CreatorDialog({ c, onClose, onSaved }: { c: Creator; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(c.name)
  const [handle, setHandle] = useState(c.handle || '')
  const [desc, setDesc] = useState(c.description || '')
  const [platform, setPlatform] = useState(c.platform || '')
  const [url, setUrl] = useState(c.url || '')
  const [err, setErr] = useState('')
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try { await api.put(`/creators/${c.id}`, { name, handle, description: desc, platform, url }); onSaved() } catch (x: any) { setErr(x.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal" role="dialog" aria-modal="true" aria-labelledby="cd-h" onClick={(e) => e.stopPropagation()} onSubmit={save}>
        <h2 id="cd-h">Channel details</h2>
        <label className="oobe-field">Name<input className="input" required value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="oobe-field">Handle<input className="input" value={handle} onChange={(e) => setHandle(e.target.value)} placeholder="@name" /></label>
        <label className="oobe-field">Description<textarea className="input" rows={4} value={desc} onChange={(e) => setDesc(e.target.value)} /></label>
        <label className="oobe-field">Platform of origin<input className="input" value={platform} onChange={(e) => setPlatform(e.target.value)} /></label>
        <label className="oobe-field">Web address<input className="input" type="url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" /></label>
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </div>
  )
}
