// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api, fitWidth, fmtTime, posterUrl, sized, type Album, type Artist, type Item } from '../../api'
import { useAudio } from '../../audio/player'
import { IconHeart, IconHeartOn } from '../../audio/icons'
import { Placeholder } from '../../components/common'
import { IconLeft, IconPause, IconPlay, IconRight } from '../../icons'
import { toast } from '../../state'
import { useTvMode } from '../../tvmode/mode'
import { albumLink, artistLink, artistPhoto, coverUrl, TrackMenu } from './parts'
import { prefetch } from './store'
import { VirtualRows } from './Virtual'
import { t, bidi } from '../../i18n'

// The pieces every Music page is made of: shelves that scroll sideways, cards for albums,
// artists and playlists with a play button that shows on hover or focus, and the dense list
// of songs with its numbers that turn into play buttons.

export const tracksUrl = (libId: number, a: { artist: string; title: string }) => `/music/tracks?library=${libId}&artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`
export const artistUrl = (libId: number, name: string) => `/music/artist?library=${libId}&name=${encodeURIComponent(name)}`

// A picture that gives way to `fallback` when it can't be had (gone, unreadable, refused),
// so no broken-picture mark or empty square is ever left in its place.
export function Art({ src, fallback = null, className }: { src: string; fallback?: ReactNode; className?: string }) {
  const [broken, setBroken] = useState('')
  if (!src || broken === src) return <>{fallback}</>
  return <img className={className} src={src} alt="" loading="lazy" decoding="async" onError={() => setBroken(src)} />
}

// Plays what a card stands for, fetching its songs first; the button says it's loading meanwhile.
async function playFrom(get: () => Promise<Item[]>, play: (items: Item[]) => void) {
  try {
    const items = await get()
    if (items.length) play(items)
    else toast(t('Nothing to play there yet'))
  } catch (e) { toast((e as Error).message) }
}

// A shelf: a heading, a "Show all", and a row that scrolls sideways with arrows on wide screens.
export function Shelf({ title, to, children, sub }: { title: string; to?: string; children: ReactNode; sub?: string }) {
  const row = useRef<HTMLDivElement>(null)
  const [ends, setEnds] = useState<[boolean, boolean]>([true, false])
  useEffect(() => {
    const el = row.current
    if (!el) return
    let frame = 0
    const look = () => { frame = 0; setEnds([el.scrollLeft < 4, el.scrollLeft + el.clientWidth >= el.scrollWidth - 4]) }
    const later = () => { if (!frame) frame = requestAnimationFrame(look) }
    look()
    el.addEventListener('scroll', later, { passive: true })
    window.addEventListener('resize', later)
    return () => { cancelAnimationFrame(frame); el.removeEventListener('scroll', later); window.removeEventListener('resize', later) }
  }, [])
  const step = (dir: number) => { const el = row.current; if (el) el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: 'smooth' }) }
  const id = 'shelf-' + useId().replace(/\W+/g, '')
  return (
    <section className="mus-shelf" aria-labelledby={id}>
      <div className="mus-shelf-head">
        <h2 id={id}>{to ? <Link to={to}>{title}</Link> : title}</h2>
        {sub && <span className="mus-muted">{sub}</span>}
        <span className="mus-grow" />
        <span className="mus-shelf-steps" aria-hidden="true">
          <button type="button" tabIndex={-1} className="mus-round" aria-label={t('Scroll back')} disabled={ends[0]} onClick={() => step(-1)}><IconLeft size={18} /></button>
          <button type="button" tabIndex={-1} className="mus-round" aria-label={t('Scroll on')} disabled={ends[1]} onClick={() => step(1)}><IconRight size={18} /></button>
        </span>
        {to && <Link className="mus-showall" to={to}>{t('Show all')}</Link>}
      </div>
      <div className="mus-shelf-row" ref={row} role="list" data-nav-row>{children}</div>
    </section>
  )
}

// The round play button on a card, or on a page's header.
export function PlayButton({ label, onPlay, playing, big, tvFirst }: { label: string; onPlay: () => void | Promise<void>; playing?: boolean; big?: boolean; tvFirst?: boolean }) {
  const [busy, setBusy] = useState(false)
  return (
    <button type="button" className={'mus-play' + (big ? ' big' : '')} aria-label={playing ? t('Pause {title}', { title: label }) : t('Play {title}', { title: label })} aria-busy={busy || undefined} data-tv-first={tvFirst || undefined}
      onClick={async (e) => { e.preventDefault(); e.stopPropagation(); setBusy(true); try { await onPlay() } finally { setBusy(false) } }}>
      {playing ? <IconPause /> : <IconPlay />}
    </button>
  )
}

