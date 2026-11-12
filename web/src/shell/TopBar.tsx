// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import ActivitySlot from '../components/ActivitySlot'
import { Brand, BrandMark } from '../components/common'
import { IconDown, IconLeft, IconMenu, IconMore, IconSearch, IconSettings, IconSidebar } from '../icons'
import { useApp } from '../state'
import { Menu, type MenuEntry } from '../ui/Menu'
import { ProfileMenu } from './Account'
import { arrange, entries, entryActive, fitCount, isActive, type NavEntry, type NavItem } from './navModel'
import { navIcon, useNavItems, useNavPrefs } from './useNav'

const GAP = 2

const itemEntry = (it: NavItem, path: string): MenuEntry => ({ key: it.id, label: it.label, text: it.label, icon: navIcon(it), to: it.to, current: isActive(it, path) })

function BarGroup({ group, items, path }: { group: string; items: NavItem[]; path: string }) {
  const here = items.some((it) => isActive(it, path))
  return (
    <Menu label={group} align="start" items={items.map((it) => itemEntry(it, path))} trigger={(props) => (
      <button type="button" {...props} className={'topnav-link' + (here ? ' active' : '')} aria-current={here ? 'true' : undefined}>
        <span>{group}</span><IconDown className="topnav-caret" />
      </button>
    )} />
  )
}

function BarEntry({ entry, path }: { entry: NavEntry; path: string }) {
  if ('group' in entry) return <BarGroup group={entry.group} items={entry.items} path={path} />
  return <NavLink to={entry.item.to} end={entry.item.end} className="topnav-link">{navIcon(entry.item)} <span>{entry.item.label}</span></NavLink>
}

// The links along the top. When they don't all fit, the rest go under More, and the
// open page's name moves onto that button so you can always see where you are. The
// widths are read once and kept: resizing only does sums, and touches the page only
// when the answer changes.
function TopNav() {
  const items = useNavItems()
  const { prefs } = useNavPrefs()
  const { pathname: path } = useLocation()
  const list = useMemo(() => entries(arrange(items, prefs), prefs), [items, prefs])
  const scroll = prefs.overflow === 'scroll'
  const bar = useRef<HTMLElement | null>(null)
  const widths = useRef<number[] | null>(null)
  const moreWidth = useRef(0)
  const [count, setCount] = useState<number | null>(null)
  const sig = list.map((e) => e.key + ('item' in e ? e.item.label : '')).join('|') + (scroll ? '!' : '')
  const activeIndex = list.findIndex((e) => entryActive(e, path))
  const activeRef = useRef(activeIndex)
  activeRef.current = activeIndex

  const work = () => {
    const w = widths.current, el = bar.current
    if (!w || !el) return
    const a = activeRef.current
    setCount(fitCount(w, GAP, el.clientWidth, moreWidth.current, a, a >= 0 ? w[a] + 28 : 0))
  }

  // Something changed what's in the bar: show everything for a moment to measure it.
  useLayoutEffect(() => { widths.current = null; setCount(null) }, [sig])
  useLayoutEffect(() => {
    if (scroll || count !== null || !bar.current) return
    const kids = [...bar.current.children] as HTMLElement[]
    moreWidth.current = kids[kids.length - 1]?.offsetWidth || 0
    widths.current = kids.slice(0, -1).map((k) => k.offsetWidth)
    work()
  })
  useLayoutEffect(work, [activeIndex])
  useEffect(() => {
    if (scroll || !bar.current || !window.ResizeObserver) return
    const ro = new ResizeObserver(work)
    ro.observe(bar.current)
    // The widths were read in a fallback font if the real one hadn't arrived.
    let live = true
    document.fonts?.ready.then(() => { if (live) { widths.current = null; setCount(null) } })
    return () => { live = false; ro.disconnect() }
  }, [scroll])

  const shown = scroll || count === null ? list : list.slice(0, count)
  const rest = scroll || count === null ? [] : list.slice(count)
  const measuring = !scroll && count === null
  const restHere = rest.find((e) => entryActive(e, path))
  const hereItem = restHere && ('item' in restHere ? restHere.item : restHere.items.find((it) => isActive(it, path)))
  const restMenu: MenuEntry[] = rest.flatMap((e): MenuEntry[] => 'item' in e
    ? [itemEntry(e.item, path)]
    : [{ kind: 'heading', key: 'h:' + e.group, label: e.group }, ...e.items.map((it) => itemEntry(it, path))])
  restMenu.push({ kind: 'separator', key: 'sep' }, { key: 'arrange', label: 'Arrange this menu', icon: <IconSettings />, to: '/settings/appearance#navigation' })

  return (
    <nav className={'topnav' + (scroll ? ' scrolls' : '')} aria-label="Libraries" ref={bar}>
      {shown.map((e) => <BarEntry key={e.key} entry={e} path={path} />)}
      {measuring && <span className="topnav-link topnav-measure" aria-hidden="true"><IconMore /><span>More</span><IconDown className="topnav-caret" /></span>}
      {rest.length > 0 && (
        <Menu label="More places" align="end" items={restMenu} trigger={(props) => (
          <button type="button" {...props} className={'topnav-link topnav-more' + (hereItem ? ' active' : '')} aria-label={hereItem ? `More: ${hereItem.label} is open` : 'More places'}>
            {hereItem ? <>{navIcon(hereItem)}<span>{hereItem.label}</span></> : <><IconMore /><span>More</span></>}
            <IconDown className="topnav-caret" />
          </button>
        )} />
      )}
    </nav>
  )
}

