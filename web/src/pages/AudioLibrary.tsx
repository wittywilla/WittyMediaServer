import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtRuntime, fmtTime, mediaUrl, type Album, type Artist, type Item, type Library } from '../api'
import { useAudio, type AudioMode } from '../audio/player'
import { BackLink, EmptyState, Placeholder, ScanButton, SkeletonGrid, Spinner } from '../components/common'
import { IconAudio, IconCheck, IconDisc, IconPlay } from '../icons'
import { useApp } from '../state'
import { AddPodcast, DownloadButton, PodcastAdmin, PodcastFinder, PodcastWays } from './Podcasts'
import LegalNote from '../components/LegalNote'

// Music and audiobook libraries: albums (books), artists (authors) and all
// songs, and the page of one album or book.

const cover = (itemId: number) => (itemId ? mediaUrl(`/api/items/${itemId}/poster`) : '')
const albumLink = (libId: number, a: { artist: string; title: string }) =>
  `/album/${libId}?artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`

export default function AudioLibrary({ lib }: { lib: Library }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user } = useApp()
  const [adding, setAdding] = useState(false)
  const [reload, setReload] = useState(0)
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') || 'albums'
  const sort = params.get('sort') || (books ? 'artist' : 'title')
  const artist = params.get('artist')
  const [albums, setAlbums] = useState<Album[] | null>(null)
  const [artists, setArtists] = useState<Artist[] | null>(null)
  const [songs, setSongs] = useState<Item[] | null>(null)
  const audio = useAudio()
  useEffect(() => {
    if (tab === 'albums') { setAlbums(null); api.get<Album[]>(`/music/albums?library=${lib.id}&sort=${sort}${artist !== null ? `&artist=${encodeURIComponent(artist)}` : ''}`).then(setAlbums).catch(() => setAlbums([])) }
    if (tab === 'artists') { setArtists(null); api.get<Artist[]>(`/music/artists?library=${lib.id}`).then(setArtists).catch(() => setArtists([])) }
    if (tab === 'songs') { setSongs(null); api.get<Item[]>(`/music/tracks?library=${lib.id}`).then(setSongs).catch(() => setSongs([])) }
  }, [lib.id, tab, sort, artist, reload])
  const set = (k: string, v: string | null) => { const p = new URLSearchParams(params); v === null ? p.delete(k) : p.set(k, v); setParams(p) }
  const tabs: [string, string][] = pods ? [['albums', 'Podcasts']] : books ? [['albums', 'Books'], ['artists', 'Authors']] : [['albums', 'Albums'], ['artists', 'Artists'], ['songs', 'Songs']]
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}{artist !== null && <span className="muted"> · {artist || 'Unknown'}</span>}</h1>
        {tab === 'albums' && albums && <span className="count">{albums.length} {pods ? (albums.length === 1 ? 'podcast' : 'podcasts') : books ? (albums.length === 1 ? 'book' : 'books') : albums.length === 1 ? 'album' : 'albums'}</span>}
        {tabs.length > 1 && <div className="seg" role="tablist">
          {tabs.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => { set('artist', null); set('tab', k) }}>{l}</button>)}
        </div>}
        {adding && <AddPodcast libraryId={lib.id} onClose={() => setAdding(false)} onAdded={() => { setAdding(false); setReload((n) => n + 1) }} />}
        <div className="grow" />
        {tab === 'albums' && !pods && (
          <select className="select" value={sort} aria-label="Sort" onChange={(e) => set('sort', e.target.value)}>
            <option value="title">Title</option><option value="artist">{books ? 'Author' : 'Artist'}</option><option value="year">Year</option><option value="added">Recently added</option>
          </select>
        )}
        <ScanButton lib={lib} />
        {pods && user?.isAdmin && <button className="btn primary" onClick={() => setAdding(true)}>Follow a podcast</button>}
      </div>
      {tab === 'albums' && (!albums ? <SkeletonGrid square={!books} /> : albums.length === 0 ? (pods ? (
        <div className="pod-empty">
          <h2>Your podcasts will live here</h2>
          <PodcastWays />
          {user?.isAdmin ? <PodcastFinder libraryId={lib.id} onAdded={() => setReload((n) => n + 1)} /> : <p className="muted">Ask whoever runs this server to follow some shows.</p>}
        </div>
      ) : <EmptyState icon={<IconAudio />} title="Nothing here yet" text="Add audio files to this library's folders and scan it." />) : (
        <div className="album-grid">
          {albums.map((a) => (
            <Link key={a.artist + '|' + a.title} to={albumLink(lib.id, a)} className="album-card">
              <div className={'album-cover' + (books ? ' book' : '')}>{a.coverItem ? <img src={cover(a.coverItem)} alt="" loading="lazy" /> : <Placeholder title={a.title || 'Unknown'} />}</div>
              <b>{a.title || 'Unknown album'}</b>
              <span className="muted">{a.artist || (books ? 'Unknown author' : pods ? '' : 'Unknown artist')}{a.artist && a.year ? ` · ${a.year}` : a.year && !pods ? ` · ${a.year}` : ''}</span>
            </Link>
          ))}
        </div>
      ))}
      {tab === 'artists' && (!artists ? <SkeletonGrid square /> : (
        <div className="album-grid">
          {artists.map((a) => (
            <button key={a.name} className="album-card artist" onClick={() => { const p = new URLSearchParams(params); p.set('tab', 'albums'); p.set('artist', a.name); setParams(p) }}>
              <div className="album-cover round">{a.coverItem ? <img src={cover(a.coverItem)} alt="" loading="lazy" /> : <span>{(a.name || '?')[0]}</span>}</div>
              <b>{a.name || 'Unknown'}</b>
              <span className="muted">{a.albums} {books ? (a.albums === 1 ? 'book' : 'books') : a.albums === 1 ? 'album' : 'albums'}</span>
            </button>
          ))}
        </div>
      ))}
      {tab === 'songs' && (!songs ? <Spinner /> : (
        <TrackList items={songs} showAlbum onPlay={(i) => audio.play(songs, i, 'music')} />
      ))}
      {pods && !!albums?.length && <LegalNote kind="webMedia" />}
    </>
  )
}

