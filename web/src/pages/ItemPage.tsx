// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, backdropUrl, codecName, fitWidth, sized, dvdStartMode, episodeLabel, fmtRuntime, fmtTime, insertFxOn, isDisc, langName, posterUrl, type Item, discName, isBluray } from '../api'
import EditDetails, { type EditSection } from '../components/edit/EditDetails'
import { useModuleItemMenu } from '../moduleRegistry'
import RefreshDialog from '../components/edit/RefreshDialog'
import TitleArtPicker, { PosterMenu, appliedOnPage } from '../components/edit/TitleArtPicker'
import TitleExtras, { TitleLogo } from '../components/edit/TitleExtras'
import { SaveButton } from '../components/SaveToCollection'

import { useRemoteAllowed } from '../remote/allowed'

const PlayOn = lazy(() => import('../remote/PlayOn'))
const WatchParty = lazy(() => import('../remote/WatchParty'))
const WhereToWatch = lazy(() => import('../components/WhereToWatch'))
const TitleCanvas = lazy(() => import('../canvas/CanvasVideo').then((m) => ({ default: m.TitleCanvas })))
const CanvasManager = lazy(() => import('../canvas/CanvasManager'))
import { BackLink, EmptyState, LoadError, PageSkeleton, artHue, isGeneratedArt } from '../components/common'
import { IconDevices, IconDisc, IconEdit, IconFilm, IconImage, IconInfo, IconMenu, IconMore, IconNext, IconPlay, IconRefresh, IconRight, IconSettings, IconStar, IconUsers } from '../icons'
import { IconButton, RowSheet, Rows, SheetRow } from '../ui'
import { useMedia } from '../shell/useNav'
import { useApp } from '../state'
import LegalNote from '../components/LegalNote'
import { num, t, tn, tx } from '../i18n'

