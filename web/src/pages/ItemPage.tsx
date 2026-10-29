import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, backdropUrl, fmtRuntime, fmtTime, isDisc, langName, posterUrl, type Item } from '../api'
import { Spinner } from '../components/common'
import { IconDisc, IconEdit, IconPlay, IconRefresh } from '../icons'
import { toast, useApp } from '../state'

export default function ItemPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const { user } = useApp()
  const [it, setIt] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const load = () => api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message))
  useEffect(() => { load() }, [id])
  if (err) return <div className="error">{err}</div>
  if (!it) return <Spinner />
  const disc = isDisc(it)
  const resume = it.progress && !it.progress.watched && it.progress.positionSec > 60 ? it.progress : null
  const main = it.disc?.titles.find((t) => t.number === it.disc?.mainTitle)
  const refresh = async () => {
    try { setIt(await api.put<Item>(`/items/${it.id}`, { refresh: true })); toast('Metadata refreshed') } catch (e: any) { toast(e.message) }
  }
  const sizeGB = it.sizeBytes ? (it.sizeBytes / 1e9).toFixed(2) + ' GB' : ''
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={{ backgroundImage: `url(${backdropUrl(it)})` }} />
        <div className="shade" />
      </div>
      <div className="detail">
        <div>
          <div className="poster">{it.poster && <img src={posterUrl(it)} alt="" />}</div>
        </div>
        <div>
          <h1>{it.title}</h1>
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {it.year && <span>{it.year}</span>}
            {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
            {it.contentRating && <span className="pill">{it.contentRating}</span>}
            {it.rating && <span>★ {it.rating.toFixed(1)}</span>}
            {it.genres.length > 0 && <span>{it.genres.join(', ')}</span>}
            {it.studio && <span>{it.studio}</span>}
          </div>
          <div className="actions">
            <button className="btn primary" onClick={() => nav(`/play/${it.id}`)}>
              <IconPlay /> {resume ? `Resume from ${fmtTime(resume.positionSec)}` : 'Play'}
            </button>
            {resume && <button className="btn" onClick={() => nav(`/play/${it.id}?start=0`)}><IconPlay /> Play from start</button>}
            {disc && it.disc?.hasMenus && <button className="btn" onClick={() => nav(`/play/${it.id}?mode=menu`)}><IconDisc /> Play with DVD menu</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={refresh} title="Fetch metadata from TMDB"><IconRefresh /> Refresh</button>}
          </div>
          {it.overview && <p className="overview">{it.overview}</p>}
          {disc && it.disc?.scrambled && <div className="error" style={{ maxWidth: 760 }}>This disc image is CSS-scrambled and cannot be played. Decrypt it (e.g. with MakeMKV or DVD Decrypter in ISO mode) and rescan.</div>}
          <div className="facts">
            {it.width > 0 && <div className="fact"><div className="k">Video</div><div className="v">{it.width}×{it.height} {it.videoCodec?.toUpperCase()}{disc && main ? ` · ${main.standard.toUpperCase()} ${main.wide ? '16:9' : '4:3'}` : ''}</div></div>}
            {it.audioLangs.length > 0 && <div className="fact"><div className="k">Audio</div><div className="v">{[...new Set(it.audioLangs.map(langName))].join(', ')}</div></div>}
            {it.subLangs.length > 0 && <div className="fact"><div className="k">Subtitles</div><div className="v">{[...new Set(it.subLangs.map(langName))].join(', ')}</div></div>}
            <div className="fact"><div className="k">Source</div><div className="v">{disc ? (it.kind === 'dvd-iso' ? 'DVD image' : 'DVD folder') : it.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
            {disc && it.disc && <div className="fact"><div className="k">Disc</div><div className="v">{it.disc.label} · {it.disc.titles.length} titles{it.disc.hasMenus ? ' · menus' : ''}</div></div>}
            <div className="fact" style={{ gridColumn: '1 / -1' }}><div className="k">File</div><div className="v" style={{ fontWeight: 400, color: 'var(--fg-2)', wordBreak: 'break-all', fontSize: 13 }}>{it.path}</div></div>
          </div>

          {disc && it.disc && it.disc.titles.length > 0 && (
            <>
              <h3 className="section-title">Disc titles</h3>
              <table className="titles-table">
                <thead><tr><th>#</th><th>Length</th><th>Chapters</th><th>Audio</th><th>Subtitles</th><th></th></tr></thead>
                <tbody>
                  {it.disc.titles.filter((t) => t.durationSec >= 1).map((t) => (
                    <tr key={t.number} className={t.number === it.disc!.mainTitle ? 'main-row' : ''}>
                      <td>{t.number}{t.number === it.disc!.mainTitle && ' · Main feature'}</td>
                      <td>{fmtTime(t.durationSec)}</td>
                      <td>{t.chapters.length}</td>
                      <td style={{ fontSize: 13 }}>{t.audio.slice(0, 3).join(', ')}</td>
                      <td style={{ fontSize: 13 }}>{[...new Set(t.subtitles.map(langName))].join(', ')}</td>
                      <td><button className="btn sm" onClick={() => nav(`/play/${it.id}?mode=title&ttn=${t.number}`)}><IconPlay /> Play</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {main && main.chapters.length > 1 && (
                <>
                  <h3 className="section-title">Chapters</h3>
                  <div className="chapters">
                    {main.chapters.map((c, i) => <button key={i} onClick={() => nav(`/play/${it.id}?mode=title&ttn=${main.number}&chapter=${i + 1}`)}>Chapter {i + 1} · {fmtTime(c)}</button>)}
                  </div>
                </>
              )}
            </>
          )}
          <p style={{ marginTop: 30 }}><Link to={`/library/${it.libraryId}`} className="muted">‹ Back to library</Link></p>
        </div>
      </div>
      {edit && <EditModal item={it} onClose={() => setEdit(false)} onSaved={(x) => { setIt(x); setEdit(false) }} />}
    </>
  )
}

function EditModal({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: (it: Item) => void }) {
  const [f, setF] = useState({ title: item.title, year: item.year ? String(item.year) : '', overview: item.overview, tagline: item.tagline, genres: item.genres.join(', '), poster: item.poster, backdrop: item.backdrop })
  const [err, setErr] = useState('')
  const save = async () => {
    try {
      const r = await api.put<Item>(`/items/${item.id}`, {
        title: f.title, year: f.year ? Number(f.year) : null, overview: f.overview, tagline: f.tagline,
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
          <label>Poster (URL or file path)<input className="input" value={f.poster} onChange={up('poster')} /></label>
          <label>Backdrop (URL or file path)<input className="input" value={f.backdrop} onChange={up('backdrop')} /></label>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></div>
      </div>
    </div>
  )
}
