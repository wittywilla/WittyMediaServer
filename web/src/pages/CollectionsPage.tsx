// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import SpecPage from '../spec/SpecPage'
import { handledIds, withIds } from '../spec/model'
import { applyRuleField, collectionSpec, genresOf, toggleType } from '../spec/specs/smartlist'
import { api, fitWidth, posterUrl, sized, type Item } from '../api'
import { type Collection, type CollectionRule } from '../gallery'
import '../gallery.css'
import { Dialog } from '../ui/Overlay'
import { Button } from '../ui/Button'

// Collections and smart playlists, across every library. A smart playlist is a
// saved rule the server works out each time (docs/COLLECTIONS.md); this page
// lists them and builds the rule.

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
