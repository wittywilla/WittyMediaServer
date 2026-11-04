import { lazy, Suspense, useEffect, useState } from 'react'
import { Link, Navigate, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, backdropUrl, codecName, dvdStartMode, episodeLabel, fmtRuntime, fmtTime, getPrefs, insertFxOn, isDisc, langName, posterUrl, type Item, type Prefs, discName, isBluray } from '../api'
import DiscSections from './DiscSections'
import EditDetails, { type EditSection } from '../components/edit/EditDetails'
import RefreshDialog from '../components/edit/RefreshDialog'
import TitleArtPicker, { PosterMenu, appliedOnPage } from '../components/edit/TitleArtPicker'
import TitleExtras, { TitleLogo } from '../components/edit/TitleExtras'
import { BookDetails } from './Books'
import { SaveButton } from '../components/SaveToCollection'

const GameDetails = lazy(() => import('./games/Details'))
const RigGameDetails = lazy(() => import('./Rigs').then((m) => ({ default: m.RigGameDetails })))
import { BackLink, EmptyState, LoadError, Spinner, artHue, isGeneratedArt } from '../components/common'
import { IconDisc, IconEdit, IconImage, IconInfo, IconMenu, IconNext, IconPlay, IconRefresh, IconRight, IconSettings, IconStar } from '../icons'
import { toast, useApp } from '../state'
import LegalNote from '../components/LegalNote'

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
  const [discSettings, setDiscSettings] = useState(false)
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [broken, setBroken] = useState(false)
  const load = () => { setBroken(false); return api.get<Item>(`/items/${id}`).then(setIt).catch((e) => { if (e.status >= 500) setBroken(true); else setErr(e.message) }) }
  useEffect(() => { load() }, [id])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  // Coming back from picking a time in the player opens where it was.
  useEffect(() => { if (params.get('edit')) { const p = new URLSearchParams(params); p.delete('edit'); setParams(p, { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps
  if (broken) return <LoadError what="This title" onRetry={load} />
  if (err) return <EmptyState icon={<IconInfo />} title="This title isn’t available" text="It may have been removed, or this profile may not be allowed to see it." action={<Link className="btn" to="/">Back to home</Link>} />
  if (!it) return <Spinner />
  if (it.kind === 'book') return <BookDetails it={it} />
  // Pictures and creator videos have their own places.
  const libType = libraries.find((l) => l.id === it.libraryId)?.type
  if (libType === 'photos') return <Navigate replace to={`/library/${it.libraryId}?view=${it.id}`} />
  if (libType === 'shorts') return <Navigate replace to={`/library/${it.libraryId}?v=${it.id}`} />
  if (libType === 'channels' || libType === 'musicvideos') return <Navigate replace to={`/watch/${it.id}`} />
  if (libType === 'radio') return <Navigate replace to={`/radio/${it.id}`} />
  if (it.kind === 'game' && it.console === 'rig') return <Suspense fallback={<Spinner />}><RigGameDetails it={it} /></Suspense>
  if (it.kind === 'game') return <Suspense fallback={<Spinner />}><GameDetails it={it} reload={load} /></Suspense>
  if (it.kind === 'cd-image') return <Navigate replace to={`/cd/${it.id}`} />
  // Songs, chapters and episodes live on their album, book or podcast page.
  if (it.kind === 'audio') return <Navigate replace to={`/album/${it.libraryId}?artist=${encodeURIComponent(it.albumArtist || it.artist || '')}&album=${encodeURIComponent(it.album || '')}`} />
  const disc = isDisc(it)
  const resume = it.progress?.resumeSec ? it.progress : null
  const main = it.disc?.titles.find((t) => t.number === it.disc?.mainTitle)
  const sizeGB = it.sizeBytes ? (it.sizeBytes / 1e9).toFixed(2) + ' GB' : ''
  const start = dvdStartMode(it)
  const fx = insertFxOn(it)
  const playable = !(disc && it.disc?.scrambled)
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={backdropUrl(it) ? { backgroundImage: `url(${backdropUrl(it)})` } : undefined} />
        <div className="shade" />
      </div>
      <div className="detail">
        <div>
          {/* A frame from the video: shown whole on a colour of its own. */}
          <div className={'poster' + (isGeneratedArt(it) ? ' generated' : '')} style={isGeneratedArt(it) ? { '--art-hue': artHue(it.title) } as React.CSSProperties : undefined}>
            {it.poster && <img src={posterUrl(it)} alt="" />}
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
              {it.season != null && <><IconRight /><Link to={`/show/${it.showId}?season=${it.season}`}>{it.season === 0 ? 'Specials' : `Season ${it.season}`}</Link></>}
              {episodeLabel(it) && <><IconRight /><span>{episodeLabel(it)}</span></>}
            </div>
          )}
          <TitleLogo target="item" id={it.id} name={it.title} version={it.updatedAt} />
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {it.aired ? <span>{it.aired}</span> : it.year && <span>{it.year}</span>}
            {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
            {it.contentRating && <span className="pill">{it.contentRating}</span>}
            {it.rating && <span className="star"><IconStar /> {it.rating.toFixed(1)}</span>}
            {(it.genres || []).length > 0 && <span title={it.genres.join(', ')}>{it.genres.slice(0, 4).map((g, i) => <span key={g}>{i > 0 ? ', ' : ''}<Link to={`/library/${it.libraryId}?genre=${encodeURIComponent(g)}`}>{g}</Link></span>)}{it.genres.length > 4 ? '…' : ''}</span>}
          </div>
          {disc ? (
            <div className="actions disc-actions">
              <button data-tv-first className="btn primary insert" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=${start}${fx ? '&insert=1' : ''}`)}
                title={start === 'menu' ? `Start the disc like a ${discName(it)} player: first-play, intros, then the main menu` : 'Start the disc straight into the main feature'}>
                <IconDisc /> Insert {discName(it)}
              </button>
              {resume
                ? <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&ttn=${resume.discTitle || it.disc?.mainTitle || 1}`)}><IconPlay /> Resume from {fmtTime(resume.positionSec)}</button>
                : <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&start=0`)}><IconPlay /> {it.disc?.content === 'tv' ? 'Play from the start' : 'Play movie'}</button>}
              {it.disc?.hasMenus && <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=menu`)}><IconMenu /> Main menu</button>}
              {user?.isAdmin && (
                <div className="admin-tools" role="group" aria-label="Manage">
                  <button className="btn ghost square" onClick={() => setDiscSettings(true)} title="Disc settings" aria-label="Disc settings"><IconSettings /></button>
                  <button className="btn ghost square" aria-haspopup="dialog" onClick={() => setArt(true)} title="Artwork" aria-label="Artwork"><IconImage /></button>
                  <button className="btn ghost square" onClick={() => setEdit('match')} title="Edit details" aria-label="Edit details"><IconEdit /></button>
                  <button className="btn ghost square" onClick={() => setRefreshing(true)} title="Check for new details" aria-label="Check for new details"><IconRefresh /></button>
                </div>
              )}
              <div className="disc-hint muted">Insert {discName(it)} starts with the {start === 'menu' ? 'disc menu' : 'main feature'}{fx ? ' · with the tray effect' : ''}{user?.isAdmin ? ' — change it in Disc settings.' : '.'}</div>
            </div>
          ) : (
            <div className="actions">
              <button data-tv-first className="btn primary" onClick={() => nav(`/play/${it.id}`)}>
                <IconPlay /> {resume ? `Resume from ${fmtTime(resume.positionSec)}` : 'Play'}
              </button>
              {resume && <button className="btn" onClick={() => nav(`/play/${it.id}?start=0`)}><IconPlay /> Play from start</button>}
              {it.nextId && <button className="btn" onClick={() => nav(`/item/${it.nextId}`)}><IconNext /> Next episode</button>}
              <SaveButton itemId={it.id} />
              {user?.isAdmin && <button className="btn ghost" aria-haspopup="dialog" onClick={() => setArt(true)}><IconImage /> Artwork</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit('match')}><IconEdit /> Edit</button>}
              {user?.isAdmin && it.showId == null && <button className="btn ghost" onClick={() => setRefreshing(true)} title="See what's new about this, and choose what to take"><IconRefresh /> Refresh</button>}
            </div>
          )}
          {it.overview && <p className="overview">{it.overview}</p>}
          {it.provider === 'wikidata' && (
            <p className="fine-print">
              Facts from <a href={`https://www.wikidata.org/wiki/${it.providerId}`} target="_blank" rel="noreferrer">Wikidata</a>; plot summary from Wikipedia,
              shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.
            </p>
          )}
          {it.provider === 'tvmaze' && <p className="fine-print">TV information from <a href={`https://www.tvmaze.com/shows/${it.providerId}`} target="_blank" rel="noreferrer">TVmaze</a>, shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>}
          {it.provider === 'tvdb' && <p className="fine-print">Metadata provided by <a href="https://thetvdb.com" target="_blank" rel="noreferrer">TheTVDB</a>. Please consider adding missing information or subscribing.</p>}
          {it.provider === 'anilist' && <p className="fine-print">Anime information from <a href={`https://anilist.co/anime/${it.providerId}`} target="_blank" rel="noreferrer">AniList</a>.</p>}
          {(it.provider === 'tmdb' || it.provider === 'tmdb-tv') && <p className="fine-print">Details from <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer">TMDB</a>. This product uses the TMDB API but is not endorsed or certified by TMDB.</p>}
          {disc && it.disc?.scrambled && <div className="error">This disc is encrypted, so it can't be played: encrypted media isn't supported, and nothing here bypasses copy protection.</div>}
          <div className="facts">
            {it.width > 0 && <div className="fact"><div className="k">Video</div><div className="v">{it.width}×{it.height} {codecName(it.videoCodec)}{disc && main ? ` · ${isBluray(it) ? main.standard : main.standard.toUpperCase()} ${main.wide ? '16:9' : '4:3'}` : ''}</div></div>}
            {(it.audioLangs || []).length > 0 && <div className="fact"><div className="k">Audio</div><div className="v">{[...new Set(it.audioLangs.map(langName))].join(', ')}</div></div>}
            {(it.subLangs || []).length > 0 && <div className="fact"><div className="k">Subtitles</div><div className="v">{[...new Set(it.subLangs.map(langName))].join(', ')}</div></div>}
            <div className="fact"><div className="k">Source</div><div className="v">{disc ? `${discName(it)} ${it.kind.endsWith('-iso') ? 'image' : 'folder'}` : it.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
            {disc && it.disc && <div className="fact"><div className="k">Disc</div><div className="v">{it.disc.label} · {it.disc.titles.length} titles{it.disc.hasMenus ? ' · menus' : ''}</div></div>}
            {user?.isAdmin && <div className="fact wide"><div className="k">File</div><div className="v">{it.path}</div></div>}
          </div>

          <TitleExtras target="item" id={it.id} libraryId={it.libraryId} />
          {disc && <DiscSections item={it} admin={!!user?.isAdmin} playable={playable} />}
          <div className="page-back">{it.showId != null
            ? <BackLink to={`/show/${it.showId}?season=${it.season ?? -1}`} label={`Back to ${it.showTitle}`} />
            : <BackLink to={`/library/${it.libraryId}`} label="Back to library" />}</div>
          <LegalNote kind="metadata" />
        </div>
      </div>
      {edit && <EditDetails target="item" id={it.id} start={edit} onClose={() => setEdit('')} onChanged={load} />}
      {art && <TitleArtPicker target="item" id={it.id} onClose={() => setArt(false)} onApplied={appliedOnPage(load)} />}
      {refreshing && <RefreshDialog target="item" id={it.id} onClose={() => setRefreshing(false)} onChanged={load} />}
      {discSettings && <DiscSettings item={it} prefs={prefs} onClose={() => setDiscSettings(false)} onSaved={(x) => { setIt(x); setDiscSettings(false); toast('Disc settings saved') }} />}
    </>
  )
}

