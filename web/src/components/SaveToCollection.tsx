// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { canPost, type Collection } from '../gallery'
import { IconPlus } from '../icons'
import { toast, useApp } from '../state'
import { Dialog } from '../ui/Overlay'
import SpecPage from '../spec/SpecPage'
import { handledIds, withIds } from '../spec/model'
import { saveSpec } from '../spec/specs/collections'

// SaveButton: Save, on a film, show or video's own page.
export function SaveButton({ itemId, showId, label = 'Save' }: { itemId?: number; showId?: number; label?: string }) {
  const { user } = useApp()
  const [open, setOpen] = useState(false)
  if (!canPost(user) && user?.kind !== 'kids') return null
  return (
    <>
      <button className="btn" onClick={() => setOpen(true)}><IconPlus /> {label}</button>
      {open && <SaveDialog itemId={itemId} showId={showId} onClose={() => setOpen(false)} />}
    </>
  )
}

function SaveDialog({ itemId, showId, onClose }: { itemId?: number; showId?: number; onClose: () => void }) {
  const { user } = useApp()
  const [list, setList] = useState<Collection[] | null>(null)
  const [holding, setHolding] = useState<number[]>([])
  const [name, setName] = useState('')
  const body = { items: itemId ? [itemId] : [], shows: showId ? [showId] : [] }
  const load = useCallback(() => {
    api.get<Collection[]>('/collections').then((l) => setList(l.filter((c) => c.kind !== 'smart' && c.ownerId === user?.id))).catch(() => setList([]))
    api.get<number[]>(`/collections/holding?item=${itemId || 0}&show=${showId || 0}`).then(setHolding).catch(() => {})
  }, [itemId, showId, user?.id])
  useEffect(load, [load])
  const toggle = async (c: Collection) => {
    try {
      if (holding.includes(c.id)) {
        const res = await fetch(`/api/collections/${c.id}/items`, { method: 'DELETE', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: JSON.stringify(body) })
        if (!res.ok) throw new Error()
        setHolding(holding.filter((x) => x !== c.id))
      } else {
        await api.post(`/collections/${c.id}/items`, body)
        setHolding([...holding, c.id])
      }
    } catch { toast("Couldn't save that") }
  }
  const later = async () => {
    try { const r = await api.post<{ collectionId: number }>('/collections/watch-later/items', body); setHolding([...holding, r.collectionId]); load(); toast('Saved to Watch later') } catch (x: any) { toast(x.message) }
  }
  const make = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    try {
      const c = await api.post<Collection>('/collections', { name, kind: 'manual' })
      await api.post(`/collections/${c.id}/items`, body)
      setName(''); load()
    } catch (x: any) { toast(x.message) }
  }
  const hasLater = !!list?.some((c) => c.system === 'watchlater')
  const page = useMemo(() => saveSpec((list || []).map((c) => ({ id: c.id, name: c.name })), holding, hasLater), [list, holding, hasLater])
  const press = (id: string) => {
    if (id === 'later') later()
    else if (id === 'done') onClose()
    else { const c = list?.find((x) => 'list:' + x.id === id); if (c) toggle(c) }
  }
  return (
    <Dialog title="Save to" onClose={onClose}>
      {list && <SpecPage page={page} embedded handled={handledIds(withIds(page))} onAction={press} />}
      {canPost(user) && <form className="share-link" onSubmit={make}><input className="input" placeholder="New collection" aria-label="New collection's name" maxLength={200} value={name} onChange={(e) => setName(e.target.value)} /><button className="btn">Add</button></form>}
    </Dialog>
  )
}
