// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link, NavLink } from 'react-router-dom'
import { useState, type ReactNode } from 'react'
import { artSet, CARD_SIZES, fitWidth, fmtTime, mediaUrl, posterUrl, sized, type Album, type Artist, type Item } from '../../api'
import { PlaylistPicker, Pop } from '../../audio/controls'
import { IconAddList, IconHeart, IconHeartOn, IconMore, IconStation } from '../../audio/icons'
import { useAudio } from '../../audio/player'
import { Placeholder } from '../../components/common'
import { IconPlay, IconPlus, IconRight } from '../../icons'
import { PlayOnMenuItem } from '../../remote/PlayOnButton'
import { toast, useApp } from '../../state'
import { SongVideoItems, TieDialog } from './SongVideo'
import './music.css'

// What the music pages are built from.

export const coverUrl = (itemId: number) => (itemId ? mediaUrl(`/api/items/${itemId}/poster`) : '')
export const albumLink = (libId: number, a: { artist: string; title: string }) => `/album/${libId}?artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`
export const artistLink = (libId: number, name: string) => `/artist/${libId}?name=${encodeURIComponent(name)}`
export const artistPhoto = (libId: number, name: string) => mediaUrl(`/api/music/artist-photo?library=${libId}&name=${encodeURIComponent(name)}`)

// A heading, with the way to see all of it at the far end.
export function Shelf({ title, to, count, children, wide }: { title: string; to?: string; count?: number; children: ReactNode; wide?: boolean }) {
  return (
    <section className={'mh-shelf' + (wide ? ' wide' : '')} aria-label={title}>
      <div className="mh-shelf-head">
        <h2>{title}{count !== undefined && <span className="muted"> {count.toLocaleString()}</span>}</h2>
        {to && <Link className="mh-seeall" to={to}>See all <IconRight size={18} /></Link>}
      </div>
      {children}
    </section>
  )
}

export function ArtistTile({ libId, a, noun }: { libId: number; a: Artist; noun: [string, string] }) {
  const n = a.albums
  return (
    <Link className="mh-artist" to={artistLink(libId, a.name)} title={a.name || 'Unknown'}>
      <span className="mh-artist-photo"><ArtistPicture libId={libId} name={a.name} /></span>
      {a.coverItem > 0 && <span className="mh-artist-reflect" aria-hidden="true"><ArtistPicture libId={libId} name={a.name} /></span>}
      <b>{a.name || 'Unknown'}</b>
      <span className="muted">{n} {n === 1 ? noun[0] : noun[1]}</span>
    </Link>
  )
}

function ArtistPicture({ libId, name }: { libId: number; name: string }) {
  return <>
    <span className="mh-initial" aria-hidden="true">{(name || '?')[0]}</span>
    <img src={artistPhoto(libId, name)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
  </>
}

export function SeeAllTile({ to, round }: { to: string; round?: boolean }) {
  return <Link className={'mh-seeall-tile' + (round ? ' round' : '')} to={to}><span>See all</span></Link>
}

export function AlbumTile({ libId, a, book, noArtist }: { libId: number; a: Album; book?: boolean; noArtist?: boolean }) {
  return (
    <Link to={albumLink(libId, a)} className="album-card mh-album" title={a.title}>
      <div className={'album-cover' + (book ? ' book' : '')}>{a.coverItem ? <img src={sized(coverUrl(a.coverItem), 320)} srcSet={artSet(coverUrl(a.coverItem))} sizes={CARD_SIZES} alt="" loading="lazy" /> : <Placeholder title={a.title || 'Unknown'} />}</div>
      <b>{a.title || 'Unknown album'}</b>
      <span className="muted">{[noArtist ? '' : a.artist, a.year].filter(Boolean).join(' · ')}</span>
    </Link>
  )
}

// One song, as a compact row: for shelves, and lists that come from many albums.
export function SongRow({ t, onPlay }: { t: Item; onPlay: () => void }) {
  const { current, playing } = useAudio()
  const on = current?.id === t.id
  return (
    <div className={'mh-song' + (on ? ' on' : '')}>
      <button type="button" className="mh-song-play" onClick={onPlay} aria-label={`Play ${t.title}`} aria-current={on ? 'true' : undefined}>
        <span className="mh-song-art">{t.poster && <img src={sized(posterUrl(t), fitWidth(64))} alt="" loading="lazy" />}{on && playing && <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i>}</span>
        <span className="mh-song-text"><b>{t.title}</b><small>{[t.artist || t.albumArtist, t.album].filter(Boolean).join(' · ')}</small></span>
        <span className="mh-song-time">{t.runtimeSec ? fmtTime(t.runtimeSec) : ''}</span>
      </button>
      <TrackMenu t={t} />
    </div>
  )
}

// A song's other actions, in one place on every list.
export function TrackMenu({ t }: { t: Item }) {
  const audio = useAudio()
  const liked = audio.liked(t.id)
  const [adding, setAdding] = useState(false)
  const [tying, setTying] = useState(false)
  return (<>
    <Pop label={`More for ${t.title}`} icon={<IconMore />} className="mh-more">
      {(close) => adding ? <PlaylistPicker items={[t]} onDone={() => { setAdding(false); close() }} /> : (
        <div className="mp-menu" role="menu">
          <button type="button" role="menuitem" onClick={() => { audio.playNext([t]); toast('Plays next'); close() }}><IconPlay size={18} /> Play next</button>
          <button type="button" role="menuitem" onClick={() => { audio.enqueue([t]); toast('Added to the queue'); close() }}><IconPlus size={18} /> Add to the queue</button>
          <button type="button" role="menuitem" onClick={() => { audio.toggleLike(t.id); close() }}>{liked ? <IconHeartOn size={18} /> : <IconHeart size={18} />} {liked ? 'Liked' : 'Like'}</button>
          <button type="button" role="menuitem" onClick={() => { audio.startRadio(t); close() }}><IconStation size={18} /> Start a station from this</button>
          <button type="button" role="menuitem" onClick={() => setAdding(true)}><IconAddList size={18} /> Add to a playlist…</button>
          <PlayOnMenuItem source={{ kind: 'item', itemId: t.id, itemKind: t.kind, title: t.title }} onChosen={close} />
          <SongVideoItems t={t} close={close} onTie={() => setTying(true)} />
          {t.artist && <Link role="menuitem" className="mh-menu-link" to={artistLink(t.libraryId, t.albumArtist || t.artist)} onClick={close}>Go to artist</Link>}
          {t.album && <Link role="menuitem" className="mh-menu-link" to={albumLink(t.libraryId, { artist: t.albumArtist || t.artist || '', title: t.album })} onClick={close}>Go to album</Link>}
        </div>
      )}
    </Pop>
    {tying && <TieDialog item={{ id: t.id, title: t.title }} role="song" onClose={() => setTying(false)} />}
  </>)
}

// On phones, a row across the top to move between kinds of media with a thumb.
export function LibrarySwitch() {
  const { libraries } = useApp()
  if (libraries.length < 2) return null
  return (
    <nav className="mh-libs" aria-label="Libraries">
      {libraries.map((l) => <NavLink key={l.id} to={`/library/${l.id}`}>{l.name}</NavLink>)}
    </nav>
  )
}

export function shuffledCopy<T>(list: T[]) {
  const a = [...list]
  for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]] }
  return a
}
