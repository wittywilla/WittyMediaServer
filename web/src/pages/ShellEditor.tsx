// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout editor (docs/plans/DESIGN-2.md 7.5, the canvas's LayoutEditor): a picture of the shell for each kind of device, where
// every region can be picked up and put down: OK or Enter picks one up, the arrows move it between the places it may go, OK drops it, Back
// cancels; or drag it with a mouse or a finger. The panel beside it has the same choices as named controls, so nothing needs the picture.
// For this profile on this device ("For me") or for everyone (administrators). The server merges the layers and sends the finished
// shell; this only edits the two layers' data (`/sdui/placements`, `/sdui/config/experience`).
import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { announce } from '../a11y'
import { api } from '../api'
import { bump, sduiAsk, SDUI_BASE } from '../sdui/client'
import { query, type Device } from '../sdui/device'
import { useDevice } from '../sdui/useSdui'
import { toast, useApp } from '../state'
import { loadThemeOverrides } from '../theme'
import { Button, Segmented, Select, Switch, ask } from '../ui'
import { Dialog } from '../ui/Overlay'
import { ranked } from '../ui/spatial'
import BrandDialog from '../components/BrandDialog'
import { applyPreset, DEVICES, deviceOf, LAYOUT_PRESETS, merged, zoneOf, type DeviceId, type Placement, type Placements, type Region, type Zone } from './layoutModel.ts'
import './layoutEditor.css'

type Answer = { experience: string; regions: Region[]; placements: Placements; own?: Placements; editable: boolean }
type Policy = { placements?: Record<string, Placements>; [k: string]: unknown }

const WORDS: Record<string, string> = { start: 'Left', end: 'Right', top: 'Top', bottom: 'Bottom', center: 'Centre', nav: 'In the menu', floating: 'Floating', dock: 'In the Dock', side: 'At the side', billboard: 'Billboard', banner: 'Banner', carousel: 'Carousel', row: 'Row' }
const ZONES: Zone[] = ['top', 'start', 'main', 'end', 'bottom']
const rtl = () => typeof document !== 'undefined' && getComputedStyle(document.documentElement).direction === 'rtl'
const word = (p: string) => (rtl() && (p === 'start' || p === 'end') ? (p === 'start' ? 'Right' : 'Left') : WORDS[p] || p)

// The device the picture is for: this window's own, or the form factor chosen. Another form factor is asked for without this
// browser's own name, so what is saved for it is kept for that form factor.
function deviceFor(own: Device, id: DeviceId): Device {
  if (deviceOf(own.formFactor) === id) return own
  const { name: _name, viewport: _viewport, deviceClass: _class, ...rest } = own
  void _name; void _viewport; void _class
  return { ...rest, formFactor: id, ...(id === 'tv' ? { deviceClass: 'hd' } : {}) }
}

