import { SaveButton } from '../components/SaveToCollection'
import RatingField from '../components/RatingField'
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, episodeLabel, fmtRuntime, fmtTime, getPrefs, posterUrl, seasonPosterUrl, showBackdropUrl, showPosterUrl, type Item, type Prefs, type ShowDetail } from '../api'
import ArtworkPicker from '../components/ArtworkPicker'
import { BackLink, EmptyState, SeenBadge, Spinner } from '../components/common'
import { IconCheck, IconEdit, IconImage, IconInfo, IconPlay, IconRefresh } from '../icons'
import { toast, useApp } from '../state'

export default function ShowPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const [d, setD] = useState<ShowDetail | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const [art, setArt] = useState(false)
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [viewOverride, setViewOverride] = useState<'' | 'cards' | 'list' | null>(null)
  const load = () => api.get<ShowDetail>(`/shows/${id}`).then(setD).catch((e) => setErr(e.message))
  useEffect(() => { load(); setViewOverride(null) }, [id])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  if (err) return <EmptyState icon={<IconInfo />} title="This show didn’t load" text={err} />
  if (!d) return <Spinner />
  const { show, seasons, episodes } = d
  const seasonParam = params.get('season')
  const season = seasonParam != null ? Number(seasonParam) : (episodes.find((e) => e.id === d.nextUpId)?.season ?? seasons.find((s) => s.number > 0)?.number ?? seasons[0]?.number ?? 0)
  const eps = episodes.filter((e) => (e.season ?? -1) === season || (season === -1 && e.season == null))
  const next = episodes.find((e) => e.id === d.nextUpId)
  const nextResume = next?.progress?.resumeSec ? next.progress : null
  const refresh = async () => {
    try { await api.put(`/shows/${show.id}`, { refresh: true }); await load(); toast('Updated from TheTVDB') } catch (e: any) { toast(e.message) }
  }
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
        <div className="bg" style={{ backgroundImage: `url(${showBackdropUrl(show)})` }} />
        <div className="shade" />
      </div>
      <div className="detail">
        <div><div className="poster"><img src={showPosterUrl(show)} alt="" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} /></div></div>
        <div>
          <h1>{show.title}</h1>
          <div className="meta">
            {show.year && <span>{show.year}</span>}
            {show.contentRating && <span className="pill">{show.contentRating}</span>}
            {show.network && <span>{show.network}</span>}
            {show.status && <span>{show.status}</span>}
            <span>{show.seasonCount} {show.seasonCount === 1 ? 'season' : 'seasons'} · {show.episodeCount} episodes</span>
            {(show.genres || []).length > 0 && <span>{show.genres.join(', ')}</span>}
          </div>
          <div className="actions">
            {next && <button className="btn primary" onClick={() => nav(`/play/${next.id}`)}>
              <IconPlay /> {nextResume ? `Resume ${episodeLabel(next)} from ${fmtTime(nextResume.positionSec)}` : `Play ${episodeLabel(next) || next.title}`}
            </button>}
            <SaveButton showId={show.id} />
            {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setArt(true)}><IconImage /> Artwork</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={refresh} title="Fetch metadata from TheTVDB"><IconRefresh /> Refresh</button>}
          </div>
          {show.overview && <p className="overview">{show.overview}</p>}

          <div className="season-tabs">
            {seasons.map((s) => (
              <button key={s.number} className={'season-tab' + (s.number === season ? ' active' : '')} onClick={() => setParams({ season: String(s.number) }, { replace: true })}>
                <div className="season-poster">{<img src={seasonPosterUrl(show, s.number)} alt="" loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />}</div>
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
          {show.provider === 'tvdb' && <p className="fine-print">Metadata provided by TheTVDB. Please consider adding missing information or subscribing.</p>}
          {show.provider === 'anilist' && <p className="fine-print">Anime information from <a href={`https://anilist.co/anime/${show.providerId}`} target="_blank" rel="noreferrer">AniList</a>.</p>}
          {show.provider === 'tvmaze' && <p className="fine-print">TV information from <a href={`https://www.tvmaze.com/shows/${show.providerId}`} target="_blank" rel="noreferrer">TVmaze</a>, shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>}
        </div>
      </div>
      {edit && <ShowEditor show={show} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
      {art && <ArtworkPicker base={`/shows/${show.id}`} onClose={() => setArt(false)}
        onPick={async (kind, url) => { await api.put(`/shows/${show.id}`, { [kind]: url }); await load(); toast(kind === 'poster' ? 'Poster updated' : 'Backdrop updated') }} />}
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
              <td className="thumb-cell">{e.poster ? <img src={posterUrl(e)} alt="" loading="lazy" /> : <span />}</td>
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
    <div className="episode" onClick={() => nav(`/item/${ep.id}`)}>
      {/* Real controls inside the clickable row: a remote or keyboard can reach both. */}
      <button type="button" className="thumb" aria-label={`Play ${ep.title}`} onClick={(e) => { e.stopPropagation(); nav(`/play/${ep.id}`) }}>
        {ep.poster && <img src={posterUrl(ep)} alt="" loading="lazy" />}
        <span className="play"><span><IconPlay /></span></span>
        {pct > 0 && <span className="progress"><i style={{ width: pct + '%' }} /></span>}
        {ep.progress?.watched && <SeenBadge />}
      </button>
      <div className="info">
        <Link className="t" to={`/item/${ep.id}`} onClick={(e) => e.stopPropagation()}><span className="num">{episodeLabel(ep)}</span> {ep.title}</Link>
        <div className="sub">{[ep.aired, fmtRuntime(ep.runtimeSec)].filter(Boolean).join(' · ')}</div>
        {ep.overview && <p>{ep.overview}</p>}
      </div>
    </div>
  )
}

function ShowEditor({ show, onClose, onSaved }: { show: ShowDetail['show']; onClose: () => void; onSaved: () => void }) {
  const mmss = (v?: number) => (v != null ? fmtTime(v) : '')
  const [f, setF] = useState({ title: show.title, year: show.year ? String(show.year) : '', overview: show.overview, genres: (show.genres || []).join(', '), tvdbId: '', contentRating: show.contentRating || '', network: show.network || '',
    introStart: mmss(show.introStart), introEnd: mmss(show.introEnd) })
  const secs = (s: string) => { if (!s.trim()) return -1; const p = s.trim().split(':').map(Number); return p.reduce((a, x) => a * 60 + (x || 0), 0) }
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try {
      await api.put(`/shows/${show.id}`, f.tvdbId ? { tvdbId: f.tvdbId } : {
        title: f.title, year: f.year ? Number(f.year) : null, overview: f.overview, genres: f.genres.split(',').map((s) => s.trim()).filter(Boolean), contentRating: f.contentRating, network: f.network,
        introStart: Math.max(0, secs(f.introStart)), introEnd: secs(f.introEnd),
      })
      onSaved()
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const up = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value })
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Edit show</h2>
        <div className="form full">
          <div className="row2">
            <label>Title<input className="input" value={f.title} onChange={up('title')} /></label>
            <label>Year<input className="input" value={f.year} onChange={up('year')} /></label>
          </div>
          <label>Overview<textarea className="input" rows={5} value={f.overview} onChange={up('overview')} /></label>
          <label>Genres (comma separated)<input className="input" value={f.genres} onChange={up('genres')} /></label>
          <RatingField tv value={f.contentRating} onChange={(v) => setF({ ...f, contentRating: v })} />
          <label>Network<input className="input" value={f.network} onChange={up('network')} placeholder="The network it first aired on" /></label>
          <div className="row2">
            <label>Intro starts at <span className="muted">(m:ss)</span><input className="input" value={f.introStart} onChange={up('introStart')} placeholder="0:00" /></label>
            <label>Intro ends at <span className="muted">(m:ss — empty: no Skip intro)</span><input className="input" value={f.introEnd} onChange={up('introEnd')} placeholder="1:05" /></label>
          </div>
          <div className="hint">Used for episodes without an “Intro”/“Opening” chapter; files with such a chapter are detected automatically.</div>
          <label>Match by {show.provider === 'tvmaze' ? 'TVmaze' : show.provider === 'anilist' ? 'AniList' : 'TheTVDB'} id <span className="muted">(optional — fixes a wrong match; current: {show.providerId || 'none'})</span>
            <input className="input" value={f.tvdbId} onChange={up('tvdbId')} placeholder="e.g. 76163" />
          </label>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save} disabled={busy}>{f.tvdbId ? 'Match & refresh' : 'Save'}</button></div>
      </div>
    </div>
  )
}
