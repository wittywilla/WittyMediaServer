import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtRuntime, fmtTime, posterUrl, type Album, type Artist, type Item, type Library } from '../api'
import { useArtColour } from '../audio/art'
import { IconHeart, IconHeartOn, IconStation } from '../audio/icons'
import { useAudio, type AudioMode } from '../audio/player'
import { BackLink, EmptyState, Placeholder, ScanButton, SkeletonGrid, Spinner } from '../components/common'
import { IconAudio, IconCheck, IconDisc, IconPlay } from '../icons'
import { toast, useApp } from '../state'
import { AddPodcast, DownloadButton, PodcastAdmin, PodcastFinder, PodcastWays } from './Podcasts'
import LegalNote from '../components/LegalNote'
import CoverFlow, { type Cover } from './music/CoverFlow'
import { AlbumTile, albumLink, ArtistTile, artistLink, coverUrl, LibrarySwitch, SeeAllTile, Shelf, shuffledCopy, SongRow, TrackMenu } from './music/parts'

// Music, audiobook and podcast libraries. The front page has what you came back for: a shelf
// of covers, artists, albums and songs, each with its "see all". Those pages sort and filter.
// Then the page of one album or book. The first thing played from any of them keeps going in
// the bar along the bottom (audio/player.tsx).

type Home = { hero: Album[]; albums: Album[]; artists: Artist[]; songs: Item[]; recent: Item[]; continue: Item[]; genres: string[]; counts: { albums: number; artists: number; songs: number; liked: number } }

export default function AudioLibrary({ lib }: { lib: Library }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user } = useApp()
  const [adding, setAdding] = useState(false)
  const [reload, setReload] = useState(0)
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') || 'home'
  const set = (k: string, v: string | null) => { const p = new URLSearchParams(params); v === null ? p.delete(k) : p.set(k, v); setParams(p) }
  const tabs: [string, string][] = pods ? [['home', 'Home'], ['albums', 'Podcasts']]
    : books ? [['home', 'Home'], ['albums', 'Books'], ['artists', 'Authors']]
      : [['home', 'Home'], ['albums', 'Albums'], ['artists', 'Artists'], ['songs', 'Songs'], ['playlists', 'Playlists']]
  return (
    <div className="mh">
      <LibrarySwitch />
      <div className="toolbar mh-toolbar">
        <h1>{lib.name}</h1>
        <div className="seg" role="tablist" aria-label={`Parts of ${lib.name}`}>
          {tabs.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => { const p = new URLSearchParams(); if (k !== 'home') p.set('tab', k); setParams(p) }}>{l}</button>)}
        </div>
        {adding && <AddPodcast libraryId={lib.id} onClose={() => setAdding(false)} onAdded={() => { setAdding(false); setReload((n) => n + 1) }} />}
        <div className="grow" />
        <ScanButton lib={lib} />
        {pods && user?.isAdmin && <button className="btn primary" onClick={() => setAdding(true)}>Follow a podcast</button>}
      </div>
      {tab === 'home' && <HomeTab lib={lib} reload={reload} onReload={() => setReload((n) => n + 1)} />}
      {tab === 'albums' && <AlbumsTab lib={lib} params={params} set={set} reload={reload} onReload={() => setReload((n) => n + 1)} />}
      {tab === 'artists' && <ArtistsTab lib={lib} />}
      {tab === 'songs' && <SongsTab lib={lib} params={params} set={set} />}
      {tab === 'playlists' && <PlaylistsTab lib={lib} />}
      {pods && <LegalNote kind="webMedia" />}
    </div>
  )
}

