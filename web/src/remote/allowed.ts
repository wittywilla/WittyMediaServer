// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Whether this profile may use watch parties and play on other devices. Both can be turned
// off for a profile (Settings, Profiles, Limits), and the server then refuses them; this is
// only so the app doesn't offer what would be refused. Asked once for each profile.
import { useEffect, useState } from 'react'
import { api, type MyLimits } from '../api'
import { useApp } from '../state'

export type RemoteAllowed = { party: boolean; playOn: boolean }
const YES: RemoteAllowed = { party: true, playOn: true }
const known = new Map<number, RemoteAllowed>()

export function useRemoteAllowed(): RemoteAllowed {
  const { user } = useApp()
  const id = user?.id || 0
  const [got, setGot] = useState<RemoteAllowed>(() => known.get(id) || YES)
  useEffect(() => {
    if (!id) return
    let gone = false
    setGot(known.get(id) || YES)
    api.get<MyLimits>('/me/limits').then((l) => {
      const a = { party: l.features.syncplay !== false, playOn: l.features.remoteControl !== false }
      known.set(id, a)
      if (!gone) setGot(a)
    }).catch(() => { /* an older server, or offline: the server still decides */ })
    return () => { gone = true }
  }, [id])
  return got
}
