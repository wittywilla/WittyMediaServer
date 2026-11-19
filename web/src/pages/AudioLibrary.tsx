// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fitWidth, fmtRuntime, fmtTime, posterUrl, sized, type Album, type Artist, type Item, type Library } from '../api'
import { useArtColour } from '../audio/art'
import { IconHeart, IconHeartOn, IconStation } from '../audio/icons'
import { useAudio, type AudioMode } from '../audio/player'
import { BackLink, EmptyState, Placeholder, ScanButton, SkeletonGrid, Spinner } from '../components/common'
import { IconAudio, IconCheck, IconDisc, IconPlay } from '../icons'
import { PlayOnButton, queueOf } from '../remote/PlayOnButton'
import { toast, useApp } from '../state'
import { emptyLibraryWords } from '../profileWords'
import { AddPodcast, DownloadButton, PodcastAdmin, PodcastFinder, PodcastWays } from './Podcasts'
import LegalNote from '../components/LegalNote'
import LibraryHeader, { type SortOption } from '../components/LibraryHeader'
import CoverFlow, { type Cover } from './music/CoverFlow'
import { AlbumTile, albumLink, ArtistTile, artistLink, coverUrl, LibrarySwitch, SeeAllTile, Shelf, shuffledCopy, SongRow, TrackMenu } from './music/parts'
import { hasMusicVideos, MusicVideosShelf, MusicVideosTab } from './music/MusicVideos'
import { num, t, tk, tn, tx } from '../i18n'

// Music, audiobook and podcast libraries. The front page has what you came back for: a shelf
// of covers, artists, albums and songs, each with its "see all". Those pages sort and filter.
// Then the page of one album or book. The first thing played from any of them keeps going in
// the bar along the bottom (audio/player.tsx).

type Home = { hero: Album[]; albums: Album[]; artists: Artist[]; songs: Item[]; recent: Item[]; continue: Item[]; genres: string[]; counts: { albums: number; artists: number; songs: number; liked: number } }

export default function AudioLibrary({ lib }: { lib: Library }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user, libraries } = useApp()
  const videos = lib.type === 'music' && hasMusicVideos(libraries)
  const [adding, setAdding] = useState(false)
  const [reload, setReload] = useState(0)
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') || 'home'
  const set = (k: string, v: string | null) => { const p = new URLSearchParams(params); v === null ? p.delete(k) : p.set(k, v); setParams(p) }
  const view = params.get('view') || ''
  const sorts: SortOption[] | undefined =
    tab === 'albums' && !pods ? [{ key: 'title', label: t('Title') }, { key: 'artist', label: books ? t('Author') : t('Artist') }, { key: 'year', label: t('Year'), desc: true }, { key: 'added', label: t('Recently added'), desc: true }, { key: 'played', label: t('Most played'), desc: true }]
      : tab === 'artists' ? [{ key: 'name', label: t('Name') }, { key: 'albums', label: books ? t('Most books') : t('Most albums'), desc: true }]
        : tab === 'songs' && (!view || view === 'liked') ? [{ key: 'title', label: t('Title') }, { key: 'artist', label: t('Artist') }, { key: 'album', label: t('Album') }, { key: 'year', label: t('Year'), desc: true }, { key: 'added', label: t('Date added'), desc: true }, { key: 'runtime', label: t('Length'), desc: true }]
          : undefined
  const sort = params.get('sort') || (tab === 'artists' ? 'name' : books ? 'artist' : 'title')
  const desc = params.get('desc') === '1'
  const chooseSort = (key: string, d: boolean) => { const p = new URLSearchParams(params); p.set('sort', key); if (d) p.set('desc', '1'); else p.delete('desc'); setParams(p) }
  const tabs: [string, string][] = pods ? [['home', t('Home')], ['albums', t('Podcasts')]]
    : books ? [['home', t('Home')], ['albums', t('Books')], ['artists', t('Authors')]]
      : [['home', t('Home')], ['albums', t('Albums')], ['artists', t('Artists')], ['songs', t('Songs')], ...(videos ? [['videos', t('Music videos')] as [string, string]] : []), ['playlists', t('Playlists')]]
  return (
    <div className="mh">
      <LibrarySwitch />
      <LibraryHeader
        title={lib.name}
        tabs={<div className="seg" role="tablist" aria-label={t('Parts of {name}', { name: lib.name })}>
          {tabs.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => { const p = new URLSearchParams(); if (k !== 'home') p.set('tab', k); setParams(p) }}>{l}</button>)}
        </div>}
        sorts={sorts}
        sort={sort}
        desc={desc}
        onSort={chooseSort}
        onDirection={(d) => chooseSort(sort, d)}
        actions={<>
          <ScanButton lib={lib} />
          {pods && user?.isAdmin && <button className="btn primary" onClick={() => setAdding(true)}>{t('Follow a podcast')}</button>}
        </>}
      />
      {adding && <AddPodcast libraryId={lib.id} onClose={() => setAdding(false)} onAdded={() => { setAdding(false); setReload((n) => n + 1) }} />}
      {tab === 'home' && <HomeTab lib={lib} reload={reload} onReload={() => setReload((n) => n + 1)} />}
      {tab === 'albums' && <AlbumsTab lib={lib} params={params} set={set} reload={reload} onReload={() => setReload((n) => n + 1)} />}
      {tab === 'artists' && <ArtistsTab lib={lib} sort={sort} desc={desc} />}
      {tab === 'songs' && <SongsTab lib={lib} params={params} set={set} />}
      {tab === 'videos' && videos && <MusicVideosTab lib={lib} />}
      {tab === 'playlists' && <PlaylistsTab lib={lib} />}
      {pods && <LegalNote kind="webMedia" />}
    </div>
  )
}

