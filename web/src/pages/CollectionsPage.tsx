// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import SpecPage from '../spec/SpecPage'
import { handledIds, withIds } from '../spec/model'
import { applyRuleField, collectionSpec, genresOf, toggleType } from '../spec/specs/smartlist'
import { useNavigate, useParams } from 'react-router-dom'
import { api, fitWidth, posterUrl, sized, type Item, type Show } from '../api'
import { useAudio } from '../audio/player'
import { BackLink, EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'
import { type Collection, type CollectionRule } from '../gallery'
import { IconClose, IconEdit, IconLibrary, IconPlay, IconTrash } from '../icons'
import { toast, useApp } from '../state'
import '../gallery.css'
import { Dialog, ask } from '../ui/Overlay'
import { Button } from '../ui/Button'

// Collections and smart playlists, across every library. A smart playlist is a
// saved rule the server works out each time (docs/COLLECTIONS.md); this page
// lists them and builds the rule.

export function CollectionPage() {
  const { id } = useParams()
  const { user } = useApp()
  const nav = useNavigate()
  const audio = useAudio()
  const [c, setC] = useState<Collection | null>(null)
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[]>([])
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const load = useCallback(() => {
    api.get<{ collection: Collection; items: Item[]; shows: Show[] }>(`/collections/${id}`).then((r) => { setC(r.collection); setItems(r.items); setShows(r.shows || []) }).catch((e) => setErr(e.message))
  }, [id])
  useEffect(load, [load])
  if (err) return <EmptyState title="Collection not found" text={err} />
  if (!c || !items) return <Spinner />
  const own = c.ownerId === user?.id || user?.isAdmin
  const playAll = () => {
    const songs = items.filter((x) => x.kind === 'audio')
    if (songs.length && songs.length === items.length) { audio.play(songs); return }
    const first = items.find((x) => x.kind === 'video' || x.kind.startsWith('dvd') || x.kind.startsWith('bluray'))
    if (first) nav(`/play/${first.id}?collection=${c.id}`)
  }
  const remove = async () => {
    if (!await ask({ title: `Delete “${c.name}”?`, body: 'What’s in it stays in its library.', confirm: 'Delete', danger: true })) return
    await api.del(`/collections/${c.id}`)
    nav('/collections', { replace: true })
  }
  const take = async (body: { items?: number[]; shows?: number[] }) => {
    try {
      const res = await fetch(`/api/collections/${c.id}/items`, { method: 'DELETE', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: JSON.stringify(body) })
      if (!res.ok) throw new Error()
      load()
    } catch { toast("Couldn't change this collection") }
  }
  return (
    <>
      <BackLink to="/collections" label="Collections" />
      <div className="toolbar">
        <h1>{c.name}</h1>
        <span className="count">{c.count} {c.count === 1 ? 'item' : 'items'}{c.kind === 'smart' ? ' · smart' : ''}</span>
        <div style={{ flex: 1 }} />
        {items.length > 0 && <button className="btn sm primary" onClick={playAll}><IconPlay /> Play all</button>}
        {own && <button className="btn sm ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
        {own && !c.system && <button className="btn sm ghost" onClick={remove}><IconTrash /> Delete</button>}
      </div>
      {c.description && <p className="muted" style={{ marginTop: -6 }}>{c.description}</p>}
      {items.length + shows.length === 0 ? <EmptyState icon={<IconLibrary />} title="Nothing here yet" text={c.kind === 'smart' ? 'Nothing matches this rule at the moment.' : 'Use Save on a movie, show or video to add it.'} /> : (
        <div className="grid">
          {shows.map((s) => <div key={'s' + s.id} className="col-cell"><ShowCard show={s} />{own && c.kind !== 'smart' && <button className="btn sm ghost col-x" aria-label={`Remove ${s.title}`} onClick={() => take({ shows: [s.id] })}><IconClose /></button>}</div>)}
          {items.map((it) => <div key={it.id} className="col-cell"><ItemCard item={it} />{own && c.kind !== 'smart' && <button className="btn sm ghost col-x" aria-label={`Remove ${it.title}`} onClick={() => take({ items: [it.id] })}><IconClose /></button>}</div>)}
        </div>
      )}
      {edit && <CollectionDialog collection={c} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
    </>
  )
}

export function CollectionDialog({ collection, onClose, onSaved }: { collection?: Collection; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(collection?.name || '')
  const [desc, setDesc] = useState(collection?.description || '')
  const [shared, setShared] = useState(collection?.shared || false)
  const [kind, setKind] = useState<'manual' | 'smart'>(collection?.kind === 'smart' ? 'smart' : 'manual')
  const [rule, setRule] = useState<CollectionRule>(collection?.rule || { sort: 'added', desc: true, limit: 100 })
  const [genresText, setGenresText] = useState(collection?.rule?.genres?.join(', ') || '')
  const [preview, setPreview] = useState<{ count: number; items: Item[] } | null>(null)
  const [err, setErr] = useState('')
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const ruleOut = useMemo(() => ({ ...rule, genres: genresOf(genresText) }), [rule, genresText])
  useEffect(() => {
    if (kind !== 'smart') return
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      api.post<{ count: number; items: Item[] }>('/collections/preview', { rule: ruleOut }).then(setPreview).catch(() => setPreview(null))
    }, 300)
    return () => clearTimeout(timer.current)
  }, [ruleOut, kind])
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setErr('')
    if (!name.trim()) { setErr('Give it a name.'); return }
    const body = { name, description: desc, kind, shared, rule: kind === 'smart' ? ruleOut : undefined }
    try {
      if (collection) await api.put(`/collections/${collection.id}`, body)
      else await api.post('/collections', body)
      onSaved()
    } catch (x: any) { setErr(x.message) }
  }
  const spec = useMemo(() => collectionSpec({ name, desc, kind, canChooseKind: !collection, rule, genresText, shared, canShare: !collection?.system, preview, error: err }), [name, desc, kind, collection, rule, genresText, shared, preview, err])
  const handled = useMemo(() => handledIds(withIds(spec)), [spec])
  const onField = (id: string, v: string | number | boolean | File) => {
    if (id === 'name') setName(String(v))
    else if (id === 'desc') setDesc(String(v))
    else if (id === 'shared') setShared(!!v)
    else if (id === 'genres') setGenresText(String(v))
    else setRule((r) => applyRuleField(r, id, v as string | number | boolean))
  }
  const onAction = (id: string) => {
    if (id === 'kind:manual') setKind('manual')
    else if (id === 'kind:smart') setKind('smart')
    else if (id.startsWith('type:')) setRule((r) => toggleType(r, id.slice(5)))
  }
  const matchIndex = spec.sections.findIndex((x) => x.kind === 'note' && x.text.includes('match'))
  return (
    <Dialog title={collection ? 'Edit collection' : 'New collection'} size="wide" className="col-dialog" onClose={onClose}
      actions={<><Button type="button" variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" form="col-form">{collection ? 'Save' : 'Make it'}</Button></>}>
      <form id="col-form" className="ui-dialog-form" onSubmit={save}>
        <SpecPage embedded page={spec} handled={handled} onAction={onAction} onField={onField} after={matchIndex >= 0 ? { [matchIndex]: <div className="col-preview-row">{preview?.items.filter((it) => it.poster).slice(0, 8).map((it) => <img key={it.id} src={sized(posterUrl(it), fitWidth(96))} alt={it.title} title={it.title} loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />)}</div> } : undefined} />
      </form>
    </Dialog>
  )
}
