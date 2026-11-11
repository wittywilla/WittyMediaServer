// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type Library } from '../../api'
import { EmptyState, Spinner } from '../../components/common'
import { IconAudio, IconChannels } from '../../icons'
import { LoadMore, VideoTile } from '../creators/parts'
import { artistLink, Shelf } from './parts'
import type { MusicVideoCard } from './links'
import '../../creators.css'

// The Music videos section of a music library: the videos of every music video library
// this person can see, by artist. A video tied to one of this library's songs says so.

type Page = { videos: MusicVideoCard[]; more: boolean }

const PAGE = 120

// Whether there's anywhere music videos could come from.
export const hasMusicVideos = (libraries: Library[]) => libraries.some((l) => l.type === 'musicvideos')

function useMusicVideos(search: string) {
  const [items, setItems] = useState<MusicVideoCard[] | null>(null)
  const [more, setMore] = useState(false)
  const busy = useRef(false)
  const asked = useRef(0) // a newer search makes an older answer stale
  const load = useCallback(async (from: number) => {
    if (busy.current && from > 0) return
    busy.current = true
    const n = ++asked.current
    try {
      const u = new URLSearchParams({ limit: String(PAGE), offset: String(from) })
      if (search) u.set('search', search)
      const r = await api.get<Page>(`/music/videos?${u}`)
      if (n !== asked.current) return
      setItems((x) => (from === 0 || !x ? r.videos : [...x, ...r.videos]))
      setMore(r.more)
    } catch { if (n === asked.current) setItems((x) => x || []) } finally { if (n === asked.current) busy.current = false }
  }, [search])
  useEffect(() => { setItems(null); void load(0) }, [load])
  return { items, more, loadMore: () => void load(items?.length || 0) }
}

export function MusicVideosTab({ lib }: { lib: Library }) {
  const [q, setQ] = useState('')
  const [search, setSearch] = useState('')
  useEffect(() => { const t = setTimeout(() => setSearch(q.trim()), 250); return () => clearTimeout(t) }, [q])
  // In artist order from the server, each tile naming its artist: most artists have one or
  // two videos, so a heading each would leave the page mostly empty.
  const { items, more, loadMore } = useMusicVideos(search)
  return (
    <>
      <div className="mh-filters">
        {items && <span className="count">{items.length.toLocaleString()}{more ? '+' : ''} {items.length === 1 ? 'video' : 'videos'}</span>}
        <div className="grow" />
        <input className="input mh-find" type="search" aria-label="Find a music video" placeholder="Find a video or an artist…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {!items ? <Spinner /> : items.length === 0
        ? <EmptyState icon={<IconChannels />} title={search ? 'Nothing matches that' : 'No music videos yet'} text={search ? '' : 'Add a library of the kind Music videos & concerts, and its videos appear here by artist.'} />
        : <div className="cr-grid mh-videos">{items.map((v) => <MusicVideoTile key={v.id} v={v} lib={lib} />)}</div>}
      <LoadMore more={more} onMore={loadMore} />
    </>
  )
}

// A video, and the song it's tied to underneath when it's in this library.
export function MusicVideoTile({ v, lib }: { v: MusicVideoCard; lib: Library }) {
  return (
    <div className="mh-video">
      <VideoTile v={v} to={`/watch/${v.id}`} />
      {v.song && (
        <Link className="mh-video-song" to={v.song.libraryId === lib.id && v.song.album ? `/album/${lib.id}?artist=${encodeURIComponent(v.song.artist)}&album=${encodeURIComponent(v.song.album)}` : artistLink(v.song.libraryId, v.song.artist)}>
          <IconAudio size={14} /> <span>{v.song.album ? `On ${v.song.album}` : 'In your music'}</span>
        </Link>
      )}
    </div>
  )
}

// A shelf of them for the front page, when there are any.
export function MusicVideosShelf({ lib }: { lib: Library }) {
  const [items, setItems] = useState<MusicVideoCard[] | null>(null)
  useEffect(() => { api.get<Page>('/music/videos?limit=12&sort=latest').then((r) => setItems(r.videos)).catch(() => setItems([])) }, [])
  if (!items?.length) return null
  return (
    <Shelf title="Music videos" to={`/library/${lib.id}?tab=videos`}>
      <div className="ch-shelf-row mh-video-row">{items.map((v) => <MusicVideoTile key={v.id} v={v} lib={lib} />)}</div>
    </Shelf>
  )
}
