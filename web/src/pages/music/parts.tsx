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
import { num, t, tn } from '../../i18n'
import { MusicInfoDialog, MusicInfoItem } from '../../wittyinfo/MusicInfo'
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
        <h2>{title}{count !== undefined && <span className="muted"> {num(count)}</span>}</h2>
        {to && <Link className="mh-seeall" to={to}>{t('See all')} <IconRight size={18} /></Link>}
      </div>
      {children}
    </section>
  )
}

export function ArtistTile({ libId, a, books }: { libId: number; a: Artist; books?: boolean }) {
  const n = a.albums
  return (
    <Link className="mh-artist" to={artistLink(libId, a.name)} title={a.name || t('Unknown')}>
      <span className="mh-artist-photo"><ArtistPicture libId={libId} name={a.name} /></span>
      {a.coverItem > 0 && <span className="mh-artist-reflect" aria-hidden="true"><ArtistPicture libId={libId} name={a.name} /></span>}
      <b>{a.name || t('Unknown')}</b>
      <span className="muted">{books ? tn('{count} book', '{count} books', n) : tn('{count} album', '{count} albums', n)}</span>
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
  return <Link className={'mh-seeall-tile' + (round ? ' round' : '')} to={to}><span>{t('See all')}</span></Link>
}

export function AlbumTile({ libId, a, book, noArtist }: { libId: number; a: Album; book?: boolean; noArtist?: boolean }) {
  return (
    <Link to={albumLink(libId, a)} className="album-card mh-album" title={a.title}>
      <div className={'album-cover' + (book ? ' book' : '')}>{a.coverItem ? <img src={sized(coverUrl(a.coverItem), 320)} srcSet={artSet(coverUrl(a.coverItem))} sizes={CARD_SIZES} alt="" loading="lazy" /> : <Placeholder title={a.title || t('Unknown')} />}</div>
      <b>{a.title || t('Unknown album')}</b>
      <span className="muted">{[noArtist ? '' : a.artist, a.year].filter(Boolean).join(' · ')}</span>
    </Link>
  )
}

// One song, as a compact row: for shelves, and lists that come from many albums.
export function SongRow({ t: song, onPlay }: { t: Item; onPlay: () => void }) {
  const { current, playing } = useAudio()
  const on = current?.id === song.id
  return (
    <div className={'mh-song' + (on ? ' on' : '')}>
      <button type="button" className="mh-song-play" onClick={onPlay} aria-label={t('Play {title}', { title: song.title })} aria-current={on ? 'true' : undefined}>
        <span className="mh-song-art">{song.poster && <img src={sized(posterUrl(song), fitWidth(64))} alt="" loading="lazy" />}{on && playing && <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i>}</span>
        <span className="mh-song-text"><b>{song.title}</b><small>{[song.artist || song.albumArtist, song.album].filter(Boolean).join(' · ')}</small></span>
        <span className="mh-song-time">{song.runtimeSec ? fmtTime(song.runtimeSec) : ''}</span>
      </button>
      <TrackMenu t={song} />
    </div>
  )
}

// A song's other actions, in one place on every list.
export function TrackMenu({ t: song }: { t: Item }) {
  const audio = useAudio()
  const liked = audio.liked(song.id)
  const [adding, setAdding] = useState(false)
  const [tying, setTying] = useState(false)
  const [info, setInfo] = useState(false)
  const { user } = useApp()
  return (<>
    <Pop label={t('More for {title}', { title: song.title })} icon={<IconMore />} className="mh-more">
      {(close) => adding ? <PlaylistPicker items={[song]} onDone={() => { setAdding(false); close() }} /> : (
        <div className="mp-menu" role="menu">
          <button type="button" role="menuitem" onClick={() => { audio.playNext([song]); toast(t('Plays next')); close() }}><IconPlay size={18} /> {t('Play next')}</button>
          <button type="button" role="menuitem" onClick={() => { audio.enqueue([song]); toast(t('Added to the queue')); close() }}><IconPlus size={18} /> {t('Add to the queue')}</button>
          <button type="button" role="menuitem" onClick={() => { audio.toggleLike(song.id); close() }}>{liked ? <IconHeartOn size={18} /> : <IconHeart size={18} />} {liked ? t('Liked') : t('Like')}</button>
          <button type="button" role="menuitem" onClick={() => { audio.startRadio(song); close() }}><IconStation size={18} /> {t('Start a station from this')}</button>
          <button type="button" role="menuitem" onClick={() => setAdding(true)}><IconAddList size={18} /> {t('Add to a playlist…')}</button>
          <PlayOnMenuItem source={{ kind: 'item', itemId: song.id, itemKind: song.kind, title: song.title }} onChosen={close} />
          <SongVideoItems t={song} close={close} onTie={() => setTying(true)} />
          {song.artist && <Link role="menuitem" className="mh-menu-link" to={artistLink(song.libraryId, song.albumArtist || song.artist)} onClick={close}>{t('Go to artist')}</Link>}
          {user?.isAdmin && song.kind === 'audio' && <MusicInfoItem label={t('Lyrics and details')} onOpen={() => setInfo(true)} close={close} />}
          {song.album && <Link role="menuitem" className="mh-menu-link" to={albumLink(song.libraryId, { artist: song.albumArtist || song.artist || '', title: song.album })} onClick={close}>{t('Go to album')}</Link>}
        </div>
      )}
    </Pop>
    {info && <MusicInfoDialog album={false} id={song.id} title={song.title} onClose={() => setInfo(false)} />}
    {tying && <TieDialog item={{ id: song.id, title: song.title }} role="song" onClose={() => setTying(false)} />}
  </>)
}

// On phones, a row across the top to move between kinds of media with a thumb.
export function LibrarySwitch() {
  const { libraries } = useApp()
  if (libraries.length < 2) return null
  return (
    <nav className="mh-libs" aria-label={t('Libraries')}>
      {libraries.map((l) => <NavLink key={l.id} to={`/library/${l.id}`}>{l.name}</NavLink>)}
    </nav>
  )
}

export function shuffledCopy<T>(list: T[]) {
  const a = [...list]
  for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]] }
  return a
}
