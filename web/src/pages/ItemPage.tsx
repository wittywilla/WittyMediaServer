import RatingField from '../components/RatingField'
import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { api, backdropUrl, codecName, dvdStartMode, episodeLabel, fmtRuntime, fmtTime, getPrefs, insertFxOn, isDisc, langName, posterUrl, type Item, type Prefs, discName, isBluray } from '../api'
import ArtworkPicker from '../components/ArtworkPicker'
import DiscSections from './DiscSections'
import { BookDetails } from './Books'
import { Spinner, isGeneratedArt } from '../components/common'
import { IconDisc, IconEdit, IconImage, IconMenu, IconNext, IconPlay, IconRefresh, IconSettings } from '../icons'
import { toast, useApp } from '../state'

export default function ItemPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user, libraries } = useApp()
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
  // Songs, chapters and episodes live on their album, book or podcast page.
  if (it.kind === 'audio') return <Navigate replace to={`/album/${it.libraryId}?artist=${encodeURIComponent(it.albumArtist || it.artist || '')}&album=${encodeURIComponent(it.album || '')}`} />
  const disc = isDisc(it)
  const resume = it.progress?.resumeSec ? it.progress : null
  const main = it.disc?.titles.find((t) => t.number === it.disc?.mainTitle)
  const refresh = async () => {
    try { setIt(await api.put<Item>(`/items/${it.id}`, { refresh: true })); toast('Metadata refreshed') } catch (e: any) { toast(e.message) }
  }
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
          <div className={'poster' + (isGeneratedArt(it) ? ' generated' : '')}>
            {/* A frame from the video: shown whole over a blurred fill of itself. */}
            {it.poster && isGeneratedArt(it) && <div className="blur" style={{ backgroundImage: `url(${posterUrl(it)})` }} />}
            {it.poster && <img src={posterUrl(it)} alt="" />}
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
            {(it.genres || []).length > 0 && <span title={it.genres.join(', ')}>{it.genres.slice(0, 4).join(', ')}{it.genres.length > 4 ? '…' : ''}</span>}
            {it.studio && <span>{it.studio}</span>}
          </div>
          {disc ? (
            <div className="actions disc-actions">
              <button className="btn primary insert" disabled={!playable} onClick={() => nav(`/play/${it.id}?mode=${start}${fx ? '&insert=1' : ''}`)}
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
                  <button className="btn ghost square" onClick={() => setArt(true)} title="Artwork" aria-label="Artwork"><IconImage /></button>
                  <button className="btn ghost square" onClick={() => setEdit(true)} title="Edit details" aria-label="Edit details"><IconEdit /></button>
                  <button className="btn ghost square" onClick={refresh} title="Refresh details from TMDB" aria-label="Refresh details"><IconRefresh /></button>
                </div>
              )}
              <div className="disc-hint muted">Insert {discName(it)} starts with the {start === 'menu' ? 'disc menu' : 'main feature'}{fx ? ' · with the tray effect' : ''}{user?.isAdmin ? ' — change it in Disc settings.' : '.'}</div>
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
          {disc && it.disc?.scrambled && <div className="error" style={{ maxWidth: 760 }}>This disc image is {isBluray(it) ? 'encrypted (AACS)' : 'CSS-scrambled'}, so it can't be played. Make a decrypted image of the disc you own with a tool that is legal where you live, then rescan.</div>}
          <div className="facts">
            {it.width > 0 && <div className="fact"><div className="k">Video</div><div className="v">{it.width}×{it.height} {codecName(it.videoCodec)}{disc && main ? ` · ${isBluray(it) ? main.standard : main.standard.toUpperCase()} ${main.wide ? '16:9' : '4:3'}` : ''}</div></div>}
            {(it.audioLangs || []).length > 0 && <div className="fact"><div className="k">Audio</div><div className="v">{[...new Set(it.audioLangs.map(langName))].join(', ')}</div></div>}
            {(it.subLangs || []).length > 0 && <div className="fact"><div className="k">Subtitles</div><div className="v">{[...new Set(it.subLangs.map(langName))].join(', ')}</div></div>}
            <div className="fact"><div className="k">Source</div><div className="v">{disc ? `${discName(it)} ${it.kind.endsWith('-iso') ? 'image' : 'folder'}` : it.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
            {disc && it.disc && <div className="fact"><div className="k">Disc</div><div className="v">{it.disc.label} · {it.disc.titles.length} titles{it.disc.hasMenus ? ' · menus' : ''}</div></div>}
            {user?.isAdmin && <div className="fact" style={{ gridColumn: '1 / -1' }}><div className="k">File</div><div className="v" style={{ fontWeight: 400, color: 'var(--fg-2)', wordBreak: 'break-all', fontSize: 13 }}>{it.path}</div></div>}
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
  const [content, setContent] = useState(item.disc?.contentManual ? item.disc.content || '' : '')
  const [err, setErr] = useState('')
  const save = async () => {
    try { onSaved(await api.put<Item>(`/items/${item.id}`, { playMode, insertFx, discContent: content })) } catch (e: any) { setErr(e.message) }
  }
  const guessed = !item.disc?.contentManual && item.disc?.content === 'tv' ? 'TV series' : 'film'
  const def = prefs?.dvdStart === 'title' ? 'main feature' : 'disc menu'
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Disc settings</h2>
        <div className="form" style={{ maxWidth: 'none' }}>
          <label>What’s on this disc
            <select className="select" value={content} onChange={(e) => setContent(e.target.value)}>
              <option value="">Work it out from the name ({item.disc?.contentManual ? 'automatic' : guessed})</option>
              <option value="movie">A film</option>
              <option value="tv">Episodes of a TV series</option>
            </select>
            <span className="muted" style={{ fontWeight: 400, fontSize: 12.5 }}>Sorts the disc under Films or TV series in its library. Discs of a series aren’t looked up as films.</span>
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
