import { SaveButton } from '../components/SaveToCollection'
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, episodeLabel, fitWidth, fmtRuntime, fmtTime, getPrefs, posterUrl, sized, seasonPosterUrl, showBackdropUrl, showPosterUrl, type Item, type Prefs, type ShowDetail } from '../api'
import EditDetails, { type EditSection } from '../components/edit/EditDetails'
import RefreshDialog from '../components/edit/RefreshDialog'
import TitleExtras, { TitleLogo } from '../components/edit/TitleExtras'
import BrandLogo from '../components/BrandLogo'
import { BackLink, EmptyState, SeenBadge, Spinner } from '../components/common'
import { IconCheck, IconEdit, IconImage, IconInfo, IconPlay, IconRefresh } from '../icons'
import { toast, useApp } from '../state'
import LegalNote from '../components/LegalNote'
import LogoNotice from '../components/LogoNotice'

export default function ShowPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const [d, setD] = useState<ShowDetail | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState<EditSection | ''>((params.get('edit') as EditSection) || '')
  const [refreshing, setRefreshing] = useState(false)
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [viewOverride, setViewOverride] = useState<'' | 'cards' | 'list' | null>(null)
  const load = () => api.get<ShowDetail>(`/shows/${id}`).then(setD).catch((e) => setErr(e.message))
  useEffect(() => { load(); setViewOverride(null) }, [id])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  // Coming back from picking a time in the player opens where it was.
  useEffect(() => { if (params.get('edit')) { const p = new URLSearchParams(params); p.delete('edit'); setParams(p, { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps
  if (err) return <EmptyState icon={<IconInfo />} title="This show didn’t load" text={err} />
  if (!d) return <Spinner />
  const { show, seasons, episodes } = d
  const seasonParam = params.get('season')
  const season = seasonParam != null ? Number(seasonParam) : (episodes.find((e) => e.id === d.nextUpId)?.season ?? seasons.find((s) => s.number > 0)?.number ?? seasons[0]?.number ?? 0)
  const eps = episodes.filter((e) => (e.season ?? -1) === season || (season === -1 && e.season == null))
  const next = episodes.find((e) => e.id === d.nextUpId)
  const nextResume = next?.progress?.resumeSec ? next.progress : null
  const unmatched = episodes.filter((e) => e.season == null)
  // Layout: this show's own choice, else the server-wide default.
  const showView = viewOverride ?? show.view
  const view = showView || prefs?.episodeView || 'cards'
  const setView = async (v: '' | 'cards' | 'list') => {
    setViewOverride(v)
    if (user?.isAdmin) { try { await api.put(`/shows/${show.id}/view`, { view: v }) } catch (e: any) { toast(e.message) } }
  }
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={{ backgroundImage: `url(${sized(showBackdropUrl(show), fitWidth(innerWidth))})` }} />
        <div className="shade" />
      </div>
      <div className="detail">
        <div><div className="poster"><img src={sized(showPosterUrl(show), fitWidth(260))} alt="" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} /></div></div>
        <div>
          <TitleLogo target="show" id={show.id} name={show.title} version={show.updatedAt} />
          <div className="meta">
            {show.year && <span>{show.year}</span>}
            {show.contentRating && <span className="pill">{show.contentRating}</span>}
            {show.network && <span><Link className="meta-brand" to={`/brand/network/${encodeURIComponent(show.network)}?library=${show.libraryId}`} aria-label={show.network}><BrandLogo kind="network" name={show.network} label={false} /></Link></span>}
            {show.status && <span>{show.status}</span>}
            <span>{show.seasonCount} {show.seasonCount === 1 ? 'season' : 'seasons'} · {show.episodeCount} episodes</span>
            {(show.genres || []).length > 0 && <span>{show.genres.join(', ')}</span>}
          </div>
          <div className="actions">
            {next && <button data-tv-first className="btn primary" onClick={() => nav(`/play/${next.id}`)}>
              <IconPlay /> {nextResume ? `Resume ${episodeLabel(next)} from ${fmtTime(nextResume.positionSec)}` : `Play ${episodeLabel(next) || next.title}`}
            </button>}
            <SaveButton showId={show.id} />
            {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit('match')}><IconEdit /> Edit</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit('artwork')}><IconImage /> Artwork</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setRefreshing(true)} title="See what's new about this, and choose what to take"><IconRefresh /> Refresh</button>}
          </div>
          {show.overview && <p className="overview">{show.overview}</p>}
          <TitleExtras target="show" id={show.id} libraryId={show.libraryId} />

          <div className="season-tabs">
            {seasons.map((s) => (
              <button key={s.number} className={'season-tab' + (s.number === season ? ' active' : '')} onClick={() => setParams({ season: String(s.number) }, { replace: true })}>
                <div className="season-poster">{<img src={sized(seasonPosterUrl(show, s.number), fitWidth(200))} alt="" loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />}</div>
                <div className="t">{s.title}</div>
                <div className="sub">{s.episodeCount} ep{s.unwatched > 0 && s.unwatched < s.episodeCount ? ` · ${s.unwatched} left` : s.unwatched === 0 ? ' · watched' : ''}</div>
              </button>
            ))}
            {unmatched.length > 0 && <button className={'season-tab' + (season === -1 ? ' active' : '')} onClick={() => setParams({ season: '-1' }, { replace: true })}><div className="season-poster" /><div className="t">Other videos</div><div className="sub">{unmatched.length}</div></button>}
          </div>

          <div className="view-toggle">
            <span className="muted">Layout</span>
            <div className="seg">
              <button className={view === 'cards' ? 'active' : ''} onClick={() => setView('cards')}>Cards</button>
              <button className={view === 'list' ? 'active' : ''} onClick={() => setView('list')}>Playlist</button>
            </div>
            {showView && <button className="btn sm ghost" onClick={() => setView('')} title="Use the layout set in Settings">Use default</button>}
          </div>
          {view === 'list' ? <EpisodeTable eps={eps} /> : (
            <div className="episodes">
              {eps.map((e) => <EpisodeRow key={e.id} ep={e} />)}
            </div>
          )}
          <div className="page-back"><BackLink to={`/library/${show.libraryId}`} label="Back to library" /></div>
          {show.provider === 'tvdb' && <p className="fine-print">Metadata provided by <a href="https://thetvdb.com" target="_blank" rel="noreferrer">TheTVDB</a>. Please consider adding missing information or subscribing.</p>}
          {show.provider === 'tmdb-tv' && <p className="fine-print">Details from <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer">TMDB</a>. This product uses the TMDB API but is not endorsed or certified by TMDB.</p>}
          {show.provider === 'anilist' && <p className="fine-print">Anime information from <a href={`https://anilist.co/anime/${show.providerId}`} target="_blank" rel="noreferrer">AniList</a>.</p>}
          {show.provider === 'tvmaze' && <p className="fine-print">TV information from <a href={`https://www.tvmaze.com/shows/${show.providerId}`} target="_blank" rel="noreferrer">TVmaze</a>, shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>}
          <LegalNote kind="metadata" />
          {show.network && <LogoNotice logos={[{ kind: 'network', name: show.network }]} />}
        </div>
      </div>
      {edit && <EditDetails target="show" id={show.id} start={edit} onClose={() => setEdit('')} onChanged={load} />}
      {refreshing && <RefreshDialog target="show" id={show.id} onClose={() => setRefreshing(false)} onChanged={load} />}
    </>
  )
}