function HomeTab({ lib, reload, onReload }: { lib: Library; reload: number; onReload: () => void }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user } = useApp()
  const [home, setHome] = useState<Home | null>(null)
  const [failed, setFailed] = useState(false)
  const [hero, setHero] = useState<Album | null>(null)
  const audio = useAudio()
  const nav = useNavigate()
  useEffect(() => {
    setFailed(false)
    api.get<Home>(`/music/home?library=${lib.id}`).then((h) => { setHome(h); setHero(h.hero[0] || null) }).catch(() => setFailed(true))
  }, [lib.id, reload])
  const rgb = useArtColour(hero?.coverItem ? coverUrl(hero.coverItem) : null)
  const covers = useMemo<Cover<Album>[]>(() => (home?.hero || []).map((a) => ({ key: a.artist + '|' + a.title, url: coverUrl(a.coverItem), label: `${a.title}${a.artist ? ` by ${a.artist}` : ''}`, item: a })), [home])
  if (failed) return <EmptyState icon={<IconAudio />} title="This library wouldn't load" text="The server didn't answer. Try again in a moment." />
  if (!home) return <SkeletonGrid square={!books} />
  if (!home.hero.length) {
    if (pods) {
      return (
        <div className="pod-empty">
          <h2>Your podcasts will live here</h2>
          <PodcastWays />
          {user?.isAdmin ? <PodcastFinder libraryId={lib.id} onAdded={onReload} /> : <p className="muted">Ask whoever runs this server to follow some shows.</p>}
        </div>
      )
    }
    return <EmptyState icon={<IconAudio />} title="Nothing here yet" text="Add audio files to this library's folders and scan it." />
  }
  const mode: AudioMode = pods ? 'podcasts' : books ? 'audiobooks' : 'music'
  const playAll = async () => {
    try {
      const all = await api.get<Item[]>(`/music/songs?library=${lib.id}&sort=random&limit=500`)
      if (all.length) audio.play(all, 0, 'music')
    } catch (e) { toast((e as Error).message) }
  }
  const playAlbum = async (a: { artist: string; title: string }, at?: number) => {
    try {
      const tracks = await api.get<Item[]>(`/music/tracks?library=${lib.id}&artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`)
      if (!tracks.length) return
      const chosen = at ? tracks.findIndex((t) => t.id === at) : -1
      const from = chosen >= 0 ? chosen : books ? Math.max(0, tracks.findIndex((t) => !t.progress?.watched)) : 0
      audio.play(tracks, from, mode)
    } catch (e) { toast((e as Error).message) }
  }
  const noun: [string, string] = books ? ['book', 'books'] : pods ? ['show', 'shows'] : ['album', 'albums']
  return (
    <div className="mh-home" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mh-bg" aria-hidden="true" />
      <div className="mh-hero">
        <CoverFlow covers={covers} tall={books} onChange={(c) => setHero(c.item)} onOpen={(c) => nav(albumLink(lib.id, c.item))} fallback={(c) => <Placeholder title={c.item.title || 'Unknown'} />} />
        {hero && (
          <div className="mh-hero-text">
            <h2><Link to={albumLink(lib.id, hero)}>{hero.title || 'Unknown album'}</Link></h2>
            <p className="mh-hero-sub">
              {hero.artist && !pods && <Link to={artistLink(lib.id, hero.artist)}>{hero.artist}</Link>}
              {[hero.year, hero.genre].filter(Boolean).map((v) => <span key={String(v)}> · {v}</span>)}
            </p>
          </div>
        )}
        <div className="mh-hero-actions">
          {!books && !pods && <button className="btn mh-play-all" onClick={() => void playAll()}><IconPlay /> Play my music</button>}
          {hero && <button className={'btn ' + (books || pods ? 'mh-play-all' : 'mh-play-album')} onClick={() => void playAlbum(hero)}><IconPlay /> {books ? 'Play this book' : pods ? 'Play this show' : 'Play this album'}</button>}
        </div>
      </div>

      {home.continue.length > 0 && (
        <Shelf title="Continue listening">
          <div className="mh-row mh-continue">
            {home.continue.map((t) => {
              const pct = t.progress && t.runtimeSec ? Math.min(100, (t.progress.positionSec / t.runtimeSec) * 100) : 0
              return (
                <div key={t.id} className="mh-resume">
                  <span className="mh-resume-art">{t.poster && <img src={posterUrl(t)} alt="" loading="lazy" />}</span>
                  <span className="mh-resume-text">
                    <b>{t.album || t.title}</b>
                    <small>{t.artist || t.albumArtist}</small>
                    <small>{Math.round(pct)}% listened</small>
                    <span className="mh-resume-bar" aria-hidden="true"><i style={{ width: pct + '%' }} /></span>
                    <button className="btn sm" onClick={() => void playAlbum({ artist: t.albumArtist || t.artist || '', title: t.album || '' }, t.id)}><IconPlay size={16} /> Resume</button>
                  </span>
                </div>
              )
            })}
          </div>
        </Shelf>
      )}

      {!pods && home.artists.length > 0 && (
        <Shelf title={books ? 'Authors' : 'Artists'} to={`?tab=artists`}>
          <div className="mh-row mh-artists">
            {home.artists.map((a) => <ArtistTile key={a.name} libId={lib.id} a={a} noun={noun} />)}
            {home.counts.artists > home.artists.length || home.artists.length > 4 ? <SeeAllTile to="?tab=artists" round /> : null}
          </div>
        </Shelf>
      )}
      <Shelf title={books ? 'Books' : pods ? 'Shows' : 'Albums'} count={home.counts.albums} to="?tab=albums">
        <div className="mh-row mh-albums">
          {home.albums.map((a) => <AlbumTile key={a.artist + '|' + a.title} libId={lib.id} a={a} book={books} />)}
          {home.counts.albums > home.albums.length && <SeeAllTile to="?tab=albums" />}
        </div>
      </Shelf>
      {!books && !pods && (
        <Shelf title="Songs" count={home.counts.songs} to="?tab=songs">
          <div className="mh-songs">
            {home.songs.map((t, i) => <SongRow key={t.id} t={t} onPlay={() => audio.play(home.songs, i, 'music')} />)}
          </div>
        </Shelf>
      )}
      {home.recent.length > 0 && !books && !pods && (
        <Shelf title="Played lately" to="?tab=songs&view=recent">
          <div className="mh-songs">
            {home.recent.map((t, i) => <SongRow key={t.id} t={t} onPlay={() => audio.play(home.recent, i, 'music')} />)}
          </div>
        </Shelf>
      )}
    </div>
  )
}

