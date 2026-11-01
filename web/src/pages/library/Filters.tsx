import { useEffect, useState } from 'react'
import { api, langName } from '../../api'
import type { Facet, Facets, SavedView } from '../../metadata'
import { Button, Chip, Chips, Dialog, Field, Input, Sheet } from '../../ui'

// Everything a library can be narrowed down by. What's chosen lives in the address.

export const FILTER_KEYS = ['genre', 'studio', 'network', 'kind', 'disc', 'decade', 'year', 'content', 'minRating', 'resolution', 'audio', 'subtitle', 'format', 'tag', 'edition', 'series', 'watched', 'platform', 'search']

const LABELS: Record<string, string> = {
  genre: 'Category', studio: 'Studio', network: 'Network', kind: 'Type', disc: 'On disc', decade: 'Decade', year: 'Year', content: 'Age rating', minRating: 'Score', resolution: 'Picture',
  audio: 'Audio language', subtitle: 'Subtitles', format: 'Sound', tag: 'Tag', edition: 'Edition', series: 'Collection', watched: 'Watched', platform: 'Listed on', search: 'Words',
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
export function ActiveFilters({ params, setParam, onSave, onClear, platforms }: { params: URLSearchParams; setParam: (k: string, v: string) => void; onSave: () => void; onClear: () => void; platforms?: { key: string; name: string }[] }) {
  const on = FILTER_KEYS.filter((k) => params.get(k))
  if (on.length === 0) return null
  // A service is filtered by its key, and shown by its name.
  const shown = (k: string) => (k === 'platform' && platforms?.find((p) => p.key === params.get(k))?.name) || friendly(k, params.get(k)!)
  return (
    <div className="lib-active" role="group" aria-label="Filters" data-nav-group>
      {on.map((k) => (
        <Chip key={k} onRemove={() => setParam(k, '')} removeLabel={`Take off ${LABELS[k]} ${shown(k)}`}>
          <span className="muted">{LABELS[k]}:</span>&nbsp;{shown(k)}
        </Chip>
      ))}
      <Button size="sm" variant="ghost" onClick={onSave}>Save this view</Button>
      {on.length > 1 && <Button size="sm" variant="ghost" onClick={onClear}>Clear all</Button>}
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
      <Chips label={title}>
        {shown.map((f) => {
          const on = cur.toLowerCase() === f.value.toLowerCase()
          return (
            <Chip key={f.value} selected={on} onClick={() => setParam(k, on ? '' : f.value)}>
              {label ? label(f) : f.label || friendly(k, f.value)}<span className="lib-facet-n" aria-label={`, ${f.count} ${f.count === 1 ? 'title' : 'titles'}`}>{f.count}</span>
            </Chip>
          )
        })}
        {!all && list.length > limit && <Chip onClick={() => setAll(true)}>{list.length - limit} more</Chip>}
      </Chips>
    </section>
  )
}

export function FilterPanel({ library, params, setParam, onClear, onSave, onClose }: { library: number; params: URLSearchParams; setParam: (k: string, v: string) => void; onClear: () => void; onSave: () => void; onClose: () => void }) {
  const [f, setF] = useState<Facets | null>(null)
  const [err, setErr] = useState('')
  const [search, setSearch] = useState(params.get('search') || '')
  useEffect(() => { api.get<Facets>(`/browse/facets?library=${library}`).then(setF).catch((e) => setErr(e.message)) }, [library])
  const g = { params, setParam }
  return (
    <Sheet title="Filters" className="lib-filter" onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClear}>Clear all</Button>
        <Button variant="ghost" disabled={!FILTER_KEYS.some((k) => params.get(k))} onClick={onSave}>Save this view</Button>
        <Button variant="primary" onClick={onClose}>Done</Button>
      </>}>
          <form className="field-row" onSubmit={(e) => { e.preventDefault(); setParam('search', search.trim()) }}>
            <Input aria-label="Words in the title" placeholder="Words in the title" value={search} onChange={(e) => setSearch(e.target.value)} />
            <Button type="submit">Find</Button>
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
                <Chips label="Score">
                  {['9', '8', '7', '6'].map((n) => {
                    const on = params.get('minRating') === n
                    return <Chip key={n} selected={on} onClick={() => setParam('minRating', on ? '' : n)}>{n} or more</Chip>
                  })}
                </Chips>
              </section>
              <section className="lib-facet" aria-label="Watched">
                <h3>Watched</h3>
                <Chips label="Watched">
                  {[['no', 'Not watched yet'], ['yes', 'Watched']].map(([v, l]) => {
                    const on = params.get('watched') === v
                    return <Chip key={v} selected={on} onClick={() => setParam('watched', on ? '' : v)}>{l}</Chip>
                  })}
                </Chips>
              </section>
              <FacetGroup k="resolution" title="Picture" list={f.resolutions} {...g} />
              <FacetGroup k="format" title="Sound" list={f.formats.filter((x) => x.value !== 'none')} {...g} />
              <FacetGroup k="audio" title="Audio language" list={f.audio} {...g} />
              <FacetGroup k="subtitle" title="Subtitles" list={f.subtitles} {...g} />
              <FacetGroup k="edition" title="Edition" list={f.editions} {...g} />
              <FacetGroup k="tag" title="Tags" list={f.tags} {...g} />
            </>
          )}
    </Sheet>
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
  return (
    <Dialog title="Save this view" size="sm" onClose={onClose}
      description="It's yours alone, and it keeps itself up to date as your library changes."
      actions={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={!name.trim()} onClick={save}>Save</Button>
      </>}>
      <form className="form full" onSubmit={(e) => { e.preventDefault(); save() }}>
        <Field label="Name"><Input autoFocus value={name} maxLength={60} onChange={(e) => setName(e.target.value)} /></Field>
        <label className="lib-check"><input type="checkbox" checked={pinned} onChange={(e) => setPinned(e.target.checked)} /> Pin it to Home</label>
        {err && <div className="error" role="alert">{err}</div>}
      </form>
    </Dialog>
  )
}