// Compact "playlist" view, like a disc's title list.
function EpisodeTable({ eps }: { eps: Item[] }) {
  const nav = useNavigate()
  return (
    <table className="titles-table episode-table">
      <thead><tr><th></th><th>#</th><th>Title</th><th>Aired</th><th>Length</th><th></th><th></th></tr></thead>
      <tbody>
        {eps.map((e) => {
          const p = e.progress
          const state = p?.watched ? <IconCheck size={16} /> : p?.resumeSec ? fmtTime(p.positionSec) : ''
          return (
            <tr key={e.id} onClick={() => nav(`/item/${e.id}`)}>
              <td className="thumb-cell">{e.poster ? <img src={sized(posterUrl(e), fitWidth(160))} alt="" loading="lazy" /> : <span />}</td>
              <td className="num">{episodeLabel(e).replace(/^S\d+ \u00b7 /, '') || '\u2013'}</td>
              <td><Link to={`/item/${e.id}`} onClick={(ev) => ev.stopPropagation()}>{e.title}</Link></td>
              <td className="muted">{e.aired || ''}</td>
              <td>{e.runtimeSec ? fmtTime(e.runtimeSec) : ''}</td>
              <td className="muted">{state}</td>
              <td><button className="btn sm" onClick={(ev) => { ev.stopPropagation(); nav(`/play/${e.id}`) }}><IconPlay /> Play</button></td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

function EpisodeRow({ ep }: { ep: Item }) {
  const nav = useNavigate()
  const pct = ep.progress && ep.progress.durationSec > 0 && !ep.progress.watched ? Math.min(100, (ep.progress.positionSec / ep.progress.durationSec) * 100) : 0
  return (
    <div className="episode">
      {/* The title's link covers the whole row, so a click anywhere opens it; a remote or keyboard reaches both controls. */}
      <button type="button" className="thumb" aria-label={`Play ${ep.title}`} onClick={() => nav(`/play/${ep.id}`)}>
        {ep.poster && <img src={sized(posterUrl(ep), fitWidth(320))} alt="" loading="lazy" />}
        <span className="play"><span><IconPlay /></span></span>
        {pct > 0 && <span className="progress"><i style={{ width: pct + '%' }} /></span>}
        {ep.progress?.watched && <SeenBadge />}
      </button>
      <div className="info">
        <Link className="t" to={`/item/${ep.id}`}><span className="num">{episodeLabel(ep)}</span> {ep.title}</Link>
        <div className="sub">{[ep.aired, fmtRuntime(ep.runtimeSec)].filter(Boolean).join(' · ')}</div>
        {ep.overview && <p>{ep.overview}</p>}
      </div>
    </div>
  )
}
