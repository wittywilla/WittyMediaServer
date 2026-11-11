// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useSyncExternalStore } from 'react'
import { api } from '../api'

// What a person has chosen for themselves. It's kept on the server in /me/ui, so
// it follows them from device to device, with a copy on this device so the first
// paint is already right. Other parts of the app write to /me/ui as well, so a
// write always merges into what's there now instead of replacing it.

type UI = Record<string, unknown>

let ui: UI = {}
let owner = 0
const listeners = new Set<() => void>()
const tell = () => listeners.forEach((l) => l())
const keep = () => { try { localStorage.setItem('wms.ui.' + owner, JSON.stringify(ui)) } catch { /* it's only a copy */ } }

// loadUserPrefs: when someone signs in, or the profile changes.
export function loadUserPrefs(userId: number) {
  if (userId === owner) return
  owner = userId
  try { ui = JSON.parse(localStorage.getItem('wms.ui.' + owner) || '{}') as UI } catch { ui = {} }
  tell()
  api.get<UI>('/me/ui').then((fresh) => { if (owner === userId) { ui = fresh || {}; keep(); tell() } }).catch(() => { /* this device's copy will do */ })
}

let writes: Promise<unknown> = Promise.resolve()

// setUserPref: null takes the choice away again.
export function setUserPref(key: string, value: unknown): Promise<void> {
  ui = { ...ui }
  if (value === null) delete ui[key]; else ui[key] = value
  keep()
  tell()
  const done = writes.then(async () => {
    const now = await api.get<UI>('/me/ui')
    const next = { ...now }
    if (value === null) delete next[key]; else next[key] = value
    await api.put('/me/ui', next)
  })
  writes = done.catch(() => { /* the next write starts clean */ })
  return done
}

export function getUserPref<T>(key: string, fallback: T): T {
  const v = ui[key]
  return v === undefined ? fallback : v as T
}

const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export function useUserPref<T>(key: string, fallback: T): [T, (v: T | null) => Promise<void>] {
  const v = useSyncExternalStore(subscribe, () => ui[key])
  const set = useCallback((next: T | null) => setUserPref(key, next), [key])
  return [v === undefined ? fallback : v as T, set]
}
