// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app's own tools on server-driven pages: what the hand-built library, music and
// collection pages offered that the page model has no components for (docs/SDUI.md, "How
// the web client uses it"). A video library gets Filters with every filter, saved views,
// Select and Edit together, Categories, Fix matches and Scan; a music library Play all and
// its Songs and Playlists; Collections New; a collection Play all, Edit, Delete and
// Remove titles. They sit under the page's title, as on the pages they come from.

import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { announce } from '../../a11y'
import { api, type Item, type Library, type Show } from '../../api'
import { useAudio } from '../../audio/player'
import { canPost, type Collection } from '../../gallery'
import { IconShuffle } from '../../audio/icons'
import { IconCheck, IconEdit, IconPlay, IconPlus, IconRefresh, IconTrash, IconTune } from '../../icons'
import type { BrowseResult, BrowseRow, SavedView } from '../../metadata'
import { toast, useApp } from '../../state'
import { Button, Dialog, LinkButton, ask } from '../../ui'
import { bump } from '../client'
import type { Document } from '../types'
import { toolsFor } from '../native'
import { ActiveFilters, FILTER_KEYS, FilterPanel, SaveViewDialog } from '../../pages/library/Filters'
import '../../pages/library/library.css'

const BulkEdit = lazy(() => import('../../pages/library/BulkEdit'))
const CategoryEditor = lazy(() => import('../../pages/library/CategoryEditor'))
const CollectionDialog = lazy(() => import('../../pages/CollectionsPage').then((m) => ({ default: m.CollectionDialog })))

export default function PageTools({ doc }: { doc: Document }) {
  const { libraries } = useApp()
  const kind = toolsFor(doc, libraries)
  const lib = libraries.find((l) => String(l.id) === doc.page.params?.library)
  if (kind === 'video' && lib) return <VideoTools lib={lib} />
  if (kind === 'audio' && lib) return <AudioTools lib={lib} />
  if (kind === 'collections') return <CollectionsTools />
  if (kind === 'collection') {
    const id = String(doc.page.params?.collection || '')
    return /^\d+$/.test(id) ? <CollectionTools id={Number(id)} /> : null
  }
  return null
}

// ---------------------------------------------------------------- a library of films or shows

function useParamsHere() {
  const loc = useLocation()
  const nav = useNavigate()
  const params = useMemo(() => new URLSearchParams(loc.search), [loc.search])
  const setParam = useCallback((k: string, v: string) => {
    const p = new URLSearchParams(loc.search)
    if (v) p.set(k, v); else p.delete(k)
    const s = p.toString()
    nav(loc.pathname + (s ? '?' + s : ''), { replace: true })
  }, [loc.pathname, loc.search, nav])
  const clear = useCallback(() => {
    const p = new URLSearchParams(loc.search)
    for (const k of FILTER_KEYS) p.delete(k)
    const s = p.toString()
    nav(loc.pathname + (s ? '?' + s : ''), { replace: true })
  }, [loc.pathname, loc.search, nav])
  return { params, setParam, clear }
}

