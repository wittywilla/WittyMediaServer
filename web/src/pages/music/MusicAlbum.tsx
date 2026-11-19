// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, fmtRuntime, type Album, type Item, type Library } from '../../api'
import { Pop } from '../../audio/controls'
import { IconHeart, IconHeartOn, IconMore, IconShuffle, IconStation } from '../../audio/icons'
import { useAudio } from '../../audio/player'
import { EmptyState } from '../../components/common'
import LegalNote from '../../components/LegalNote'
import { IconAudio, IconDisc, IconPlay, IconPlus } from '../../icons'
import { PlayOnMenuItem, queueOf } from '../../remote/PlayOnButton'
import { toast, useApp } from '../../state'
import { ActionBar, ListHeader } from './Header'
import { artistLink, coverUrl, shuffledCopy } from './parts'
import { useMusic } from './store'
import { AlbumCard, PageShape, PlayButton, Shelf, tracksUrl } from './ui'
import { TrackTable } from './ui'
import { t, tn } from '../../i18n'

// One album: the cover and its colour at the top, the big play button, and the songs in
// a dense list. Then other versions of it, and more by the same artist.

const AlbumCanvas = lazy(() => import('../../canvas/CanvasVideo').then((m) => ({ default: m.AlbumCanvas })))
const AlbumCanvasManager = lazy(() => import('../../canvas/AlbumCanvasManager'))

// A name without what tends to mark a version of the same album: "(Deluxe)", "[2011 Remaster]".
const baseTitle = (s: string) => s.toLowerCase().replace(/[([{].*?[)\]}]/g, ' ').replace(/\b(deluxe|remaster(ed)?|expanded|special|anniversary|edition|version)\b/g, ' ').replace(/[^\p{L}\p{N}]+/gu, ' ').trim()