function Cover({ id, title, round }: { id: number; title: string; round?: boolean }) {
  return (
    <span className={'mus-cover' + (round ? ' round' : '')}>
      <Art src={id ? sized(coverUrl(id), fitWidth(180)) : ''} fallback={<Placeholder title={title || t('Unknown')} />} />
    </span>
  )
}

export function AlbumCard({ libId, a, sub }: { libId: number; a: Album; sub?: string }) {
  const audio = useAudio()
  const to = albumLink(libId, a)
  const mine = audio.current && audio.current.album === a.title && (audio.current.albumArtist || audio.current.artist) === a.artist
  return (
    <div className="mus-card" role="listitem">
      <Link to={to} className="mus-card-link" onPointerEnter={() => prefetch(tracksUrl(libId, a))} onFocus={() => prefetch(tracksUrl(libId, a))}>
        <Cover id={a.coverItem} title={a.title} />
        <b title={a.title}>{a.title || t('Unknown album')}</b>
        <span className="mus-muted">{sub ?? [a.year, a.artist].filter(Boolean).map(bidi).join(' · ')}</span>
      </Link>
      <span className="mus-card-over"><PlayButton label={a.title || t('this album')} playing={!!mine && audio.playing}
        onPlay={() => (mine ? audio.toggle() : playFrom(() => api.get<Item[]>(tracksUrl(libId, a)), (t) => audio.play(t, 0, 'music')))} /></span>
    </div>
  )
}

export function ArtistCard({ libId, a }: { libId: number; a: Artist }) {
  const audio = useAudio()
  return (
    <div className="mus-card artist" role="listitem">
      <Link to={artistLink(libId, a.name)} className="mus-card-link" onPointerEnter={() => prefetch(artistUrl(libId, a.name))} onFocus={() => prefetch(artistUrl(libId, a.name))}>
        <span className="mus-cover round"><span className="mus-initial" aria-hidden="true">{(a.name || '?')[0]}</span><Art src={artistPhoto(libId, a.name)} /></span>
        <b title={a.name}>{a.name || t('Unknown artist')}</b>
        <span className="mus-muted">{t('Artist')}</span>
      </Link>
      <span className="mus-card-over"><PlayButton label={a.name || t('this artist')} onPlay={() => playFrom(async () => (await api.get<{ tracks: Item[] }>(artistUrl(libId, a.name))).tracks, (t) => audio.play(t, 0, 'music'))} /></span>
    </div>
  )
}

// A playlist's card: its own cover, or a tile in a colour of its own with its first letter.
export function ListCard({ to, name, sub, coverId, hue, onPlay }: { to: string; name: string; sub: string; coverId?: number; hue?: number; onPlay: () => Promise<void> }) {
  return (
    <div className="mus-card" role="listitem">
      <Link to={to} className="mus-card-link">
        {coverId ? <Cover id={coverId} title={name} /> : <span className="mus-cover mus-tile" style={{ '--h': hue ?? 260 } as React.CSSProperties} aria-hidden="true"><b>{name[0]}</b></span>}
        <b title={name}>{name}</b>
        <span className="mus-muted">{sub}</span>
      </Link>
      <span className="mus-card-over"><PlayButton label={name} onPlay={onPlay} /></span>
    </div>
  )
}