export default function ItemPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user, libraries } = useApp()
  const [it, setIt] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const [params, setParams] = useSearchParams()
  const [edit, setEdit] = useState<EditSection | ''>((params.get('edit') as EditSection) || '')
  const [refreshing, setRefreshing] = useState(false)
  const [art, setArt] = useState(false)
  const [canvas, setCanvas] = useState(false)
  const [broken, setBroken] = useState(false)
  const [more, setMore] = useState(false)
  const [sendTo, setSendTo] = useState<'device' | 'party' | null>(null)
  const may = useRemoteAllowed()
  // A disc's own tools (Disc settings, names of its titles) are WittyDisc's, with its player.
  const discMenu = useModuleItemMenu(it && isDisc(it) ? 'disc' : null, it, !!user?.isAdmin)
  // A phone has one row: the main action, Save, and "⋯" for the rest.
  const phone = useMedia('(max-width: 760px)')
  const load = () => { setBroken(false); return api.get<Item>(`/items/${id}`).then(setIt).catch((e) => { if (e.status >= 500) setBroken(true); else setErr(e.message) }) }
  useEffect(() => { load() }, [id])
  // Coming back from picking a time in the player opens where it was.
  useEffect(() => { if (params.get('edit')) { const p = new URLSearchParams(params); p.delete('edit'); setParams(p, { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps
  if (broken) return <LoadError what={t('This title')} onRetry={load} />
  if (err) return <EmptyState icon={<IconInfo />} title={t('This title isn’t available')} text={t('It may have been removed, or this profile may not be allowed to see it.')} action={<Link className="btn" to="/">{t('Back to home')}</Link>} />
  if (!it) return <PageSkeleton shape="item" />
  // A book's page is WittyInk's server-driven one; this is only drawn when there is none: read it.
  if (it.kind === 'book') return <Navigate replace to={`/read/${it.id}`} />
  // Pictures and creator videos have their own places.
  const libType = libraries.find((l) => l.id === it.libraryId)?.type
  if (libType === 'photos') return <Navigate replace to={`/library/${it.libraryId}?view=${it.id}`} />
  if (libType === 'shorts') return <Navigate replace to={`/library/${it.libraryId}?v=${it.id}`} />
  if (libType === 'channels' || libType === 'musicvideos') return <Navigate replace to={`/watch/${it.id}`} />
  if (libType === 'radio') return <Navigate replace to={`/radio/${it.id}`} />
  if (it.kind === 'game' && it.console === 'rig') return <Navigate to={`/rigs/game/${it.id}`} replace /> // a gaming PC's game: WittyRigs' page
  // A game's page is WittyPlay's (pages/PlayGame.tsx): from a search, a shelf or a link, it opens there, where it's played.
  if (it.kind === 'game') return <Navigate to={`/game/${it.id}`} replace /> // WittyPlay's game page
  if (it.kind === 'cd-image') return <Navigate replace to={`/cd/${it.id}`} />
  // Songs, chapters and episodes live on their album, book or podcast page.
  if (it.kind === 'audio') return <Navigate replace to={`/album/${it.libraryId}?artist=${encodeURIComponent(it.albumArtist || it.artist || '')}&album=${encodeURIComponent(it.album || '')}`} />
  const disc = isDisc(it)
  const resume = it.progress?.resumeSec ? it.progress : null
  const main = it.disc?.titles.find((x) => x.number === it.disc?.mainTitle)
  const sizeGB = it.sizeBytes ? t('{size} GB', { size: num(it.sizeBytes / 1e9, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) }) : ''
  const start = dvdStartMode(it)
  const fx = insertFxOn(it)
  const playable = !(disc && it.disc?.scrambled)
  const discHint = (start === 'menu' ? t('Insert {disc} starts with the disc menu', { disc: discName(it) }) : t('Insert {disc} starts with the main feature', { disc: discName(it) }))
    + (fx ? ' · ' + t('with the tray effect') : '') + (user?.isAdmin ? ' — ' + t('change it in Disc settings.') : '.')
  // What a phone puts behind "⋯": everything but the main action and Save.
  const rest: { key: string; icon: React.ReactNode; label: string; run: () => void; disabled?: boolean }[] = disc ? [
    resume
      ? { key: 'resume', icon: <IconPlay />, label: t('Resume from {time}', { time: fmtTime(resume.positionSec) }), disabled: !playable, run: () => nav(`/play/${it.id}?mode=title&ttn=${resume.discTitle || it.disc?.mainTitle || 1}`) }
      : { key: 'title', icon: <IconPlay />, label: it.disc?.content === 'tv' ? t('Play from the start') : t('Play movie'), disabled: !playable, run: () => nav(`/play/${it.id}?mode=title&start=0`) },
    ...(it.disc?.hasMenus ? [{ key: 'menu', icon: <IconMenu />, label: t('Main menu'), disabled: !playable, run: () => nav(`/play/${it.id}?mode=menu`) }] : []),
    { key: 'device', icon: <IconDevices />, label: t('Play on…'), disabled: !playable, run: () => setSendTo('device') },
    ...discMenu.entries.map((e) => ({ key: e.key, icon: e.icon === 'edit' ? <IconEdit /> : <IconSettings />, label: tx(e.label), run: () => discMenu.choose(e) })),
  ] : [
    ...(resume ? [{ key: 'start', icon: <IconPlay />, label: t('Play from start'), run: () => nav(`/play/${it.id}?start=0`) }] : []),
    ...(it.nextId ? [{ key: 'next', icon: <IconNext />, label: t('Next episode'), run: () => nav(`/item/${it.nextId}`) }] : []),
    ...(may.playOn ? [{ key: 'device', icon: <IconDevices />, label: t('Play on…'), run: () => setSendTo('device') }] : []),
    ...(may.party ? [{ key: 'party', icon: <IconUsers />, label: t('Watch party'), run: () => setSendTo('party') }] : []),
  ]
  if (user?.isAdmin) rest.push(
    { key: 'art', icon: <IconImage />, label: t('Artwork'), run: () => setArt(true) },
    { key: 'canvas', icon: <IconFilm />, label: t('Canvas'), run: () => setCanvas(true) },
    { key: 'edit', icon: <IconEdit />, label: t('Edit'), run: () => setEdit('match') },
    ...(disc || it.showId == null ? [{ key: 'refresh', icon: <IconRefresh />, label: t('Refresh'), run: () => setRefreshing(true) }] : []),
  )
  const moreButton = rest.length > 0 && <IconButton label={t('More')} hideTip className="item-more" aria-haspopup="dialog" onClick={() => setMore(true)}><IconMore /></IconButton>
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={backdropUrl(it) ? { backgroundImage: `url(${sized(backdropUrl(it), fitWidth(innerWidth))})` } : undefined} />
        <Suspense fallback={null}><TitleCanvas itemId={it.id} /></Suspense>
        <div className="shade" />
      </div>
      <div className="detail">
        <div>
          {/* A frame from the video: shown whole on a colour of its own. */}
          <div className={'poster' + (isGeneratedArt(it) ? ' generated' : '')} style={isGeneratedArt(it) ? { '--art-hue': artHue(it.title) } as React.CSSProperties : undefined}>
            {it.poster && <img src={sized(posterUrl(it), fitWidth(260))} alt="" />}
            {user?.isAdmin && <PosterMenu target="item" id={it.id} onChanged={load} />}
          </div>
        </div>
        <div>
          {/* Where this belongs: its library, or its show and season. */}
          {it.showId == null && libraries.find((l) => l.id === it.libraryId) && (
            <div className="crumbs"><Link to={`/library/${it.libraryId}`}>{libraries.find((l) => l.id === it.libraryId)!.name}</Link></div>
          )}
          {it.showId != null && (
            <div className="crumbs">
              <Link to={`/show/${it.showId}`}>{it.showTitle}</Link>
              {it.season != null && <><IconRight /><Link to={`/show/${it.showId}?season=${it.season}`}>{it.season === 0 ? t('Specials') : t('Season {number}', { number: it.season })}</Link></>}
              {episodeLabel(it) && <><IconRight /><span>{episodeLabel(it)}</span></>}
            </div>
          )}
          <TitleLogo target="item" id={it.id} name={it.title} version={it.updatedAt} />
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {it.aired ? <span>{it.aired}</span> : it.year && <span>{it.year}</span>}
            {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
            {it.contentRating && <span className="pill">{it.contentRating}</span>}
            {it.rating && <span className="star"><IconStar /> {num(it.rating, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}</span>}
            {(it.genres || []).length > 0 && <span title={it.genres.join(', ')}>{it.genres.slice(0, 4).map((g, i) => <span key={g}>{i > 0 ? ', ' : ''}<Link to={`/library/${it.libraryId}?genre=${encodeURIComponent(g)}`}>{g}</Link></span>)}{it.genres.length > 4 ? '…' : ''}</span>}
          </div>
          {phone && disc ? (
            <div className="actions item-actions disc-actions">
              <button data-tv-first className="btn primary insert" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=${start}${fx ? '&insert=1' : ''}`)}><IconDisc /> {t('Insert {disc}', { disc: discName(it) })}</button>
              {moreButton}
              <div className="disc-hint muted">{discHint}</div>
            </div>
          ) : phone ? (
            <div className="actions item-actions">
              <button data-tv-first className="btn primary" onClick={() => nav(`/play/${it.id}`)} aria-label={resume ? t('Resume from {time}', { time: fmtTime(resume.positionSec) }) : undefined}>
                <IconPlay /> {resume ? t('Resume') : t('Play')}
              </button>
              <SaveButton itemId={it.id} />
              {moreButton}
            </div>
          ) : disc ? (
            <div className="actions disc-actions">
              <button data-tv-first className="btn primary insert" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=${start}${fx ? '&insert=1' : ''}`)}
                title={start === 'menu' ? t('Start the disc like a {disc} player: first-play, intros, then the main menu', { disc: discName(it) }) : t('Start the disc straight into the main feature')}>
                <IconDisc /> {t('Insert {disc}', { disc: discName(it) })}
              </button>
              {resume
                ? <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&ttn=${resume.discTitle || it.disc?.mainTitle || 1}`)}><IconPlay /> {t('Resume from {time}', { time: fmtTime(resume.positionSec) })}</button>
                : <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&start=0`)}><IconPlay /> {it.disc?.content === 'tv' ? t('Play from the start') : t('Play movie')}</button>}
              {it.disc?.hasMenus && <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=menu`)}><IconMenu /> {t('Main menu')}</button>}
              <button className="btn" aria-haspopup="dialog" disabled={!playable} onClick={() => setSendTo('device')}><IconDevices /> {t('Play on…')}</button>
              {user?.isAdmin && (
                <div className="admin-tools" role="group" aria-label={t('Manage')}>
                  {discMenu.entries.map((e) => <button key={e.key} className="btn ghost square" aria-haspopup="dialog" onClick={() => discMenu.choose(e)} title={tx(e.label)} aria-label={tx(e.label)}>{e.icon === 'edit' ? <IconEdit /> : <IconSettings />}</button>)}
                  <button className="btn ghost square" aria-haspopup="dialog" onClick={() => setArt(true)} title={t('Artwork')} aria-label={t('Artwork')}><IconImage /></button>
                  <button className="btn ghost square" aria-haspopup="dialog" onClick={() => setCanvas(true)} title={t('Canvas: the looping video behind this title')} aria-label={t('Canvas')}><IconFilm /></button>
                  <button className="btn ghost square" onClick={() => setEdit('match')} title={t('Edit details')} aria-label={t('Edit details')}><IconEdit /></button>
                  <button className="btn ghost square" onClick={() => setRefreshing(true)} title={t('Check for new details')} aria-label={t('Check for new details')}><IconRefresh /></button>
                </div>
              )}
              <div className="disc-hint muted">{discHint}</div>
            </div>
          ) : (
            <div className="actions">
              <button data-tv-first className="btn primary" onClick={() => nav(`/play/${it.id}`)}>
                <IconPlay /> {resume ? t('Resume from {time}', { time: fmtTime(resume.positionSec) }) : t('Play')}
              </button>
              {resume && <button className="btn" onClick={() => nav(`/play/${it.id}?start=0`)}><IconPlay /> {t('Play from start')}</button>}
              {it.nextId && <button className="btn" onClick={() => nav(`/item/${it.nextId}`)}><IconNext /> {t('Next episode')}</button>}
              {may.playOn && <button className="btn" aria-haspopup="dialog" onClick={() => setSendTo('device')}><IconDevices /> {t('Play on…')}</button>}
              {may.party && <button className="btn" aria-haspopup="dialog" onClick={() => setSendTo('party')}><IconUsers /> {t('Watch party')}</button>}
              <SaveButton itemId={it.id} />
              {user?.isAdmin && <button className="btn ghost" aria-haspopup="dialog" onClick={() => setArt(true)}><IconImage /> {t('Artwork')}</button>}
              {user?.isAdmin && <button className="btn ghost" aria-haspopup="dialog" onClick={() => setCanvas(true)} title={t('The looping video behind this title')}><IconFilm /> {t('Canvas')}</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit('match')}><IconEdit /> {t('Edit')}</button>}
              {user?.isAdmin && it.showId == null && <button className="btn ghost" onClick={() => setRefreshing(true)} title={t('See what’s new about this, and choose what to take')}><IconRefresh /> {t('Refresh')}</button>}
            </div>
          )}
          {it.overview && <p className="overview">{it.overview}</p>}
          {it.provider === 'wikidata' && (
            <p className="fine-print">
              {t('Facts from')} <a href={`https://www.wikidata.org/wiki/${it.providerId}`} target="_blank" rel="noreferrer">Wikidata</a>; {t('plot summary from Wikipedia, shared under')} <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.
            </p>
          )}
          {it.provider === 'tvmaze' && <p className="fine-print">{t('TV information from')} <a href={`https://www.tvmaze.com/shows/${it.providerId}`} target="_blank" rel="noreferrer">TVmaze</a>, {t('shared under')} <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>}
          {it.provider === 'tvdb' && <p className="fine-print">{t('Metadata provided by')} <a href="https://thetvdb.com" target="_blank" rel="noreferrer">TheTVDB</a>. {t('Please consider adding missing information or subscribing.')}</p>}
          {it.provider === 'anilist' && <p className="fine-print">{t('Anime information from')} <a href={`https://anilist.co/anime/${it.providerId}`} target="_blank" rel="noreferrer">AniList</a>.</p>}
          {(it.provider === 'tmdb' || it.provider === 'tmdb-tv') && <p className="fine-print">{t('Details from')} <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer">TMDB</a>. {t('This product uses the TMDB API but is not endorsed or certified by TMDB.')}</p>}
          {disc && it.disc?.scrambled && <div className="error">{t('This disc is encrypted, so it can’t be played: encrypted media isn’t supported, and nothing here bypasses copy protection.')}</div>}
          <div className="facts">
            {it.width > 0 && <div className="fact"><div className="k">{t('Video')}</div><div className="v">{it.width}×{it.height} {codecName(it.videoCodec)}{disc && main ? ` · ${isBluray(it) ? main.standard : main.standard.toUpperCase()} ${main.wide ? '16:9' : '4:3'}` : ''}</div></div>}
            {(it.audioLangs || []).length > 0 && <div className="fact"><div className="k">{t('Audio')}</div><div className="v">{[...new Set(it.audioLangs.map(langName))].join(', ')}</div></div>}
            {(it.subLangs || []).length > 0 && <div className="fact"><div className="k">{t('Subtitles')}</div><div className="v">{[...new Set(it.subLangs.map(langName))].join(', ')}</div></div>}
            <div className="fact"><div className="k">{t('Source')}</div><div className="v">{disc ? (it.kind.endsWith('-iso') ? t('{disc} image', { disc: discName(it) }) : t('{disc} folder', { disc: discName(it) })) : it.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
            {disc && it.disc && <div className="fact"><div className="k">{t('Disc')}</div><div className="v">{it.disc.label} · {tn('{count} title', '{count} titles', it.disc.titles.length)}{it.disc.hasMenus ? ' · ' + t('menus') : ''}</div></div>}
            {user?.isAdmin && <div className="fact wide"><div className="k">{t('File')}</div><div className="v">{it.path}</div></div>}
          </div>

          <TitleExtras target="item" id={it.id} libraryId={it.libraryId} />
          {it.showId == null && <Suspense fallback={null}><WhereToWatch itemId={it.id} /></Suspense>}
          <div className="page-back">{it.showId != null
            ? <BackLink to={`/show/${it.showId}?season=${it.season ?? -1}`} label={t('Back to {name}', { name: it.showTitle || '' })} />
            : <BackLink to={`/library/${it.libraryId}`} label={t('Back to library')} />}</div>
          <LegalNote kind="metadata" />
        </div>
      </div>
      {more && (
        <RowSheet title={it.title} onClose={() => setMore(false)}>
          <Rows>{rest.map((r) => <SheetRow key={r.key} icon={r.icon} label={r.label} disabled={r.disabled} onSelect={r.run} />)}</Rows>
        </RowSheet>
      )}
      {sendTo && (
        <Suspense fallback={null}>
          {sendTo === 'device'
            ? <PlayOn source={{ kind: 'item', itemId: it.id, itemKind: it.kind, title: it.title }} onClose={() => setSendTo(null)} />
            : <WatchParty item={{ id: it.id, title: it.title, kind: it.kind, showId: it.showId }} onClose={() => setSendTo(null)} />}
        </Suspense>
      )}
      {edit && <EditDetails target="item" id={it.id} start={edit} onClose={() => setEdit('')} onChanged={load} />}
      {art && <TitleArtPicker target="item" id={it.id} onClose={() => setArt(false)} onApplied={appliedOnPage(load)} />}
      {canvas && <Suspense fallback={null}><CanvasManager itemId={it.id} title={it.title} kind={disc ? 'disc' : 'video'} runtimeSec={it.runtimeSec} onClose={() => setCanvas(false)} /></Suspense>}
      {refreshing && <RefreshDialog target="item" id={it.id} onClose={() => setRefreshing(false)} onChanged={load} />}
      {discMenu.dialog(() => { api.get<Item>(`/items/${it.id}`).then(setIt).catch(() => {}) })}
    </>
  )
}
