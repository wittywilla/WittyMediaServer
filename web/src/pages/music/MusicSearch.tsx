// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { type Album, type Artist, type Item, type Library } from '../../api'
import { useAudio } from '../../audio/player'
import { IconSearch } from '../../icons'
import { useTvMode } from '../../tvmode/mode'
import { musicTab } from './musicRoute'
import { useMusic } from './store'
import { AlbumCard, ArtistCard, PageShape, Shelf, TrackTable } from './ui'

// Search inside Music: what you type stays in the address, so Back comes back to the same
// answers. Nothing typed, it's the genres to browse.

export default function MusicSearch({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const tv = useTvMode()
  const [q, setQ] = useState(params.get('q') || '')
  const term = (params.get('q') || '').trim()
  // The address follows the typing a moment later, replacing itself so Back isn't a letter at a time.
  useEffect(() => {
    const t = setTimeout(() => {
      if (q.trim() === term) return
      const p = new URLSearchParams(params)
      if (q.trim()) p.set('q', q.trim()); else p.delete('q')
      setParams(p, { replace: true })
    }, 250)
    return () => clearTimeout(t)
  }, [q]) // eslint-disable-line react-hooks/exhaustive-deps
  const enc = encodeURIComponent(term)
  const songs = useMusic<Item[]>(term ? `/music/songs?library=${lib.id}&search=${enc}&limit=50` : null)
  const albums = useMusic<Album[]>(term ? `/music/albums?library=${lib.id}&search=${enc}` : null)
  const artists = useMusic<Artist[]>(`/music/artists?library=${lib.id}`).data
  const genres = useMusic<string[]>(`/genres?library=${lib.id}`).data
  const audio = useAudio()
  const low = term.toLowerCase()
  const foundArtists = term ? (artists || []).filter((a) => a.name.toLowerCase().includes(low)).sort((a, b) => Number(!a.name.toLowerCase().startsWith(low)) - Number(!b.name.toLowerCase().startsWith(low)) || b.tracks - a.tracks).slice(0, 20) : []
  const nothing = term && songs.data && albums.data && artists && !songs.data.length && !albums.data.length && !foundArtists.length
  return (
    <div className="mus-page mus-search">
      <form className="mus-search-box" role="search" onSubmit={(e) => { e.preventDefault(); (document.activeElement as HTMLElement)?.blur?.() }}>
        <IconSearch />
        <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="What do you want to listen to?" aria-label="Search music" autoFocus={!tv} data-tv-first />
      </form>
      {!term ? (
        <section aria-labelledby="mus-browse">
          <h2 id="mus-browse" className="mus-h2">Browse all</h2>
          {!genres ? <PageShape kind="grid" /> : genres.length === 0 ? <p className="mus-muted">Your music has no genres in its tags yet.</p> : (
            <div className="mus-browse">
              {genres.map((g, i) => <Link key={g} className="mus-genre big" style={{ '--h': (i * 47) % 360 } as React.CSSProperties} to={musicTab(lib.id, 'genre', '&genre=' + encodeURIComponent(g))}>{g}</Link>)}
            </div>
          )}
        </section>
      ) : nothing ? (
        <p className="mus-none" role="status">Nothing found for “{term}”. Check the spelling, or try fewer words.</p>
      ) : (
        <>
          {!songs.data ? <PageShape kind="list" /> : songs.data.length > 0 && (
            <section aria-labelledby="mus-found-songs">
              <h2 id="mus-found-songs" className="mus-h2">Songs</h2>
              <TrackTable items={songs.data.slice(0, 50)} label="Songs found" showArt showAlbum numbers="index" onPlay={(i) => audio.play(songs.data!, i, 'music')} />
            </section>
          )}
          {foundArtists.length > 0 && <Shelf title="Artists">{foundArtists.map((a) => <ArtistCard key={a.name} libId={lib.id} a={a} />)}</Shelf>}
          {albums.data && albums.data.length > 0 && <Shelf title="Albums">{albums.data.slice(0, 30).map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} />)}</Shelf>}
          <p className="mus-sr" role="status">{songs.data && albums.data ? `${songs.data.length} songs, ${foundArtists.length} artists and ${albums.data.length} albums found` : ''}</p>
        </>
      )}
    </div>
  )
}
