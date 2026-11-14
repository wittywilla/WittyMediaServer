// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Suspense, useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import { Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { profileName } from '../api'
import { Brand, BrandMark, Spinner } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconBack } from '../icons'
import { sduiIcon } from '../sdui/render/icons'
import { useMenu } from '../shell/useNav'
import { Avatar } from '../pages/AuthPage'
import { useApp } from '../state'
import { SduiRoute } from '../sdui/useSdui'
import { focusFirst, OWN_BACK, typing } from '../tv'
import Libraries from './Libraries'
import More from './More'
import { backStep, stepTab, tabPath, tabRootOf, tabsFromMenu, tvKey, type TabId, type TvTab } from './nav'
import Search from './Search'
import You from './You'

// TV mode's frame (docs/plans/TV-MODE.md): the bar along the top on each tab's first screen, and the
// whole screen for anything opened from a tab. Every page of the website shows in it; the tabs only a TV
// has (Libraries, Games, Search with its keyboard, You) are this folder's own.


// While one of these is open it has the keys: dialogs and menus close on Back, the player and books have
// their own, and a game being played has the controller.
const BUSY = `${OWN_BACK}, .wp-playing, .wp-capture`

const pill = (id: TabId) => document.querySelector<HTMLElement>(`.tvm-bar [data-tab="${CSS.escape(id)}"]`)

// The menu: a row along the top, or (a layout that says tvSideMenu) a column down the left that
// shows only icons until it has focus, and then opens to show their names (docs/SDUI.md).
function TopBar({ tab, tabs, side }: { tab: TabId; tabs: TvTab[]; side?: { size: number; open: number } }) {
  const { user, brandName } = useApp()
  const shown = user ? profileName(user) : ''
  const style = side ? { '--tvm-side': String(side.size || 120), '--tvm-side-open': String(side.open || 400) } as React.CSSProperties : undefined
  return (
    <header className={'tvm-bar' + (side ? ' tvm-side' : '')} data-nav-zone="bar" data-keep-focus style={style}>
      <Link to="/" replace className="brand tvm-brand" data-nav-skip aria-label={brandName || 'Home'}><BrandMark />{!side && <Brand name={brandName} />}</Link>
      <nav className="tvm-tabs" aria-label="TV mode" aria-orientation={side ? 'vertical' : undefined} data-nav-group data-nav-entry=".tvm-pill[aria-current=page]">
        {tabs.map((t) => {
          const you = t.screen === 'profiles'
          return (
            <Link key={t.id} to={t.path} replace data-tab={t.id} className={'tvm-pill' + (you ? ' tvm-you' : '')}
              aria-current={t.id === tab ? 'page' : undefined} aria-label={you ? `You, ${shown}` : undefined}>
              {you ? <Avatar name={shown || '?'} avatar={user?.avatar} /> : sduiIcon(t.icon)}
              <span>{you ? shown : t.label}</span>
            </Link>
          )
        })}
      </nav>
    </header>
  )
}

