// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, forgetMissingModule, inMainMenus } from '../api'
import { IconChart, IconGamepad, IconHome, IconLibrary, IconPlug, IconTv, libIcon } from '../icons'
import { useModulePages } from '../moduleBundles'
import { setOwnerNavStyle } from '../navPrefs'
import { useApp } from '../state'
import { featureOff, useMyLimits } from '../myLimits'
import { useUserPref } from '../ui/userPrefs'
import type { NavItem, NavPrefs } from './navModel'

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

// useNavItems: everything this person could have in the main menu.
export function useNavItems(): NavItem[] {
  const { libraries } = useApp()
  const couch = useCanUseCouch()
  const modulePages = useModulePages()
  // Live TV turned off for this profile: the guide isn't offered.
  const noTv = featureOff(useMyLimits(), 'liveTv')
  return useMemo(() => {
    const list: NavItem[] = [
      { id: 'home', label: 'Home', to: '/', end: true },
      ...(noTv ? [] : [{ id: 'tv', label: 'TV guide', to: '/tv' }]),
      ...libraries.filter(inMainMenus).map((l) => ({ id: 'lib:' + l.id, label: l.name, to: `/library/${l.id}`, kind: l.type })),
    ]
    if (libraries.length > 0) list.push({ id: 'collections', label: 'Collections', to: '/collections' })
    if (couch) list.push({ id: 'couch', label: 'Game PCs', to: '/couch' })
    for (const p of modulePages) {
      if (p.where === 'main') list.push({ id: `m:${p.module}:${p.id}`, label: p.title, to: `/m/${p.module}/${p.id}`, kind: 'module:' + (p.icon ?? '') })
    }
    return list
  }, [libraries, couch, modulePages, noTv])
}

export function navIcon(item: NavItem): ReactNode {
  switch (item.id) {
    case 'home': return <IconHome />
    case 'tv': return <IconTv />
    case 'collections': return <IconLibrary />
    case 'couch': return <IconGamepad />
    default:
      if (item.kind?.startsWith('module:')) return item.kind === 'module:chart' ? <IconChart /> : <IconPlug />
      return libIcon(item.kind || '')
  }
}

// The owner's arrangement, for anyone who hasn't made their own. Asked for once.
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

// useNavPrefs: the arrangement in force. Someone's own replaces the owner's whole,
// so "Use the server default" is one clear step back.
export function useNavPrefs() {
  const [mine, setMine] = useUserPref<NavPrefs | null>('nav', null)
  const [owner, setOwner] = useState<NavPrefs>(ownerPrefs)
  useEffect(() => {
    let live = true
    askOwner().then((p) => { if (live) setOwner(p) })
    ownerListeners.add(setOwner)
    return () => { live = false; ownerListeners.delete(setOwner) }
  }, [])
  return { prefs: mine || owner, mine, owner, setMine }
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