function AlbumsTab({ lib, params, set, reload, onReload }: { lib: Library; params: URLSearchParams; set: (k: string, v: string | null) => void; reload: number; onReload: () => void }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user } = useApp()
  const sort = params.get('sort') || (books ? 'artist' : 'title')
  const genre = params.get('genre') || ''
  const artist = params.get('artist')
  const [albums, setAlbums] = useState<Album[] | null>(null)
  const [genres, setGenres] = useState<string[]>([])
  useEffect(() => {
    setAlbums(null)
    api.get<Album[]>(`/music/albums?library=${lib.id}&sort=${sort}${genre ? `&genre=${encodeURIComponent(genre)}` : ''}${artist !== null ? `&artist=${encodeURIComponent(artist)}` : ''}`).then(setAlbums).catch(() => setAlbums([]))
  }, [lib.id, sort, genre, artist, reload])
  useEffect(() => { if (!pods) api.get<string[]>(`/genres?library=${lib.id}`).then(setGenres).catch(() => setGenres([])) }, [lib.id, pods])
  const noun = pods ? ['podcast', 'podcasts'] : books ? ['book', 'books'] : ['album', 'albums']
  return (
    <>
      <div className="mh-filters">
        {albums && <span className="count">{albums.length} {albums.length === 1 ? noun[0] : noun[1]}</span>}
        {artist !== null && <button className="btn sm ghost" onClick={() => set('artist', null)}>{artist || 'Unknown'} ✕</button>}
        <div className="grow" />
        {genres.length > 0 && (
          <select className="select" value={genre} aria-label="Genre" onChange={(e) => set('genre', e.target.value || null)}>
            <option value="">All genres</option>
            {genres.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
        )}
        {!pods && (
          <select className="select" value={sort} aria-label="Sort" onChange={(e) => set('sort', e.target.value)}>
            <option value="title">Title</option><option value="artist">{books ? 'Author' : 'Artist'}</option><option value="year">Year</option><option value="added">Recently added</option><option value="played">Most played</option>
          </select>
        )}
      </div>
      {!albums ? <SkeletonGrid square={!books} /> : albums.length === 0 ? (pods ? (
        <div className="pod-empty">
          <h2>Your podcasts will live here</h2>
          <PodcastWays />
          {user?.isAdmin ? <PodcastFinder libraryId={lib.id} onAdded={onReload} /> : <p className="muted">Ask whoever runs this server to follow some shows.</p>}
        </div>
      ) : <EmptyState icon={<IconAudio />} title="Nothing here" text={genre ? 'Nothing in this genre.' : "Add audio files to this library's folders and scan it."} />) : (
        <div className="album-grid">
          {albums.map((a) => <AlbumTile key={a.artist + '|' + a.title} libId={lib.id} a={a} book={books} />)}
        </div>
      )}
    </>
  )
}

