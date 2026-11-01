import { useEffect, useRef, useState } from 'react'
import { api, langName } from '../../api'
import { IconClose } from '../../icons'
import type { Facet, Facets, SavedView } from '../../metadata'

// Everything a library can be narrowed down by. What's chosen lives in the address.

export const FILTER_KEYS = ['genre', 'studio', 'network', 'kind', 'disc', 'decade', 'year', 'content', 'minRating', 'resolution', 'audio', 'subtitle', 'format', 'tag', 'edition', 'watched', 'platform', 'search']

const LABELS: Record<string, string> = {
  genre: 'Category', studio: 'Studio', network: 'Network', kind: 'Type', disc: 'On disc', decade: 'Decade', year: 'Year', content: 'Age rating', minRating: 'Score', resolution: 'Picture',
  audio: 'Audio language', subtitle: 'Subtitles', format: 'Sound', tag: 'Tag', edition: 'Edition', watched: 'Watched', platform: 'Listed on', search: 'Words',
}

const friendly = (k: string, v: string) => {
  if (k === 'audio' || k === 'subtitle') return langName(v)
  if (k === 'minRating') return `${v} or more`
  if (k === 'decade') return `${v}s`
  if (k === 'watched') return v === 'yes' ? 'Watched' : 'Not watched yet'
  if (k === 'kind') return ({ video: 'Video files', dvd: 'DVDs', bluray: 'Blu-rays', disc: 'DVDs and Blu-rays', show: 'TV shows' } as Record<string, string>)[v] || v
  if (k === 'disc') return ({ movie: 'Films', tv: 'TV series', music: 'Music' } as Record<string, string>)[v] || v
  if (k === 'resolution') return ({ '4k': '4K', '1080p': '1080p', '720p': '720p', sd: 'SD' } as Record<string, string>)[v] || v
  return v
}

// The chosen filters, each with a way to take it off.
export function ActiveFilters({ params, setParam, onSave, onClear }: { params: URLSearchParams; setParam: (k: string, v: string) => void; onSave: () => void; onClear: () => void }) {
  const on = FILTER_KEYS.filter((k) => params.get(k))
  if (on.length === 0) return null
  return (
    <div className="lib-active" role="group" aria-label="Filters">
      {on.map((k) => (
        <span key={k} className="chip tag lib-active-chip">
          <span className="muted">{LABELS[k]}:</span>&nbsp;{friendly(k, params.get(k)!)}
          <button type="button" aria-label={`Take off ${LABELS[k]} ${friendly(k, params.get(k)!)}`} onClick={() => setParam(k, '')}><IconClose size={14} /></button>
        </span>
      ))}
      <button type="button" className="btn sm ghost" onClick={onSave}>Save this view</button>
      {on.length > 1 && <button type="button" className="btn sm ghost" onClick={onClear}>Clear all</button>}
    </div>
  )
}

function FacetGroup({ k, title, list, label, params, setParam, limit = 24 }: { k: string; title: string; list: Facet[]; label?: (f: Facet) => string; params: URLSearchParams; setParam: (k: string, v: string) => void; limit?: number }) {
  const [all, setAll] = useState(false)
  if (!list?.length) return null
  const cur = params.get(k) || ''
  const shown = all ? list : list.slice(0, limit)
  return (
    <section className="lib-facet" aria-label={title}>
      <h3>{title}</h3>
      <div className="chips">
        {shown.map((f) => {
          const on = cur.toLowerCase() === f.value.toLowerCase()
          return (
            <button key={f.value} type="button" className={'chip' + (on ? ' active' : '')} aria-pressed={on} onClick={() => setParam(k, on ? '' : f.value)}>
              {label ? label(f) : f.label || friendly(k, f.value)}<span className="lib-facet-n" aria-label={`, ${f.count} ${f.count === 1 ? 'title' : 'titles'}`}>{f.count}</span>
            </button>
          )
        })}
        {!all && list.length > limit && <button type="button" className="chip" onClick={() => setAll(true)}>{list.length - limit} more</button>}
      </div>
    </section>
  )
}

