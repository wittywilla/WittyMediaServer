// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useCallback, useEffect, useRef, type ReactNode } from 'react'
import { Link, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { profileName } from '../api'
import { Spinner } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconBack, IconGamepad, IconHome, IconLibrary, IconSearch, IconTv } from '../icons'
import { Avatar } from '../pages/AuthPage'
import { useApp } from '../state'
import { focusFirst, OWN_BACK, typing } from '../tv'
import Libraries from './Libraries'
import { backStep, stepTab, tabPath, tabRootOf, tvKey, TV_TABS, type TabId } from './nav'
import Search from './Search'
import You from './You'

// TV mode's frame (docs/plans/TV-MODE.md): the bar along the top on each tab's first screen, and the
// whole screen for anything opened from a tab. Every page of the website shows in it; the tabs only a TV
// has (Libraries, Games, Search with its keyboard, You) are this folder's own.

const Games = lazy(() => import('./Games'))
const GamesLibraryScreen = lazy(() => import('./Games').then((m) => ({ default: m.GamesLibraryScreen })))
const GameScreen = lazy(() => import('./Games').then((m) => ({ default: m.GameScreen })))

const ICONS: Record<Exclude<TabId, 'you'>, ReactNode> = {
  home: <IconHome />, libraries: <IconLibrary />, games: <IconGamepad />, live: <IconTv />, search: <IconSearch />,
}

// While one of these is open it has the keys: dialogs and menus close on Back, the player and books have
// their own, and a game being played has the controller.
const BUSY = `${OWN_BACK}, .wp-playing, .wp-capture`

const pill = (id: TabId) => document.querySelector<HTMLElement>(`.tvm-bar [data-tab="${id}"]`)

function TopBar({ tab }: { tab: TabId }) {
  const { user } = useApp()
  const shown = user ? profileName(user) : ''
  return (
    <header className="tvm-bar" data-keep-focus>
      <nav className="tvm-tabs" aria-label="TV mode" data-nav-group data-nav-entry=".tvm-pill[aria-current=page]">
        {TV_TABS.map((t) => (
          <Link key={t.id} to={t.path} replace data-tab={t.id} className={'tvm-pill' + (t.id === 'you' ? ' tvm-you' : '')}
            aria-current={t.id === tab ? 'page' : undefined} aria-label={t.id === 'you' ? `You, ${shown}` : undefined}>
            {t.id === 'you' ? <Avatar name={shown || '?'} /> : ICONS[t.id]}
            <span>{t.id === 'you' ? shown : t.label}</span>
          </Link>
        ))}
      </nav>
    </header>
  )
}

export default function TvFrame({ pages }: { pages: ReactNode }) {
  const loc = useLocation()
  const nav = useNavigate()
  const tab = tabRootOf(loc.pathname)
  // The tab a screen was opened from, for a screen that was the first one shown.
  const lastTab = useRef<TabId>('home')
  if (tab) lastTab.current = tab
  // Where focus was in each tab, for coming down from the bar.
  const leftIn = useRef(new Map<string, HTMLElement>())
  // The tab whose pill is to have focus once it's drawn.
  const toPill = useRef<TabId | null>(null)

  const choose = useCallback((id: TabId) => {
    toPill.current = id
    if (tabPath(id) !== location.pathname) nav(tabPath(id), { replace: true })
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
      if (tabRootOf(location.pathname) && el.closest?.('#main') && el.id !== 'main') leftIn.current.set(location.pathname, el)
    }
    window.addEventListener('focusin', onFocus, { passive: true })
    return () => window.removeEventListener('focusin', onFocus)
  }, [])

  // Back from a screen opened from a tab: to where it was opened, or, for the first screen shown, its tab.
  const goBack = useCallback(() => {
    const idx = (window.history.state as { idx?: number } | null)?.idx
    if (idx) nav(-1)
    else nav(tabPath(lastTab.current), { replace: true })
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
      const here = tabRootOf(location.pathname)
      if (e.key === 'ArrowDown' && inBar) { if (intoTab()) stop(e); return }
      const key = tvKey(e, typing(target))
      if (!key) return
      if (key === 'options') {
        // Start: the screen's options, where it has some.
        const options = Array.from(document.querySelectorAll<HTMLElement>('[data-tv-options]')).find((el) => el.getClientRects().length)
        if (options) { stop(e); options.click() }
        return
      }
      if (key === 'prevTab' || key === 'nextTab') {
        if (!here) return
        stop(e)
        const to = stepTab(here, key === 'prevTab' ? -1 : 1)
        if (to) choose(to)
        return
      }
      const step = backStep({ tab: here, inBar })
      if (step === 'history') { stop(e); goBack() }
      else if (step === 'bar' && here) { stop(e); choose(here) }
      else if (step === 'home') { stop(e); choose('home') }
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
    <div className={'tvm' + (tab ? ' tvm-tab' : ' tvm-pushed')}>
      <a className="skip-link" data-nav-skip href="#main">Skip to content</a>
      {tab
        ? <TopBar tab={tab} />
        : <button type="button" className="tvm-back" data-nav-skip onClick={goBack} aria-label="Back"><IconBack /></button>}
      <main className="content tvm-main" id="main" tabIndex={-1}>
        <Suspense fallback={<Spinner />}>
          <Routes>
            <Route path="/libraries" element={<Libraries />} />
            <Route path="/games" element={<Games />} />
            <Route path="/games/library/:id" element={<GamesLibraryScreen />} />
            <Route path="/games/game/:id" element={<GameScreen />} />
            <Route path="/search" element={<Search />} />
            <Route path="/you" element={<You />} />
            <Route path="*" element={pages} />
          </Routes>
        </Suspense>
        <LegalNote kind="general" className="legal-foot" />
      </main>
    </div>
  )
}