function ArtistsTab({ lib }: { lib: Library }) {
  const books = lib.type === 'audiobooks'
  const [artists, setArtists] = useState<Artist[] | null>(null)
  const [q, setQ] = useState('')
  useEffect(() => { setArtists(null); api.get<Artist[]>(`/music/artists?library=${lib.id}`).then(setArtists).catch(() => setArtists([])) }, [lib.id])
  const shown = (artists || []).filter((a) => !q || a.name.toLowerCase().includes(q.toLowerCase()))
  return (
    <>
      <div className="mh-filters">
        {artists && <span className="count">{artists.length} {books ? (artists.length === 1 ? 'author' : 'authors') : artists.length === 1 ? 'artist' : 'artists'}</span>}
        <div className="grow" />
        <input className="input mh-find" type="search" aria-label={`Find ${books ? 'an author' : 'an artist'}`} placeholder={`Find ${books ? 'an author' : 'an artist'}…`} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {!artists ? <SkeletonGrid square /> : (
        <div className="album-grid mh-artist-grid">
          {shown.map((a) => <ArtistTile key={a.name} libId={lib.id} a={a} noun={books ? ['book', 'books'] : ['album', 'albums']} />)}
        </div>
      )}
    </>
  )
}

const VIEWS: [string, string][] = [['', 'All songs'], ['liked', 'Liked'], ['played', 'Most played'], ['recent', 'Played lately'], ['added', 'Newest']]

