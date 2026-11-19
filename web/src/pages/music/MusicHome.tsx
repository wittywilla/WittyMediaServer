// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { api, fitWidth, sized, type Album, type Artist, type Item, type Library } from '../../api'
import { useAudio } from '../../audio/player'
import { EmptyState } from '../../components/common'
import { IconAudio, IconPlay } from '../../icons'
import { emptyLibraryWords } from '../../profileWords'
import { toast, useApp } from '../../state'
import { musicTab } from './musicRoute'
import { albumLink, coverUrl, shuffledCopy } from './parts'
import { useMusic } from './store'
import { AlbumCard, Art, ArtistCard, ListCard, PageShape, PlayButton, Shelf, tracksUrl } from './ui'
import { hasMusicVideos } from './MusicVideos'
import { num, t, tk, tx } from '../../i18n'

// Music's front page: a greeting with the things played lately as quick tiles, then shelves
// that scroll sideways. Everything comes in one request (GET /music/home).

type Home = { hero: Album[]; albums: Album[]; artists: Artist[]; songs: Item[]; recent: Item[]; continue: Item[]; genres: string[]; counts: { albums: number; artists: number; songs: number; liked: number } }

const greeting = () => { const h = new Date().getHours(); return h < 5 ? t('Good night') : h < 12 ? t('Good morning') : h < 18 ? t('Good afternoon') : t('Good evening') }

// The albums of songs played lately, newest first, each once.
function recentAlbums(recent: Item[]): (Album & { from: Item })[] {
  const seen = new Set<string>()
  const out: (Album & { from: Item })[] = []
  for (const t of recent) {
    const artist = t.albumArtist || t.artist || ''
    const key = artist + '|' + (t.album || '')
    if (!t.album || seen.has(key)) continue
    seen.add(key)
    out.push({ artist, title: t.album, year: t.year ?? undefined, tracks: 0, durationSec: 0, coverItem: t.poster ? t.id : 0, addedAt: 0, from: t })
  }
  return out
}

const VIEWS: { view: string; name: string; sub: string; hue: number }[] = [
  { view: 'liked', name: tk('Liked songs'), sub: tk('Everything with a heart'), hue: 255 },
  { view: 'played', name: tk('Most played'), sub: tk('Your favourites, by plays'), hue: 150 },
  { view: 'recent', name: tk('Played lately'), sub: tk('What you’ve had on'), hue: 25 },
  { view: 'added', name: tk('Newest'), sub: tk('Just came in'), hue: 320 },
]