// The list of songs: number (a play button on hover or focus), title and artist, the album
// where they come from many, a heart, the length, and the menu. Arrow keys move between rows.
// On a television a song is one stop going down (#567): the title, artist and album links are
// for a pointer, and the row's menu reaches the artist and the album.
export function TrackTable({ items, onPlay, showAlbum, showArt, numbers = 'track', discs, label }: {
  items: Item[]; onPlay: (i: number) => void; showAlbum?: boolean; showArt?: boolean; numbers?: 'track' | 'index'; discs?: boolean; label: string
}) {
  const { current, playing, liked, toggleLike, toggle } = useAudio()
  const tv = useTvMode()
  const manyDiscs = discs && new Set(items.map((t) => t.discNo || 1)).size > 1
  const rowH = tv ? (showArt ? 72 : 64) : showArt ? 60 : 52
  const move = (e: React.KeyboardEvent, i: number) => {
    const to = e.key === 'ArrowDown' ? i + 1 : e.key === 'ArrowUp' ? i - 1 : e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : -2
    // Past either end, the key is the app's to move on with (to the buttons above, or what's below).
    if (to === -2 || to < 0 || to >= items.length) return
    e.preventDefault()
    e.stopPropagation()
    const list = (e.currentTarget as HTMLElement).closest('.mus-tracks') as HTMLElement
    const focus = () => (list.querySelector(`[data-row="${to}"] .mus-tr-play`) as HTMLElement | null)?.focus({ preventScroll: false })
    const top = list.getBoundingClientRect().top + window.scrollY + to * rowH
    if (top < window.scrollY + 80 || top + rowH > window.scrollY + window.innerHeight - 120) window.scrollTo({ top: top - window.innerHeight / 2 })
    requestAnimationFrame(() => requestAnimationFrame(focus))
  }
  return (
    <div className={'mus-tracks' + (showAlbum ? ' with-album' : '') + (showArt ? ' with-art' : '')}>
      <div className="mus-tr head" aria-hidden="true">
        <span className="mus-tr-n">#</span><span>{t('Title')}</span>{showAlbum && <span className="mus-tr-album">{t('Album')}</span>}<span /><span className="mus-tr-d">{t('Time')}</span><span />
      </div>
      <VirtualRows items={items} rowHeight={rowH} label={label} render={(tr, i) => {
        const on = current?.id === tr.id
        const hearted = liked(tr.id)
        const newDisc = manyDiscs && (i === 0 || (items[i - 1].discNo || 1) !== (tr.discNo || 1))
        const artist = tr.artist || tr.albumArtist || ''
        return (
          <div key={tr.id + '-' + i} role="listitem" data-row={i} className={'mus-tr' + (on ? ' on' : '') + (on && playing ? ' playing' : '')} onDoubleClick={() => onPlay(i)}
            data-disc={newDisc ? t('Disc {n}', { n: tr.discNo || 1 }) : undefined} aria-current={on ? 'true' : undefined}>
            <button type="button" className="mus-tr-play" onKeyDown={(e) => move(e, i)} onClick={() => (on ? toggle() : onPlay(i))}
              aria-label={on && playing ? t('Pause {title}', { title: tr.title }) : artist ? t('Play {title}, by {artist}', { title: tr.title, artist }) : t('Play {title}', { title: tr.title })}>
              <span className="mus-tr-n">{numbers === 'track' ? tr.track ?? i + 1 : i + 1}</span>
              <span className="mus-tr-icon">{on && playing ? <IconPause size={16} /> : <IconPlay size={16} />}</span>
              {on && playing && <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i>}
            </button>
            <span className="mus-tr-main">
              {showArt && <span className="mus-tr-art"><Art src={tr.poster ? sized(posterUrl(tr), fitWidth(48)) : ''} /></span>}
              <span className="mus-tr-words">
                <button type="button" tabIndex={-1} data-nav-skip={tv || undefined} className="mus-tr-title" onClick={() => onPlay(i)}>{tr.title}</button>
                {artist && <Link tabIndex={-1} data-nav-skip={tv || undefined} className="mus-tr-sub" to={artistLink(tr.libraryId, tr.albumArtist || artist)}>{artist}</Link>}
              </span>
            </span>
            {showAlbum && <span className="mus-tr-album">{tr.album && <Link tabIndex={-1} data-nav-skip={tv || undefined} to={albumLink(tr.libraryId, { artist: tr.albumArtist || artist, title: tr.album })}>{tr.album}</Link>}</span>}
            <button type="button" className={'mus-tr-heart' + (hearted ? ' liked' : '')} aria-pressed={hearted} aria-label={hearted ? t('Remove {title} from liked songs', { title: tr.title }) : t('Like {title}', { title: tr.title })} onClick={() => toggleLike(tr.id)}>
              {hearted ? <IconHeartOn size={18} /> : <IconHeart size={18} />}
            </button>
            <span className="mus-tr-d">{tr.runtimeSec ? fmtTime(tr.runtimeSec) : ''}</span>
            <TrackMenu t={tr} />
          </div>
        )
      }} />
    </div>
  )
}

// While a page's first answer comes: its shape, in the page's own colours, not a spinner.
export function PageShape({ kind }: { kind: 'list' | 'shelves' | 'grid' }) {
  return (
    <div className={'mus-shape ' + kind} role="status" aria-label={t('Loading')}>
      {kind === 'list' && <><span className="mus-shape-head" />{Array.from({ length: 8 }, (_, i) => <span key={i} className="mus-shape-row" />)}</>}
      {kind === 'shelves' && Array.from({ length: 3 }, (_, i) => <span key={i} className="mus-shape-shelf"><i /><i /><i /><i /><i /><i /></span>)}
      {kind === 'grid' && <span className="mus-shape-shelf grid">{Array.from({ length: 12 }, (_, i) => <i key={i} />)}</span>}
    </div>
  )
}
