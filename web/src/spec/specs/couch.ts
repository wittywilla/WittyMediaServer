// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { R, mk, secR } from '../model.ts'
import type { SpecPageData } from '../types.ts'

// The list of a person's own game PCs (canvas page "gamepcs"), from GET /couch/hosts. Each row opens that PC.
export interface HostRow { id: number; name: string; paired: boolean; state: string; message: string; currentGame?: string }

export function couchHostsSpec(hosts: HostRow[]): SpecPageData {
  const rows = hosts.map((h) => {
    const up = h.state === 'online' || h.state === 'busy'
    const running = h.state === 'busy' && !!h.currentGame
    const value = running ? `Running: ${h.currentGame}` : !h.paired ? 'Needs pairing' : up ? 'Online' : ''
    return { ...R(h.name, value, up, h.paired && !running ? h.message : ''), id: `host.${h.id}`, to: `/couch/${h.id}` }
  })
  return mk('gamepcs', 'Game PCs', 'Your game PCs', 'Stream from a PC you own.', [secR('PCs', rows)])
}
