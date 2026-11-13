// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Settings, Remote Access, Peer-to-peer connections (part of canvas "remoteaccess"). Real data from GET /p2p/settings.
export type LayerId = 'dht' | 'relay' | 'holePunch' | 'portMap' | 'ipv6'
export const LAYERS: { id: LayerId; label: string; desc: string }[] = [
  { id: 'portMap', label: 'Ask my router for a port', desc: 'Asks the router at home to forward one port to this computer (UPnP, NAT-PMP or PCP), renews it, and removes it when this is turned off. Nothing leaves your home for it.' },
  { id: 'ipv6', label: 'Ask my router to let IPv6 in', desc: 'Where your connection has IPv6, asks the router to let connections reach this computer on that one port.' },
  { id: 'dht', label: 'Public address book', desc: 'Announces this server’s ID and internet address on the public IPFS network, never what’s on it, so apps find it by its ID and a TV that has never been at home can join by a code. Off, apps can only use addresses they already know, which stops working when your address changes, and a TV can’t join by a code.' },
  { id: 'relay', label: 'Relays', desc: 'Lets a relay introduce an app to this server, and carry the connection when nothing else works. A relay sees addresses and amounts, never what is sent.' },
  { id: 'holePunch', label: 'Hole punching', desc: 'Has the app and this server connect to each other at the same moment, so both routers let the connection through. Needs relays.' },
]
export interface DeviceLine { id: string; user?: string; name: string; platform: string; how: string; created: number; lastSeen: number; connected: boolean; path?: string }
const PATH: Record<string, string> = { direct: 'Connected directly', relayed: 'Connected through a relay' }

// Devices that may connect: a row each, and a Remove chip each (ids remove:<id>) and the invite chip (invite).
export function devicesSpec(devices: DeviceLine[], ago: (t?: number) => string): SpecPageData {
  const sections: SpecSection[] = []
  if (!devices.length) sections.push(secN('', 'None yet. Open the app on a phone, TV or computer while it’s at home and signed in: it adds itself. Or invite a device that isn’t at home.'))
  else {
    sections.push(secR('', devices.map((d): SpecRow => ({
      id: 'device:' + d.id, title: `${d.name}${d.user ? ` · ${d.user}` : ''}`, value: d.connected ? PATH[d.path || 'direct'] : undefined, on: d.connected,
      sub: `${d.platform || 'app'} · added ${ago(d.created)} ${d.how === 'invite' ? 'by invite' : 'while signed in'} · last connected ${ago(d.lastSeen)} · ID ending ${d.id.slice(-8)}`,
    }))))
  }
  const chips: SpecChip[] = devices.map((d) => ({ ...Cc(`Remove ${d.name}`), id: 'remove:' + d.id }))
  chips.push({ ...Cc('Invite a device that isn’t at home'), id: 'invite' })
  sections.push(secP('', chips))
  return mk('remoteaccess', 'Settings · Remote access', 'Devices that may connect', '', sections)
}

// Who may try, the ways of connecting, relay limit, helping others. Field ids: devicesOnly, layer:<id>, relayKbps, relayForOthers.
export function optionsSpec(s: { devicesOnly: boolean; layers: Record<LayerId, boolean>; relayKbps: number; relayForOthers: boolean }): SpecPageData {
  const rates = [1000, 2000, 4000, 8000, 20000]
  const layers: SpecField[] = LAYERS.map((l) => ({ id: 'layer:' + l.id, kind: 'switch', label: l.label, hint: l.desc, value: s.layers[l.id], disabled: l.id === 'holePunch' && !s.layers.relay }))
  return mk('remoteaccess', 'Settings · Remote access', 'Peer-to-peer options', '', [
    secF('Who may try', [{ id: 'devicesOnly', kind: 'switch', label: 'Only paired devices', hint: 'A device that isn’t on the list above gets no answer at all, not even the sign-in page. Leave this on unless you have a reason.', value: s.devicesOnly }]),
    secF('Ways of connecting', layers),
    secF('Relays', [
      { id: 'relayKbps', kind: 'select', label: 'Limit through a relay', hint: 'A relay is someone else’s internet connection. This is the most this server sends through one; the quality limits further down this page apply as well.', value: String(s.relayKbps), options: [...rates.map((k) => ({ value: String(k), label: `${k / 1000} Mbit/s` })), { value: '0', label: 'No limit' }] },
      { id: 'relayForOthers', kind: 'switch', label: 'Help other servers', hint: 'While this server can be reached from outside, lets up to 16 other WittyMediaServer owners’ connections pass through it when theirs can’t connect directly. It uses your internet connection (at most 2 GiB per connection), and you see none of what passes: it is encrypted between their devices and their servers.', value: s.relayForOthers },
    ]),
  ])
}
