// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { api, langName } from '../../api'
import type { Facets, SavedView } from '../../metadata'
import { Button, Chip, Dialog, Field, Input, Sheet } from '../../ui'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { FILTER_KEYS, FILTER_LABELS, facetLabel, filterSpec, pressFilter } from '../../spec/specs/filters'

// Everything a library can be narrowed down by. What's chosen lives in the address.

export { FILTER_KEYS }
const LABELS = FILTER_LABELS
const friendly = (k: string, v: string) => facetLabel(k, v, langName)

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

export function FilterPanel({ library, params, setParam, onClear, onSave, onClose }: { library: number; params: URLSearchParams; setParam: (k: string, v: string) => void; onClear: () => void; onSave: () => void; onClose: () => void }) {
  const [f, setF] = useState<Facets | null>(null)
  const [err, setErr] = useState('')
  const [more, setMore] = useState<ReadonlySet<string>>(new Set())
  const [search, setSearch] = useState(params.get('search') || '')
  useEffect(() => { api.get<Facets>(`/browse/facets?library=${library}`).then(setF).catch((e) => setErr(e.message)) }, [library])
  const now = Object.fromEntries(params.entries())
  const page = useMemo(() => filterSpec({ facets: f, params: now, expanded: more, lang: langName }), [f, params, more]) // eslint-disable-line react-hooks/exhaustive-deps
  const press = (id: string) => {
    const p = pressFilter(id, now)
    if (!p) return
    if ('set' in p) setParam(p.set[0], p.set[1])
    else if ('more' in p) setMore(new Set([...more, p.more]))
    else if (p.verb === 'clear') onClear()
    else if (p.verb === 'save') onSave()
    else onClose()
  }
  return (
    <Sheet title="Filters" className="lib-filter" onClose={onClose}>
      {err && <div className="error" role="alert">{err}</div>}
      {!f && !err && <div className="loading-box"><div className="spinner" /></div>}
      {f && (
        <SpecPage page={page} embedded handled={handledIds(page)} onAction={press}
          before={(
            <form className="field-row" onSubmit={(e) => { e.preventDefault(); setParam('search', search.trim()) }}>
              <Input aria-label="Words in the title" placeholder="Words in the title" value={search} onChange={(e) => setSearch(e.target.value)} />
              <Button type="submit">Find</Button>
            </form>
          )} />
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