export function FilterPanel({ library, params, setParam, onClear, onSave, onClose }: { library: number; params: URLSearchParams; setParam: (k: string, v: string) => void; onClear: () => void; onSave: () => void; onClose: () => void }) {
  const [f, setF] = useState<Facets | null>(null)
  const [err, setErr] = useState('')
  const [search, setSearch] = useState(params.get('search') || '')
  useEffect(() => { api.get<Facets>(`/browse/facets?library=${library}`).then(setF).catch((e) => setErr(e.message)) }, [library])
  const pressed = useRef(false)
  const g = { params, setParam }
  return (
    <div className="modal-backdrop lib-filter-back" onMouseDown={(e) => { pressed.current = e.target === e.currentTarget }} onClick={(e) => { if (e.target === e.currentTarget && (pressed.current || e.detail === 0)) onClose() }}>
      <div className="modal lib-filter" role="dialog" aria-modal="true" aria-labelledby="flt-h">
        <header className="lib-filter-head">
          <h2 id="flt-h">Filters</h2>
          <button type="button" className="btn ghost square" aria-label="Close" onClick={onClose}><IconClose /></button>
        </header>
        <div className="lib-filter-body">
          <form className="field-row" onSubmit={(e) => { e.preventDefault(); setParam('search', search.trim()) }}>
            <input className="input" aria-label="Words in the title" placeholder="Words in the title" value={search} onChange={(e) => setSearch(e.target.value)} />
            <button type="submit" className="btn">Find</button>
          </form>
          {err && <div className="error" role="alert">{err}</div>}
          {!f && !err && <div className="loading-box"><div className="spinner" /></div>}
          {f && (
            <>
              <FacetGroup k="genre" title="Category" list={f.genres} {...g} />
              <FacetGroup k="kind" title="Type" list={f.kinds} {...g} />
              <FacetGroup k="studio" title="Studio" list={f.studios} limit={16} {...g} />
              <FacetGroup k="network" title="Network" list={f.networks} limit={16} {...g} />
              <FacetGroup k="platform" title="Listed on" list={f.platforms} {...g} />
              <FacetGroup k="decade" title="Decade" list={f.decades} {...g} />
              <FacetGroup k="year" title="Year" list={f.years} limit={10} {...g} />
              <FacetGroup k="content" title="Age rating" list={f.content} {...g} />
              <section className="lib-facet" aria-label="Score">
                <h3>Score</h3>
                <div className="chips">
                  {['9', '8', '7', '6'].map((n) => {
                    const on = params.get('minRating') === n
                    return <button key={n} type="button" className={'chip' + (on ? ' active' : '')} aria-pressed={on} onClick={() => setParam('minRating', on ? '' : n)}>{n} or more</button>
                  })}
                </div>
              </section>
              <section className="lib-facet" aria-label="Watched">
                <h3>Watched</h3>
                <div className="chips">
                  {[['no', 'Not watched yet'], ['yes', 'Watched']].map(([v, l]) => {
                    const on = params.get('watched') === v
                    return <button key={v} type="button" className={'chip' + (on ? ' active' : '')} aria-pressed={on} onClick={() => setParam('watched', on ? '' : v)}>{l}</button>
                  })}
                </div>
              </section>
              <FacetGroup k="resolution" title="Picture" list={f.resolutions} {...g} />
              <FacetGroup k="format" title="Sound" list={f.formats.filter((x) => x.value !== 'none')} {...g} />
              <FacetGroup k="audio" title="Audio language" list={f.audio} {...g} />
              <FacetGroup k="subtitle" title="Subtitles" list={f.subtitles} {...g} />
              <FacetGroup k="edition" title="Edition" list={f.editions} {...g} />
              <FacetGroup k="tag" title="Tags" list={f.tags} {...g} />
            </>
          )}
        </div>
        <footer className="lib-filter-foot">
          <button type="button" className="btn ghost" onClick={onClear}>Clear all</button>
          <button type="button" className="btn ghost" disabled={!FILTER_KEYS.some((k) => params.get(k))} onClick={onSave}>Save this view</button>
          <button type="button" className="btn primary" onClick={onClose}>Done</button>
        </footer>
      </div>
    </div>
  )
}

export function SaveViewDialog({ library, params, onClose, onSaved }: { library: number; params: URLSearchParams; onClose: () => void; onSaved: () => void }) {
  const on = FILTER_KEYS.filter((k) => params.get(k))
  const [name, setName] = useState(on.map((k) => friendly(k, params.get(k)!)).join(', ').slice(0, 60))
  const [pinned, setPinned] = useState(true)
  const [err, setErr] = useState('')
  const save = async () => {
    setErr('')
    const q = new URLSearchParams()
    for (const k of on) q.set(k, params.get(k)!)
    for (const k of ['sort', 'desc']) { const v = params.get(k); if (v) q.set(k, v) }
    try { await api.post<SavedView>('/views', { libraryId: library, name, query: q.toString(), pinned }); onSaved() } catch (e: any) { setErr(e.message) }
  }
  const pressed = useRef(false)
  return (
    <div className="modal-backdrop" onMouseDown={(e) => { pressed.current = e.target === e.currentTarget }} onClick={(e) => { if (e.target === e.currentTarget && (pressed.current || e.detail === 0)) onClose() }}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="sv-h">
        <h2 id="sv-h">Save this view</h2>
        <form className="form full" onSubmit={(e) => { e.preventDefault(); save() }}>
          <label>Name<input className="input" autoFocus value={name} maxLength={60} onChange={(e) => setName(e.target.value)} /></label>
          <label className="lib-check"><input type="checkbox" checked={pinned} onChange={(e) => setPinned(e.target.checked)} /> Pin it to Home</label>
          <p className="hint">It's yours alone, and it keeps itself up to date as your library changes.</p>
          {err && <div className="error" role="alert">{err}</div>}
          <div className="actions">
            <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn primary" disabled={!name.trim()}>Save</button>
          </div>
        </form>
      </div>
    </div>
  )
}
