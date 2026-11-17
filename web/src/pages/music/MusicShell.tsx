// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useMemo, useState } from 'react'
import { Link, NavLink, useLocation, useSearchParams } from 'react-router-dom'
import { fitWidth, sized, type Album, type Artist, type Library } from '../../api'
import { IconHeartOn } from '../../audio/icons'
import { IconHome, IconLibrary, IconSearch } from '../../icons'
import { useTvMode } from '../../tvmode/mode'
import { albumLink, artistLink, artistPhoto, coverUrl } from './parts'
import { useKeepScroll, useMusic } from './store'
import { VirtualRows } from './Virtual'
import { PageShape } from './ui'
import MusicHome from './MusicHome'
import MusicSearch from './MusicSearch'
import MusicLibraryView from './MusicLibraryView'
import MusicAlbum from './MusicAlbum'
import MusicArtist from './MusicArtist'
import MusicList from './MusicList'
import { musicHome, musicTab } from './musicRoute'
import './mus.css'

// Music, laid out as the big streaming apps are: a library panel down the left on wide
// screens (Home, Search, and your playlists, artists and albums with chips to narrow them),
// three tabs along the bottom on phones, and the page beside or above them. The bar playing
// along the bottom is the app's own (audio/MiniPlayer.tsx), so it carries on everywhere.
//
// One look for every layout for now. The pages are the app's own; a layout that draws Music
// its own way (server-driven) can take these addresses back in musicRoute.ts.

// The pages come with the shell, so moving between them never waits for code.
const MusicVideosPage = lazy(() => import('./MusicVideos').then((m) => ({ default: ({ lib }: { lib: Library }) => <div className="mus-page"><h1 className="mus-h1">Music videos</h1><m.MusicVideosTab lib={lib} /></div> })))


export default function MusicShell({ lib }: { lib: Library }) {
  const loc = useLocation()
  const [params] = useSearchParams()
  const tv = useTvMode()
  useKeepScroll()
  const where = loc.pathname.split('/')[1]
  const tab = params.get('tab') || ''
  let page
  if (where === 'album') page = <MusicAlbum lib={lib} />
  else if (where === 'artist') page = <MusicArtist lib={lib} />
  else if (tab === 'search') page = <MusicSearch lib={lib} />
  else if (tab === 'library' || tab === 'albums' || tab === 'artists' || tab === 'playlists') page = <MusicLibraryView lib={lib} />
  else if (tab === 'songs' || tab === 'playlist' || tab === 'genre') page = <MusicList lib={lib} />
  else if (tab === 'videos') page = <MusicVideosPage lib={lib} />
  else page = <MusicHome lib={lib} />
  const shape = where === 'album' || where === 'artist' || tab === 'songs' || tab === 'playlist' ? 'list' : tab ? 'grid' : 'shelves'
  return (
    <div className={'mus' + (tv ? ' tv' : '')}>
      <Side lib={lib} />
      <div className="mus-main" key={where + tab}>
        <Suspense fallback={<PageShape kind={shape} />}>{page}</Suspense>
      </div>
      <PhoneTabs lib={lib} />
    </div>
  )
}

// Which of Home, Search and Your library an address is in.
function section(pathname: string, tab: string) {
  if (pathname.startsWith('/library/') && !tab) return 'home'
  if (tab === 'search' || tab === 'genre') return 'search'
  return 'library'
}

function PhoneTabs({ lib }: { lib: Library }) {
  const loc = useLocation()
  const [params] = useSearchParams()
  const at = section(loc.pathname, params.get('tab') || '')
  const tabs: [string, string, string, React.ReactNode][] = [
    ['home', 'Home', musicHome(lib.id), <IconHome key="h" />],
    ['search', 'Search', musicTab(lib.id, 'search'), <IconSearch key="s" />],
    ['library', 'Your library', musicTab(lib.id, 'library'), <IconLibrary key="l" />],
  ]
  return (
    <nav className="mus-tabs" aria-label="Music">
      {tabs.map(([id, label, to, icon]) => <Link key={id} to={to} aria-current={at === id ? 'page' : undefined}>{icon}<span>{label}</span></Link>)}
    </nav>
  )
}

type Kind = 'playlists' | 'artists' | 'albums'
type Row = { key: string; to: string; name: string; sub: string; art: string; round?: boolean; tile?: string }
type Collection = { id: number; name: string; kind: string; count: number; coverId: number; system?: string }

