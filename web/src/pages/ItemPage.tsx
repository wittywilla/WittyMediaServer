import RatingField from '../components/RatingField'
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, backdropUrl, dvdStartMode, episodeLabel, fmtRuntime, fmtTime, getPrefs, insertFxOn, isDisc, langName, posterUrl, type Item, type Prefs } from '../api'
import ArtworkPicker from '../components/ArtworkPicker'
import DiscSections from './DiscSections'
import { BookDetails } from './Books'
import { Spinner, isGeneratedArt } from '../components/common'
import { IconDisc, IconEdit, IconImage, IconMenu, IconNext, IconPlay, IconRefresh, IconSettings } from '../icons'
import { toast, useApp } from '../state'

export default function ItemPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user } = useApp()
  const [it, setIt] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const [art, setArt] = useState(false)
  const [discSettings, setDiscSettings] = useState(false)
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const load = () => api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message))
  useEffect(() => { load() }, [id])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  if (err) return <div className="error">{err}</div>
  if (!it) return <Spinner />
  if (it.kind === 'book') return <BookDetails it={it} />
  const disc = isDisc(it)
  const resume = it.progress && !it.progress.watched && it.progress.positionSec > 60 ? it.progress : null
  const main = it.disc?.titles.find((t) => t.number === it.disc?.mainTitle)
  const refresh = async () => {
    try { setIt(await api.put<Item>(`/items/${it.id}`, { refresh: true })); toast('Metadata refreshed') } catch (e: any) { toast(e.message) }
  }
  const sizeGB = it.sizeBytes ? (it.sizeBytes / 1e9).toFixed(2) + ' GB' : ''
  const start = dvdStartMode(it, prefs)
  const fx = insertFxOn(it, prefs)
  const playable = !(disc && it.disc?.scrambled)
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={{ backgroundImage: `url(${backdropUrl(it)})` }} />
        <div className="shade" />
      </div>
      <div className="detail">
        <div>
          <div className={'poster' + (isGeneratedArt(it) ? ' generated' : '')}>{it.poster && <img src={posterUrl(it)} alt="" />}</div>
        </div>
        <div>
          {it.showId != null && (
            <div className="crumbs">
              <Link to={`/show/${it.showId}`}>{it.showTitle}</Link>
              {it.season != null && <> › <Link to={`/show/${it.showId}?season=${it.season}`}>{it.season === 0 ? 'Specials' : `Season ${it.season}`}</Link></>}
              {episodeLabel(it) && <> › {episodeLabel(it)}</>}
            </div>
          )}
          <h1>{it.title}</h1>
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {it.aired ? <span>{it.aired}</span> : it.year && <span>{it.year}</span>}
            {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
            {it.contentRating && <span className="pill">{it.contentRating}</span>}
            {it.rating && <span>★ {it.rating.toFixed(1)}</span>}
            {(it.genres || []).length > 0 && <span>{it.genres.join(', ')}</span>}
            {it.studio && <span>{it.studio}</span>}
          </div>
          {disc ? (
            <div className="actions disc-actions">
              <button className="btn primary insert" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=${start}${fx ? '&insert=1' : ''}`)}
                title={start === 'menu' ? 'Start the disc like a DVD player: first-play, intros, then the main menu' : 'Start the disc straight into the main feature'}>
                <IconDisc /> Insert DVD
              </button>
              {resume
                ? <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&ttn=${resume.discTitle || it.disc?.mainTitle || 1}`)}><IconPlay /> Resume from {fmtTime(resume.positionSec)}</button>
                : <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=title&start=0`)}><IconPlay /> Play movie</button>}
              {it.disc?.hasMenus && <button className="btn" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=menu`)}><IconMenu /> Main menu</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setDiscSettings(true)} title="Disc settings"><IconSettings /> Disc settings</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setArt(true)}><IconImage /> Artwork</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={refresh} title="Fetch metadata from TMDB"><IconRefresh /> Refresh</button>}
              <div className="disc-hint muted">Insert DVD starts with the {start === 'menu' ? 'disc menu' : 'main feature'}{fx ? ' · with the tray effect' : ''}{user?.isAdmin ? ' — change it in Disc settings.' : '.'}</div>
            </div>
          ) : (
            <div className="actions">
              <button className="btn primary" onClick={() => nav(`/play/${it.id}`)}>
                <IconPlay /> {resume ? `Resume from ${fmtTime(resume.positionSec)}` : 'Play'}
              </button>
              {resume && <button className="btn" onClick={() => nav(`/play/${it.id}?start=0`)}><IconPlay /> Play from start</button>}
              {it.nextId && <button className="btn" onClick={() => nav(`/item/${it.nextId}`)}><IconNext /> Next episode</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setArt(true)}><IconImage /> Artwork</button>}
              {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
              {user?.isAdmin && it.showId == null && <button className="btn ghost" onClick={refresh} title="Fetch metadata from TMDB"><IconRefresh /> Refresh</button>}
            </div>
          )}
          {it.overview && <p className="overview">{it.overview}</p>}
          {it.provider === 'wikidata' && (
            <p className="muted" style={{ fontSize: 12 }}>
              Facts from <a href={`https://www.wikidata.org/wiki/${it.providerId}`} target="_blank" rel="noreferrer">Wikidata</a>; plot summary from Wikipedia,
              shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.
            </p>
          )}
          {disc && it.disc?.scrambled && <div className="error" style={{ maxWidth: 760 }}>This disc image is CSS-scrambled and cannot be played. Decrypt it (e.g. with MakeMKV or DVD Decrypter in ISO mode) and rescan.</div>}
          <div className="facts">
            {it.width > 0 && <div className="fact"><div className="k">Video</div><div className="v">{it.width}×{it.height} {it.videoCodec?.toUpperCase()}{disc && main ? ` · ${main.standard.toUpperCase()} ${main.wide ? '16:9' : '4:3'}` : ''}</div></div>}
            {(it.audioLangs || []).length > 0 && <div className="fact"><div className="k">Audio</div><div className="v">{[...new Set(it.audioLangs.map(langName))].join(', ')}</div></div>}
            {(it.subLangs || []).length > 0 && <div className="fact"><div className="k">Subtitles</div><div className="v">{[...new Set(it.subLangs.map(langName))].join(', ')}</div></div>}
            <div className="fact"><div className="k">Source</div><div className="v">{disc ? (it.kind === 'dvd-iso' ? 'DVD image' : 'DVD folder') : it.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
            {disc && it.disc && <div className="fact"><div className="k">Disc</div><div className="v">{it.disc.label} · {it.disc.titles.length} titles{it.disc.hasMenus ? ' · menus' : ''}</div></div>}
            <div className="fact" style={{ gridColumn: '1 / -1' }}><div className="k">File</div><div className="v" style={{ fontWeight: 400, color: 'var(--fg-2)', wordBreak: 'break-all', fontSize: 13 }}>{it.path}</div></div>
          </div>

          {disc && <DiscSections item={it} admin={!!user?.isAdmin} playable={playable} />}
          <p style={{ marginTop: 30 }}>{it.showId != null
            ? <Link to={`/show/${it.showId}?season=${it.season ?? -1}`} className="muted">‹ Back to {it.showTitle}</Link>
            : <Link to={`/library/${it.libraryId}`} className="muted">‹ Back to library</Link>}</p>
        </div>
      </div>
      {edit && <EditModal item={it} onClose={() => setEdit(false)} onSaved={(x) => { setIt(x); setEdit(false) }} />}
      {art && <ArtworkPicker base={`/items/${it.id}`} onClose={() => setArt(false)}
        onPick={async (kind, url) => { setIt(await api.put<Item>(`/items/${it.id}`, { [kind]: url })); toast(kind === 'poster' ? 'Poster updated' : 'Backdrop updated') }} />}
      {discSettings && <DiscSettings item={it} prefs={prefs} onClose={() => setDiscSettings(false)} onSaved={(x) => { setIt(x); setDiscSettings(false); toast('Disc settings saved') }} />}
    </>
  )
}