export function ShellEditor({ onClose }: { onClose: () => void }) {
  const { user } = useApp()
  const own = useDevice()
  const admin = !!user?.isAdmin
  const [scope, setScope] = useState<'me' | 'everyone'>(admin ? 'everyone' : 'me')
  const [on, setOn] = useState<DeviceId>(deviceOf(own.formFactor))
  const device = useMemo(() => deviceFor(own, on), [own, on])
  const [got, setGot] = useState<Answer | null>(null)
  const [looks, setLooks] = useState<Record<string, string>>({})
  const [edit, setEditNow] = useState<Placements>({})
  // Undo and redo: every change is a step.
  const [past, setPast] = useState<Placements[]>([])
  const [future, setFuture] = useState<Placements[]>([])
  const setEdit = (next: Placements | ((e: Placements) => Placements)) => setEditNow((e) => { const n = typeof next === 'function' ? next(e) : next; setPast((p) => [...p, e]); setFuture([]); return n })
  const undo = () => setPast((p) => { if (!p.length) return p; setFuture((f) => [edit, ...f]); setEditNow(p[p.length - 1]); announce('Undone'); return p.slice(0, -1) })
  const redo = () => setFuture((f) => { if (!f.length) return f; setPast((p) => [...p, edit]); setEditNow(f[0]); announce('Redone'); return f.slice(1) })
  const [busy, setBusy] = useState(false)
  const [etag, setEtag] = useState('')
  const [policy, setPolicy] = useState<Policy>({})
  const [sel, setSel] = useState('nav')
  // The region picked up, and the place it is over: a position of its own.
  const [held, setHeld] = useState<{ id: string; over: string; drag?: boolean } | null>(null)
  const [naming, setNaming] = useState(false)
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
  useEffect(() => { void load().catch((e) => toast((e as Error).message, { tone: 'danger' })) }, [scope, on]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { void api.get<{ experiences: { id: string; name: string }[] }>('/sdui/experiences').then((r) => setLooks(Object.fromEntries(r.experiences.map((x) => [x.id, x.name])))).catch(() => {}) }, [])

  const stage = useRef<HTMLDivElement | null>(null)
  if (!got) return <Dialog title="Edit layout" size="wide" className="layout-editor" onClose={onClose}><p className="muted">Loading…</p></Dialog>

  const look = looks[got.experience] || got.experience
  const now = (r: Region): Placement => merged(got.placements, edit, r.id)
  const region = (id: string) => got.regions.find((r) => r.id === id)!
  const place = (r: Region) => now(r).position || r.positions?.[0]
  const setP = (r: Region, patch: Placement, say: string) => { setEdit((e) => ({ ...e, [r.id]: { ...(e[r.id] || {}), ...patch } })); announce(`${r.label}: ${say}`) }
  const navAt = place(region('nav')), dockAt = got.regions.some((r) => r.id === 'dock') ? place(region('dock')) : undefined
  const zone = (r: Region, position = place(r)): Zone => zoneOf(r.id, position, navAt, dockAt)
  const shown = got.regions.filter((r) => now(r).visible !== false)
  const hidden = got.regions.filter((r) => now(r).visible === false)
  const dirty = past.length > 0
  const heldRegionNow = held ? region(held.id) : null
  // A side column is there when something is in it, or something could be dropped in it.
  const used = (z: Zone) => shown.some((r) => zone(r) === z) || !!heldRegionNow?.positions?.some((p) => zone(heldRegionNow, p) === z)

  // Picking up, moving between the places a region may go, and dropping.
  const targets = () => Array.from(stage.current?.querySelectorAll<HTMLElement>('[data-drop]') || [])
  const pickUp = (r: Region, drag = false) => {
    if (!r.positions?.length) { announce(`${r.label} has no other place to go. Its choices are on the right.`); return }
    setSel(r.id)
    setHeld({ id: r.id, over: place(r) || '', drag })
    announce(`${r.label} picked up. Arrows choose where, OK drops it, Back cancels.`)
  }
  const dropOn = (position: string) => {
    if (!held) return
    const r = region(held.id)
    if (position !== place(r)) { setP(r, { position }, `moved to ${word(position).toLowerCase()}`) } else announce(`${r.label} stays where it was`)
    setHeld(null)
    requestAnimationFrame(() => stage.current?.querySelector<HTMLElement>(`[data-tile="${r.id}"]`)?.focus())
  }
  const cancel = () => { if (!held) return; announce('Cancelled'); const id = held.id; setHeld(null); requestAnimationFrame(() => stage.current?.querySelector<HTMLElement>(`[data-tile="${id}"]`)?.focus()) }
  const KEYS: Record<string, 'up' | 'down' | 'left' | 'right'> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }
  const onTileKey = (e: KeyboardEvent, r: Region) => {
    if (!held) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pickUp(r) }
      return
    }
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); dropOn(held.over) }
    else if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.key === 'Backspace') { e.preventDefault(); e.stopPropagation(); cancel() }
    else if (KEYS[e.key]) {
      e.preventDefault(); e.stopPropagation()
      const all = targets()
      const from = all.find((t) => t.dataset.drop === held.over)
      if (!from) return
      const to = ranked(from.getBoundingClientRect(), all.filter((t) => t !== from), (t) => t.getBoundingClientRect(), KEYS[e.key])[0]
      if (to) { setHeld({ ...held, over: to.dataset.drop || held.over }); announce(`Drop here: ${word(to.dataset.drop || '')}`) }
    }
  }
  // A mouse or a finger: press a tile's grip, move over a drop place, let go.
  const down = (e: PointerEvent, r: Region) => {
    if (e.button !== 0 || !r.positions?.length) return
    e.currentTarget.setPointerCapture(e.pointerId)
    setSel(r.id)
    setHeld({ id: r.id, over: place(r) || '', drag: true })
  }
  const overAt = (x: number, y: number) => document.elementsFromPoint(x, y).find((el) => (el as HTMLElement).dataset?.drop) as HTMLElement | undefined
  const move = (e: PointerEvent) => { if (held?.drag) { const t = overAt(e.clientX, e.clientY); if (t && t.dataset.drop !== held.over) setHeld({ ...held, over: t.dataset.drop || held.over }) } }
  const up = (e: PointerEvent) => { if (held?.drag) { const t = overAt(e.clientX, e.clientY); dropOn(t?.dataset.drop || held.over) } }

  const switchDevice = async (id: DeviceId) => {
    if (id === on) return
    if (dirty && !(await ask({ title: 'Leave these changes?', body: 'The layout is kept for each kind of device. Changes you haven’t saved for this one will be lost.', confirm: 'Leave them', cancel: 'Stay' }))) return
    setGot(null); setOn(id)
  }

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
      bump(); void loadThemeOverrides(); announce(`Back to the ${look} look’s own layout`); toast(`Back to the ${look} look’s own layout`, { tone: 'ok' })
      await load()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }

  const selected = region(sel) || got.regions[0]
  const sp = now(selected)
  const tileName = (r: Region) => { const p = now(r); return r.styles?.length ? `${r.label} · ${word(p.style || r.styles[0])}` : r.positions?.length ? `${r.label} · ${word(place(r) || '')}` : r.label }
  const heldRegion = held ? region(held.id) : null

  return (
    <Dialog title="Edit layout" size="wide" className="layout-editor" onClose={onClose}
      actions={<>
        <Button variant="primary" busy={busy} onClick={() => void save()}>{scope === 'everyone' ? 'Save for everyone' : 'Save for me'}</Button>
        <Button variant="ghost" disabled={!past.length} onClick={undo}>Undo</Button>
        <Button variant="ghost" disabled={!future.length} onClick={redo}>Redo</Button>
        <Button variant="ghost" disabled={busy} onClick={() => void reset()}>Reset to {look}</Button>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
      </>}>
      <div className="le-top">
        {admin && <Segmented label="Who this is for" value={scope} onChange={(v) => { if (v !== scope) { if (dirty) void ask({ title: 'Leave these changes?', body: 'Each is kept on its own.', confirm: 'Leave them', cancel: 'Stay' }).then((y) => { if (y) setScope(v) }); else setScope(v) } }} options={[{ value: 'everyone', label: 'For everyone' }, { value: 'me', label: 'For me' }]} />}
        <p className="muted">Where things sit in the {look} look. The menu and You can be moved but never hidden, so Settings and the way out are always there.</p>
      </div>
      {!got.editable && scope === 'me' && <p role="note" className="small-note">This profile may not rearrange. An administrator can allow it.</p>}

      <div className="le-body">
        <div className="le-left">
          <div ref={stage} className={'le-stage dev-' + on + (held ? ' holding' : '')} role="group" aria-label={`Layout of the ${look} look on a ${DEVICES.find((d) => d.id === on)?.label}`}
            style={{ gridTemplateColumns: `${used('start') ? '14rem' : '0'} minmax(0, 1fr) ${used('end') ? '14rem' : '0'}` }}
            onPointerMove={move} onPointerUp={up} onPointerCancel={() => setHeld(null)}>
            {ZONES.map((z) => {
              const here = shown.filter((r) => zone(r) === z)
              const drops = heldRegion ? (heldRegion.positions || []).filter((p) => zone(heldRegion, p) === z) : []
              return (
                <div key={z} className={'le-zone z-' + z}>
                  {here.map((r) => {
                    const isHeld = held?.id === r.id
                    const movable = !!r.positions?.length
                    return (
                      <div key={r.id} className={'le-tile' + (sel === r.id ? ' sel' : '') + (isHeld ? ' held' : '') + (r.id === 'hero' ? ' le-hero' : '')}>
                        <button type="button" data-tile={r.id} className="le-tile-main" aria-pressed={isHeld} aria-describedby="le-help"
                          aria-roledescription={movable ? 'movable region' : undefined}
                          onFocus={() => { if (!held) setSel(r.id) }} onClick={() => setSel(r.id)} onKeyDown={(e) => onTileKey(e, r)}>
                          {tileName(r)}{isHeld && <span className="sr-only"> (picked up)</span>}
                        </button>
                        {movable && <span className="le-grip" aria-hidden="true" onPointerDown={(e) => down(e, r)}>⠿</span>}
                      </div>
                    )
                  })}
                  {drops.map((p) => (
                    <button key={p} type="button" tabIndex={-1} data-drop={p} className={'le-drop' + (held?.over === p ? ' over' : '')} onClick={() => { if (held && !held.drag) dropOn(p) }}>
                      Drop here · {word(p)}{p === place(heldRegion!) ? ' (where it is)' : ''}
                    </button>
                  ))}
                </div>
              )
            })}
          </div>
          {hidden.length > 0 && (
            <div className="le-hidden" role="group" aria-label="Hidden regions">
              <span>Hidden</span>
              {hidden.map((r) => <Button key={r.id} size="sm" onClick={() => setP(r, { visible: true }, 'shown')}>Show {r.label}</Button>)}
            </div>
          )}
          <div className="le-keys" id="le-help">
            <span><kbd>OK</kbd> pick up</span><span><kbd>▲ ▼ ◀ ▶</kbd> move</span><span><kbd>OK</kbd> drop</span><span><kbd>Back</kbd> cancel</span>
            <span className="le-note">Moves are announced to screen readers. Drag works with a pointer or touch.</span>
          </div>
          <div className="le-devices" role="group" aria-label="Preview for">
            {DEVICES.map((d) => <button key={d.id} type="button" className={'le-device d-' + d.id + (d.id === on ? ' on' : '')} aria-pressed={d.id === on} onClick={() => void switchDevice(d.id)}>{d.label}</button>)}
          </div>
          <div className="le-presets" role="group" aria-label="Presets">
            <span>Presets</span>
            {LAYOUT_PRESETS.map((p) => <Button key={p.id} size="sm" onClick={() => { setEdit((e) => applyPreset(got.regions, e, p)); announce(p.label) }}>{p.label}</Button>)}
          </div>
        </div>

        <aside className="le-panel" aria-label="Selected region">
          <div className="le-eyebrow">Selected</div>
          <h3>{selected.label}</h3>
          <div className="le-rows">
            <Row label={`Show ${selected.label}`} text="Show"><Switch label={`Show ${selected.label}`} checked={sp.visible !== false} disabled={selected.locked} onChange={(v) => setP(selected, { visible: v }, v ? 'shown' : 'hidden')} /></Row>
            {selected.positions && selected.positions.length > 0 && <Row label="Position"><Select inputSize="sm" aria-label={`${selected.label} position`} value={place(selected)} onChange={(e) => setP(selected, { position: e.target.value }, `at ${word(e.target.value).toLowerCase()}`)} options={selected.positions.map((x) => ({ value: x, label: word(x) }))} /></Row>}
            {selected.styles && selected.styles.length > 0 && <Row label="Style"><Select inputSize="sm" aria-label={`${selected.label} style`} value={sp.style || selected.styles[0]} onChange={(e) => setP(selected, { style: e.target.value }, word(e.target.value))} options={selected.styles.map((x) => ({ value: x, label: word(x) }))} /></Row>}
          </div>
          <div className="le-eyebrow">Shell</div>
          <div className="le-rows">
            {(['nav', 'search', 'dock'] as const).filter((id) => got.regions.some((r) => r.id === id) && id !== selected.id).map((id) => {
              const r = region(id)
              return <Row key={id} label={id === 'nav' ? 'Navigation' : id === 'dock' ? 'Controls row (Dock)' : 'Search'}>
                <Select inputSize="sm" aria-label={id === 'nav' ? 'Navigation position' : id === 'dock' ? 'Controls row position' : 'Search position'} value={place(r)} onChange={(e) => setP(r, { position: e.target.value }, `at ${word(e.target.value).toLowerCase()}`)} options={(r.positions || []).map((x) => ({ value: x, label: word(x) }))} />
              </Row>
            })}
            {admin && <Row label="Server name"><Button size="sm" variant="ghost" onClick={() => setNaming(true)}>Edit name…</Button></Row>}
          </div>
          <p className="le-foot muted">Settings and You can’t be hidden. Reset puts everything back to the {look} look. The shelves and widgets on Home are arranged with Customise this page, at the foot of Home.</p>
        </aside>
      </div>
      {naming && <BrandDialog onClose={() => setNaming(false)} />}
    </Dialog>
  )
}

function Row({ label, text, children }: { label: string; text?: string; children: React.ReactNode }) {
  return <div className="le-row"><span>{text || label}</span>{children}</div>
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