export function TrackList({ items, onPlay, showAlbum, episodes, extra }: { items: Item[]; onPlay: (i: number) => void; showAlbum?: boolean; episodes?: boolean; extra?: (it: Item) => React.ReactNode }) {
  const { current, playing } = useAudio()
  return (
    <ol className="track-list">
      {items.map((t, i) => {
        const on = current?.id === t.id
        const pct = t.progress && t.runtimeSec ? Math.min(100, (t.progress.positionSec / t.runtimeSec) * 100) : 0
        return (
          <li key={t.id} className={on ? 'on' : ''}>
            <button onClick={() => onPlay(i)} aria-label={`Play ${t.title}`}>
              <span className="tl-n">{on && playing ? <IconPlay /> : episodes ? '' : t.track ?? i + 1}</span>
              <span className="tl-t"><b>{t.title}</b>{showAlbum && <span className="muted">{[t.artist, t.album].filter(Boolean).join(' · ')}</span>}
                {episodes && <span className="muted">{[t.aired, t.overview].filter(Boolean).join(' · ').slice(0, 160)}</span>}
                {pct > 0 && <span className="tl-prog" style={{ width: pct + '%' }} />}</span>
              <span className="tl-d muted">{t.progress?.watched && <IconCheck />}{t.runtimeSec ? fmtTime(t.runtimeSec) : ''}</span>
            </button>
            {extra && <span className="tl-x">{extra(t)}</span>}
          </li>
        )
      })}
    </ol>
  )
}

