// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { fitWidth, fmtRuntime, sized, type Album, type Item, type Library } from '../../api'
import { useArtColour } from '../../audio/art'
import { IconShuffle, IconStation } from '../../audio/icons'
import { useAudio } from '../../audio/player'
import { EmptyState } from '../../components/common'
import { WebLink } from '../../components/WebLink'
import { IconAudio } from '../../icons'
import { useApp } from '../../state'
import { ActionBar } from './Header'
import { artistPhoto, shuffledCopy } from './parts'
import { useMusic } from './store'
import { AlbumCard, artistUrl, PageShape, PlayButton, Shelf, TrackTable } from './ui'
import { list, t, tn } from '../../i18n'

// One artist: their picture across the top, fading into the page, their name over it, the
// play button, the songs played most, then the discography in rows. A biography last, if the
// owner turned online information on.

type ArtistInfo = { name: string; albums: Album[]; tracks: Item[]; top: Item[]; genres: string[]; online: boolean; bio?: string; bioUrl?: string; bioTitle?: string }

export default function MusicArtist({ lib }: { lib: Library }) {
  const [params] = useSearchParams()
  const name = params.get('name') ?? ''
  const { user } = useApp()
  const audio = useAudio()
  const [more, setMore] = useState(false)
  const [noPhoto, setNoPhoto] = useState(false)
  const { data: info, failed } = useMusic<ArtistInfo>(artistUrl(lib.id, name))
  const photo = artistPhoto(lib.id, name)
  const rgb = useArtColour(sized(photo, 32))
  const byYear = useMemo(() => [...(info?.albums || [])].sort((a, b) => (b.year || 0) - (a.year || 0)), [info])
  if (failed) return <EmptyState icon={<IconAudio />} title={t('Nobody by that name here')} text={t('They may have been moved or renamed.')} />
  if (!info) return <PageShape kind="list" />
  // Few songs to an album means it's most likely a single or an EP: tags rarely say.
  const full = byYear.filter((a) => a.tracks >= 7)
  const small = byYear.filter((a) => a.tracks < 7)
  const top = (info.top.length ? info.top : info.tracks).slice(0, more ? 10 : 5)
  const total = info.tracks.reduce((s, t) => s + t.runtimeSec, 0)
  const title = name || t('Unknown artist')
  const mine = !!audio.current && info.tracks.some((t) => t.id === audio.current!.id)
  return (
    <div className="mus-page mus-artist" style={{ '--art': rgb } as React.CSSProperties}>
      <header className={'mus-hero' + (noPhoto ? ' no-photo' : '')}>
        {!noPhoto && <img className="mus-hero-img" src={sized(photo, fitWidth(1280))} alt="" onError={() => setNoPhoto(true)} />}
        <div className="mus-hero-words">
          <span className="mus-kind">{t('Artist')}</span>
          <h1 className={'mus-title' + (title.length > 30 ? ' long' : '')} title={title}>{title}</h1>
          <p className="mus-byline">{[tn('{count} album', '{count} albums', info.albums.length), tn('{count} song', '{count} songs', info.tracks.length), fmtRuntime(total), list(info.genres.slice(0, 3))].filter(Boolean).join(' · ')}</p>
        </div>
      </header>
      <ActionBar title={title}>
        <PlayButton big tvFirst label={title} playing={mine && audio.playing} onPlay={() => (mine ? audio.toggle() : audio.play(info.tracks, 0, 'music'))} />
        <button type="button" className="mus-icon" aria-label={t('Shuffle {title}', { title })} title={t('Shuffle')} onClick={() => audio.play(shuffledCopy(info.tracks), 0, 'music')}><IconShuffle /></button>
        {info.tracks[0] && <button type="button" className="mus-icon" aria-label={t('Start a station from {title}', { title })} title={t('Station')} onClick={() => audio.startRadio(info.tracks[0])}><IconStation /></button>}
      </ActionBar>
      {top.length > 0 && (
        <section className="mus-popular" aria-labelledby="mus-popular">
          <h2 id="mus-popular" className="mus-h2">{info.top.length ? t('Popular with you') : t('Songs')}</h2>
          <TrackTable items={top} label={t('Popular songs')} showArt numbers="index" onPlay={(i) => audio.play(top, i, 'music')} />
          {(info.top.length || info.tracks.length) > 5 && <button type="button" className="mus-text-btn" aria-expanded={more} onClick={() => setMore(!more)}>{more ? t('Show less') : t('See more')}</button>}
        </section>
      )}
      {full.length > 0 && <Shelf title={t('Albums')}>{full.map((a) => <AlbumCard key={a.title} libId={lib.id} a={a} sub={[a.year, t('Album')].filter(Boolean).join(' · ')} />)}</Shelf>}
      {small.length > 0 && <Shelf title={t('Singles and EPs')}>{small.map((a) => <AlbumCard key={a.title} libId={lib.id} a={a} sub={[a.year, a.tracks === 1 ? t('Single') : t('EP')].filter(Boolean).join(' · ')} />)}</Shelf>}
      {(info.bio || (!info.online && user?.isAdmin)) && (
        <section className="mus-about-box" aria-labelledby="mus-about">
          <h2 id="mus-about" className="mus-h2">{t('About')}</h2>
          {info.bio && <p className="mus-about">{info.bio}</p>}
          {info.bio && info.bioUrl && <p className="mus-credit">{t('From Wikipedia, available under CC BY-SA 4.0. It’s the article with this name, which may not always be the right person.')} <WebLink href={info.bioUrl}>{t('Read the article')}</WebLink></p>}
          {!info.online && !info.bio && user?.isAdmin && <p className="mus-credit">{t('Biographies and photos can come from Wikipedia, if you turn on online music information.')} <Link to="/settings/metadata#lyrics">{t('Online music information')}</Link>. {t('An artist.jpg in their folder is used first.')}</p>}
        </section>
      )}
    </div>
  )
}
