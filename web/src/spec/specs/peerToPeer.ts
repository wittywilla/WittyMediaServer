// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

// Settings, Remote Access, Peer-to-peer connections (part of canvas "remoteaccess"). Real data from GET /p2p/settings.
export type LayerId = 'dht' | 'relay' | 'holePunch' | 'portMap' | 'ipv6'
// Labels and descriptions are marked with tk: shown with t().
export const LAYERS: { id: LayerId; label: string; desc: string }[] = [
  { id: 'portMap', label: tk('Ask my router for a port'), desc: tk('Asks the router at home to forward one port to this computer (UPnP, NAT-PMP or PCP), renews it, and removes it when this is turned off. Nothing leaves your home for it.') },
  { id: 'ipv6', label: tk('Ask my router to let IPv6 in'), desc: tk('Where your connection has IPv6, asks the router to let connections reach this computer on that one port.') },
  { id: 'dht', label: tk('Public address book'), desc: tk('Announces this server’s ID and internet address on the public IPFS network, never what’s on it, so apps find it by its ID and a TV that has never been at home can join by a code. Off, apps can only use addresses they already know, which stops working when your address changes, and a TV can’t join by a code.') },
  { id: 'relay', label: tk('Relays'), desc: tk('Lets a relay introduce an app to this server, and carry the connection when nothing else works. A relay sees addresses and amounts, never what is sent.') },
  { id: 'holePunch', label: tk('Hole punching'), desc: tk('Has the app and this server connect to each other at the same moment, so both routers let the connection through. Needs relays.') },
]
// pending: the device has never made a request, so it hasn't really connected, whatever else is true (an invite
// shown through a relay is not a connection). why: what the server knows about why it can't, in a sentence.
export interface DeviceLine { id: string; user?: string; name: string; platform: string; how: string; created: number; lastSeen: number; connected: boolean; path?: string; pending?: boolean; why?: string }
const PATH: Record<string, string> = { direct: tk('Connected directly'), relayed: tk('Connected through a relay') }

// Devices that may connect: a row each, with its Remove button in the row (action id remove:<id>), so it can't be
// taken for the next heading's. New devices come in by Connect a device, at the top of the page.
export function devicesSpec(devices: DeviceLine[], ago: (at?: number) => string): SpecPageData {
  const sections: SpecSection[] = []
  if (!devices.length) sections.push(secN('', t('None yet. A device you connect with Connect a device, above, adds itself here, and so does an app that is signed in at home.')))
  else {
    sections.push(secR('', devices.map((d): SpecRow => {
      const vars = { platform: d.platform || t('app'), added: ago(d.created), seen: ago(d.lastSeen), id: d.id.slice(-8) }
      const pending = d.pending || !d.lastSeen
      // Until its first request gets through it is invited, never "connected": showing an invite isn't connecting.
      const value = pending
        ? (d.how === 'invite' ? t('Invited — hasn’t connected yet') : t('Hasn’t connected from away yet'))
        : d.connected ? t(PATH[d.path || 'direct']) : undefined
      const details = pending
        ? (d.how === 'invite' ? t('{platform} · invited {added} · ID ending {id}', vars) : t('{platform} · added {added} while signed in · ID ending {id}', vars))
        : d.how === 'invite'
          ? t('{platform} · added {added} by invite · last connected {seen} · ID ending {id}', vars)
          : t('{platform} · added {added} while signed in · last connected {seen} · ID ending {id}', vars)
      return {
        id: 'device:' + d.id, title: d.user ? `${d.name} · ${d.user}` : d.name, value, on: !pending && d.connected,
        sub: d.why ? `${details}. ${d.why}` : details,
        actions: [{ id: 'remove:' + d.id, title: t('Remove'), label: t('Remove {name}', { name: d.name }), danger: true }],
      }
    })))
  }
  return mk('remoteaccess', t('Settings · Remote access'), t('Devices that may connect'), '', sections)
}

// Who may try, the ways of connecting, relay limit, helping others. Field ids: devicesOnly, layer:<id>, relayKbps,
// relayForOthers, and while helping, shareKbps and shareMaxHeight.
export function optionsSpec(s: { devicesOnly: boolean; layers: Record<LayerId, boolean>; relayKbps: number; relayForOthers: boolean; shareKbps: number; shareMaxHeight: number }): SpecPageData {
  const rates = [1000, 2000, 4000, 8000, 20000]
  const layers: SpecField[] = LAYERS.map((l) => ({ id: 'layer:' + l.id, kind: 'switch', label: t(l.label), hint: t(l.desc), value: s.layers[l.id], disabled: l.id === 'holePunch' && !s.layers.relay }))
  const relays: SpecField[] = [
    { id: 'relayKbps', kind: 'select', label: t('Limit through a relay'), hint: t('A relay is someone else’s internet connection. This is the most this server sends through one; a relay that allows less gets less, and films through it are made to fit. The quality limits further down this page apply as well.'), value: String(s.relayKbps), options: [...rates.map((k) => ({ value: String(k), label: t('{rate} Mbit/s', { rate: k / 1000 }) })), { value: '0', label: t('No limit') }] },
    { id: 'relayForOthers', kind: 'switch', label: t('Help other servers'), hint: t('While this server can be reached from outside, lets up to 16 other WittyMediaServer owners’ connections pass through it when theirs can’t connect directly. It uses your internet connection, within the limits below, which their servers are told before they use it. You see none of what passes: it is encrypted between their devices and their servers.'), value: s.relayForOthers },
  ]
  if (s.relayForOthers) {
    relays.push(
      { id: 'shareKbps', kind: 'select', label: t('Upload you share'), hint: t('The most of your internet connection that passes through for everyone together. Each server using it is told this before it does.'), value: String(s.shareKbps), options: [...[2000, 4000, 8000, 20000, 50000].map((k) => ({ value: String(k), label: t('{rate} Mbit/s', { rate: k / 1000 }) })), { value: '0', label: t('No limit') }] },
      { id: 'shareMaxHeight', kind: 'select', label: t('Picture size through your relay'), hint: t('Servers using your relay make the films they send through it this size or smaller. Smaller pictures use less of your connection.'), value: String(s.shareMaxHeight), options: [...[480, 720, 1080].map((h) => ({ value: String(h), label: t('{height}p', { height: h }) })), { value: '0', label: t('Any size') }] },
    )
  }
  return mk('remoteaccess', t('Settings · Remote access'), t('Peer-to-peer options'), '', [
    secF(t('Who may try'), [{ id: 'devicesOnly', kind: 'switch', label: t('Only paired devices'), hint: t('A device that isn’t on the list above gets no answer at all, not even the sign-in page. Leave this on unless you have a reason.'), value: s.devicesOnly }]),
    secF(t('Ways of connecting'), layers),
    secF(t('Relays'), relays),
  ])
}
