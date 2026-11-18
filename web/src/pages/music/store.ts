// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigationType } from 'react-router-dom'
import { api } from '../../api'

// What the music pages have already fetched, kept for the visit. A page opened again (Back, or
// the same artist from another link) draws at once from what was kept and quietly asks again,
// so moving around Music never flashes a spinner over a page that was just there.

const MAX = 80
const kept = new Map<string, unknown>()
const asking = new Map<string, Promise<unknown>>()
// Who is waiting for an answer at an address, so one request serves every page that wants it.
const waiting = new Map<string, Set<(v: unknown) => void>>()

function remember(url: string, v: unknown) {
  kept.delete(url)
  kept.set(url, v)
  if (kept.size > MAX) kept.delete(kept.keys().next().value as string)
}

function ask<T>(url: string): Promise<T> {
  let p = asking.get(url) as Promise<T> | undefined
  if (!p) {
    p = api.get<T>(url).then((v) => { remember(url, v); waiting.get(url)?.forEach((f) => f(v)); return v }).finally(() => asking.delete(url))
    asking.set(url, p)
  }
  return p
}

export const keptAnswer = <T>(url: string) => kept.get(url) as T | undefined

// Forget what was kept for addresses starting with this, after a change (a like, a playlist edited).
export function forget(prefix: string) { for (const k of [...kept.keys()]) if (k.startsWith(prefix)) kept.delete(k) }

// useMusic: the answer at `url` (an /api path), or undefined while the first one comes. `failed` is
// set when it couldn't be had and nothing was kept. A null url asks for nothing.
export function useMusic<T>(url: string | null): { data: T | undefined; failed: boolean } {
  const [, redraw] = useState(0)
  const [failed, setFailed] = useState(false)
  const now = useRef(url)
  now.current = url
  useEffect(() => {
    setFailed(false)
    if (!url) return
    const set = waiting.get(url) || new Set()
    const hear = () => { if (now.current === url) redraw((n) => n + 1) }
    set.add(hear)
    waiting.set(url, set)
    ask<T>(url).catch(() => { if (now.current === url && !kept.has(url)) setFailed(true) })
    return () => { set.delete(hear); if (!set.size) waiting.delete(url) }
  }, [url])
  return { data: url ? (kept.get(url) as T | undefined) : undefined, failed }
}

// useMusicPages: every page of a long list (`limit` at a time, `offset` after), joined, so a
// library of more songs than one answer holds still lists them all. The first page shows at
// once; the rest follow.
export function useMusicPages<T>(url: string | null, size: number): { data: T[] | undefined; failed: boolean; more: boolean } {
  const [pages, setPages] = useState(1)
  useEffect(() => setPages(1), [url])
  const at = (n: number) => (url ? `${url}&limit=${size}&offset=${n * size}` : null)
  const first = useMusic<T[]>(at(0))
  useEffect(() => {
    if (!url) return
    let live = true
    // Each page after the first, in turn, while the one before was full.
    const next = (n: number) => {
      const before = kept.get(at(n - 1)!) as T[] | undefined
      if (!before || before.length < size || n > 40) return
      ask<T[]>(at(n)!).then(() => { if (live) { setPages(n + 1); next(n + 1) } }).catch(() => {})
    }
    if (first.data) next(1)
    return () => { live = false }
  }, [url, first.data]) // eslint-disable-line react-hooks/exhaustive-deps
  // The same list object until a page is added, so what draws it isn't redone for nothing.
  const joined = useMemo(() => {
    if (!first.data) return undefined
    const all: T[] = [...first.data]
    let n = 1
    for (; n < pages; n++) { const p = kept.get(at(n)!) as T[] | undefined; if (!p) break; all.push(...p) }
    const last = (n === 1 ? first.data : (kept.get(at(n - 1)!) as T[] | undefined)) || []
    return { all, more: last.length === size }
  }, [first.data, pages, url]) // eslint-disable-line react-hooks/exhaustive-deps
  return { data: joined?.all, failed: !joined && first.failed, more: !!joined?.more }
}

// Fetches an address ahead of time, as a pointer resting on a link to it does.
export function prefetch(url: string) { if (!kept.has(url) && !asking.has(url)) ask(url).catch(() => {}) }

// useKeepScroll: each place in the history keeps where the page was scrolled to. Back and Forward
// return to it once the page is tall enough again; a new page starts at the top.
const places = new Map<string, number>()
export function useKeepScroll() {
  const loc = useLocation()
  const how = useNavigationType()
  // The place being read now: a scroll that lands after a move counts for the new page, never the old.
  const at = useRef(loc.key)
  useEffect(() => {
    let frame = 0
    const save = () => { if (!frame) frame = requestAnimationFrame(() => { frame = 0; places.set(at.current, window.scrollY) }) }
    window.addEventListener('scroll', save, { passive: true })
    return () => { cancelAnimationFrame(frame); window.removeEventListener('scroll', save) }
  }, [])
  useLayoutEffect(() => {
    places.set(at.current, places.get(at.current) ?? 0)
    at.current = loc.key
    const want = how === 'POP' ? places.get(loc.key) ?? 0 : 0
    if (!want) { window.scrollTo(0, 0); return }
    // The page may still be drawing what it kept: try for a moment until it's tall enough.
    let tries = 0, frame = 0
    const go = () => {
      window.scrollTo(0, want)
      if (Math.abs(window.scrollY - want) > 2 && tries++ < 40) frame = requestAnimationFrame(go)
    }
    go()
    const stop = () => cancelAnimationFrame(frame)
    window.addEventListener('wheel', stop, { once: true, passive: true })
    window.addEventListener('touchstart', stop, { once: true, passive: true })
    return () => { stop(); window.removeEventListener('wheel', stop); window.removeEventListener('touchstart', stop) }
  }, [loc.key]) // eslint-disable-line react-hooks/exhaustive-deps
}
