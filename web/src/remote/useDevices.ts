// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The controller's view of the devices it may steer (docs/REMOTE-PROTOCOL.md, "Watch them
// live"): the events stream, kept open for exactly as long as something showing them is on
// screen, and the request that sends one of them a command.
import { useEffect, useRef, useState } from 'react'
import { ApiError } from '../api'
import { clockAhead } from './position'
import type { RemoteCommand, RemoteState } from './device'
import { call, readEvents, sleep, steadyNow } from './stream'

export type RemoteDevice = {
  id: string; name: string; kind: string; userId: number; userName: string; online: boolean; mine: boolean; thisSession: boolean
  capabilities: { mediaKinds: string[]; features: string[] }; state: RemoteState; stateAt: number; title?: string; itemKind?: string; hidden?: boolean
}

export const sendCommand = (deviceId: string, cmd: RemoteCommand) => call<{ ok: boolean; sourceStopped?: boolean }>('POST', `/remote-control/devices/${encodeURIComponent(deviceId)}/commands`, { v: 1, ...cmd })

// The server's clock now, in its milliseconds, from how far ahead of this device it was last seen to be.
export type ServerClock = () => number

// useDevices keeps the list live while the component using it is mounted, and no longer.
export function useDevices() {
  const [devices, setDevices] = useState<RemoteDevice[] | null>(null)
  const [error, setError] = useState('')
  const [off, setOff] = useState(false)
  const [again, setAgain] = useState(0)
  const ahead = useRef(0)
  const serverNow = useRef<ServerClock>(() => steadyNow() + ahead.current).current
  useEffect(() => {
    const ctl = new AbortController()
    setError('')
    const merge = (d: RemoteDevice) => setDevices((list) => {
      const rest = (list || []).filter((x) => x.id !== d.id)
      return [...rest, d].sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
    })
    ;(async () => {
      let wait = 1500
      while (!ctl.signal.aborted) {
        try {
          await readEvents('/remote-control/events', (event, data) => {
            if (event === 'snapshot') { ahead.current = clockAhead(data.serverTimeMs, steadyNow()); setDevices(data.devices); setError(''); wait = 1500 }
            else if (event === 'device') merge(data)
            else if (event === 'gone') setDevices((list) => (list || []).filter((x) => x.id !== data.id))
          }, ctl.signal)
        } catch (e) {
          if (e instanceof ApiError && e.data?.code === 'disabled') { setOff(true); return }
          if (!ctl.signal.aborted) setError((e as Error).message)
        }
        await sleep(wait, ctl.signal)
        wait = Math.min(wait * 2, 15000)
      }
    })()
    return () => ctl.abort()
  }, [again])
  return { devices, error, off, retry: () => setAgain((n) => n + 1), serverNow }
}