// The panel down the left: where to go, then everything in the library, narrowed by chips.
function Side({ lib }: { lib: Library }) {
  const loc = useLocation()
  const [params] = useSearchParams()
  const at = section(loc.pathname, params.get('tab') || '')
  const [kind, setKind] = useState<Kind | ''>('')
  const [find, setFind] = useState('')
  const lists = useMusic<Collection[]>('/collections').data
  const artists = useMusic<Artist[]>(kind === 'artists' || kind === '' ? `/music/artists?library=${lib.id}` : null).data
  const albums = useMusic<Album[]>(kind === 'albums' || kind === '' ? `/music/albums?library=${lib.id}&sort=title` : null).data
  const rows = useMemo<Row[] | null>(() => {
    const out: Row[] = []
    if (kind === '' || kind === 'playlists') {
      out.push({ key: 'liked', to: musicTab(lib.id, 'songs', '&view=liked'), name: 'Liked songs', sub: 'Playlist', art: '', tile: 'liked' })
      for (const c of (lists || []).filter((c) => !c.system)) out.push({ key: 'c' + c.id, to: musicTab(lib.id, 'playlist', '&id=' + c.id), name: c.name, sub: c.kind === 'smart' ? 'Smart playlist' : `Playlist · ${c.count}`, art: c.coverId ? sized(coverUrl(c.coverId), fitWidth(48)) : '' })
    }
    if (kind === '' || kind === 'artists') for (const a of artists || []) out.push({ key: 'r' + a.name, to: artistLink(lib.id, a.name), name: a.name || 'Unknown artist', sub: 'Artist', art: artistPhoto(lib.id, a.name), round: true })
    if (kind === '' || kind === 'albums') for (const a of albums || []) out.push({ key: 'a' + a.artist + '|' + a.title, to: albumLink(lib.id, a), name: a.title || 'Unknown album', sub: `Album · ${a.artist || 'Unknown'}`, art: a.coverItem ? sized(coverUrl(a.coverItem), fitWidth(48)) : '' })
    const q = find.trim().toLowerCase()
    return q ? out.filter((r) => r.name.toLowerCase().includes(q) || r.sub.toLowerCase().includes(q)) : out
  }, [kind, lists, artists, albums, find, lib.id])
  const here = loc.pathname + loc.search
  const chips: [Kind, string][] = [['playlists', 'Playlists'], ['artists', 'Artists'], ['albums', 'Albums']]
  return (
    <aside className="mus-side" aria-label="Music">
      <nav className="mus-side-nav">
        <NavLink to={musicHome(lib.id)} end aria-current={at === 'home' ? 'page' : undefined} className={at === 'home' ? 'active' : ''}><IconHome /> Home</NavLink>
        <NavLink to={musicTab(lib.id, 'search')} aria-current={at === 'search' ? 'page' : undefined} className={at === 'search' ? 'active' : ''}><IconSearch /> Search</NavLink>
      </nav>
      <section className="mus-side-lib" aria-labelledby="mus-yours">
        <div className="mus-side-head">
          <Link id="mus-yours" to={musicTab(lib.id, 'library')} className={at === 'library' ? 'active' : ''}><IconLibrary /> Your library</Link>
        </div>
        <div className="mus-chips" role="group" aria-label="Show only">
          {kind && <button type="button" className="mus-chip clear" aria-label="Show everything" onClick={() => setKind('')}>✕</button>}
          {chips.filter(([k]) => !kind || k === kind).map(([k, l]) => <button key={k} type="button" className={'mus-chip' + (kind === k ? ' on' : '')} aria-pressed={kind === k} onClick={() => setKind(kind === k ? '' : k)}>{l}</button>)}
        </div>
        <input className="mus-side-find" type="search" placeholder="Find in your library" aria-label="Find in your library" value={find} onChange={(e) => setFind(e.target.value)} />
        <div className="mus-side-list">
          {!rows || (!artists && !albums && kind !== 'playlists') ? <PageShape kind="list" /> : rows.length === 0 ? <p className="mus-muted mus-side-none">Nothing by that name.</p> : (
            <VirtualRows items={rows} rowHeight={56} inBox label={kind ? `Your ${kind}` : 'Your library'} render={(r) => (
              <Link key={r.key} to={r.to} className={'mus-side-row' + (here === r.to ? ' on' : '')} aria-current={here === r.to ? 'page' : undefined} role="listitem">
                <span className={'mus-side-art' + (r.round ? ' round' : '') + (r.tile ? ' ' + r.tile : '')}>{r.tile === 'liked' ? <IconHeartOn size={18} /> : r.art ? <img src={r.art} alt="" loading="lazy" decoding="async" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} /> : <span aria-hidden="true">{r.name[0]}</span>}</span>
                <span className="mus-side-words"><b>{r.name}</b><small>{r.sub}</small></span>
              </Link>
            )} />
          )}
        </div>
      </section>
    </aside>
  )
}
