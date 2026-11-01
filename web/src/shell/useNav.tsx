import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, inMainMenus } from '../api'
import { IconGamepad, IconHome, IconLibrary, IconTv, libIcon } from '../icons'
import { setOwnerNavStyle } from '../navPrefs'
import { useApp } from '../state'
import { useUserPref } from '../ui/userPrefs'
import type { NavItem, NavPrefs } from './navModel'

// Whether to offer Game PCs. It's asked for once, as the answer rarely changes.
function useCanUseCouch() {
  const { user } = useApp()
  const [can, setCan] = useState(false)
  useEffect(() => {
    if (user) api.get<{ allowed: boolean }>('/couch').then((r) => setCan(r.allowed)).catch(() => setCan(false))
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  return can
}

// useNavItems: everything this person could have in the main menu.
export function useNavItems(): NavItem[] {
  const { libraries } = useApp()
  const couch = useCanUseCouch()
  return useMemo(() => {
    const list: NavItem[] = [
      { id: 'home', label: 'Home', to: '/', end: true },
      { id: 'tv', label: 'TV guide', to: '/tv' },
      ...libraries.filter(inMainMenus).map((l) => ({ id: 'lib:' + l.id, label: l.name, to: `/library/${l.id}`, kind: l.type })),
    ]
    if (libraries.length > 0) list.push({ id: 'collections', label: 'Collections', to: '/collections' })
    if (couch) list.push({ id: 'couch', label: 'Game PCs', to: '/couch' })
    return list
  }, [libraries, couch])
}

export function navIcon(item: NavItem): ReactNode {
  switch (item.id) {
    case 'home': return <IconHome />
    case 'tv': return <IconTv />
    case 'collections': return <IconLibrary />
    case 'couch': return <IconGamepad />
    default: return libIcon(item.kind || '')
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