// The row of sections on a phone, scrolling sideways with your thumb. Home and TV
// are on the bar at the bottom, so they aren't here too.
function SectionRow({ list }: { list: NavItem[] }) {
  const { brandName } = useApp()
  const { pathname: path } = useLocation()
  const row = useRef<HTMLElement | null>(null)
  useEffect(() => {
    const el = row.current, on = el?.querySelector<HTMLElement>('.active')
    if (el && on) el.scrollLeft = on.offsetLeft - (el.clientWidth - on.offsetWidth) / 2
  }, [path, list])
  if (list.length === 0) return <Link to="/" className="brand section-row-brand"><BrandMark /><Brand name={brandName} /></Link>
  return (
    <nav className="section-row" aria-label="Sections" ref={row}>
      {list.map((it) => <NavLink key={it.id} to={it.to} end={it.end}>{it.label}</NavLink>)}
    </nav>
  )
}

// The pages a phone's row of sections and bar at the bottom lead to. Anything else is
// below one of them, and has a Back button in the row's place.
const TAB_PAGES = ['/', '/tv', '/search', '/you', '/settings']
const pathOf = (to: string) => to.split(/[?#]/)[0]

// Where Back goes when there's nothing in this tab's history to go back through, such as
// a page opened from a link: the page above it.
function above(path: string) {
  for (const top of ['/settings', '/tv']) if (path.startsWith(top + '/')) return top
  return '/'
}

// Back, on a phone: a round button where the row of sections would be.
function TopBack() {
  const nav = useNavigate()
  const { pathname } = useLocation()
  const back = () => { if ((window.history.state?.idx ?? 0) > 0) nav(-1); else nav(above(pathname)) }
  return <button type="button" className="top-back" onClick={back} aria-label="Back"><IconLeft size={24} /></button>
}

// The bar gains its surface once the page is scrolled under it. A class set straight on
// the bar, only when it changes: no React state for scrolling.
function useUnder(bar: RefObject<HTMLElement | null>) {
  useEffect(() => {
    let frame = 0
    const check = () => { frame = 0; bar.current?.classList.toggle('under', window.scrollY > 2) }
    const later = () => { if (!frame) frame = requestAnimationFrame(check) }
    check()
    window.addEventListener('scroll', later, { passive: true })
    return () => { window.removeEventListener('scroll', later); cancelAnimationFrame(frame) }
  }, [bar])
}

// allTabs: a layout with tabs along the top and no bar at the bottom (topTabs on a phone): the
// row holds Home and Live TV too, and the search and the profile stay in the bar.
export function TopBar({ onMenu, showMenu, topNav, menuOpen, wide, allTabs }: { onMenu: () => void; showMenu: boolean; topNav: boolean; menuOpen: boolean; wide: boolean; allTabs?: boolean }) {
  const nav = useNavigate()
  const loc = useLocation()
  const [q, setQ] = useState('')
  useEffect(() => { if (!loc.pathname.startsWith('/search')) setQ('') }, [loc.pathname])
  const { brandName } = useApp()
  const items = useNavItems()
  const { prefs } = useNavPrefs()
  const list = useMemo(() => arrange(items, prefs).filter((it) => allTabs || (it.id !== 'home' && it.id !== 'tv' && it.id !== 'livetv')), [items, prefs, allTabs])
  const bar = useRef<HTMLElement | null>(null)
  useUnder(bar)
  const top = TAB_PAGES.includes(loc.pathname) || list.some((it) => pathOf(it.to) === loc.pathname)
  return (
    <header className="topbar" ref={bar}>
      {top ? <SectionRow list={list} /> : <TopBack />}
      {showMenu && (
        <button type="button" className="btn icon menu-toggle" onClick={onMenu} aria-expanded={menuOpen} aria-label={wide ? (menuOpen ? 'Fold the side menu' : 'Open the side menu') : 'Open the menu'}>
          {wide ? <IconSidebar /> : <IconMenu />}
        </button>
      )}
      {topNav && !showMenu && <Link to="/" className="brand top-brand"><BrandMark /><Brand name={brandName} /></Link>}
      {topNav && <TopNav />}
      <form className="search" role="search" onSubmit={(e) => { e.preventDefault(); if (q.trim()) nav('/search?q=' + encodeURIComponent(q.trim())) }}>
        <IconSearch />
        <input aria-label="Search" type="search" placeholder="Search your libraries…" value={q} onChange={(e) => setQ(e.target.value)} />
      </form>
      {!topNav && <div className="spacer" />}
      <ActivitySlot />
      <ProfileMenu />
    </header>
  )
}
