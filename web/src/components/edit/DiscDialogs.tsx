// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// An administrator's tools for one disc, from a title page's "Manage this title" menu: the
// disc's own settings (what's on it, how it starts, the insert effect) and the names of its
// titles and scenes, which the disc's page and the player show.
import { useEffect, useState } from 'react'
import { api, discName, type DiscLabels, type DiscView, type Item, type Prefs, getPrefs } from '../../api'
import { Button, Dialog, Field, Input, Select, Spinner, toast } from '../../ui'

export function DiscSettingsDialog({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: (it: Item) => void }) {
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  const [playMode, setPlayMode] = useState(item.playMode)
  const [insertFx, setInsertFx] = useState(item.insertFx)
  const [content, setContent] = useState(item.disc?.contentManual ? item.disc.content || '' : '')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { getPrefs().then(setPrefs).catch(() => {}) }, [])
  const save = async () => {
    setBusy(true); setErr('')
    try { onSaved(await api.put<Item>(`/items/${item.id}`, { playMode, insertFx, discContent: content })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const guessed = !item.disc?.contentManual && item.disc?.content === 'tv' ? 'TV series' : 'movie'
  const def = prefs?.dvdStart === 'title' ? 'main feature' : 'disc menu'
  const name = discName(item)
  return (
    <Dialog title="Disc settings" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" busy={busy} onClick={save}>Save</Button></>}>
      <Field label="What’s on this disc" hint="Sorts the disc under Movies or TV series in its library. Discs of a series aren’t looked up as movies.">
        <Select value={content} onChange={(e) => setContent(e.target.value)} options={[
          { value: '', label: `Work it out from the name (${item.disc?.contentManual ? 'automatic' : guessed})` },
          { value: 'movie', label: 'A movie' }, { value: 'tv', label: 'Episodes of a TV series' }]} />
      </Field>
      <Field label="When this disc is inserted">
        <Select value={playMode} onChange={(e) => setPlayMode(e.target.value as Item['playMode'])} options={[
          { value: '', label: `Server default (${def})` },
          { value: 'menu', label: `Start with the disc menu (like a ${name} player)`, disabled: !item.disc?.hasMenus },
          { value: 'title', label: 'Skip menus and play the main feature' }]} />
      </Field>
      <Field label={`Insert ${name} effect (tray animation and sound)`} error={err || undefined}>
        <Select value={insertFx} onChange={(e) => setInsertFx(e.target.value as Item['insertFx'])} options={[
          { value: '', label: `Server default (${prefs?.insertFx === false ? 'off' : 'on'})` },
          { value: 'on', label: 'On for this disc' }, { value: 'off', label: 'Off for this disc' }]} />
      </Field>
    </Dialog>
  )
}

// DiscNamesDialog names the film, its scenes and the bonus titles. Names are kept on the
// server for everyone and shown on the disc's page and in the player.
export function DiscNamesDialog({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: () => void }) {
  const [view, setView] = useState<DiscView | null>(null)
  const [labels, setLabels] = useState<DiscLabels>({ titles: {}, chapters: {} })
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const unit = item.disc?.format === 'bluray' ? 'playlist' : 'title'
  useEffect(() => {
    api.get<DiscView>(`/items/${item.id}/disc`).then((v) => {
      const l: DiscLabels = { titles: {}, chapters: {} }
      for (const t of [v.main, ...v.extras]) {
        if (!t) continue
        if (t.label) l.titles[t.ttn] = t.label
        for (const c of t.chapters) if (c.label) (l.chapters[t.ttn] ||= {})[c.n] = c.label
      }
      setLabels(l); setView(v)
    }).catch((e) => setErr(e.message))
  }, [item.id])
  const setTitle = (ttn: number, v: string) => setLabels((l) => ({ ...l, titles: { ...l.titles, [ttn]: v } }))
  const setChapter = (ttn: number, n: number, v: string) => setLabels((l) => ({ ...l, chapters: { ...l.chapters, [ttn]: { ...(l.chapters[ttn] || {}), [n]: v } } }))
  const save = async () => {
    setBusy(true); setErr('')
    try { await api.put(`/items/${item.id}/disc/labels`, labels); toast('Names saved', { tone: 'ok' }); onSaved() } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const main = view?.main
  return (
    <Dialog title="Name titles and scenes" size="wide" onClose={onClose} description={`Your own names for the film, its scenes and the disc’s other ${unit}s, for everyone.`}
      actions={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" busy={busy} disabled={!view} onClick={save}>Save names</Button></>}>
      {!view && !err && <Spinner />}
      {err && <p className="error" role="alert">{err}</p>}
      {main && (
        <>
          <Field label="The film"><Input value={labels.titles[main.ttn] || ''} placeholder={item.title} onChange={(e) => setTitle(main.ttn, e.target.value)} /></Field>
          {main.chapters.map((c) => (
            <Field key={c.n} label={`Scene ${c.n}`}><Input value={labels.chapters[main.ttn]?.[c.n] || ''} placeholder={`Scene ${c.n}`} onChange={(e) => setChapter(main.ttn, c.n, e.target.value)} /></Field>
          ))}
        </>
      )}
      {view?.extras.map((t, i) => (
        <Field key={t.ttn} label={`Bonus ${unit} ${t.ttn}`}><Input value={labels.titles[t.ttn] || ''} placeholder={`Bonus feature ${i + 1}`} onChange={(e) => setTitle(t.ttn, e.target.value)} /></Field>
      ))}
    </Dialog>
  )
}
