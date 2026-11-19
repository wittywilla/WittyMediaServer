// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { fmtRuntime, type Album, type Item, type Library } from '../../api'
import { IconHeartOn, IconShuffle } from '../../audio/icons'
import { useAudio } from '../../audio/player'
import { EmptyState } from '../../components/common'
import { IconAudio, IconSearch } from '../../icons'
import { ActionBar, ListHeader } from './Header'
import { musicTab } from './musicRoute'
import { coverUrl, shuffledCopy } from './parts'
import { useMusic, useMusicPages } from './store'
import { AlbumCard, PageShape, PlayButton, Shelf, TrackTable } from './ui'
import { num, t, tk, tn, tx } from '../../i18n'

// A list of songs as a playlist page: Liked songs and the other lists the library makes
// itself, all songs, a playlist of yours, or a genre. A header with a cover or a coloured
// tile, the play button, and the songs (only the rows near the screen are drawn).

const VIEWS: Record<string, { name: string; sub: string; hue: number }> = {
  '': { name: tk('All songs'), sub: tk('Every song in this library'), hue: 200 },
  liked: { name: tk('Liked songs'), sub: tk('Everything you’ve pressed the heart on'), hue: 255 },
  played: { name: tk('Most played'), sub: tk('Your favourites, by how often you play them'), hue: 150 },
  recent: { name: tk('Played lately'), sub: tk('What you’ve been listening to'), hue: 25 },
  added: { name: tk('Newest'), sub: tk('What came to this library most recently'), hue: 320 },
}
const SORTS: [string, string][] = [['title', tk('Title')], ['artist', tk('Artist')], ['album', tk('Album')], ['year', tk('Year')], ['added', tk('Date added')], ['runtime', tk('Length')]]

type Collection = { collection: { id: number; name: string; kind: string; description?: string }; items: Item[] }

export default function MusicList({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const audio = useAudio()
  const tab = params.get('tab')
  const view = params.get('view') || ''
  const genre = params.get('genre') || ''
  const listId = params.get('id') || ''
  const sort = params.get('sort') || ''
  // Finding within the list, typed here and kept out of the address.
  const [find, setFind] = useState('')
  const [search, setSearch] = useState('')
  useEffect(() => { const wait = setTimeout(() => setSearch(find.trim()), 250); return () => clearTimeout(wait) }, [find])
  let url: string | null = null
  if (tab === 'songs') {
    const u = new URLSearchParams({ library: String(lib.id) })
    if (view) u.set('view', view)
    if (sort && (!view || view === 'liked')) u.set('sort', sort)
    if (search) u.set('search', search)
    url = `/music/songs?${u}`
  } else if (tab === 'genre') url = `/music/songs?library=${lib.id}&genre=${encodeURIComponent(genre)}${search ? '&search=' + encodeURIComponent(search) : ''}`
  const songs = useMusicPages<Item>(url, 2000)
  const coll = useMusic<Collection>(tab === 'playlist' && listId ? `/collections/${listId}?limit=1000` : null)
  const genreAlbums = useMusic<Album[]>(tab === 'genre' && genre ? `/music/albums?library=${lib.id}&genre=${encodeURIComponent(genre)}&sort=played` : null).data
  const found = tab === 'playlist' ? coll.data?.items.filter((i) => i.kind === 'audio' && (!search || i.title.toLowerCase().includes(search.toLowerCase()))) : songs.data
  const failed = tab === 'playlist' ? coll.failed : songs.failed
  const facts = tab === 'playlist'
    ? { name: coll.data?.collection.name || t('Playlist'), sub: coll.data?.collection.kind === 'smart' ? t('Smart playlist') : t('Playlist'), hue: (Number(listId) * 67) % 360, kind: t('Playlist') }
    : tab === 'genre' ? { name: genre || t('Genre'), sub: t('Genre'), hue: 30, kind: t('Genre') }
      : (() => { const v = VIEWS[view] || VIEWS['']; return { name: tx(v.name), sub: tx(v.sub), hue: v.hue, kind: t('Playlist') } })()
  if (failed) return <EmptyState icon={<IconAudio />} title={t('That list wouldn’t load')} text={t('It may have been removed, or the server didn’t answer.')} />
  const items = found || []
  const total = items.reduce((s, t) => s + t.runtimeSec, 0)
  const cover = tab === 'playlist' ? items.find((t) => t.poster)?.id : undefined
  const mine = !!audio.current && items.some((t) => t.id === audio.current!.id)
  const sortable = tab === 'songs' && (!view || view === 'liked')
  return (
    <div className="mus-page mus-list">
      <ListHeader art={cover ? coverUrl(cover) : ''} title={facts.name} kind={facts.kind}
        tile={<span className="mus-tile big" style={{ '--h': facts.hue } as React.CSSProperties} aria-hidden="true">{view === 'liked' ? <IconHeartOn /> : <b>{facts.name[0]}</b>}</span>}>
        <p className="mus-byline">{facts.sub}{found ? ' · ' + tn('{count} song', '{count} songs', items.length, { count: num(items.length) + (songs.more ? '+' : '') }) + (total ? ' · ' + fmtRuntime(total) : '') : ''}</p>
      </ListHeader>
      <ActionBar title={facts.name}>
        <PlayButton big tvFirst label={facts.name} playing={mine && audio.playing} onPlay={() => (mine ? audio.toggle() : items.length ? audio.play(items, 0, 'music') : undefined)} />
        <button type="button" className="mus-icon" aria-label={t('Shuffle {title}', { title: facts.name })} title={t('Shuffle')} disabled={!items.length} onClick={() => audio.play(shuffledCopy(items), 0, 'music')}><IconShuffle /></button>
        <span className="mus-grow" />
        <label className="mus-find"><IconSearch size={18} /><input type="search" value={find} onChange={(e) => setFind(e.target.value)} placeholder={t('Find in this list')} aria-label={t('Find in this list')} /></label>
        {sortable && (
          <select className="select sm" aria-label={t('Sort songs')} value={sort || 'title'} onChange={(e) => { const p = new URLSearchParams(params); p.set('sort', e.target.value); setParams(p, { replace: true }) }}>
            {SORTS.map(([k, l]) => <option key={k} value={k}>{tx(l)}</option>)}
          </select>
        )}
      </ActionBar>
      {!found ? <PageShape kind="list" /> : items.length === 0 ? (
        <p className="mus-none">{search ? t('Nothing in this list matches that.') : view === 'liked' ? t('Songs you like will be here. Press the heart on any song.') : view === 'played' || view === 'recent' ? t('Nothing played yet.') : t('No songs here yet.')}</p>
      ) : <TrackTable items={items} label={facts.name} showArt showAlbum numbers="index" onPlay={(i) => audio.play(items, i, 'music')} />}
      {tab === 'genre' && genreAlbums && genreAlbums.length > 0 && <Shelf title={t('{genre} albums', { genre })}>{genreAlbums.slice(0, 30).map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} />)}</Shelf>}
      {tab === 'playlist' && <p className="mus-more-link"><Link to={`/collections/${listId}`}>{t('Edit this playlist')}</Link></p>}
      {tab === 'songs' && !view && <p className="mus-more-link"><Link to={musicTab(lib.id, 'library')}>{t('Back to your library')}</Link></p>}
    </div>
  )
}
