// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react'
import { api, forgetMissingModule, getToken, inMainMenus, serverBase } from '../api'
import { IconChart, IconGamepad, IconHome, IconLibrary, IconPlug, IconTv, libIcon } from '../icons'
import { useModulePages } from '../moduleBundles'
import { setOwnerNavStyle } from '../navPrefs'
import { deletePersonal, fetchNavigation, getPersonal, keptNavigation, NAV_PERSONAL, onChange, onPreview, putPersonal } from '../sdui/client'
import type { Device } from '../sdui/device'
import { fromOldNav } from '../sdui/personal'
import { sduiIcon } from '../sdui/render/icons'
import type { Navigation, Personal } from '../sdui/types'
import { useDevice, useFeatures } from '../sdui/useSdui'
import { useApp } from '../state'
import { getUserPref, setUserPref, useUserPref } from '../ui/userPrefs'
import type { NavItem, NavPrefs } from './navModel'

// The main menu. A server with server-driven pages says what is in it, in what order, for
// this profile on this device (GET /sdui/navigation), and keeps each profile's own order
// and hidden entries (docs/SDUI.md, "Navigation"). The shell draws it as it always has:
// the side menu, the bar along the top, a phone's bar at the bottom, a TV's top menu.
// An older server has no such answer, and the menu is worked out here as before.