function useScan(lib: Library, admin: boolean) {
  const [scanning, setScanning] = useState<{ done?: number; found?: number } | null>(null)
  useEffect(() => {
    if (!admin) return
    let t = 0
    let was = false
    let live = true
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: { done?: number; found?: number } | null }>(`/libraries/${lib.id}/scan`)
        if (!live) return
        setScanning(r.scanning ? r.progress || {} : null)
        // The page shows what the scan found once it has finished.
        if (was && !r.scanning) bump()
        was = r.scanning
        t = window.setTimeout(poll, r.scanning ? 1500 : 10000)
      } catch { if (live) t = window.setTimeout(poll, 10000) }
    }
    poll()
    return () => { live = false; window.clearTimeout(t) }
  }, [lib.id, admin])
  const start = async () => {
    try { await api.post(`/libraries/${lib.id}/scan`); setScanning({}); toast(`Scanning ${lib.name}`) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  return { scanning, start }
}

function ScanTool({ lib }: { lib: Library }) {
  const { user } = useApp()
  const { scanning, start } = useScan(lib, !!user?.isAdmin)
  if (!user?.isAdmin) return null
  return scanning
    ? <Button size="sm" variant="ghost" icon={<IconRefresh />} disabled>Scanning… {scanning.done ?? 0}/{scanning.found ?? '?'}</Button>
    : <Button size="sm" variant="ghost" icon={<IconRefresh />} onClick={() => void start()}>Scan</Button>
}

function VideoTools({ lib }: { lib: Library }) {
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const { params, setParam, clear } = useParamsHere()
  const [views, setViews] = useState<SavedView[]>([])
  const [queue, setQueue] = useState(0)
  const [open, setOpen] = useState<'' | 'filters' | 'save' | 'categories' | 'select'>('')
  const loadViews = useCallback(() => { api.get<{ views: SavedView[] }>('/views').then((r) => setViews(r.views.filter((v) => v.libraryId === lib.id))).catch(() => {}) }, [lib.id])
  useEffect(loadViews, [loadViews])
  useEffect(() => {
    if (!admin) return
    api.get<{ counts: Record<string, number> }>(`/match/queue?library=${lib.id}&limit=1`).then((r) => setQueue(r.counts.review || 0)).catch(() => {})
  }, [lib.id, admin])
  const active = FILTER_KEYS.filter((k) => params.get(k)).length
  const here = `/library/${lib.id}`
  return (
    <div className="sdui-tools" role="toolbar" aria-label={`${lib.name} tools`}>
      <div className="sdui-tools-row">
        <Button size="sm" variant="ghost" icon={<IconTune />} onClick={() => setOpen('filters')} aria-haspopup="dialog" data-tv-options>Filters{active > 0 ? ` (${active})` : ''}</Button>
        {admin && <Button size="sm" variant="ghost" icon={<IconCheck />} onClick={() => setOpen('select')} aria-haspopup="dialog">Select</Button>}
        {admin && <Button size="sm" variant="ghost" icon={<IconEdit />} onClick={() => setOpen('categories')} aria-haspopup="dialog">Categories</Button>}
        {admin && queue > 0 && <LinkButton size="sm" variant="ghost" to={`/fix-matches?library=${lib.id}`}>Fix matches <span className="lib-badge">{queue}</span></LinkButton>}
        <ScanTool lib={lib} />
      </div>
      <ActiveFilters params={params} setParam={setParam} onSave={() => setOpen('save')} onClear={clear} />
      {views.length > 0 && !active && (
        <div className="chips lib-views" role="group" aria-label="Saved views">
          {views.map((v) => <Link key={v.id} className="chip" to={`${here}?${v.query}`}>{v.name}</Link>)}
        </div>
      )}
      {open === 'filters' && <FilterPanel library={lib.id} params={params} setParam={setParam} onClear={clear} onSave={() => setOpen('save')} onClose={() => setOpen('')} />}
      {open === 'save' && <SaveViewDialog library={lib.id} params={params} onClose={() => setOpen('')} onSaved={() => { setOpen(''); loadViews(); toast('View saved'); bump() }} />}
      <Suspense fallback={null}>
        {open === 'categories' && <CategoryEditor library={lib.id} onClose={() => setOpen('')} onChanged={bump} />}
        {open === 'select' && <SelectTitles lib={lib} params={params} onClose={() => setOpen('')} />}
      </Suspense>
    </div>
  )
}

const rowKey = (r: BrowseRow) => r.type + (r.type === 'item' ? r.item.id : r.show.id)
const rowTitle = (r: BrowseRow) => (r.type === 'item' ? r.item.title : r.show.title)

// Choosing titles to edit together: the ones this page shows (its filters), as a list.
function SelectTitles({ lib, params, onClose }: { lib: Library; params: URLSearchParams; onClose: () => void }) {
  const [rows, setRows] = useState<BrowseRow[] | null>(null)
  const [total, setTotal] = useState(0)
  const [failed, setFailed] = useState('')
  const [chosen, setChosen] = useState<Set<string>>(() => new Set())
  const [find, setFind] = useState('')
  const [editing, setEditing] = useState(false)
  useEffect(() => {
    const q = new URLSearchParams()
    for (const k of FILTER_KEYS) { const v = params.get(k); if (v) q.set(k, v) }
    q.set('library', String(lib.id))
    q.set('sort', params.get('sort') || 'title')
    if (params.get('desc')) q.set('desc', '1')
    api.get<BrowseResult>(`/browse?${q}&limit=1000`).then((r) => { setRows(r.results); setTotal(r.total) }).catch((e) => setFailed((e as Error).message))
  }, [lib.id, params])
  const shown = (rows || []).filter((r) => !find || rowTitle(r).toLowerCase().includes(find.toLowerCase()))
  const flip = (k: string) => setChosen((s) => { const n = new Set(s); if (n.has(k)) n.delete(k); else n.add(k); return n })
  const all = shown.length > 0 && shown.every((r) => chosen.has(rowKey(r)))
  const picked = (rows || []).filter((r) => chosen.has(rowKey(r)))
  if (editing) {
    return <BulkEdit titles={picked.map((r) => (r.type === 'item' ? { target: 'item' as const, id: r.item.id, title: r.item.title } : { target: 'show' as const, id: r.show.id, title: r.show.title }))}
      onClose={() => setEditing(false)} onDone={() => { bump(); onClose() }} />
  }
  return (
    <Dialog title="Choose titles to edit together" size="wide" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" disabled={!picked.length} onClick={() => setEditing(true)}>Edit {picked.length || ''} together</Button></>}>
      {failed ? <p className="error" role="alert">{failed}</p> : !rows ? <p className="muted" role="status">Loading…</p> : (
        <div className="sdui-pick">
          <div className="sdui-pick-head">
            <input className="input" type="search" placeholder="Find a title" aria-label="Find a title" value={find} onChange={(e) => setFind(e.target.value)} />
            <label className="sdui-pick-all"><input type="checkbox" checked={all} onChange={() => setChosen((s) => { const n = new Set(s); for (const r of shown) { if (all) n.delete(rowKey(r)); else n.add(rowKey(r)) } return n })} /> All shown</label>
          </div>
          {total > rows.length && <p className="muted">The first {rows.length} of {total}. Filter the page to choose among the rest.</p>}
          <ul className="sdui-pick-list" aria-label="Titles">
            {shown.map((r) => {
              const k = rowKey(r)
              return <li key={k}><label><input type="checkbox" checked={chosen.has(k)} onChange={() => flip(k)} /> <span>{rowTitle(r)}</span>{r.type === 'item' && r.item.year ? <span className="muted"> · {r.item.year}</span> : null}</label></li>
            })}
          </ul>
        </div>
      )}
    </Dialog>
  )
}

// ---------------------------------------------------------------- music, audiobooks, podcasts

function AudioTools({ lib }: { lib: Library }) {
  const audio = useAudio()
  const music = lib.type === 'music'
  const playAll = async () => {
    try {
      const all = await api.get<Item[]>(`/music/songs?library=${lib.id}&sort=random&limit=500`)
      if (all.length) { audio.play(all, 0, 'music'); announce(`Playing ${all.length} songs`) } else toast('Nothing to play yet')
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  return (
    <div className="sdui-tools" role="toolbar" aria-label={`${lib.name} tools`}>
      <div className="sdui-tools-row">
        {music && <Button size="sm" variant="primary" icon={<IconShuffle />} onClick={() => void playAll()}>Play all</Button>}
        {music && <LinkButton size="sm" variant="ghost" to={`/library/${lib.id}?tab=songs`}>Songs</LinkButton>}
        {music && <LinkButton size="sm" variant="ghost" to={`/library/${lib.id}?tab=playlists`}>Playlists</LinkButton>}
        <ScanTool lib={lib} />
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- collections

function CollectionsTools() {
  const { user } = useApp()
  const nav = useNavigate()
  const [making, setMaking] = useState(false)
  if (!canPost(user)) return null
  return (
    <div className="sdui-tools" role="toolbar" aria-label="Collections tools">
      <div className="sdui-tools-row">
        <Button size="sm" variant="primary" icon={<IconPlus />} onClick={() => setMaking(true)} aria-haspopup="dialog">New collection</Button>
      </div>
      <Suspense fallback={null}>
        {making && <CollectionDialog onClose={() => setMaking(false)} onSaved={() => { setMaking(false); bump(); nav('/collections') }} />}
      </Suspense>
    </div>
  )
}

function CollectionTools({ id }: { id: number }) {
  const { user } = useApp()
  const nav = useNavigate()
  const audio = useAudio()
  const [got, setGot] = useState<{ collection: Collection; items: Item[]; shows: Show[] } | null>(null)
  const [open, setOpen] = useState<'' | 'edit' | 'remove'>('')
  const load = useCallback(() => { api.get<{ collection: Collection; items: Item[]; shows: Show[] }>(`/collections/${id}`).then(setGot).catch(() => setGot(null)) }, [id])
  useEffect(load, [load])
  if (!got) return null
  const c = got.collection
  const own = c.ownerId === user?.id || !!user?.isAdmin
  const items = got.items || []
  const playAll = () => {
    const songs = items.filter((x) => x.kind === 'audio')
    if (songs.length && songs.length === items.length) { audio.play(songs); return }
    const first = items.find((x) => x.kind === 'video' || x.kind.startsWith('dvd') || x.kind.startsWith('bluray'))
    if (first) nav(`/play/${first.id}?collection=${c.id}`)
    else toast('Nothing here plays one after another')
  }
  const remove = async () => {
    if (!await ask({ title: `Delete “${c.name}”?`, body: 'What’s in it stays in its library.', confirm: 'Delete', danger: true })) return
    try { await api.del(`/collections/${c.id}`); bump(); nav('/collections', { replace: true }) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const changed = () => { load(); bump() }
  return (
    <div className="sdui-tools" role="toolbar" aria-label={`${c.name} tools`}>
      <div className="sdui-tools-row">
        {items.length > 0 && <Button size="sm" variant="primary" icon={<IconPlay />} onClick={playAll}>Play all</Button>}
        {own && <Button size="sm" variant="ghost" icon={<IconEdit />} onClick={() => setOpen('edit')} aria-haspopup="dialog">Edit</Button>}
        {own && c.kind !== 'smart' && (items.length > 0 || got.shows.length > 0) && <Button size="sm" variant="ghost" onClick={() => setOpen('remove')} aria-haspopup="dialog">Remove titles</Button>}
        {own && !c.system && <Button size="sm" variant="ghost" icon={<IconTrash />} onClick={() => void remove()}>Delete</Button>}
      </div>
      <Suspense fallback={null}>
        {open === 'edit' && <CollectionDialog collection={c} onClose={() => setOpen('')} onSaved={() => { setOpen(''); changed() }} />}
      </Suspense>
      {open === 'remove' && <RemoveTitles c={c} items={items} shows={got.shows || []} onClose={() => setOpen('')} onChanged={changed} />}
    </div>
  )
}

function RemoveTitles({ c, items, shows, onClose, onChanged }: { c: Collection; items: Item[]; shows: Show[]; onClose: () => void; onChanged: () => void }) {
  const take = async (body: { items?: number[]; shows?: number[] }, name: string) => {
    try { await api.del(`/collections/${c.id}/items`, body); announce(`Removed ${name}`); onChanged() } catch (e) { toast((e as Error).message || 'Couldn’t change this collection', { tone: 'danger' }) }
  }
  return (
    <Dialog title={`Remove from ${c.name}`} onClose={onClose} actions={<Button variant="primary" onClick={onClose}>Done</Button>}>
      <ul className="sdui-pick-list" aria-label="Titles in this collection">
        {shows.map((s) => <li key={'s' + s.id}><span>{s.title}</span><Button size="sm" variant="ghost" aria-label={`Remove ${s.title}`} onClick={() => void take({ shows: [s.id] }, s.title)}>Remove</Button></li>)}
        {items.map((it) => <li key={it.id}><span>{it.title}</span><Button size="sm" variant="ghost" aria-label={`Remove ${it.title}`} onClick={() => void take({ items: [it.id] }, it.title)}>Remove</Button></li>)}
      </ul>
    </Dialog>
  )
}