function DiscSettings({ item, prefs, onClose, onSaved }: { item: Item; prefs: Prefs | null; onClose: () => void; onSaved: (it: Item) => void }) {
  const [playMode, setPlayMode] = useState(item.playMode)
  const [insertFx, setInsertFx] = useState(item.insertFx)
  const [content, setContent] = useState(item.disc?.contentManual ? item.disc.content || '' : '')
  const [err, setErr] = useState('')
  const save = async () => {
    try { onSaved(await api.put<Item>(`/items/${item.id}`, { playMode, insertFx, discContent: content })) } catch (e: any) { setErr(e.message) }
  }
  const guessed = !item.disc?.contentManual && item.disc?.content === 'tv' ? 'TV series' : 'movie'
  const def = prefs?.dvdStart === 'title' ? 'main feature' : 'disc menu'
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Disc settings</h2>
        <div className="form full">
          <label>What’s on this disc
            <select className="select" value={content} onChange={(e) => setContent(e.target.value)}>
              <option value="">Work it out from the name ({item.disc?.contentManual ? 'automatic' : guessed})</option>
              <option value="movie">A movie</option>
              <option value="tv">Episodes of a TV series</option>
            </select>
            <span className="hint">Sorts the disc under Movies or TV series in its library. Discs of a series aren’t looked up as movies.</span>
          </label>
          <label>When this disc is inserted
            <select className="select" value={playMode} onChange={(e) => setPlayMode(e.target.value as Item['playMode'])}>
              <option value="">Server default ({def})</option>
              <option value="menu" disabled={!item.disc?.hasMenus}>Start with the disc menu (like a {discName(item)} player)</option>
              <option value="title">Skip menus and play the main feature</option>
            </select>
          </label>
          <label>Insert {discName(item)} effect (tray animation and sound)
            <select className="select" value={insertFx} onChange={(e) => setInsertFx(e.target.value as Item['insertFx'])}>
              <option value="">Server default ({prefs?.insertFx === false ? 'off' : 'on'})</option>
              <option value="on">On for this disc</option>
              <option value="off">Off for this disc</option>
            </select>
          </label>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></div>
      </div>
    </div>
  )
}