export default function TvFrame({ pages }: { pages: ReactNode }) {
  const loc = useLocation()
  const nav = useNavigate()
  // The bar: the server's menu for a television, where it sends one.
  const menu = useMenu()
  const tabs = useMemo(() => tabsFromMenu(menu.all), [menu.all])
  // The layout the server gives a television: its top menu, or a menu down the side.
  const sideMenu = !!menu.server && menu.shell?.nav === 'tvSideMenu'
  const tabsNow = useRef(tabs)
  tabsNow.current = tabs
  const tab = tabRootOf(loc.pathname, tabs)
  // The tab a screen was opened from, for a screen that was the first one shown.
  const lastTab = useRef<TabId>(tabs[0]?.id || 'home')
  if (tab) lastTab.current = tab
  // Where focus was in each tab, for coming down from the bar.
  const leftIn = useRef(new Map<string, HTMLElement>())
  // The tab whose pill is to have focus once it's drawn.
  const toPill = useRef<TabId | null>(null)

  const choose = useCallback((id: TabId) => {
    toPill.current = id
    if (tabPath(id, tabsNow.current) !== location.pathname) nav(tabPath(id, tabsNow.current), { replace: true })
    else { pill(id)?.focus({ preventScroll: true }); window.scrollTo({ top: 0 }) }
  }, [nav])

  useEffect(() => {
    const id = toPill.current
    if (!id || id !== tab) return
    toPill.current = null
    window.scrollTo({ top: 0 })
    pill(id)?.focus({ preventScroll: true })
  }, [tab])

  useEffect(() => {
    const onFocus = (e: FocusEvent) => {
      const el = e.target as HTMLElement
      if (tabRootOf(location.pathname, tabsNow.current) && el.closest?.('#main') && el.id !== 'main') leftIn.current.set(location.pathname, el)
    }
    window.addEventListener('focusin', onFocus, { passive: true })
    return () => window.removeEventListener('focusin', onFocus)
  }, [])

  // Back from a screen opened from a tab: to where it was opened, or, for the first screen shown, its tab.
  const goBack = useCallback(() => {
    const idx = (window.history.state as { idx?: number } | null)?.idx
    if (idx) nav(-1)
    else nav(tabPath(lastTab.current, tabsNow.current), { replace: true })
  }, [nav])

  useEffect(() => {
    const stop = (e: KeyboardEvent) => { e.preventDefault(); e.stopImmediatePropagation() }
    // Down from the bar: where focus was in the tab, or its main action.
    const intoTab = () => {
      const was = leftIn.current.get(location.pathname)
      if (was?.isConnected && was.getClientRects().length) {
        was.focus({ preventScroll: true })
        was.scrollIntoView({ block: 'nearest' })
        return true
      }
      return focusFirst()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || document.querySelector(BUSY)) return
      const target = e.target instanceof HTMLElement ? e.target : null
      const inBar = !!target?.closest('.tvm-bar')
      const here = tabRootOf(location.pathname, tabsNow.current)
      if (e.key === 'ArrowDown' && inBar) { if (intoTab()) stop(e); return }
      const key = tvKey(e, typing(target))
      if (!key) return
      // Page keys switch tabs while the bar has focus; in the page they page its list (LT and RT, ui/shortcuts.ts).
      if (key === 'prevTab' || key === 'nextTab') {
        if (!here || !inBar) return
        stop(e)
        const to = stepTab(here, key === 'prevTab' ? -1 : 1, tabsNow.current)
        if (to) choose(to)
        return
      }
      const first = tabsNow.current[0]?.id || 'home'
      const step = backStep({ tab: here, inBar, first })
      if (step === 'history') { stop(e); goBack() }
      else if (step === 'bar' && here) { stop(e); choose(here) }
      else if (step === 'home') { stop(e); choose(first) }
      else {
        // Out of the app: to an app that shows this page, if it said where. An untouched Escape is for the
        // desktop app around the page, which opens its own menu then.
        const out = document.documentElement.dataset.appBack
        if (out) { stop(e); window.location.assign(out) }
        else if (e.key !== 'Escape') stop(e)
      }
    }
    // First in line, before the page-wide keys in tv.ts.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [choose, goBack])

  return (
    <div className={'tvm' + (tab ? ' tvm-tab' : ' tvm-pushed') + (sideMenu ? ' tvm-sidemenu' : '')}>
      <a className="skip-link" data-nav-skip href="#main">Skip to content</a>
      {tab
        ? <TopBar tab={tab} tabs={tabs} side={sideMenu ? { size: menu.shell!.size, open: menu.shell!.open || 400 } : undefined} />
        : <button type="button" className="tvm-back" data-nav-skip onClick={goBack} aria-label="Back"><IconBack /></button>}
      <main className="content tvm-main" id="main" data-nav-zone="stage" tabIndex={-1}>
        <Suspense fallback={<Spinner />}>
          <Routes>
            <Route path="/libraries" element={<SduiRoute fallback={<Libraries />} />} />
            <Route path="/search" element={<SduiRoute fallback={<Search />} />} />
            <Route path="/you" element={<You />} />
            <Route path="/more" element={<More />} />
            <Route path="*" element={pages} />
          </Routes>
        </Suspense>
        <LegalNote kind="general" className="legal-foot" />
      </main>
    </div>
  )
}