// Whether to offer Game PCs. It's asked for once per person, however many menus want
// to know, as the answer rarely changes.
let couchAsked: { user: number; answer: Promise<boolean> } | null = null
function useCanUseCouch() {
  const { user } = useApp()
  const [can, setCan] = useState(false)
  useEffect(() => {
    if (!user) return
    if (couchAsked?.user !== user.id) couchAsked = { user: user.id, answer: api.get<{ allowed: boolean }>('/couch').then((r) => r.allowed).catch(() => { forgetMissingModule(); return false }) }
    let live = true
    couchAsked.answer.then((a) => { if (live) setCan(a) })
    return () => { live = false }
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  return can
}

// ---------------------------------------------------------------- the server's menu, kept once for every part of the shell

type Kept = { key: string; nav: Navigation | null; personal: Personal | null }
let kept: Kept = { key: '', nav: null, personal: null }
const listeners = new Set<() => void>()
const tell = () => listeners.forEach((l) => l())
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }
let loading = ''

function load(device: Device, key: string, again = false) {
  if (loading === key && !again) return
  loading = key
  fetchNavigation(device).then(async (nav) => {
    if (loading !== key) return
    kept = { key, nav, personal: kept.key === key ? kept.personal : null }
    tell()
    // The stored arrangement itself places the entries only this app has (below).
    if (nav.personal?.editable) {
      const p = await getPersonal(NAV_PERSONAL).then((r) => r.personal || {}).catch(() => null)
      if (loading === key && p) { kept = { ...kept, personal: p }; tell() }
    }
  }).catch(() => { /* the menu as it was stays */ })
}

// useServerNav: the server's menu for this device; null when the server has none (an
// older one) or it hasn't arrived yet.
function useServerNav(): { on: boolean; nav: Navigation | null; personal: Personal | null; device: Device } {
  const features = useFeatures()
  const device = useDevice()
  const { user } = useApp()
  const on = !!features?.has('sdui-navigation')
  const key = on && user ? [serverBase(), getToken(), device.formFactor, device.platform].join(' ') : ''
  const state = useSyncExternalStore(subscribe, () => kept)
  useEffect(() => {
    if (!key) return
    if (kept.key !== key) { const had = keptNavigation(device); kept = { key, nav: had, personal: null }; tell() }
    load(device, key, true)
    const again = () => load(device, key, true)
    const offChange = onChange(again), offPreview = onPreview(again)
    return () => { offChange(); offPreview() }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps
  return { on, nav: state.key === key ? state.nav : null, personal: state.key === key ? state.personal : null, device }
}

// The entries only this app has, which a server's menu doesn't know: Game PCs and the
// pages modules add to the main menu.
function useOwnEntries(): NavItem[] {
  const couch = useCanUseCouch()
  const modulePages = useModulePages()
  return useMemo(() => {
    const list: NavItem[] = []
    if (couch) list.push({ id: 'couch', label: 'Game PCs', to: '/couch' })
    for (const p of modulePages) {
      if (p.where === 'main') list.push({ id: `m:${p.module}:${p.id}`, label: p.title, to: `/m/${p.module}/${p.id}`, kind: 'module:' + (p.icon ?? '') })
    }
    return list
  }, [couch, modulePages])
}

function placeOwn(server: NavItem[], own: NavItem[], personal: Personal | null): NavItem[] {
  const hidden = new Set(personal?.hidden || [])
  const routes = new Set(server.map((s) => s.to))
  const extra = own.filter((o) => !hidden.has(o.id) && !routes.has(o.to))
  if (!extra.length) return server
  const order = personal?.order || []
  const out = [...server]
  // Before Settings, or where the profile's own order has it.
  for (const e of extra) {
    const at = order.indexOf(e.id)
    let to = out.findIndex((x) => x.screen === 'settings' || x.screen === 'profiles')
    if (to < 0) to = out.length
    if (at >= 0) {
      const after = order.slice(0, at).reverse().map((id) => out.findIndex((x) => x.id === id)).find((i) => i >= 0)
      const before = order.slice(at + 1).map((id) => out.findIndex((x) => x.id === id)).find((i) => i >= 0)
      if (after !== undefined) to = after + 1; else if (before !== undefined) to = before
    }
    out.splice(to, 0, e)
  }
  return out
}

export type MenuState = {
  // The server's menu is in force (false: an older server, the menu worked out here).
  server: boolean
  // Everything in the menu, in order, hidden entries gone.
  all: NavItem[]
  // What the side menu and the top bar list: without Search, You and Settings, which the
  // shell has its own places for.
  items: NavItem[]
  // The arrangement list: every entry the profile could have, hidden ones marked.
  rows: { id: string; title: string; hidden: boolean; fixed: boolean; item?: NavItem }[]
  editable: boolean
  save: (order: string[], hidden: string[]) => Promise<void>
  reset: () => Promise<void>
}

// useMenu: the main menu for this profile on this device.
export function useMenu(): MenuState {
  const { libraries } = useApp()
  const srv = useServerNav()
  const own = useOwnEntries()
  const legacy = useLegacyItems(own)
  const [oldPrefs, setOld] = useUserPref<NavPrefs | null>('nav', null)
  const owner = useOwnerPrefs()
  return useMemo<MenuState>(() => {
    if (!srv.on) {
      const prefs = oldPrefs || owner
      const order = prefs.order || [], hidden = new Set(prefs.hidden || [])
      const at = new Map(order.map((id, i) => [id, i]))
      const sorted = legacy.map((item, i) => ({ item, rank: at.get(item.id) ?? order.length + i })).sort((a, b) => a.rank - b.rank).map((x) => x.item)
      const shown = sorted.filter((it) => !hidden.has(it.id))
      return {
        server: false, all: shown, items: shown, editable: true,
        rows: sorted.map((it) => ({ id: it.id, title: it.label, hidden: hidden.has(it.id), fixed: false, item: it })),
        save: (o, h) => setOld({ ...prefs, order: o, hidden: h }),
        reset: () => setOld(null),
      }
    }
    const kindOf = (id: string) => { const m = /^lib-(\d+)$/.exec(id); return m ? libraries.find((l) => String(l.id) === m[1])?.type : undefined }
    const fromServer: NavItem[] = (srv.nav?.items || []).map((e) => ({ id: e.id, label: e.label, to: e.route, end: e.route === '/', kind: kindOf(e.id), icon: e.icon, placement: e.placement, screen: e.screen }))
    const all = placeOwn(fromServer, own, srv.personal)
    const sections = srv.nav?.personal?.sections || []
    const ownHidden = new Set(srv.personal?.hidden || [])
    const known = new Set(sections.map((s) => s.id))
    const rows = [
      ...sections.map((s) => ({ id: s.id, title: s.title, hidden: !!s.hidden, fixed: s.id === 'settings' || fromServer.find((x) => x.id === s.id)?.screen === 'settings', item: all.find((x) => x.id === s.id) })),
      ...own.filter((o) => !known.has(o.id)).map((o) => ({ id: o.id, title: o.label, hidden: ownHidden.has(o.id), fixed: false, item: o })),
    ]
    // The profile's order places this app's own entries among the server's.
    const order = srv.personal?.order || []
    if (order.length) {
      const pos = new Map(order.map((id, i) => [id, i]))
      const base = new Map(rows.map((r, i) => [r.id, i]))
      rows.sort((a, b) => (pos.get(a.id) ?? order.length + base.get(a.id)!) - (pos.get(b.id) ?? order.length + base.get(b.id)!))
    }
    return {
      server: true, all, rows,
      items: all.filter((it) => it.screen !== 'settings' && it.screen !== 'profiles' && it.id !== 'search' && it.to !== '/search'),
      editable: !!srv.nav?.personal?.editable,
      save: async (o, h) => { kept = { ...kept, personal: { order: o, hidden: h } }; await putPersonal(NAV_PERSONAL, { order: o, hidden: h.filter((id) => id !== 'settings') }) },
      reset: async () => { kept = { ...kept, personal: {} }; await deletePersonal(NAV_PERSONAL) },
    }
  }, [srv.on, srv.nav, srv.personal, own, legacy, oldPrefs, owner, libraries]) // eslint-disable-line react-hooks/exhaustive-deps
}

// The menu as the web app worked it out before servers described it.
function useLegacyItems(own: NavItem[]): NavItem[] {
  const { libraries } = useApp()
  return useMemo(() => {
    const list: NavItem[] = [
      { id: 'home', label: 'Home', to: '/', end: true },
      { id: 'tv', label: 'TV guide', to: '/tv' },
      ...libraries.filter(inMainMenus).map((l) => ({ id: 'lib:' + l.id, label: l.name, to: `/library/${l.id}`, kind: l.type })),
    ]
    if (libraries.length > 0) list.push({ id: 'collections', label: 'Collections', to: '/collections' })
    return [...list, ...own]
  }, [libraries, own])
}

// useNavItems: what the side menu and the top bar list, in order.
export function useNavItems(): NavItem[] {
  return useMenu().items
}

export function navIcon(item: NavItem): ReactNode {
  if (item.kind?.startsWith('module:')) return item.kind === 'module:chart' ? <IconChart /> : <IconPlug />
  // A library keeps the icon of its kind, as everywhere else in the app.
  if (item.kind && /^lib[:-]/.test(item.id)) return libIcon(item.kind)
  if (item.icon) { const drawn = sduiIcon(item.icon); if (drawn) return drawn }
  switch (item.id) {
    case 'home': return <IconHome />
    case 'tv': return <IconTv />
    case 'collections': return <IconLibrary />
    case 'couch': return <IconGamepad />
    default: return libIcon(item.kind || '')
  }
}

// ---------------------------------------------------------------- how the menu looks on this profile's devices

// The owner's choices, for anyone who hasn't made their own. Asked for once.
let ownerPrefs: NavPrefs = {}
let ownerAsked: Promise<NavPrefs> | null = null
const ownerListeners = new Set<(p: NavPrefs) => void>()

function askOwner() {
  ownerAsked ||= api.get<{ navDefault?: string }>('/prefs').then((r) => {
    try { ownerPrefs = r.navDefault ? JSON.parse(r.navDefault) as NavPrefs : {} } catch { ownerPrefs = {} }
    setOwnerNavStyle(ownerPrefs.style)
    return ownerPrefs
  }).catch(() => ownerPrefs)
  return ownerAsked
}

export function setOwnerNavPrefs(p: NavPrefs) {
  ownerPrefs = p
  setOwnerNavStyle(p.style)
  ownerListeners.forEach((l) => l(p))
}

function useOwnerPrefs(): NavPrefs {
  const [owner, setOwner] = useState<NavPrefs>(ownerPrefs)
  useEffect(() => {
    let live = true
    askOwner().then((p) => { if (live) setOwner(p) })
    ownerListeners.add(setOwner)
    return () => { live = false; ownerListeners.delete(setOwner) }
  }, [])
  return owner
}

const LOOK: (keyof NavPrefs)[] = ['grouped', 'groups', 'overflow', 'collapse', 'style']
const lookOf = (p: NavPrefs | null | undefined): NavPrefs => {
  const out: NavPrefs = {}
  if (p) for (const k of LOOK) if (p[k] !== undefined) (out as Record<string, unknown>)[k] = p[k]
  return out
}

// useNavPrefs: how the menu is drawn (grouped or not, what a full bar does, what a
// folded side menu leaves): `prefs` is the choice in force, someone's own or else the
// owner's. Which entries, in what order, is not here: with a server's menu that is the
// profile's arrangement on the server (useMenu), the one place for it. Against an older
// server the order and hidden entries are still part of this choice, as they were.
export function useNavPrefs() {
  const features = useFeatures()
  const server = !!features?.has('sdui-navigation')
  const [look, setLook] = useUserPref<NavPrefs | null>('navLook', null)
  const [old, setOld] = useUserPref<NavPrefs | null>('nav', null)
  const owner = useOwnerPrefs()
  if (!server) return { prefs: old || owner, mine: old, owner, setMine: setOld, server }
  // The server arranges; only the look is kept here. The order is never applied twice.
  const mine = look || (old ? lookOf(old) : null)
  return { prefs: mine || lookOf(owner), mine, owner: lookOf(owner), setMine: (p: NavPrefs | null) => setLook(p ? lookOf(p) : null), server }
}

// The arrangement someone made before menus were server-driven carries over once: its
// order and hidden entries become the profile's arrangement on the server, how it looks
// stays a choice of its own, and the old record goes.
let migrating = false
export function useNavMigration() {
  const features = useFeatures()
  const { user } = useApp()
  const server = !!features?.has('sdui-navigation') && !!features?.has('sdui-personal')
  const [old] = useUserPref<NavPrefs | null>('nav', null)
  useEffect(() => {
    if (!server || !user || !old || migrating) return
    migrating = true
    const id = (s: string) => (s.startsWith('lib:') ? 'lib-' + s.slice(4) : s === 'tv' ? 'livetv' : s)
    const look = lookOf(old)
    if (look.groups) look.groups = Object.fromEntries(Object.entries(look.groups).map(([k, v]) => [id(k), v]))
    const moved = fromOldNav(old)
    ;(async () => {
      try {
        if (moved) {
          const now = await getPersonal(NAV_PERSONAL)
          const empty = !now.personal || (!(now.personal.order || []).length && !(now.personal.hidden || []).length)
          if (now.editable && empty) await putPersonal(NAV_PERSONAL, moved)
        }
        if (Object.keys(look).length && !getUserPref<NavPrefs | null>('navLook', null)) await setUserPref('navLook', look)
        await setUserPref('nav', null)
      } catch { /* next time */ } finally { migrating = false }
    })()
  }, [server, user?.id, !!old]) // eslint-disable-line react-hooks/exhaustive-deps
}

export function useMedia(query: string) {
  const [on, setOn] = useState(() => window.matchMedia(query).matches)
  useEffect(() => {
    const mq = window.matchMedia(query)
    const change = () => setOn(mq.matches)
    change()
    mq.addEventListener('change', change)
    return () => mq.removeEventListener('change', change)
  }, [query])
  return on
}