// AlbumPage: one album or audiobook.
export function AlbumPage() {
  const { lib: libId } = useParams()
  const [params] = useSearchParams()
  const artist = params.get('artist') ?? ''
  const album = params.get('album') ?? ''
  const [lib, setLib] = useState<Library | null>(null)
  const [tracks, setTracks] = useState<Item[] | null>(null)
  const [reloadTracks, setReloadTracks] = useState(0)
  const audio = useAudio()
  const nav = useNavigate()
  useEffect(() => {
    api.get<Library[]>('/libraries').then((ls) => setLib(ls.find((l) => String(l.id) === libId) || null)).catch(() => {})
    api.get<Item[]>(`/music/tracks?library=${libId}&artist=${encodeURIComponent(artist)}&album=${encodeURIComponent(album)}`).then(setTracks).catch(() => setTracks([]))
  }, [libId, artist, album, reloadTracks])
  // A disc whose image has gone isn't offered as one to insert.
  const [goneDiscs, setGoneDiscs] = useState<number[]>([])
  const discKey = [...new Set((tracks || []).map((t) => t.cdDisc).filter(Boolean))].join(',')
  useEffect(() => {
    let left = false
    setGoneDiscs([])
    if (discKey) discKey.split(',').forEach((d) => api.get(`/cd/${d}`).catch((e) => { if (!left && (e.status === 404 || e.status === 422)) setGoneDiscs((g) => [...g, Number(d)]) }))
    return () => { left = true }
  }, [discKey])
  if (!tracks) return <Spinner />
  const mode: AudioMode = lib?.type === 'audiobooks' ? 'audiobooks' : lib?.type === 'podcasts' ? 'podcasts' : 'music'
  const books = mode !== 'music'
  const pods = mode === 'podcasts'
  const total = tracks.reduce((s, t) => s + t.runtimeSec, 0)
  // Audiobooks continue at the first chapter that is not finished.
  const resumeAt = Math.max(0, tracks.findIndex((t) => !t.progress?.watched))
  const started = books && tracks.some((t) => t.progress && t.progress.positionSec > 5)
  const coverId = tracks.find((t) => t.poster)?.id
  // Tracks from CD images play from their disc, with no gaps between them.
  const discs = [...new Set(tracks.map((t) => t.cdDisc).filter((d): d is number => !!d))]
  const playTrack = (i: number) => {
    const t = tracks[i]
    if (t?.cdDisc) nav(`/cd/${t.cdDisc}?play=1&track=${t.track}`)
    else audio.play(tracks, i, mode)
  }
  return (
    <div className="album-page">
      <BackLink to={`/library/${libId}`} label={lib?.name || (books ? 'Audiobooks' : 'Music')} />
      <div className="album-head">
        <div className={'album-cover big' + (books ? ' book' : '')}>{coverId ? <img src={cover(coverId)} alt="" /> : <Placeholder title={tracks[0]?.album || 'Unknown'} />}</div>
        <div className="album-meta">
          <span className="muted">{pods ? 'Podcast' : books ? 'Audiobook' : 'Album'}</span>
          <h1>{album || 'Unknown album'}</h1>
          <Link to={`/library/${libId}?tab=albums&artist=${encodeURIComponent(artist)}`} className="album-artist">{artist || 'Unknown'}</Link>
          <div className="muted">{tracks.length} {pods ? 'episodes' : books ? 'chapters' : 'tracks'}{pods ? '' : ` · ${fmtRuntime(total)}`}{tracks[0]?.year && !pods ? ` · ${tracks[0].year}` : ''}</div>
          <div className="actions">
            <button className="btn primary" onClick={() => playTrack(books && !pods ? resumeAt : 0)}><IconPlay /> {pods ? 'Play newest' : started ? 'Continue' : 'Play'}</button>
            {discs.filter((d) => !goneDiscs.includes(d)).map((d) => <Link key={d} className="btn" to={`/cd/${d}?play=1`}><IconDisc /> {discs.length > 1 ? `Disc ${discs.indexOf(d) + 1}` : 'Insert disc'}</Link>)}
            {!books && <button className="btn" onClick={() => audio.play([...tracks].sort(() => Math.random() - 0.5), 0, mode)}>Shuffle</button>}
            <button className="btn ghost" onClick={() => audio.enqueue(tracks)}>Add to queue</button>
          </div>
          {tracks[0]?.overview && !pods && <p className="album-desc">{tracks[0].overview}</p>}
          {pods && libId && <PodcastAdmin libraryId={Number(libId)} title={album} onChanged={() => setReloadTracks((n) => n + 1)} />}
        </div>
      </div>
      <TrackList items={tracks} onPlay={playTrack} episodes={pods}
        extra={pods ? (t) => <DownloadButton item={t} onDone={(x) => setTracks((ts) => ts!.map((y) => (y.id === x.id ? { ...y, path: x.path } : y)))} /> : undefined} />
      <LegalNote kind={pods ? 'webMedia' : 'metadata'} />
    </div>
  )
}