export default function MusicAlbum({ lib }: { lib: Library }) {
  const { user } = useApp()
  const [params] = useSearchParams()
  const artist = params.get('artist') ?? ''
  const album = params.get('album') ?? ''
  const audio = useAudio()
  const nav = useNavigate()
  const [canvasOpen, setCanvasOpen] = useState(false)
  const { data: tracks, failed } = useMusic<Item[]>(tracksUrl(lib.id, { artist, title: album }))
  const others = useMusic<Album[]>(artist ? `/music/albums?library=${lib.id}&artist=${encodeURIComponent(artist)}&sort=year` : null).data
  // A disc whose image has gone isn't offered as one to insert.
  const [goneDiscs, setGoneDiscs] = useState<number[]>([])
  const discKey = [...new Set((tracks || []).map((t) => t.cdDisc).filter(Boolean))].join(',')
  useEffect(() => {
    let left = false
    setGoneDiscs([])
    if (discKey) discKey.split(',').forEach((d) => api.get(`/cd/${d}`).catch((e) => { if (!left && (e.status === 404 || e.status === 422)) setGoneDiscs((g) => [...g, Number(d)]) }))
    return () => { left = true }
  }, [discKey])
  if (failed) return <EmptyState icon={<IconAudio />} title={t('That album wouldn’t load')} text={t('The server didn’t answer. Try again in a moment.')} />
  if (!tracks) return <PageShape kind="list" />
  if (!tracks.length) return <EmptyState icon={<IconAudio />} title={t('No songs here')} text={t('This album may have been moved or renamed.')} />
  const coverId = tracks.find((t) => t.poster)?.id
  const total = tracks.reduce((s, t) => s + t.runtimeSec, 0)
  const discs = [...new Set(tracks.map((t) => t.cdDisc).filter((d): d is number => !!d))]
  const playTrack = (i: number) => {
    const t = tracks[i]
    if (t?.cdDisc) nav(`/cd/${t.cdDisc}?play=1&track=${t.track}`)
    else audio.play(tracks, i, 'music')
  }
  const mine = !!audio.current && tracks.some((t) => t.id === audio.current!.id)
  const allLiked = tracks.every((t) => audio.liked(t.id))
  const year = tracks[0]?.year
  const genre = tracks[0]?.genres?.[0]
  const list = (others || []).filter((a) => a.title !== album)
  const versions = list.filter((a) => baseTitle(a.title) === baseTitle(album))
  const more = list.filter((a) => baseTitle(a.title) !== baseTitle(album))
  const title = album || t('Unknown album')
  return (
    <div className="mus-page mus-album">
      <Suspense fallback={null}><AlbumCanvas trackIds={tracks.filter((t) => !t.cdDisc).map((t) => t.id)} /></Suspense>
      <ListHeader art={coverId ? coverUrl(coverId) : ''} title={title} kind={t('Album')}>
        <p className="mus-byline">
          {artist ? <Link to={artistLink(lib.id, artist)} className="mus-by">{artist}</Link> : <span className="mus-by">{t('Unknown artist')}</span>}
          {[year, genre, tn('{count} song', '{count} songs', tracks.length), fmtRuntime(total)].filter(Boolean).map((v) => <span key={String(v)}> · {v}</span>)}
        </p>
      </ListHeader>
      <ActionBar title={title}>
        <PlayButton big tvFirst label={title} playing={mine && audio.playing} onPlay={() => (mine ? audio.toggle() : playTrack(0))} />
        <button type="button" className="mus-icon" aria-label={t('Shuffle {title}', { title })} title={t('Shuffle')} onClick={() => audio.play(shuffledCopy(tracks), 0, 'music')}><IconShuffle /></button>
        <button type="button" className={'mus-icon' + (allLiked ? ' liked' : '')} aria-pressed={allLiked} aria-label={allLiked ? t('Remove every song from liked songs') : t('Like every song')} title={allLiked ? t('Liked') : t('Like all')}
          onClick={() => tracks.forEach((t) => { if (audio.liked(t.id) === allLiked) audio.toggleLike(t.id) })}>{allLiked ? <IconHeartOn /> : <IconHeart />}</button>
        {discs.filter((d) => !goneDiscs.includes(d)).map((d) => <Link key={d} className="mus-icon" to={`/cd/${d}?play=1`} aria-label={discs.length > 1 ? t('Insert disc {n}', { n: discs.indexOf(d) + 1 }) : t('Insert the disc')} title={t('Insert the disc')}><IconDisc /></Link>)}
        <Pop label={t('More for {title}', { title })} icon={<IconMore />} className="mus-icon">
          {(close) => (
            <div className="mp-menu" role="menu">
              <button type="button" role="menuitem" onClick={() => { audio.playNext(tracks); toast(t('Plays next')); close() }}><IconPlay size={18} /> {t('Play next')}</button>
              <button type="button" role="menuitem" onClick={() => { audio.enqueue(tracks); toast(t('Added to the queue')); close() }}><IconPlus size={18} /> {t('Add to the queue')}</button>
              <button type="button" role="menuitem" onClick={() => { audio.startRadio(tracks[0]); close() }}><IconStation size={18} /> {t('Start a station from this')}</button>
              <PlayOnMenuItem source={{ kind: 'item', itemId: tracks[0].id, itemKind: 'audio', title, ...queueOf(tracks.map((t) => t.id), 0) }} onChosen={close} />
              {user?.isAdmin && tracks.some((t) => !t.cdDisc) && <button type="button" role="menuitem" aria-haspopup="dialog" onClick={() => { setCanvasOpen(true); close() }}>{t('Canvas: the looping video behind these songs…')}</button>}
              {artist && <Link role="menuitem" className="mh-menu-link" to={artistLink(lib.id, artist)} onClick={close}>{t('Go to artist')}</Link>}
            </div>
          )}
        </Pop>
      </ActionBar>
      <TrackTable items={tracks} label={t('Songs on {title}', { title })} discs onPlay={playTrack} />
      {tracks[0]?.overview && <p className="mus-about">{tracks[0].overview}</p>}
      {versions.length > 0 && <Shelf title={t('Other versions')}>{versions.map((a) => <AlbumCard key={a.title} libId={lib.id} a={a} sub={String(a.year || '')} />)}</Shelf>}
      {more.length > 0 && <Shelf title={t('More by {artist}', { artist })} to={artistLink(lib.id, artist)}>{more.map((a) => <AlbumCard key={a.title} libId={lib.id} a={a} sub={String(a.year || '')} />)}</Shelf>}
      <LegalNote kind="metadata" />
      {canvasOpen && <Suspense fallback={null}><AlbumCanvasManager songs={tracks.filter((t) => !t.cdDisc)} onClose={() => setCanvasOpen(false)} /></Suspense>}
    </div>
  )
}
