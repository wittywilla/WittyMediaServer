// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Album, type Artist, type Item, type Library } from '../../api'
import { useAudio } from '../../audio/player'
import { ScanButton } from '../../components/common'
import { toast } from '../../state'
import { musicTab } from './musicRoute'
import { shuffledCopy } from './parts'
import { useMusic } from './store'
import { AlbumCard, ArtistCard, ListCard, PageShape } from './ui'
import { VirtualRows } from './Virtual'
import { num, t, tk, tn, tx } from '../../i18n'

// Your library: playlists, artists or albums as a grid, picked with chips and sorted. The
// grid only draws the rows near the screen, so a library of thousands scrolls as lightly as
// one of ten. The chip and the order are in the address, so Back returns to them.

type Kind = 'playlists' | 'artists' | 'albums'
type Collection = { id: number; name: string; kind: string; count: number; coverId: number; system?: string }

const SORTS: Record<Kind, [string, string][]> = {
  albums: [['title', tk('Title')], ['artist', tk('Artist')], ['year', tk('Newest')], ['added', tk('Recently added')], ['played', tk('Most played')]],
  artists: [['name', tk('Name')], ['albums', tk('Most albums')]],
  playlists: [['name', tk('Name')]],
}

export default function MusicLibraryView({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const audio = useAudio()
  const old = params.get('tab')
  const kind: Kind = (params.get('show') as Kind) || (old === 'artists' || old === 'albums' || old === 'playlists' ? old : 'playlists')
  const sort = params.get('sort') || SORTS[kind][0][0]
  // A new chip starts in its own first order.
  const choose = (k: 'show' | 'sort', v: string) => setParams(new URLSearchParams(k === 'show' ? { tab: 'library', show: v } : { tab: 'library', show: kind, sort: v }), { replace: true })
  const lists = useMusic<Collection[]>(kind === 'playlists' ? '/collections' : null).data
  const artists = useMusic<Artist[]>(kind === 'artists' ? `/music/artists?library=${lib.id}` : null).data
  const albumSort = ['title', 'year', 'added', 'played'].includes(sort) ? sort : 'title'
  const albums = useMusic<Album[]>(kind === 'albums' ? `/music/albums?library=${lib.id}&sort=${albumSort}` : null).data
  const cards = useMemo<ReactNode[] | null>(() => {
    if (kind === 'albums') {
      if (!albums) return null
      const list = sort === 'artist' ? [...albums].sort((a, b) => a.artist.localeCompare(b.artist) || (a.year || 0) - (b.year || 0)) : albums
      return list.map((a) => <AlbumCard key={a.artist + '|' + a.title} libId={lib.id} a={a} />)
    }
    if (kind === 'artists') {
      if (!artists) return null
      const list = sort === 'albums' ? [...artists].sort((a, b) => b.albums - a.albums || a.name.localeCompare(b.name)) : artists
      return list.map((a) => <ArtistCard key={a.name} libId={lib.id} a={a} />)
    }
    if (!lists) return null
    const playView = async (view: string) => {
      const items = await api.get<Item[]>(`/music/songs?library=${lib.id}&view=${view}&limit=500`)
      if (!items.length) { toast(t('Nothing there yet')); return }
      audio.play(items, 0, 'music')
    }
    const playList = async (id: number) => {
      const c = await api.get<{ items: Item[] }>(`/collections/${id}?limit=500`)
      const songs = c.items.filter((i) => i.kind === 'audio')
      if (!songs.length) { toast(t('No songs in that one')); return }
      audio.play(songs, 0, 'music')
    }
    return [
      <ListCard key="liked" to={musicTab(lib.id, 'songs', '&view=liked')} name={t('Liked songs')} sub={t('Playlist')} hue={255} onPlay={() => playView('liked').catch((e) => toast(e.message))} />,
      <ListCard key="all" to={musicTab(lib.id, 'songs')} name={t('All songs')} sub={t('Every song here')} hue={200} onPlay={async () => { const s = await api.get<Item[]>(`/music/songs?library=${lib.id}&sort=random&limit=500`); if (s.length) audio.play(shuffledCopy(s), 0, 'music') }} />,
      ...lists.filter((c) => !c.system).sort((a, b) => a.name.localeCompare(b.name)).map((c) => (
        <ListCard key={c.id} to={musicTab(lib.id, 'playlist', '&id=' + c.id)} name={c.name} sub={c.kind === 'smart' ? t('Smart playlist') : tn('{count} item', '{count} items', c.count)} coverId={c.coverId} hue={(c.id * 67) % 360} onPlay={() => playList(c.id).catch((e) => toast(e.message))} />
      )),
    ]
  }, [kind, sort, albums, artists, lists, lib.id, audio])
  const chips: [Kind, string][] = [['playlists', t('Playlists')], ['artists', t('Artists')], ['albums', t('Albums')]]
  const count = kind === 'albums' ? albums?.length : kind === 'artists' ? artists?.length : undefined
  return (
    <div className="mus-page">
      <div className="mus-lib-head">
        <h1 className="mus-h1">{t('Your library')}</h1>
        <span className="mus-grow" />
        <ScanButton lib={lib} />
      </div>
      <div className="mus-lib-bar">
        <div className="mus-chips" role="group" aria-label={t('Show')}>
          {chips.map(([k, l], i) => <button key={k} type="button" data-tv-first={i === 0 || undefined} className={'mus-chip' + (kind === k ? ' on' : '')} aria-pressed={kind === k} onClick={() => choose('show', k)}>{l}</button>)}
        </div>
        {count !== undefined && <span className="mus-muted">{kind === 'albums' ? tn('{count} album', '{count} albums', count, { count: num(count) }) : tn('{count} artist', '{count} artists', count, { count: num(count) })}</span>}
        <span className="mus-grow" />
        {SORTS[kind].length > 1 && (
          <label className="mus-sort">{t('Sort by')}
            <select className="select sm" value={sort} onChange={(e) => choose('sort', e.target.value)}>
              {SORTS[kind].map(([k, l]) => <option key={k} value={k}>{tx(l)}</option>)}
            </select>
          </label>
        )}
      </div>
      {!cards ? <PageShape kind="grid" /> : <CardGrid cards={cards} round={kind === 'artists'} label={kind === 'albums' ? t('Your albums') : kind === 'artists' ? t('Your artists') : t('Your playlists')} />}
    </div>
  )
}

// Cards in rows as wide as the page allows, drawn a row at a time near the screen.
function CardGrid({ cards, label, round }: { cards: ReactNode[]; label: string; round?: boolean }) {
  const box = useRef<HTMLDivElement>(null)
  const [cols, setCols] = useState(5)
  const [rowH, setRowH] = useState(260)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const fit = () => {
      const w = el.clientWidth
      const min = w < 520 ? 140 : w > 1600 ? 200 : 170
      const gap = w < 520 ? 12 : 20
      const n = Math.max(2, Math.floor((w + gap) / (min + gap)))
      const card = (w - gap * (n - 1)) / n
      setCols(n)
      // The cover, its padding, two lines of words, and the gap under it.
      setRowH(Math.round(card + 72 + gap))
    }
    fit()
    const ro = new ResizeObserver(fit)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const rows = useMemo(() => { const out: ReactNode[][] = []; for (let i = 0; i < cards.length; i += cols) out.push(cards.slice(i, i + cols)); return out }, [cards, cols])
  return (
    <div ref={box} className={'mus-grid-box' + (round ? ' round' : '')}>
      <VirtualRows items={rows} rowHeight={rowH} label={label} whole={8} render={(row, i) => <div key={i} className="mus-grid-row" role="none" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, height: rowH }}>{row}</div>} />
    </div>
  )
}
