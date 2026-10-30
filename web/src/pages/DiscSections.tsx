import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, fmtRuntime, fmtTime, langName, mediaUrl, type DiscLabels, type DiscTitleView, type DiscView, type Item } from '../api'
import { IconEdit, IconMenu, IconPlay } from '../icons'
import { toast } from '../state'

// The disc page below the facts: the feature first ("Feature Presentation"
// with its scenes), then the bonus material, then shortcuts into the disc's
// own menus. Admins can name titles and scenes; names are stored on the
// server and shown in the player too.
export default function DiscSections({ item, admin, playable }: { item: Item; admin: boolean; playable: boolean }) {
  const nav = useNavigate()
  const [view, setView] = useState<DiscView | null>(null)
  const [editing, setEditing] = useState(false)
  const [labels, setLabels] = useState<DiscLabels>({ titles: {}, chapters: {} })
  useEffect(() => {
    api.get<DiscView>(`/items/${item.id}/disc`).then((v) => {
      setView(v)
      const l: DiscLabels = { titles: {}, chapters: {} }
      for (const t of [v.main, ...v.extras]) {
        if (!t) continue
        if (t.label) l.titles[t.ttn] = t.label
        for (const c of t.chapters) if (c.label) (l.chapters[t.ttn] ||= {})[c.n] = c.label
      }
      setLabels(l)
    }).catch(() => setView({ main: null, extras: [], menus: [] }))
  }, [item.id])
  if (!view) return null
  const play = (ttn: number, chapter?: number) => nav(`/play/${item.id}?mode=title&ttn=${ttn}${chapter ? `&chapter=${chapter}` : '&start=0'}`)
  const thumb = (ttn: number, n: number) => mediaUrl(`/api/items/${item.id}/disc/thumb/${ttn}/${n}`)
  const titleName = (t: DiscTitleView, fallback: string) => labels.titles[t.ttn] || fallback
  const setTitle = (ttn: number, v: string) => setLabels((l) => ({ ...l, titles: { ...l.titles, [ttn]: v } }))
  const setChapter = (ttn: number, n: number, v: string) => setLabels((l) => ({ ...l, chapters: { ...l.chapters, [ttn]: { ...(l.chapters[ttn] || {}), [n]: v } } }))
  const save = async () => {
    try {
      await api.put(`/items/${item.id}/disc/labels`, labels)
      setEditing(false)
      toast('Names saved')
    } catch (e: any) { toast(e.message) }
  }
  const main = view.main
  return (
    <div className="disc-sections">
      {admin && (
        <div className="disc-edit-bar">
          {editing
            ? <><button className="btn sm primary" onClick={save}>Save names</button><button className="btn sm ghost" onClick={() => setEditing(false)}>Cancel</button></>
            : <button className="btn sm ghost" onClick={() => setEditing(true)} title="Give the feature, scenes and bonus features your own names"><IconEdit /> Name scenes & extras</button>}
        </div>
      )}

      {main && (
        <section aria-labelledby="feature-h">
          <h2 className="section-title" id="feature-h">Feature Presentation</h2>
          <div className="feature-card">
            <button className="feature-thumb" disabled={!playable} onClick={() => play(main.ttn)} aria-label={`Play ${titleName(main, item.title)}`}>
              <Thumb src={thumb(main.ttn, 0)} />
              <span className="play-badge"><IconPlay /></span>
            </button>
            <div className="feature-info">
              {editing
                ? <input className="input" value={labels.titles[main.ttn] || ''} placeholder={item.title} onChange={(e) => setTitle(main.ttn, e.target.value)} aria-label="Feature name" />
                : <h3>{titleName(main, item.title)}</h3>}
              <div className="muted">{fmtRuntime(main.durationSec)} · {main.chapters.length} scenes{main.angles > 1 ? ` · ${main.angles} video versions` : ''}{main.wide ? ' · widescreen' : ''}</div>
              <div className="muted">
                {main.audio.length > 0 && <>Audio: {[...new Set(main.audio.map((a) => langName(a.split(' ')[0])))].join(', ')}</>}
                {main.subtitles.length > 0 && <> · Subtitles: {[...new Set(main.subtitles.map((s) => langName(s.split(' ')[0])))].join(', ')}</>}
              </div>
              <button className="btn primary" disabled={!playable} onClick={() => play(main.ttn)}><IconPlay /> Play feature</button>
            </div>
          </div>
          {main.chapters.length > 1 && (
            <>
              <h3 className="section-title">Scenes</h3>
              <div className="scene-grid">
                {main.chapters.map((c) => {
                  const name = labels.chapters[main.ttn]?.[c.n] || ''
                  return (
                    <div key={c.n} className="scene">
                      <button className="scene-thumb" disabled={!playable} onClick={() => play(main.ttn, c.n)} aria-label={`Play scene ${c.n}${name ? ': ' + name : ''}`}>
                        <Thumb src={thumb(main.ttn, c.n)} />
                        <span className="scene-time">{fmtTime(c.start)}</span>
                      </button>
                      {editing
                        ? <input className="input sm" value={name} placeholder={`Scene ${c.n}`} onChange={(e) => setChapter(main.ttn, c.n, e.target.value)} aria-label={`Name of scene ${c.n}`} />
                        : <div className="scene-name"><span className="muted">{c.n}.</span> {name || `Scene ${c.n}`}</div>}
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </section>
      )}

      {view.extras.length > 0 && (
        <section aria-labelledby="extras-h">
          <h2 className="section-title" id="extras-h">Bonus Features</h2>
          <div className="extras-grid">
            {view.extras.map((t, i) => (
              <div key={t.ttn} className="extra">
                <button className="scene-thumb" disabled={!playable} onClick={() => play(t.ttn)} aria-label={`Play ${titleName(t, `bonus feature ${i + 1}`)}`}>
                  <Thumb src={thumb(t.ttn, 0)} />
                  <span className="scene-time">{fmtTime(t.durationSec)}</span>
                </button>
                {editing
                  ? <input className="input sm" value={labels.titles[t.ttn] || ''} placeholder={`Bonus feature ${i + 1}`} onChange={(e) => setTitle(t.ttn, e.target.value)} aria-label={`Name of title ${t.ttn}`} />
                  : <div className="scene-name">{titleName(t, `Bonus feature ${i + 1}`)}<span className="muted"> · title {t.ttn}</span></div>}
              </div>
            ))}
          </div>
        </section>
      )}

      {view.menus.length > 0 && (
        <section aria-labelledby="menus-h">
          <h2 className="section-title" id="menus-h">Disc Menus</h2>
          <div className="menu-tiles">
            {view.menus.map((m) => (
              <button key={m.kind + m.vts} className="menu-tile" disabled={!playable}
                onClick={() => nav(`/play/${item.id}?mode=menu&menu=${m.kind}${m.vts ? `&menuVts=${m.vts}` : ''}`)}>
                <IconMenu /> {m.label}
              </button>
            ))}
          </div>
        </section>
      )}

      {item.disc && item.disc.titles.length > 0 && (
        <details className="all-titles">
          <summary>All {item.disc.titles.length} titles on the disc</summary>
          <table className="titles-table">
            <thead><tr><th>#</th><th>Length</th><th>Chapters</th><th>Audio</th><th>Subtitles</th><th></th></tr></thead>
            <tbody>
              {item.disc.titles.filter((t) => t.durationSec >= 1).map((t) => (
                <tr key={t.number} className={t.number === item.disc!.mainTitle ? 'main-row' : ''}>
                  <td>{t.number}{t.number === item.disc!.mainTitle && ' · Feature'}</td>
                  <td>{fmtTime(t.durationSec)}</td>
                  <td>{(t.chapters || []).length}</td>
                  <td className="small">{(t.audio || []).slice(0, 3).join(', ')}</td>
                  <td className="small">{[...new Set((t.subtitles || []).map(langName))].join(', ')}</td>
                  <td><button className="btn sm" disabled={!playable} onClick={() => play(t.number)}><IconPlay /> Play</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </div>
  )
}

// Thumb loads a scene picture. The server already tries two moments of the
// scene; a failed load (busy server, timeout) is retried twice before the
// tile says "Preview not available".
export function Thumb({ src }: { src: string }) {
  const [tries, setTries] = useState(0)
  const [state, setState] = useState<'loading' | 'ok' | 'failed'>('loading')
  const onError = () => {
    if (tries >= 2) { setState('failed'); return }
    setTimeout(() => setTries((t) => t + 1), 3000 * (tries + 1))
  }
  if (state === 'failed') return <span className="thumb-na">Preview not available</span>
  return <img key={tries} src={tries ? `${src}&retry=${tries}` : src} alt="" loading="lazy" onLoad={() => setState('ok')} onError={onError} className={state === 'ok' ? '' : 'loading'} />
}
