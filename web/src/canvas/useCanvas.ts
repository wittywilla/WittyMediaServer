// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { api } from '../api'
import { useWatched } from '../extras/jobs'
import { deviceStreaming } from '../player/streaming'
import { useUserPref } from '../ui/userPrefs'
import { ALBUM_SAMPLE, albumCanvas, canvasBlocked, wantsListing, type CanvasInfo, type Device } from './rules'

// A title's canvas, and whether this device should show it. The rules are in rules.ts;
// this reads the device, the person's own choice and the server's listing.

type Conn = { saveData?: boolean; addEventListener?: (t: string, f: () => void) => void; removeEventListener?: (t: string, f: () => void) => void }
const connection = () => (navigator as Navigator & { connection?: Conn }).connection

const motionQuery = () => window.matchMedia('(prefers-reduced-motion: reduce)')
const subscribeDevice = (tell: () => void) => {
  const m = motionQuery(), c = connection()
  m.addEventListener('change', tell)
  c?.addEventListener?.('change', tell)
  return () => { m.removeEventListener('change', tell); c?.removeEventListener?.('change', tell) }
}
// The same signs the rest of the app goes by: the system's "reduce motion", the browser's
// data saver (ui/tilt.ts), and this device's own choice of robust streaming (player/streaming.ts).
const deviceKey = () => `${motionQuery().matches ? 1 : 0}${connection()?.saveData ? 1 : 0}${deviceStreaming() === 'robust' ? 1 : 0}`

export const CANVAS_PREF = 'canvases'

export function useDevice(): Device {
  const key = useSyncExternalStore(subscribeDevice, deviceKey)
  const [on] = useUserPref(CANVAS_PREF, true)
  return useMemo(() => ({ reducedMotion: key[0] === '1', saveData: key[1] === '1', robust: key[2] === '1', off: !on }), [key, on])
}

const tabHiddenNow = () => document.visibilityState === 'hidden'
const subscribeTab = (tell: () => void) => { document.addEventListener('visibilitychange', tell); return () => document.removeEventListener('visibilitychange', tell) }
export const useTabHidden = () => useSyncExternalStore(subscribeTab, tabHiddenNow)

// A canvas was made, uploaded or removed here: the page behind the dialog shows it at once.
const changed = new Map<number, Set<() => void>>()
export function noteCanvasChanged(itemId: number) { changed.get(itemId)?.forEach((f) => f()) }

// The canvas of one song, film or episode. Nothing is asked of the server when this device
// wouldn't show one, unless an administrator is looking, who can still make or remove one.
export function useCanvas(itemId: number | undefined, admin = false) {
  const device = useDevice()
  const ask = itemId != null && wantsListing(device, admin)
  const w = useWatched<CanvasInfo>((signal) => api.get<CanvasInfo>(`/media/${itemId}/canvas`, signal), (c) => !!c.task, ask ? String(itemId) : '')
  const reload = w.reload
  useEffect(() => {
    if (itemId == null) return
    const set = changed.get(itemId) || new Set<() => void>()
    changed.set(itemId, set)
    set.add(reload)
    return () => { set.delete(reload); if (!set.size) changed.delete(itemId) }
  }, [itemId, reload])
  return { info: w.value, failed: w.failed, blocked: canvasBlocked(device, w.value), reload, device }
}

// An album's canvas: the one most of its first few songs share (rules.ts).
export function useAlbumCanvas(trackIds: number[]) {
  const device = useDevice()
  const ids = trackIds.slice(0, ALBUM_SAMPLE)
  const key = ids.join(',')
  const [info, setInfo] = useState<CanvasInfo | null>(null)
  const ask = ids.length > 0 && wantsListing(device, false)
  useEffect(() => {
    setInfo(null)
    if (!ask) return
    const stop = new AbortController()
    Promise.all(ids.map((id) => api.get<CanvasInfo>(`/media/${id}/canvas`, stop.signal).catch(() => null)))
      .then((all) => { if (!stop.signal.aborted) setInfo(albumCanvas(all)) })
    return () => stop.abort()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, ask])
  return { info, blocked: canvasBlocked(device, info) }
}
