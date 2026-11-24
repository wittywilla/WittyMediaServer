// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Check connection's Recent tries, without the page: a device's tries at connecting from away as the server kept
// them (GET /api/p2p/attempts), and the steps of connecting (pkg/connectsteps).

export type Attempt = {
  at: number; device: string; short: string; name?: string; listed: boolean; kind: 'invite' | 'code' | 'connect'
  ok: boolean; step?: string; code?: string; reason?: string; title?: string; path?: string; count: number
}
export type StepInfo = { id: string; title: string; failed: string }
export type DeviceTries = { device: string; name: string; listed: boolean; tries: Attempt[] }

// The tries grouped by device, in the order they came (newest first), so the device that tried last is first.
export function byDevice(list: Attempt[]): DeviceTries[] {
  const out = new Map<string, DeviceTries>()
  for (const a of list) {
    let g = out.get(a.device)
    if (!g) { g = { device: a.device, name: a.name || '', listed: a.listed, tries: [] }; out.set(a.device, g) }
    if (!g.name && a.name) g.name = a.name
    g.tries.push(a)
  }
  return [...out.values()]
}

// Each step's state for a try that failed at `failed`: done before it, failed on it, not reached after. Null when
// the step isn't one of the steps.
export function stepStates(steps: StepInfo[], failed?: string): ('ok' | 'fail' | 'skip')[] | null {
  const at = steps.findIndex((s) => s.id === failed)
  if (at < 0) return null
  return steps.map((_, i) => (i < at ? 'ok' : i === at ? 'fail' : 'skip'))
}
