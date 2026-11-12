// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { serverBase } from '../../api'
import { useApp } from '../../state'
import { prefsKey, readGuidePrefs, type GuidePrefs } from './guideLogic'

// Which set of channels the guide shows and how many rows at a time: this device's own choice,
// per server and account, like the other apps keep it (docs/CHANNELS.md, "The guide"). A set
// that has gone is all channels again (chooseSet in guideLogic.ts).

const EVENT = 'wms-tv-guide-prefs'

const keyFor = (user: number | undefined) => prefsKey(serverBase(), user)

function read(key: string): GuidePrefs {
  try { return readGuidePrefs(localStorage.getItem(key)) } catch { return readGuidePrefs(null) }
}

export function useGuidePrefs(): [GuidePrefs, (patch: Partial<GuidePrefs>) => void] {
  const { user } = useApp()
  const key = keyFor(user?.id)
  const [prefs, setPrefs] = useState(() => read(key))
  useEffect(() => {
    const on = () => setPrefs(read(key))
    on()
    window.addEventListener(EVENT, on)
    window.addEventListener('storage', on)
    return () => { window.removeEventListener(EVENT, on); window.removeEventListener('storage', on) }
  }, [key])
  const set = (patch: Partial<GuidePrefs>) => {
    const next = { ...read(key), ...patch }
    setPrefs(next)
    try { localStorage.setItem(key, JSON.stringify(next)) } catch { /* kept for this visit */ }
    window.dispatchEvent(new Event(EVENT))
  }
  return [prefs, set]
}