function DiscSettings({ item, prefs, onClose, onSaved }: { item: Item; prefs: Prefs | null; onClose: () => void; onSaved: (it: Item) => void }) {
  const [playMode, setPlayMode] = useState(item.playMode)
  const [insertFx, setInsertFx] = useState(item.insertFx)
  const [err, setErr] = useState('')
  const save = async () => {
    try { onSaved(await api.put<Item>(`/items/${item.id}`, { playMode, insertFx })) } catch (e: any) { setErr(e.message) }
  }
  const def = prefs?.dvdStart === 'title' ? 'main feature' : 'disc menu'
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Disc settings</h2>
        <div className="form" style={{ maxWidth: 'none' }}>
          <label>When this disc is inserted
            <select className="select" value={playMode} onChange={(e) => setPlayMode(e.target.value as Item['playMode'])}>
              <option value="">Server default ({def})</option>
              <option value="menu" disabled={!item.disc?.hasMenus}>Start with the disc menu (like a DVD player)</option>
              <option value="title">Skip menus and play the main feature</option>
            </select>
          </label>
          <label>Insert DVD effect (tray animation and sound)
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

function EditModal({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: (it: Item) => void }) {
  const [f, setF] = useState({ title: item.title, year: item.year ? String(item.year) : '', overview: item.overview, tagline: item.tagline, genres: item.genres.join(', '), poster: item.poster, backdrop: item.backdrop, contentRating: item.contentRating || '' })
  const [err, setErr] = useState('')
  const save = async () => {
    try {
      const r = await api.put<Item>(`/items/${item.id}`, {
        title: f.title, year: f.year ? Number(f.year) : null, overview: f.overview, tagline: f.tagline, contentRating: f.contentRating,
        genres: f.genres.split(',').map((s) => s.trim()).filter(Boolean), poster: f.poster, backdrop: f.backdrop,
      })
      onSaved(r)
    } catch (e: any) { setErr(e.message) }
  }
  const up = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value })
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Edit metadata</h2>
        <div className="form" style={{ maxWidth: 'none' }}>
          <div className="row2">
            <label>Title<input className="input" value={f.title} onChange={up('title')} /></label>
            <label>Year<input className="input" value={f.year} onChange={up('year')} /></label>
          </div>
          <label>Tagline<input className="input" value={f.tagline} onChange={up('tagline')} /></label>
          <label>Overview<textarea className="input" rows={5} value={f.overview} onChange={up('overview')} /></label>
          <label>Genres (comma separated)<input className="input" value={f.genres} onChange={up('genres')} /></label>
          <RatingField value={f.contentRating} onChange={(v) => setF({ ...f, contentRating: v })} />
          <label>Poster (URL or file path)<input className="input" value={f.poster} onChange={up('poster')} /></label>
          <label>Backdrop (URL or file path)<input className="input" value={f.backdrop} onChange={up('backdrop')} /></label>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></div>
      </div>
    </div>
  )
}
