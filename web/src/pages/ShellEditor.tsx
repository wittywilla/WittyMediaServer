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
import { LayoutThumb } from '../ui/LayoutThumb'
import type { WireMap } from '../ui/layoutSpecs'
import { ranked } from '../ui/spatial'
import BrandDialog from '../components/BrandDialog'
import { applyPreset, DEVICES, deviceOf, LAYOUT_PRESETS, merged, zoneOf, type DeviceId, type Placement, type Placements, type Region, type Zone } from './layoutModel.ts'
import './layoutEditor.css'
import { useTvMode } from '../tvmode/mode'
import { OnAComputerDialog } from '../tvmode/OnAComputer'
import { t, tk, tx } from '../i18n'

type Answer = { experience: string; regions: Region[]; placements: Placements; own?: Placements; editable: boolean }
type Policy = { placements?: Record<string, Placements>; [k: string]: unknown }

const WORDS: Record<string, string> = { start: tk('Left'), end: tk('Right'), top: tk('Top'), bottom: tk('Bottom'), center: tk('Centre'), nav: tk('In the menu'), floating: tk('Floating'), dock: tk('In the Dock'), side: tk('At the side'), billboard: tk('Billboard'), banner: tk('Banner'), carousel: tk('Carousel'), row: tk('Row') }
const ZONES: Zone[] = ['top', 'start', 'main', 'end', 'bottom']
const rtl = () => typeof document !== 'undefined' && getComputedStyle(document.documentElement).direction === 'rtl'
const word = (p: string) => (rtl() && (p === 'start' || p === 'end') ? tx(p === 'start' ? WORDS.end : WORDS.start) : tx(WORDS[p]) || p)

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
  // How each look draws its menu on each kind of device, so the picture shows the phone's bottom bar and not a side menu.
  const [navs, setNavs] = useState<Record<string, Record<string, string>>>({})
  const [wires, setWires] = useState<Record<string, WireMap | undefined>>({})
  const [edit, setEditNow] = useState<Placements>({})
  // Undo and redo: every change is a step.
  const [past, setPast] = useState<Placements[]>([])
  const [future, setFuture] = useState<Placements[]>([])
  const setEdit = (next: Placements | ((e: Placements) => Placements)) => setEditNow((e) => { const n = typeof next === 'function' ? next(e) : next; setPast((p) => [...p, e]); setFuture([]); return n })
  const undo = () => setPast((p) => { if (!p.length) return p; setFuture((f) => [edit, ...f]); setEditNow(p[p.length - 1]); announce(t('Undone')); return p.slice(0, -1) })
  const redo = () => setFuture((f) => { if (!f.length) return f; setPast((p) => [...p, edit]); setEditNow(f[0]); announce(t('Redone')); return f.slice(1) })
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
  useEffect(() => { void api.get<{ experiences: { id: string; name: string; nav?: Record<string, string>; wire?: WireMap }[] }>('/sdui/experiences').then((r) => { setWires(Object.fromEntries(r.experiences.map((x) => [x.id, x.wire]))); setLooks(Object.fromEntries(r.experiences.map((x) => [x.id, x.name]))); setNavs(Object.fromEntries(r.experiences.map((x) => [x.id, x.nav || {}]))) }).catch(() => {}) }, [])

  const stage = useRef<HTMLDivElement | null>(null)
  if (!got) return <Dialog title={t('Edit layout')} size="wide" className="layout-editor" onClose={onClose}><p className="muted">{t('Loading…')}</p></Dialog>

  const look = tx(looks[got.experience]) || got.experience
  const now = (r: Region): Placement => merged(got.placements, edit, r.id)
  const region = (id: string) => got.regions.find((r) => r.id === id)!
  // The menu's place when nothing has been chosen is the look's own for this device (the server's placements name only a default).
  const navStyle = navs[got.experience]?.[on === 'desktop' && !navs[got.experience]?.desktop ? 'web' : on]
  const navDefault = !navStyle ? undefined : ['sidebar', 'rail', 'railDrawer', 'tvSideMenu', 'ornamentTabs'].includes(navStyle) ? 'start' : navStyle === 'bottomTabs' ? 'bottom' : 'top'
  const place = (r: Region): string | undefined => {
    const own = merged({}, edit, r.id).position
    if (own) return own
    if (r.id === 'nav' && navDefault) return navDefault
    if (r.id === 'brand' && navDefault && navDefault !== 'start' && (got.placements.brand?.position || 'start') === 'start') return 'top'
    return got.placements[r.id]?.position || r.positions?.[0]
  }
  const setP = (r: Region, patch: Placement, say: string) => { setEdit((e) => ({ ...e, [r.id]: { ...(e[r.id] || {}), ...patch } })); announce(t('{region}: {change}', { region: tx(r.label), change: say })) }
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
    if (!r.positions?.length) { announce(t('{region} has no other place to go. Its choices are in the panel beside the picture.', { region: tx(r.label) })); return }
    setSel(r.id)
    setHeld({ id: r.id, over: place(r) || '', drag })
    announce(t('{region} picked up. Arrows choose where, OK drops it, Back cancels.', { region: tx(r.label) }))
  }
  const dropOn = (position: string) => {
    if (!held) return
    const r = region(held.id)
    if (position !== place(r)) { setP(r, { position }, t('moved to {place}', { place: word(position).toLowerCase() })) } else announce(t('{region} stays where it was', { region: tx(r.label) }))
    setHeld(null)
    requestAnimationFrame(() => stage.current?.querySelector<HTMLElement>(`[data-tile="${r.id}"]`)?.focus())
  }
  const cancel = () => { if (!held) return; announce(t('Cancelled')); const id = held.id; setHeld(null); requestAnimationFrame(() => stage.current?.querySelector<HTMLElement>(`[data-tile="${id}"]`)?.focus()) }
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
      if (to) { setHeld({ ...held, over: to.dataset.drop || held.over }); announce(t('Drop here: {place}', { place: word(to.dataset.drop || '') })) }
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
    if (dirty && !(await ask({ title: t('Leave these changes?'), body: t('The layout is kept for each kind of device. Changes you haven’t saved for this one will be lost.'), confirm: t('Leave them'), cancel: t('Stay') }))) return
    setGot(null); setOn(id)
  }

  const save = async () => {
    setBusy(true)
    try {
      if (scope === 'me') await sduiAsk('PUT', `${SDUI_BASE}/placements?${q}`, { body: { placements: edit } })
      else await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: { ...policy, placements: { ...(policy.placements || {}), [got.experience]: edit } }, headers: { 'If-Match': etag } })
      bump(); void loadThemeOverrides()
      toast(t('Layout saved'), { tone: 'ok' }); announce(t('Layout saved'))
      onClose()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }
  const reset = async () => {
    setBusy(true)
    try {
      if (scope === 'me') await sduiAsk('DELETE', `${SDUI_BASE}/placements?${q}`)
      else await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: { ...policy, placements: Object.fromEntries(Object.entries(policy.placements || {}).filter(([k]) => k !== got.experience)) }, headers: { 'If-Match': etag } })
      bump(); void loadThemeOverrides(); announce(t('Back to the {look} look’s own layout', { look })); toast(t('Back to the {look} look’s own layout', { look }), { tone: 'ok' })
      await load()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }

  const selected = region(sel) || got.regions[0]
  const sp = now(selected)
  const tileName = (r: Region) => { const p = now(r); return r.styles?.length ? `${tx(r.label)} · ${word(p.style || r.styles[0])}` : r.positions?.length ? `${tx(r.label)} · ${word(place(r) || '')}` : tx(r.label) }
  const heldRegion = held ? region(held.id) : null

  return (
    <Dialog title={t('Edit layout')} size="wide" className="layout-editor" onClose={onClose}
      actions={<>
        <Button variant="primary" busy={busy} onClick={() => void save()}>{scope === 'everyone' ? t('Save for everyone') : t('Save for me')}</Button>
        <Button variant="ghost" disabled={!past.length} onClick={undo}>{t('Undo')}</Button>
        <Button variant="ghost" disabled={!future.length} onClick={redo}>{t('Redo')}</Button>
        <Button variant="ghost" disabled={busy} onClick={() => void reset()}>{t('Reset to {look}', { look })}</Button>
        <Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button>
      </>}>
      <div className="le-top">
        {admin && <Segmented label={t('Who this is for')} value={scope} onChange={(v) => { if (v !== scope) { if (dirty) void ask({ title: t('Leave these changes?'), body: t('Each is kept on its own.'), confirm: t('Leave them'), cancel: t('Stay') }).then((y) => { if (y) setScope(v) }); else setScope(v) } }} options={[{ value: 'everyone', label: t('For everyone') }, { value: 'me', label: t('For me') }]} />}
        <p className="muted">{t('Where things sit in the {look} look. The menu and You can be moved but never hidden, so Settings and the way out are always there.', { look })}</p>
      </div>
      {!got.editable && scope === 'me' && <p role="note" className="small-note">{t('This profile may not rearrange. An administrator can allow it.')}</p>}

      <div className="le-body">
        <div className="le-left">
          <div ref={stage} className={'le-stage dev-' + on + (held ? ' holding' : '')} role="group" aria-label={t('Layout of the {look} look on a {device}', { look, device: tx(DEVICES.find((d) => d.id === on)?.label) })}
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
                          aria-roledescription={movable ? t('movable region') : undefined}
                          onFocus={() => { if (!held) setSel(r.id) }} onClick={() => setSel(r.id)} onKeyDown={(e) => onTileKey(e, r)}>
                          {tileName(r)}{isHeld && <span className="sr-only"> {t('(picked up)')}</span>}
                        </button>
                        {movable && <span className="le-grip" aria-hidden="true" onPointerDown={(e) => down(e, r)}>⠿</span>}
                      </div>
                    )
                  })}
                  {drops.map((p) => (
                    <button key={p} type="button" tabIndex={-1} data-drop={p} className={'le-drop' + (held?.over === p ? ' over' : '')} onClick={() => { if (held && !held.drag) dropOn(p) }}>
                      {p === place(heldRegion!) ? t('Drop here · {place} (where it is)', { place: word(p) }) : t('Drop here · {place}', { place: word(p) })}
                    </button>
                  ))}
                </div>
              )
            })}
          </div>
          {hidden.length > 0 && (
            <div className="le-hidden" role="group" aria-label={t('Hidden regions')}>
              <span>{t('Hidden')}</span>
              {hidden.map((r) => <Button key={r.id} size="sm" onClick={() => setP(r, { visible: true }, t('shown'))}>{t('Show {region}', { region: tx(r.label) })}</Button>)}
            </div>
          )}
          <div className="le-keys" id="le-help">
            <span><kbd>{t('OK')}</kbd> {t('pick up')}</span><span><kbd>▲ ▼ ◀ ▶</kbd> {t('move')}</span><span><kbd>{t('OK')}</kbd> {t('drop')}</span><span><kbd>{t('Back')}</kbd> {t('cancel')}</span>
            <span className="le-note">{t('Moves are announced to screen readers. Drag works with a pointer or touch.')}</span>
          </div>
          <div className="le-devices" role="group" aria-label={t('Preview for')}>
            {DEVICES.map((d) => <button key={d.id} type="button" className={'le-device d-' + d.id + (d.id === on ? ' on' : '')} aria-pressed={d.id === on} onClick={() => void switchDevice(d.id)}><LayoutThumb wire={wires[got.experience]} formFactor={d.id} className="le-thumb" /><span>{tx(d.label)}</span></button>)}
          </div>
          <div className="le-presets" role="group" aria-label={t('Presets')}>
            <span>{t('Presets')}</span>
            {LAYOUT_PRESETS.map((p) => <Button key={p.id} size="sm" onClick={() => { setEdit((e) => applyPreset(got.regions, e, p)); announce(tx(p.label)) }}>{tx(p.label)}</Button>)}
          </div>
        </div>

        <aside className="le-panel" aria-label={t('Selected region')}>
          <div className="le-eyebrow">{t('Selected')}</div>
          <h3>{tx(selected.label)}</h3>
          <div className="le-rows">
            <Row label={t('Show {region}', { region: tx(selected.label) })} text={t('Show')}><Switch label={t('Show {region}', { region: tx(selected.label) })} checked={sp.visible !== false} disabled={selected.locked} onChange={(v) => setP(selected, { visible: v }, v ? t('shown') : t('hidden'))} /></Row>
            {selected.positions && selected.positions.length > 0 && <Row label={t('Position')}><Select inputSize="sm" aria-label={t('{region} position', { region: tx(selected.label) })} value={place(selected)} onChange={(e) => setP(selected, { position: e.target.value }, t('at {place}', { place: word(e.target.value).toLowerCase() }))} options={selected.positions.map((x) => ({ value: x, label: word(x) }))} /></Row>}
            {selected.styles && selected.styles.length > 0 && <Row label={t('Style')}><Select inputSize="sm" aria-label={t('{region} style', { region: tx(selected.label) })} value={sp.style || selected.styles[0]} onChange={(e) => setP(selected, { style: e.target.value }, word(e.target.value))} options={selected.styles.map((x) => ({ value: x, label: word(x) }))} /></Row>}
          </div>
          <div className="le-eyebrow">{t('Shell')}</div>
          <div className="le-rows">
            {(['nav', 'search', 'dock'] as const).filter((id) => got.regions.some((r) => r.id === id) && id !== selected.id).map((id) => {
              const r = region(id)
              return <Row key={id} label={id === 'nav' ? t('Navigation') : id === 'dock' ? t('Controls row (Dock)') : t('Search')}>
                <Select inputSize="sm" aria-label={id === 'nav' ? t('Navigation position') : id === 'dock' ? t('Controls row position') : t('Search position')} value={place(r)} onChange={(e) => setP(r, { position: e.target.value }, t('at {place}', { place: word(e.target.value).toLowerCase() }))} options={(r.positions || []).map((x) => ({ value: x, label: word(x) }))} />
              </Row>
            })}
            {admin && <Row label={t('Server name')}><Button size="sm" variant="ghost" onClick={() => setNaming(true)}>{t('Edit name…')}</Button></Row>}
          </div>
          <p className="le-foot muted">{t('Settings and You can’t be hidden. Reset puts everything back to the {look} look. The shelves and widgets on Home are arranged with Customise this page, at the foot of Home.', { look })}</p>
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
  // On a television the layout is edited on a computer (docs/TV-MODE.md): the button says where.
  const tv = useTvMode()
  if (!open) return null
  return tv ? <OnAComputerDialog part="shell.editor" onClose={() => setOpen(false)} /> : <ShellEditor onClose={() => setOpen(false)} />
}
