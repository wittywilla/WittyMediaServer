// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout editor (docs/plans/DESIGN-2.md 7.5): where each region of the shell is and whether it shows, for this
// profile on this device ("For me") or for everyone (administrators). Every control is a named select or switch, so it
// is complete by keyboard, pad, remote and touch, and each change is announced. The server merges the layers and sends
// the finished shell; this only edits the two layers' data (`/sdui/placements`, `/sdui/config/experience`).
import { useEffect, useState } from 'react'
import { announce } from '../a11y'
import { api } from '../api'
import { bump, sduiAsk, SDUI_BASE } from '../sdui/client'
import { query } from '../sdui/device'
import { useDevice } from '../sdui/useSdui'
import { toast, useApp } from '../state'
import { loadThemeOverrides } from '../theme'
import { Button } from '../ui'
import { Segmented } from '../ui/Choice'
import { Switch } from '../ui/Field'
import { Dialog } from '../ui/Overlay'

type Region = { id: string; label: string; positions?: string[]; locked?: boolean; styles?: string[] }
type Placement = { position?: string; visible?: boolean; style?: string }
type Answer = { experience: string; regions: Region[]; placements: Record<string, Placement>; own?: Record<string, Placement>; editable: boolean }
type Policy = { placements?: Record<string, Record<string, Placement>>; [k: string]: unknown }

const WORDS: Record<string, string> = { start: 'Start', end: 'End', top: 'Top', bottom: 'Bottom', center: 'Centre', nav: 'In the menu', floating: 'Floating', dock: 'In the Dock', side: 'Side', billboard: 'Billboard', banner: 'Banner', carousel: 'Carousel', row: 'Row' }

export function ShellEditor({ onClose }: { onClose: () => void }) {
  const { user } = useApp()
  const device = useDevice()
  const admin = !!user?.isAdmin
  const [scope, setScope] = useState<'me' | 'everyone'>('me')
  const [got, setGot] = useState<Answer | null>(null)
  const [edit, setEditNow] = useState<Record<string, Placement>>({})
  // Undo and redo: every change is a step.
  const [past, setPast] = useState<Record<string, Placement>[]>([])
  const [future, setFuture] = useState<Record<string, Placement>[]>([])
  const setEdit = (next: Record<string, Placement> | ((e: Record<string, Placement>) => Record<string, Placement>)) => setEditNow((e) => { const n = typeof next === 'function' ? next(e) : next; setPast((p) => [...p, e]); setFuture([]); return n })
  const undo = () => setPast((p) => { if (!p.length) return p; setFuture((f) => [edit, ...f]); setEditNow(p[p.length - 1]); announce('Undone'); return p.slice(0, -1) })
  const redo = () => setFuture((f) => { if (!f.length) return f; setPast((p) => [...p, edit]); setEditNow(f[0]); announce('Redone'); return f.slice(1) })
  const [busy, setBusy] = useState(false)
  const [etag, setEtag] = useState('')
  const [policy, setPolicy] = useState<Policy>({})
  const q = query(device)
  const load = async () => {
    const a = await api.get<Answer>('/sdui/placements?' + q)
    setGot(a)
    setPast([]); setFuture([])
    if (scope === 'me') setEditNow(a.own || {})
    else {
      const p = await api.get<{ etag: string; experiences: Policy }>('/sdui/config/experience')
      setEtag(p.etag); setPolicy(p.experiences || {})
      setEditNow(p.experiences?.placements?.[a.experience] || {})
    }
  }
  useEffect(() => { void load().catch((e) => toast((e as Error).message, { tone: 'danger' })) }, [scope]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!got) return <Dialog title="Edit layout" onClose={onClose}><p className="muted">Loading…</p></Dialog>
  const now = (r: Region): Placement => ({ ...(got.placements[r.id] || {}), ...(edit[r.id] || {}) })
  const set = (r: Region, patch: Placement, say: string) => { setEdit((e) => ({ ...e, [r.id]: { ...(e[r.id] || {}), ...patch } })); announce(`${r.label}: ${say}`) }
  const save = async () => {
    setBusy(true)
    try {
      if (scope === 'me') await sduiAsk('PUT', `${SDUI_BASE}/placements?${q}`, { body: { placements: edit } })
      else await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: { ...policy, placements: { ...(policy.placements || {}), [got.experience]: edit } }, headers: { 'If-Match': etag } })
      bump(); void loadThemeOverrides()
      toast('Layout saved', { tone: 'ok' }); announce('Layout saved')
      onClose()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }
  const reset = async () => {
    setBusy(true)
    try {
      if (scope === 'me') await sduiAsk('DELETE', `${SDUI_BASE}/placements?${q}`)
      else await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: { ...policy, placements: Object.fromEntries(Object.entries(policy.placements || {}).filter(([k]) => k !== got.experience)) }, headers: { 'If-Match': etag } })
      bump(); void loadThemeOverrides(); announce('Back to the look’s own layout'); toast('Back to the look’s own layout', { tone: 'ok' })
      await load()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }
  return (
    <Dialog title="Edit layout" size="wide" onClose={onClose}
      actions={<>
        <Button variant="primary" busy={busy} onClick={() => void save()}>Save</Button>
        <Button variant="ghost" disabled={!past.length} onClick={undo}>Undo</Button>
        <Button variant="ghost" disabled={!future.length} onClick={redo}>Redo</Button>
        <Button variant="ghost" disabled={busy} onClick={() => void reset()}>Reset to the look</Button>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
      </>}>
      <p className="muted">Where things sit in the {got.experience} look. The menu and You can be moved but never hidden, so Settings and the way out are always there.</p>
      {admin && <Segmented label="Who this is for" value={scope} onChange={(v) => setScope(v as 'me' | 'everyone')} options={[{ value: 'me', label: 'For me' }, { value: 'everyone', label: 'For everyone' }]} />}
      {!got.editable && scope === 'me' && <p role="note" className="small-note">This profile may not rearrange. An administrator can allow it.</p>}
      <div className="shell-regions" role="group" aria-label="Regions">
        {got.regions.map((r) => {
          const p = now(r)
          return (
            <div key={r.id} className="shell-region">
              <b>{r.label}</b>
              {r.positions && r.positions.length > 0 && (
                <label>Position
                  <select className="select" value={p.position || r.positions[0]} onChange={(e) => set(r, { position: e.target.value }, `at ${WORDS[e.target.value] || e.target.value}`)}>
                    {r.positions.map((x) => <option key={x} value={x}>{WORDS[x] || x}</option>)}
                  </select>
                </label>
              )}
              {r.styles && r.styles.length > 0 && (
                <label>Style
                  <select className="select" value={p.style || r.styles[0]} onChange={(e) => set(r, { style: e.target.value }, WORDS[e.target.value] || e.target.value)}>
                    {r.styles.map((x) => <option key={x} value={x}>{WORDS[x] || x}</option>)}
                  </select>
                </label>
              )}
              <Switch label={`Show ${r.label}`} checked={p.visible !== false} disabled={r.locked} onChange={(on) => set(r, { visible: on }, on ? 'shown' : 'hidden')} />
            </div>
          )
        })}
      </div>
    </Dialog>
  )
}

// The editor wherever it is asked for (Settings, Appearance, or Ctrl+Shift+L): one host in the app.
export const LAYOUT_EVENT = 'wms:layout-editor'
export function ShellEditorHost() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const on = () => setOpen(true)
    window.addEventListener(LAYOUT_EVENT, on)
    return () => window.removeEventListener(LAYOUT_EVENT, on)
  }, [])
  return open ? <ShellEditor onClose={() => setOpen(false)} /> : null
}
