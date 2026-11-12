// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { getToken, serverBase } from '../api'
import { useApp } from '../state'
import { getTvMode, useTvMode } from '../tvmode/mode'
import { onPreview, prefetchPage, serverFeatures } from './client'
import { describe, deviceKey, type Device } from './device'
import { resolve } from './routes'

const SduiPage = lazy(() => SduiPageCode())

function facts() {
  const mq = (q: string) => { try { return window.matchMedia(q).matches } catch { return false } }
  const shell = typeof (window as unknown as { wittyClient?: { moonlight?: unknown } }).wittyClient?.moonlight === 'object'
  return {
    width: window.innerWidth, height: window.innerHeight, tvMode: getTvMode(), desktopShell: shell,
    touch: mq('(pointer: coarse)') || navigator.maxTouchPoints > 0, pointer: mq('(any-pointer: fine)'),
    reducedMotion: mq('(prefers-reduced-motion: reduce)'), hdr: mq('(dynamic-range: high)'),
    locale: navigator.language || '', uhd: window.screen.width * (window.devicePixelRatio || 1) >= 3800,
  }
}

// useDevice: what this window is, kept the same object until something a page depends
// on changes (TV mode, the breakpoint, reduced motion), so a page isn't asked for again
// on every pixel of a resize.
export function useDevice(): Device {
  const tv = useTvMode()
  const [device, setDevice] = useState<Device>(() => describe(facts()))
  useEffect(() => {
    let timer = 0
    const check = () => {
      const next = describe(facts())
      setDevice((was) => (deviceKey(was, Number(was.viewport?.split('x')[0]) || 0) === deviceKey(next, window.innerWidth) ? was : next))
    }
    check()
    const later = () => { window.clearTimeout(timer); timer = window.setTimeout(check, 200) }
    window.addEventListener('resize', later)
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    mq.addEventListener('change', check)
    return () => { window.clearTimeout(timer); window.removeEventListener('resize', later); mq.removeEventListener('change', check) }
  }, [tv])
  return device
}

// What the server could do the last time it was asked, kept on this device so the first
// page drawn is already the right kind.
const KEY = 'wms.features.'
function keptFeatures(): Set<string> | null {
  try { const v = localStorage.getItem(KEY + serverBase()); return v ? new Set(JSON.parse(v) as string[]) : null } catch { return null }
}

// useFeatures: the server's feature names; null until the first answer on this device.
export function useFeatures(): Set<string> | null {
  const { user } = useApp()
  const [f, setF] = useState<Set<string> | null>(keptFeatures)
  useEffect(() => {
    if (!user) return
    let live = true
    serverFeatures().then((got) => {
      if (!live) return
      if (got.size) { try { localStorage.setItem(KEY + serverBase(), JSON.stringify([...got])) } catch { /* only a copy */ } }
      setF((was) => (was && was.size === got.size && [...got].every((x) => was.has(x)) ? was : got))
    })
    return () => { live = false }
  }, [user?.id, getToken()]) // eslint-disable-line react-hooks/exhaustive-deps
  return f
}

export const hasSduiPages = (f: Set<string> | null) => !!f && f.has('sdui-pages')

// usePreviewChanges: draw again when an administrator starts or stops looking at the draft.
export function usePreviewChanges() {
  const [, setN] = useState(0)
  useEffect(() => onPreview(() => setN((n) => n + 1)), [])
}

// SduiRoute: the page at this address. Server-driven where the server has such pages and
// the address is one of theirs; otherwise the page the app has always had (`fallback`).
let pageCode = false
const SduiPageCode = () => import('./SduiPage').then((m) => { pageCode = true; return m })

export function SduiRoute({ fallback, loading }: { fallback: ReactNode; loading?: ReactNode }) {
  const features = useFeatures()
  const device = useDevice()
  const loc = useLocation()
  usePreviewChanges()
  if (features === null) return <>{loading ?? <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>}</>
  const at = resolve(loc.pathname, loc.search)
  if (!features.has('sdui-pages') || !at || at.kind !== 'page' || !at.page) return <>{fallback}</>
  if (!pageCode) prefetchPage(at.page, device, at.params)
  return <Suspense fallback={loading ?? <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>}><SduiPage page={at.page} params={at.params} fallback={fallback} /></Suspense>
}