export default function MusicHome({ lib }: { lib: Library }) {
  const { user, libraries } = useApp()
  const audio = useAudio()
  const { data: home, failed } = useMusic<Home>(`/music/home?library=${lib.id}`)
  if (failed) return <EmptyState icon={<IconAudio />} title={t('Music wouldn’t load')} text={t('The server didn’t answer. Try again in a moment.')} />
  if (!home) return <PageShape kind="shelves" />
  if (!home.hero.length && !home.albums.length) return <div className="mus-page"><EmptyState icon={<IconAudio />} title={t('Nothing here yet')} text={emptyLibraryWords(user, t('Add music to this library’s folders and scan it.'))} /></div>
  const playView = async (view: string, shuffle?: boolean) => {
    const items = await api.get<Item[]>(`/music/songs?library=${lib.id}&view=${view}&limit=500`)
    if (!items.length) { toast(t('Nothing there yet')); return }
    audio.play(shuffle ? shuffledCopy(items) : items, 0, 'music')
  }
  const playAlbum = async (a: { artist: string; title: string }) => {
    const tracks = await api.get<Item[]>(tracksUrl(lib.id, a))
    if (tracks.length) audio.play(tracks, 0, 'music')
  }
  const lately = recentAlbums(home.recent)
  // An album played lately is often among the newest too: it gets one tile, not two.
  const albumKey = (a: { artist: string; title: string }) => a.artist + '|' + a.title
  const shown = new Set(lately.map(albumKey))
  const filler = home.hero.filter((a) => !shown.has(albumKey(a)))
  // Quick tiles: Liked songs, then what was played lately, then the newest albums to fill six.
  const quick: { key: string; to: string; name: string; art: string; play: () => Promise<void>; liked?: boolean }[] = [
    { key: 'liked', to: musicTab(lib.id, 'songs', '&view=liked'), name: t('Liked songs'), art: '', play: () => playView('liked'), liked: true },
    ...[...lately, ...filler].slice(0, 5).map((a) => ({ key: albumKey(a), to: albumLink(lib.id, a), name: a.title || t('Unknown album'), art: a.coverItem ? sized(coverUrl(a.coverItem), fitWidth(64)) : '', play: () => playAlbum(a) })),
  ]
  return (
    <div className="mus-page mus-home">
      <div className="mus-home-wash" aria-hidden="true" />
      <div className="mus-home-top">
        <h1 className="mus-h1">{greeting()}</h1>
        <button type="button" className="mus-pill" data-tv-first onClick={() => void playView('added', true).catch((e) => toast(e.message))}><IconPlay size={18} /> {t('Shuffle my music')}</button>
      </div>
      <div className="mus-quick" role="list" aria-label={t('Jump back in')}>
        {quick.map((q) => (
          <div key={q.key} className="mus-quick-tile" role="listitem">
            <Link to={q.to}>
              <span className={'mus-quick-art' + (q.liked ? ' liked' : '')}><Art src={q.art} fallback={<span aria-hidden="true">{q.liked ? '♥' : q.name[0]}</span>} /></span>
              <b>{q.name}</b>
            </Link>
            <PlayButton label={q.name} onPlay={() => q.play().catch((e) => toast(e.message))} />
          </div>
        ))}
      </div>
      {lately.length > 0 && (
        <Shelf title={t('Recently played')} to={musicTab(lib.id, 'songs', '&view=recent')}>
          {lately.slice(0, 20).map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} sub={a.artist} />)}
        </Shelf>
      )}
      <Shelf title={t('Made for you')}>
        {VIEWS.map((v) => <ListCard key={v.view} to={musicTab(lib.id, 'songs', '&view=' + v.view)} name={tx(v.name)} sub={tx(v.sub)} hue={v.hue} onPlay={() => playView(v.view).catch((e) => toast(e.message))} />)}
      </Shelf>
      {home.hero.length > 0 && (
        <Shelf title={t('New in your library')} to={musicTab(lib.id, 'albums', '&sort=added')}>
          {home.hero.map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} sub={a.artist} />)}
        </Shelf>
      )}
      {home.artists.length > 0 && (
        <Shelf title={t('Your artists')} to={musicTab(lib.id, 'artists')}>
          {home.artists.map((a) => <ArtistCard key={a.name} libId={lib.id} a={a} />)}
        </Shelf>
      )}
      {home.albums.length > 0 && (
        <Shelf title={t('Albums')} sub={num(home.counts.albums)} to={musicTab(lib.id, 'albums')}>
          {home.albums.map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} />)}
        </Shelf>
      )}
      {home.genres.length > 0 && (
        <section className="mus-shelf" aria-labelledby="mus-genres">
          <div className="mus-shelf-head"><h2 id="mus-genres">{t('Genres')}</h2></div>
          <div className="mus-genres">
            {home.genres.slice(0, 18).map((g, i) => <Link key={g} className="mus-genre" style={{ '--h': (i * 47) % 360 } as React.CSSProperties} to={musicTab(lib.id, 'genre', '&genre=' + encodeURIComponent(g))}>{g}</Link>)}
          </div>
        </section>
      )}
      {lib.type === 'music' && hasMusicVideos(libraries) && <p className="mus-more-link"><Link to={musicTab(lib.id, 'videos')}>{t('Music videos')}</Link></p>}
    </div>
  )
}
