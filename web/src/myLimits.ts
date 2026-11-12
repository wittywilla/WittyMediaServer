// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import type { MyLimits } from './api'

// What the signed-in profile may do (GET /me/limits), kept in one place for the parts of the
// app that leave out what is turned off: the menus, Settings, the TV guide. The screen-time
// watcher (shell/ScreenTime.tsx) asks the server, at sign-in and again now and then, and
// puts the answer here. The server refuses what is off whatever the page shows; this only
// keeps the app from offering it.

let limits: MyLimits | null = null
const listeners = new Set<() => void>()

export function publishLimits(next: MyLimits | null) {
  limits = next
  for (const l of listeners) l()
}

const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

// useMyLimits: the profile's limits, null until the server has answered (and for an administrator).
export function useMyLimits(): MyLimits | null {
  return useSyncExternalStore(subscribe, () => limits, () => null)
}

// featureOff: this part of the app is turned off for the profile. Not known yet counts as on:
// nothing disappears and comes back while a page loads.
export const featureOff = (l: Pick<MyLimits, 'features'> | null | undefined, feature: string) => !!l && l.features?.[feature] === false