function HomeTab({ lib, reload, onReload }: { lib: Library; reload: number; onReload: () => void }) {
  const books = lib.type === 'audiobooks'
  const pods = lib.type === 'podcasts'
  const { user, libraries } = useApp()
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
  const covers = useMemo<Cover<Album>[]>(() => (home?.hero || []).map((a) => ({ key: a.artist + '|' + a.title, url: coverUrl(a.coverItem), label: a.artist ? t('{title} by {artist}', { title: a.title, artist: a.artist }) : a.title, item: a })), [home])
  if (failed) return <EmptyState icon={<IconAudio />} title={t('This library wouldn’t load')} text={t('The server didn’t answer. Try again in a moment.')} />
  if (!home) return <SkeletonGrid square={!books} />
  if (!home.hero.length) {
    if (pods) {
      return (
        <div className="pod-empty">
          <h2>{t('Your podcasts will live here')}</h2>
          <PodcastWays />
          {user?.isAdmin ? <PodcastFinder libraryId={lib.id} onAdded={onReload} /> : <p className="muted">{t('Ask whoever runs this server to follow some shows.')}</p>}
        </div>
      )
    }
    return <EmptyState icon={<IconAudio />} title={t('Nothing here yet')} text={tx(emptyLibraryWords(user, t('Add audio files to this library’s folders and scan it.')))} />
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
      const chosen = at ? tracks.findIndex((song) => song.id === at) : -1
      const from = chosen >= 0 ? chosen : books ? Math.max(0, tracks.findIndex((song) => !song.progress?.watched)) : 0
      audio.play(tracks, from, mode)
    } catch (e) { toast((e as Error).message) }
  }
  return (
    <div className="mh-home" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mh-bg" aria-hidden="true" />
      <div className="mh-hero">
        <CoverFlow covers={covers} tall={books} onChange={(c) => setHero(c.item)} onOpen={(c) => nav(albumLink(lib.id, c.item))} fallback={(c) => <Placeholder title={c.item.title || t('Unknown')} />} />
        {hero && (
          <div className="mh-hero-text">
            <h2><Link to={albumLink(lib.id, hero)}>{hero.title || t('Unknown album')}</Link></h2>
            <p className="mh-hero-sub">
              {hero.artist && !pods && <Link to={artistLink(lib.id, hero.artist)}>{hero.artist}</Link>}
              {[hero.year, hero.genre].filter(Boolean).map((v, i) => <span key={String(v)}>{i > 0 || (hero.artist && !pods) ? ' · ' : ''}{v}</span>)}
            </p>
          </div>
        )}
        <div className="mh-hero-actions">
          {!books && !pods && <button data-tv-first className="btn mh-play-all" onClick={() => void playAll()}><IconPlay /> {t('Play my music')}</button>}
          {hero && <button data-tv-first={books || pods || undefined} className={'btn ' + (books || pods ? 'mh-play-all' : 'mh-play-album')} onClick={() => void playAlbum(hero)}><IconPlay /> {books ? t('Play this book') : pods ? t('Play this show') : t('Play this album')}</button>}
        </div>
      </div>

      {home.continue.length > 0 && (
        <Shelf title={t('Continue listening')}>
          <div className="mh-row mh-continue">
            {home.continue.map((song) => {
              const pct = song.progress && song.runtimeSec ? Math.min(100, (song.progress.positionSec / song.runtimeSec) * 100) : 0
              return (
                <div key={song.id} className="mh-resume">
                  <span className="mh-resume-art">{song.poster && <img src={sized(posterUrl(song), fitWidth(96))} alt="" loading="lazy" />}</span>
                  <span className="mh-resume-text">
                    <b>{song.album || song.title}</b>
                    <small>{song.artist || song.albumArtist}</small>
                    <small>{t('{percent} listened', { percent: num(pct / 100, { style: 'percent' }) })}</small>
                    <span className="mh-resume-bar" aria-hidden="true"><i style={{ width: pct + '%' }} /></span>
                    <button className="btn sm" onClick={() => void playAlbum({ artist: song.albumArtist || song.artist || '', title: song.album || '' }, song.id)}><IconPlay size={16} /> {t('Resume')}</button>
                  </span>
                </div>
              )
            })}
          </div>
        </Shelf>
      )}

      {!pods && home.artists.length > 0 && (
        <Shelf title={books ? t('Authors') : t('Artists')} to={`?tab=artists`}>
          <div className="mh-row mh-artists">
            {home.artists.map((a) => <ArtistTile key={a.name} libId={lib.id} a={a} books={books} />)}
            {home.counts.artists > home.artists.length || home.artists.length > 4 ? <SeeAllTile to="?tab=artists" round /> : null}
          </div>
        </Shelf>
      )}
      <Shelf title={books ? t('Books') : pods ? t('Shows') : t('Albums')} count={home.counts.albums} to="?tab=albums">
        <div className="mh-row mh-albums">
          {home.albums.map((a) => <AlbumTile key={a.artist + '|' + a.title} libId={lib.id} a={a} book={books} />)}
          {home.counts.albums > home.albums.length && <SeeAllTile to="?tab=albums" />}
        </div>
      </Shelf>
      {!books && !pods && (
        <Shelf title={t('Songs')} count={home.counts.songs} to="?tab=songs">
          <div className="mh-songs">
            {home.songs.map((song, i) => <SongRow key={song.id} t={song} onPlay={() => audio.play(home.songs, i, 'music')} />)}
          </div>
        </Shelf>
      )}
      {lib.type === 'music' && hasMusicVideos(libraries) && <MusicVideosShelf lib={lib} />}
      {home.recent.length > 0 && !books && !pods && (
        <Shelf title={t('Played lately')} to="?tab=songs&view=recent">
          <div className="mh-songs">
            {home.recent.map((song, i) => <SongRow key={song.id} t={song} onPlay={() => audio.play(home.recent, i, 'music')} />)}
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
  const desc = params.get('desc') === '1'
  const genre = params.get('genre') || ''
  const artist = params.get('artist')
  const [albums, setAlbums] = useState<Album[] | null>(null)
  // The list comes whole, newest or biggest first where that's the natural order for it,
  // so the other way round is a matter of reading it backwards.
  const newestFirst = sort === 'year' || sort === 'added' || sort === 'played'
  const shown = albums && desc !== newestFirst ? [...albums].reverse() : albums
  const [genres, setGenres] = useState<string[]>([])
  useEffect(() => {
    setAlbums(null)
    api.get<Album[]>(`/music/albums?library=${lib.id}&sort=${sort}${genre ? `&genre=${encodeURIComponent(genre)}` : ''}${artist !== null ? `&artist=${encodeURIComponent(artist)}` : ''}`).then(setAlbums).catch(() => setAlbums([]))
  }, [lib.id, sort, genre, artist, reload])
  useEffect(() => { if (!pods) api.get<string[]>(`/genres?library=${lib.id}`).then(setGenres).catch(() => setGenres([])) }, [lib.id, pods])
  return (
    <>
      <div className="mh-filters">
        {albums && <span className="count">{pods ? tn('{count} podcast', '{count} podcasts', albums.length) : books ? tn('{count} book', '{count} books', albums.length) : tn('{count} album', '{count} albums', albums.length)}</span>}
        {artist !== null && <button className="btn sm ghost" onClick={() => set('artist', null)}>{artist || t('Unknown')} ✕</button>}
        <div className="grow" />
        {genres.length > 0 && (
          <select className="select" value={genre} aria-label={t('Genre')} onChange={(e) => set('genre', e.target.value || null)}>
            <option value="">{t('All genres')}</option>
            {genres.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
        )}
      </div>
      {!albums ? <SkeletonGrid square={!books} /> : albums.length === 0 ? (pods ? (
        <div className="pod-empty">
          <h2>{t('Your podcasts will live here')}</h2>
          <PodcastWays />
          {user?.isAdmin ? <PodcastFinder libraryId={lib.id} onAdded={onReload} /> : <p className="muted">{t('Ask whoever runs this server to follow some shows.')}</p>}
        </div>
      ) : <EmptyState icon={<IconAudio />} title={t('Nothing here')} text={genre ? t('Nothing in this genre.') : tx(emptyLibraryWords(user, t('Add audio files to this library’s folders and scan it.')))} />) : (
        <div className="album-grid">
          {(shown || []).map((a) => <AlbumTile key={a.artist + '|' + a.title} libId={lib.id} a={a} book={books} />)}
        </div>
      )}
    </>
  )
}

function ArtistsTab({ lib, sort, desc }: { lib: Library; sort: string; desc: boolean }) {
  const books = lib.type === 'audiobooks'
  const [artists, setArtists] = useState<Artist[] | null>(null)
  const [q, setQ] = useState('')
  useEffect(() => { setArtists(null); api.get<Artist[]>(`/music/artists?library=${lib.id}`).then(setArtists).catch(() => setArtists([])) }, [lib.id])
  const shown = (artists || []).filter((a) => !q || a.name.toLowerCase().includes(q.toLowerCase()))
  if (sort === 'albums') shown.sort((a, b) => a.albums - b.albums || a.name.localeCompare(b.name))
  if (desc) shown.reverse()
  return (
    <>
      <div className="mh-filters">
        {artists && <span className="count">{books ? tn('{count} author', '{count} authors', artists.length) : tn('{count} artist', '{count} artists', artists.length)}</span>}
        <div className="grow" />
        <input className="input mh-find" type="search" aria-label={books ? t('Find an author') : t('Find an artist')} placeholder={books ? t('Find an author…') : t('Find an artist…')} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {!artists ? <SkeletonGrid square /> : (
        <div className="album-grid mh-artist-grid">
          {shown.map((a) => <ArtistTile key={a.name} libId={lib.id} a={a} books={books} />)}
        </div>
      )}
    </>
  )
}

const VIEWS: [string, string][] = [['', tk('All songs')], ['liked', tk('Liked')], ['played', tk('Most played')], ['recent', tk('Played lately')], ['added', tk('Newest')]]

function SongsTab({ lib, params, set }: { lib: Library; params: URLSearchParams; set: (k: string, v: string | null) => void }) {
  const view = params.get('view') || ''
  const sort = params.get('sort') || 'title'
  const desc = params.get('desc') === '1'
  const genre = params.get('genre') || ''
  const [q, setQ] = useState('')
  const [search, setSearch] = useState('')
  const [songs, setSongs] = useState<Item[] | null>(null)
  const [genres, setGenres] = useState<string[]>([])
  const audio = useAudio()
  useEffect(() => { const timer = setTimeout(() => setSearch(q.trim()), 250); return () => clearTimeout(timer) }, [q])
  useEffect(() => {
    setSongs(null)
    const u = new URLSearchParams({ library: String(lib.id), limit: '2000' })
    if (view) u.set('view', view)
    if (!view || view === 'liked') {
      u.set('sort', sort)
      if (desc) u.set('desc', '1')
    }
    if (genre) u.set('genre', genre)
    if (search) u.set('search', search)
    api.get<Item[]>(`/music/songs?${u}`).then(setSongs).catch(() => setSongs([]))
  }, [lib.id, view, sort, desc, genre, search])
  useEffect(() => { api.get<string[]>(`/genres?library=${lib.id}`).then(setGenres).catch(() => setGenres([])) }, [lib.id])
  const total = (songs || []).reduce((s, song) => s + song.runtimeSec, 0)
  return (
    <>
      <div className="seg mh-views" role="tablist" aria-label={t('Which songs')}>
        {VIEWS.map(([k, l]) => <button key={k} role="tab" aria-selected={view === k} className={view === k ? 'active' : ''} onClick={() => set('view', k || null)}>{tx(l)}</button>)}
      </div>
      <div className="mh-filters">
        {songs && <span className="count">{tn('{count} song', '{count} songs', songs.length, { count: num(songs.length) })}{total > 0 ? ` · ${fmtRuntime(total)}` : ''}</span>}
        <button className="btn sm primary" disabled={!songs?.length} onClick={() => songs && audio.play(songs, 0, 'music')}><IconPlay size={16} /> {t('Play')}</button>
        <button className="btn sm" disabled={!songs?.length} onClick={() => songs && audio.play(shuffledCopy(songs), 0, 'music')}>{t('Shuffle')}</button>
        <div className="grow" />
        <input className="input mh-find" type="search" aria-label={t('Find a song')} placeholder={t('Find a song…')} value={q} onChange={(e) => setQ(e.target.value)} />
        {genres.length > 0 && (
          <select className="select" value={genre} aria-label={t('Genre')} onChange={(e) => set('genre', e.target.value || null)}>
            <option value="">{t('All genres')}</option>
            {genres.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
        )}
      </div>
      {!songs ? <Spinner /> : songs.length === 0
        ? <EmptyState icon={<IconAudio />} title={search || genre ? t('Nothing matches that') : view === 'liked' ? t('No liked songs yet') : view === 'played' || view === 'recent' ? t('Nothing played yet') : t('No songs here')} text={!search && !genre && view === 'liked' ? t('Press the heart on a song to keep it here.') : ''} />
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
      if (!items.length) { toast(t('Nothing there yet')); return }
      audio.play(shuffle ? shuffledCopy(items) : items, 0, 'music')
    } catch (e) { toast((e as Error).message) }
  }
  const built: { view: string; name: string; desc: string }[] = [
    { view: 'liked', name: t('Liked songs'), desc: t('Everything you’ve pressed the heart on') },
    { view: 'played', name: t('Most played'), desc: t('Your favourites, by how often you play them') },
    { view: 'recent', name: t('Played lately'), desc: t('What you’ve been listening to') },
    { view: 'added', name: t('Newest'), desc: t('What came to this library most recently') },
  ]
  return (
    <>
      <div className="mh-lists">
        {built.map((b) => (
          <div key={b.view} className="mh-list">
            <Link to={`?tab=songs&view=${b.view}`} className="mh-list-text"><b>{b.name}</b><small>{b.desc}</small></Link>
            <button className="mp-ibtn mh-list-play" onClick={() => void playView(b.view)} aria-label={t('Play {title}', { title: b.name })}><IconPlay /></button>
          </div>
        ))}
      </div>
      <div className="mh-shelf-head"><h2>{t('Your playlists')}</h2><Link className="mh-seeall" to="/collections">{t('Make one, or a smart one')}</Link></div>
      {!lists ? <Spinner /> : lists.length === 0 ? <p className="muted">{t('None yet. Add any song to a playlist from its menu, or make a smart one in Collections that keeps itself up to date.')}</p> : (
        <div className="mh-lists">
          {lists.filter((c) => !c.system).map((c) => (
            <Link key={c.id} to={`/collections/${c.id}`} className="mh-list mh-list-text">
              <span className="mh-list-art">{c.coverId > 0 && <img src={sized(coverUrl(c.coverId), fitWidth(96))} alt="" loading="lazy" />}</span>
              <span className="mh-list-words"><b>{c.name}</b><small>{c.kind === 'smart' ? t('Smart playlist') : tn('{count} item', '{count} items', c.count)}</small></span>
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
  const manyDiscs = discs && new Set(items.map((song) => song.discNo || 1)).size > 1
  return (
    <ol className="track-list">
      {items.map((song, i) => {
        const on = current?.id === song.id
        const pct = song.progress && song.runtimeSec ? Math.min(100, (song.progress.positionSec / song.runtimeSec) * 100) : 0
        const newDisc = manyDiscs && (i === 0 || (items[i - 1].discNo || 1) !== (song.discNo || 1))
        const hearted = liked(song.id)
        return (
          <li key={song.id} className={(on ? 'on' : '') + (newDisc ? ' new-disc' : '')} data-disc={newDisc ? t('Disc {number}', { number: song.discNo || 1 }) : undefined}>
            <button onClick={() => onPlay(i)} aria-label={t('Play {title}', { title: song.title })} aria-current={on ? 'true' : undefined}>
              <span className="tl-n">{on && playing ? <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i> : episodes ? '' : song.track ?? i + 1}</span>
              <span className="tl-t"><b>{song.title}</b>{showAlbum && <span className="muted">{[song.artist, song.album].filter(Boolean).join(' · ')}</span>}
                {episodes && <span className="muted">{[song.aired, song.overview].filter(Boolean).join(' · ').slice(0, 160)}</span>}
                {pct > 0 && <span className="tl-prog" style={{ width: pct + '%' }} />}</span>
              <span className="tl-d muted">{song.progress?.watched && <IconCheck />}{song.runtimeSec ? fmtTime(song.runtimeSec) : ''}</span>
            </button>
            {showHearts && <button type="button" className={'mp-ibtn tl-heart' + (hearted ? ' liked' : '')} aria-pressed={hearted} aria-label={hearted ? t('Remove {title} from liked songs', { title: song.title }) : t('Like {title}', { title: song.title })} onClick={() => toggleLike(song.id)}>{hearted ? <IconHeartOn size={18} /> : <IconHeart size={18} />}</button>}
            {showHearts && <TrackMenu t={song} />}
            {extra && <span className="tl-x">{extra(song)}</span>}
          </li>
        )
      })}
    </ol>
  )
}

// A name without what tends to mark a version of the same album: "(Deluxe)", "[2011 Remaster]".
const baseTitle = (s: string) => s.toLowerCase().replace(/[([{].*?[)\]}]/g, ' ').replace(/\b(deluxe|remaster(ed)?|expanded|special|anniversary|edition|version)\b/g, ' ').replace(/[^\p{L}\p{N}]+/gu, ' ').trim()

// AlbumPage: one album or audiobook.
const AlbumCanvas = lazy(() => import('../canvas/CanvasVideo').then((m) => ({ default: m.AlbumCanvas })))
const AlbumCanvasManager = lazy(() => import('../canvas/AlbumCanvasManager'))

export function AlbumPage() {
  const { user } = useApp()
  const [canvasOpen, setCanvasOpen] = useState(false)
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
  const discKey = [...new Set((tracks || []).map((song) => song.cdDisc).filter(Boolean))].join(',')
  useEffect(() => {
    let left = false
    setGoneDiscs([])
    if (discKey) discKey.split(',').forEach((d) => api.get(`/cd/${d}`).catch((e) => { if (!left && (e.status === 404 || e.status === 422)) setGoneDiscs((g) => [...g, Number(d)]) }))
    return () => { left = true }
  }, [discKey])
  const coverId = tracks?.find((song) => song.poster)?.id
  const rgb = useArtColour(coverId ? coverUrl(coverId) : null)
  if (!tracks) return <Spinner />
  const mode: AudioMode = lib?.type === 'audiobooks' ? 'audiobooks' : lib?.type === 'podcasts' ? 'podcasts' : 'music'
  const books = mode !== 'music'
  const pods = mode === 'podcasts'
  const total = tracks.reduce((s, song) => s + song.runtimeSec, 0)
  // Audiobooks continue at the first chapter that is not finished.
  const resumeAt = Math.max(0, tracks.findIndex((song) => !song.progress?.watched))
  const started = books && tracks.some((song) => song.progress && song.progress.positionSec > 5)
  // Tracks from CD images play from their disc, with no gaps between them.
  const discs = [...new Set(tracks.map((song) => song.cdDisc).filter((d): d is number => !!d))]
  const playTrack = (i: number) => {
    const song = tracks[i]
    if (song?.cdDisc) nav(`/cd/${song.cdDisc}?play=1&track=${song.track}`)
    else audio.play(tracks, i, mode)
  }
  const numericLib = Number(libId)
  const genre = tracks[0]?.genres?.[0]
  const versions = others.filter((a) => baseTitle(a.title) === baseTitle(album))
  const more = others.filter((a) => baseTitle(a.title) !== baseTitle(album))
  const allLiked = tracks.length > 0 && tracks.every((song) => audio.liked(song.id))
  return (
    <div className="album-page" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mh-bg" aria-hidden="true" />
      {!books && <Suspense fallback={null}><AlbumCanvas trackIds={tracks.filter((song) => !song.cdDisc).map((song) => song.id)} /></Suspense>}
      <BackLink to={`/library/${libId}`} label={lib?.name || (books ? t('Audiobooks') : t('Music'))} />
      <div className="album-head">
        <div className={'album-cover big' + (books ? ' book' : '')}>{coverId ? <img src={coverUrl(coverId)} alt="" /> : <Placeholder title={tracks[0]?.album || t('Unknown')} />}</div>
        <div className="album-meta">
          <span className="muted">{pods ? t('Podcast') : books ? t('Audiobook') : t('Album')}</span>
          <h1>{album || t('Unknown album')}</h1>
          {pods ? <span className="album-artist">{artist}</span> : <Link to={artistLink(numericLib, artist)} className="album-artist">{artist || t('Unknown')}</Link>}
          <div className="muted">{[pods ? tn('{count} episode', '{count} episodes', tracks.length) : books ? tn('{count} chapter', '{count} chapters', tracks.length) : tn('{count} track', '{count} tracks', tracks.length), pods ? '' : fmtRuntime(total), pods ? '' : tracks[0]?.year, pods ? '' : genre].filter(Boolean).join(' · ')}</div>
          <div className="actions">
            <button data-tv-first className="btn primary" onClick={() => playTrack(books && !pods ? resumeAt : 0)}><IconPlay /> {pods ? t('Play newest') : started ? t('Continue') : t('Play')}</button>
            {discs.filter((d) => !goneDiscs.includes(d)).map((d) => <Link key={d} className="btn" to={`/cd/${d}?play=1`}><IconDisc /> {discs.length > 1 ? t('Disc {number}', { number: discs.indexOf(d) + 1 }) : t('Insert disc')}</Link>)}
            {!books && <button className="btn" onClick={() => audio.play(shuffledCopy(tracks), 0, mode)}>{t('Shuffle')}</button>}
            {!books && tracks[0] && <button className="btn ghost" onClick={() => audio.startRadio(tracks[0])}><IconStation size={18} /> {t('Station')}</button>}
            <button className="btn ghost" onClick={() => { audio.playNext(tracks); toast(t('Plays next')) }}>{t('Play next')}</button>
            <button className="btn ghost" onClick={() => { audio.enqueue(tracks); toast(t('Added to the queue')) }}>{t('Add to queue')}</button>
            {tracks[0] && <PlayOnButton className="btn ghost" size={18} source={{ kind: 'item', itemId: tracks[0].id, itemKind: 'audio', title: album || tracks[0].title, ...queueOf(tracks.map((song) => song.id), 0) }} />}
            {!books && <button className="btn ghost" aria-pressed={allLiked} onClick={() => tracks.forEach((song) => { if (audio.liked(song.id) === allLiked) audio.toggleLike(song.id) })}>{allLiked ? <IconHeartOn size={18} /> : <IconHeart size={18} />} {allLiked ? t('Liked') : t('Like all')}</button>}
            {!books && user?.isAdmin && tracks.some((song) => !song.cdDisc) && <button className="btn ghost" aria-haspopup="dialog" onClick={() => setCanvasOpen(true)} title={t('The looping video behind these songs')}>{t('Canvas')}</button>}
          </div>
          {tracks[0]?.overview && !pods && <p className="album-desc">{tracks[0].overview}</p>}
          {pods && libId && <PodcastAdmin libraryId={numericLib} title={album} onChanged={() => setReloadTracks((n) => n + 1)} />}
        </div>
      </div>
      <TrackList items={tracks} onPlay={playTrack} episodes={pods} discs={!books} heart={!books}
        extra={pods ? (song) => <DownloadButton item={song} onDone={(x) => setTracks((ts) => ts!.map((y) => (y.id === x.id ? { ...y, path: x.path } : y)))} /> : undefined} />
      {versions.length > 0 && <Shelf title={t('Other versions')}><div className="mh-row mh-albums">{versions.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} noArtist />)}</div></Shelf>}
      {more.length > 0 && !books && <Shelf title={t('More by {artist}', { artist })} to={artistLink(numericLib, artist)}><div className="mh-row mh-albums">{more.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} noArtist />)}</div></Shelf>}
      <LegalNote kind={pods ? 'webMedia' : 'metadata'} />
      {canvasOpen && <Suspense fallback={null}><AlbumCanvasManager songs={tracks.filter((song) => !song.cdDisc)} onClose={() => setCanvasOpen(false)} /></Suspense>}
    </div>
  )
}