function SongsTab({ lib, params, set }: { lib: Library; params: URLSearchParams; set: (k: string, v: string | null) => void }) {
  const view = params.get('view') || ''
  const sort = params.get('sort') || 'title'
  const genre = params.get('genre') || ''
  const [q, setQ] = useState('')
  const [search, setSearch] = useState('')
  const [songs, setSongs] = useState<Item[] | null>(null)
  const [genres, setGenres] = useState<string[]>([])
  const audio = useAudio()
  useEffect(() => { const t = setTimeout(() => setSearch(q.trim()), 250); return () => clearTimeout(t) }, [q])
  useEffect(() => {
    setSongs(null)
    const u = new URLSearchParams({ library: String(lib.id), limit: '2000' })
    if (view) u.set('view', view)
    if (!view || view === 'liked') u.set('sort', sort)
    if (genre) u.set('genre', genre)
    if (search) u.set('search', search)
    api.get<Item[]>(`/music/songs?${u}`).then(setSongs).catch(() => setSongs([]))
  }, [lib.id, view, sort, genre, search])
  useEffect(() => { api.get<string[]>(`/genres?library=${lib.id}`).then(setGenres).catch(() => setGenres([])) }, [lib.id])
  const total = (songs || []).reduce((s, t) => s + t.runtimeSec, 0)
  return (
    <>
      <div className="seg mh-views" role="tablist" aria-label="Which songs">
        {VIEWS.map(([k, l]) => <button key={k} role="tab" aria-selected={view === k} className={view === k ? 'active' : ''} onClick={() => set('view', k || null)}>{l}</button>)}
      </div>
      <div className="mh-filters">
        {songs && <span className="count">{songs.length.toLocaleString()} {songs.length === 1 ? 'song' : 'songs'}{total > 0 ? ` · ${fmtRuntime(total)}` : ''}</span>}
        <button className="btn sm primary" disabled={!songs?.length} onClick={() => songs && audio.play(songs, 0, 'music')}><IconPlay size={16} /> Play</button>
        <button className="btn sm" disabled={!songs?.length} onClick={() => songs && audio.play(shuffledCopy(songs), 0, 'music')}>Shuffle</button>
        <div className="grow" />
        <input className="input mh-find" type="search" aria-label="Find a song" placeholder="Find a song…" value={q} onChange={(e) => setQ(e.target.value)} />
        {genres.length > 0 && (
          <select className="select" value={genre} aria-label="Genre" onChange={(e) => set('genre', e.target.value || null)}>
            <option value="">All genres</option>
            {genres.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
        )}
        {(!view || view === 'liked') && (
          <select className="select" value={sort} aria-label="Sort" onChange={(e) => set('sort', e.target.value)}>
            <option value="title">Title</option><option value="artist">Artist</option><option value="album">Album</option><option value="year">Year</option><option value="added">Date added</option><option value="runtime">Length</option>
          </select>
        )}
      </div>
      {!songs ? <Spinner /> : songs.length === 0
        ? <EmptyState icon={<IconAudio />} title={search || genre ? 'Nothing matches that' : view === 'liked' ? 'No liked songs yet' : view === 'played' || view === 'recent' ? 'Nothing played yet' : 'No songs here'} text={!search && !genre && view === 'liked' ? 'Press the heart on a song to keep it here.' : ''} />
        : <TrackList items={songs} showAlbum onPlay={(i) => audio.play(songs, i, 'music')} />}
    </>
  )
}

function PlaylistsTab({ lib }: { lib: Library }) {
  const audio = useAudio()
  const [lists, setLists] = useState<{ id: number; name: string; kind: string; count: number; coverId: number; system?: string }[] | null>(null)
  useEffect(() => { api.get<typeof lists>('/collections').then(setLists).catch(() => setLists([])) }, [])
  const playView = async (view: string, shuffle?: boolean) => {
    try {
      const items = await api.get<Item[]>(`/music/songs?library=${lib.id}&view=${view}&limit=500`)
      if (!items.length) { toast('Nothing there yet'); return }
      audio.play(shuffle ? shuffledCopy(items) : items, 0, 'music')
    } catch (e) { toast((e as Error).message) }
  }
  const built: { view: string; name: string; desc: string }[] = [
    { view: 'liked', name: 'Liked songs', desc: 'Everything you\'ve pressed the heart on' },
    { view: 'played', name: 'Most played', desc: 'Your favourites, by how often you play them' },
    { view: 'recent', name: 'Played lately', desc: 'What you\'ve been listening to' },
    { view: 'added', name: 'Newest', desc: 'What came to this library most recently' },
  ]
  return (
    <>
      <div className="mh-lists">
        {built.map((b) => (
          <div key={b.view} className="mh-list">
            <Link to={`?tab=songs&view=${b.view}`} className="mh-list-text"><b>{b.name}</b><small>{b.desc}</small></Link>
            <button className="mp-ibtn mh-list-play" onClick={() => void playView(b.view)} aria-label={`Play ${b.name}`}><IconPlay /></button>
          </div>
        ))}
      </div>
      <div className="mh-shelf-head"><h2>Your playlists</h2><Link className="mh-seeall" to="/collections">Make one, or a smart one</Link></div>
      {!lists ? <Spinner /> : lists.length === 0 ? <p className="muted">None yet. Add any song to a playlist from its menu, or make a smart one in Collections that keeps itself up to date.</p> : (
        <div className="mh-lists">
          {lists.filter((c) => !c.system).map((c) => (
            <Link key={c.id} to={`/collections/${c.id}`} className="mh-list mh-list-text">
              <span className="mh-list-art">{c.coverId > 0 && <img src={coverUrl(c.coverId)} alt="" loading="lazy" />}</span>
              <span className="mh-list-words"><b>{c.name}</b><small>{c.kind === 'smart' ? 'Smart playlist' : `${c.count} ${c.count === 1 ? 'item' : 'items'}`}</small></span>
            </Link>
          ))}
        </div>
      )}
    </>
  )
}

export function TrackList({ items, onPlay, showAlbum, episodes, extra, discs, heart }: { items: Item[]; onPlay: (i: number) => void; showAlbum?: boolean; episodes?: boolean; extra?: (it: Item) => React.ReactNode; discs?: boolean; heart?: boolean }) {
  const { current, playing, liked, toggleLike } = useAudio()
  const showHearts = heart ?? !episodes
  // A heading wherever the disc changes, when an album has more than one.
  const manyDiscs = discs && new Set(items.map((t) => t.discNo || 1)).size > 1
  return (
    <ol className="track-list">
      {items.map((t, i) => {
        const on = current?.id === t.id
        const pct = t.progress && t.runtimeSec ? Math.min(100, (t.progress.positionSec / t.runtimeSec) * 100) : 0
        const newDisc = manyDiscs && (i === 0 || (items[i - 1].discNo || 1) !== (t.discNo || 1))
        const hearted = liked(t.id)
        return (
          <li key={t.id} className={(on ? 'on' : '') + (newDisc ? ' new-disc' : '')} data-disc={newDisc ? `Disc ${t.discNo || 1}` : undefined}>
            <button onClick={() => onPlay(i)} aria-label={`Play ${t.title}`} aria-current={on ? 'true' : undefined}>
              <span className="tl-n">{on && playing ? <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i> : episodes ? '' : t.track ?? i + 1}</span>
              <span className="tl-t"><b>{t.title}</b>{showAlbum && <span className="muted">{[t.artist, t.album].filter(Boolean).join(' · ')}</span>}
                {episodes && <span className="muted">{[t.aired, t.overview].filter(Boolean).join(' · ').slice(0, 160)}</span>}
                {pct > 0 && <span className="tl-prog" style={{ width: pct + '%' }} />}</span>
              <span className="tl-d muted">{t.progress?.watched && <IconCheck />}{t.runtimeSec ? fmtTime(t.runtimeSec) : ''}</span>
            </button>
            {showHearts && <button type="button" className={'mp-ibtn tl-heart' + (hearted ? ' liked' : '')} aria-pressed={hearted} aria-label={hearted ? `Remove ${t.title} from liked songs` : `Like ${t.title}`} onClick={() => toggleLike(t.id)}>{hearted ? <IconHeartOn size={18} /> : <IconHeart size={18} />}</button>}
            {showHearts && <TrackMenu t={t} />}
            {extra && <span className="tl-x">{extra(t)}</span>}
          </li>
        )
      })}
    </ol>
  )
}

// A name without what tends to mark a version of the same album: "(Deluxe)", "[2011 Remaster]".
const baseTitle = (s: string) => s.toLowerCase().replace(/[([{].*?[)\]}]/g, ' ').replace(/\b(deluxe|remaster(ed)?|expanded|special|anniversary|edition|version)\b/g, ' ').replace(/[^\p{L}\p{N}]+/gu, ' ').trim()

// AlbumPage: one album or audiobook.
export function AlbumPage() {
  const { lib: libId } = useParams()
  const [params] = useSearchParams()
  const artist = params.get('artist') ?? ''
  const album = params.get('album') ?? ''
  const [lib, setLib] = useState<Library | null>(null)
  const [tracks, setTracks] = useState<Item[] | null>(null)
  const [reloadTracks, setReloadTracks] = useState(0)
  const [others, setOthers] = useState<Album[]>([])
  const audio = useAudio()
  const nav = useNavigate()
  useEffect(() => {
    api.get<Library[]>('/libraries').then((ls) => setLib(ls.find((l) => String(l.id) === libId) || null)).catch(() => {})
    api.get<Item[]>(`/music/tracks?library=${libId}&artist=${encodeURIComponent(artist)}&album=${encodeURIComponent(album)}`).then(setTracks).catch(() => setTracks([]))
  }, [libId, artist, album, reloadTracks])
  // Other albums by the same artist: other versions of this one, and the rest.
  useEffect(() => {
    setOthers([])
    if (!artist) return
    api.get<Album[]>(`/music/albums?library=${libId}&artist=${encodeURIComponent(artist)}&sort=year`).then((l) => setOthers(l.filter((a) => a.title !== album))).catch(() => {})
  }, [libId, artist, album])
  // A disc whose image has gone isn't offered as one to insert.
  const [goneDiscs, setGoneDiscs] = useState<number[]>([])
  const discKey = [...new Set((tracks || []).map((t) => t.cdDisc).filter(Boolean))].join(',')
  useEffect(() => {
    let left = false
    setGoneDiscs([])
    if (discKey) discKey.split(',').forEach((d) => api.get(`/cd/${d}`).catch((e) => { if (!left && (e.status === 404 || e.status === 422)) setGoneDiscs((g) => [...g, Number(d)]) }))
    return () => { left = true }
  }, [discKey])
  const coverId = tracks?.find((t) => t.poster)?.id
  const rgb = useArtColour(coverId ? coverUrl(coverId) : null)
  if (!tracks) return <Spinner />
  const mode: AudioMode = lib?.type === 'audiobooks' ? 'audiobooks' : lib?.type === 'podcasts' ? 'podcasts' : 'music'
  const books = mode !== 'music'
  const pods = mode === 'podcasts'
  const total = tracks.reduce((s, t) => s + t.runtimeSec, 0)
  // Audiobooks continue at the first chapter that is not finished.
  const resumeAt = Math.max(0, tracks.findIndex((t) => !t.progress?.watched))
  const started = books && tracks.some((t) => t.progress && t.progress.positionSec > 5)
  // Tracks from CD images play from their disc, with no gaps between them.
  const discs = [...new Set(tracks.map((t) => t.cdDisc).filter((d): d is number => !!d))]
  const playTrack = (i: number) => {
    const t = tracks[i]
    if (t?.cdDisc) nav(`/cd/${t.cdDisc}?play=1&track=${t.track}`)
    else audio.play(tracks, i, mode)
  }
  const numericLib = Number(libId)
  const genre = tracks[0]?.genres?.[0]
  const versions = others.filter((a) => baseTitle(a.title) === baseTitle(album))
  const more = others.filter((a) => baseTitle(a.title) !== baseTitle(album))
  const allLiked = tracks.length > 0 && tracks.every((t) => audio.liked(t.id))
  return (
    <div className="album-page" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mh-bg" aria-hidden="true" />
      <BackLink to={`/library/${libId}`} label={lib?.name || (books ? 'Audiobooks' : 'Music')} />
      <div className="album-head">
        <div className={'album-cover big' + (books ? ' book' : '')}>{coverId ? <img src={coverUrl(coverId)} alt="" /> : <Placeholder title={tracks[0]?.album || 'Unknown'} />}</div>
        <div className="album-meta">
          <span className="muted">{pods ? 'Podcast' : books ? 'Audiobook' : 'Album'}</span>
          <h1>{album || 'Unknown album'}</h1>
          {pods ? <span className="album-artist">{artist}</span> : <Link to={artistLink(numericLib, artist)} className="album-artist">{artist || 'Unknown'}</Link>}
          <div className="muted">{[`${tracks.length} ${pods ? 'episodes' : books ? 'chapters' : tracks.length === 1 ? 'track' : 'tracks'}`, pods ? '' : fmtRuntime(total), pods ? '' : tracks[0]?.year, pods ? '' : genre].filter(Boolean).join(' · ')}</div>
          <div className="actions">
            <button className="btn primary" onClick={() => playTrack(books && !pods ? resumeAt : 0)}><IconPlay /> {pods ? 'Play newest' : started ? 'Continue' : 'Play'}</button>
            {discs.filter((d) => !goneDiscs.includes(d)).map((d) => <Link key={d} className="btn" to={`/cd/${d}?play=1`}><IconDisc /> {discs.length > 1 ? `Disc ${discs.indexOf(d) + 1}` : 'Insert disc'}</Link>)}
            {!books && <button className="btn" onClick={() => audio.play(shuffledCopy(tracks), 0, mode)}>Shuffle</button>}
            {!books && tracks[0] && <button className="btn ghost" onClick={() => audio.startRadio(tracks[0])}><IconStation size={18} /> Station</button>}
            <button className="btn ghost" onClick={() => { audio.playNext(tracks); toast('Plays next') }}>Play next</button>
            <button className="btn ghost" onClick={() => { audio.enqueue(tracks); toast('Added to the queue') }}>Add to queue</button>
            {!books && <button className="btn ghost" aria-pressed={allLiked} onClick={() => tracks.forEach((t) => { if (audio.liked(t.id) === allLiked) audio.toggleLike(t.id) })}>{allLiked ? <IconHeartOn size={18} /> : <IconHeart size={18} />} {allLiked ? 'Liked' : 'Like all'}</button>}
          </div>
          {tracks[0]?.overview && !pods && <p className="album-desc">{tracks[0].overview}</p>}
          {pods && libId && <PodcastAdmin libraryId={numericLib} title={album} onChanged={() => setReloadTracks((n) => n + 1)} />}
        </div>
      </div>
      <TrackList items={tracks} onPlay={playTrack} episodes={pods} discs={!books} heart={!books}
        extra={pods ? (t) => <DownloadButton item={t} onDone={(x) => setTracks((ts) => ts!.map((y) => (y.id === x.id ? { ...y, path: x.path } : y)))} /> : undefined} />
      {versions.length > 0 && <Shelf title="Other versions"><div className="mh-row mh-albums">{versions.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} noArtist />)}</div></Shelf>}
      {more.length > 0 && !books && <Shelf title={`More by ${artist}`} to={artistLink(numericLib, artist)}><div className="mh-row mh-albums">{more.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} noArtist />)}</div></Shelf>}
      <LegalNote kind={pods ? 'webMedia' : 'metadata'} />
    </div>
  )
}

