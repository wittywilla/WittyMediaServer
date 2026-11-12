// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Settings, Remote Access (canvas "remoteaccess"): who can connect, limits for viewers away, who is connected from
// outside, sign-in protection, advanced addresses. Real data from GET /remote and GET /sessions?all=1.
export type Choice = 'home' | 'private' | 'internet'
export const CHOICES: { id: Choice; name: string; tag?: string; who: string }[] = [
  { id: 'home', name: 'Home network only', tag: 'Safest', who: 'Only devices on your Wi-Fi or cable at home. A phone on mobile data can’t connect.' },
  { id: 'private', name: 'Home and away, through a private network', tag: 'Recommended', who: 'Devices at home, and your own devices anywhere once they’ve joined your private network with Tailscale or NetBird. Nothing is opened on your router, so the internet can’t see this server.' },
  { id: 'internet', name: 'Home and away, through the internet', who: 'Devices at home, and anyone anywhere who has the address: they reach the sign-in page. Through a port on your router, a reverse proxy you already run, or a rented server, always encrypted with HTTPS.' },
]

// Row ids "choice:<id>". The one chosen is `on`. Home is not offered when the server's configuration forces access open.
export function chooserSpec(choice: Choice, forced: boolean, busy: boolean): SpecPageData {
  const rows: SpecRow[] = CHOICES.map((c) => {
    const locked = c.id === 'home' && forced
    return {
      id: 'choice:' + c.id, title: c.name, value: c.tag, on: choice === c.id, disabled: locked || busy,
      sub: locked ? `${c.who} Can’t be chosen here: the server’s configuration (the remoteAccess or publicUrl setting) lets devices away from home in.` : c.who,
    }
  })
  return mk('remoteaccess', 'Settings · Remote access', 'Who can connect', 'Choose one. You can change your mind at any time, and only what your choice needs is shown below it.', [secR('', rows)])
}

// "Limits for viewers away from home": ids maxHeight and maxBitrate.
export function limitsSpec(maxHeight: number, maxBitrate: string): SpecPageData {
  const heights = [[0, 'No limit'], [2160, '4K'], [1080, '1080p'], [720, '720p'], [480, '480p'], [360, '360p']] as const
  const rates = ['20M', '12M', '8M', '6M', '4M', '3M', '2M', '1M']
  return mk('remoteaccess', 'Settings · Remote access', 'Limits for viewers away from home', '', [secF('', [
    { id: 'maxHeight', kind: 'select', label: 'Highest resolution', hint: 'Larger files are converted first, which takes work from this computer.', value: String(maxHeight), options: heights.map(([v, l]) => ({ value: String(v), label: l })) },
    { id: 'maxBitrate', kind: 'select', label: 'Most data per stream', hint: 'Video and sound together. Keep it below your connection’s upload speed, divided by how many people may watch at once.', value: maxBitrate, options: [{ value: '', label: 'No limit' }, ...rates.map((r) => ({ value: r, label: `${r.replace('M', '')} Mb/s` }))] },
  ])])
}

export interface OutsideSession { id: string; userName: string; device: string; where: string; via: string; ip: string; lastSeen: number; streaming: boolean; current: boolean; plainAway?: boolean }
const SHORT: Record<string, string> = { machine: 'This computer', home: 'At home', vpn: 'VPN', internet: 'Internet' }

// People connected from outside: a row each, and a "Sign out <name> on <device>" chip each (ids out:<id>).
export function outsideSpec(list: OutsideSession[], viaLabel: (where: string, via: string) => string, ago: (t: number) => string): SpecPageData {
  const rows: SpecRow[] = list.map((v) => ({
    id: 'who:' + v.id, title: `${v.userName} · ${v.device || 'Unknown device'}`,
    value: [v.current ? 'This device' : '', v.streaming ? 'Streaming' : '', v.plainAway ? 'Signed in unencrypted' : ''].filter(Boolean).join(' · ') || undefined,
    sub: `${SHORT[v.where] || v.where} · ${viaLabel(v.where, v.via)} · ${v.ip} · ${ago(v.lastSeen)}`, on: v.current,
  }))
  const chips: SpecChip[] = list.filter((v) => !v.current).map((v) => ({ ...Cc(`Sign ${v.userName} out on ${v.device || 'this device'}`), id: 'out:' + v.id }))
  const sections: SpecSection[] = rows.length ? [secR('', rows)] : [secN('', 'Nobody yet.')]
  if (chips.length) sections.push(secP('', chips))
  return mk('remoteaccess', 'Settings · Remote access', 'People connected from outside', '', sections)
}

export interface Lockout { kind: string; addr?: string; name?: string; until: number; fails: number }
// Sign-in protection. Ids: lockout:<i> rows, clear chip, twofa link.
export function protectionSpec(o: { failures: number; lockouts: Lockout[]; weak: string[]; unknown: string[]; adminsWithout2fa: number; fmtTime: (t: number) => string }): SpecPageData {
  const lockText = (l: Lockout) => {
    const until = o.fmtTime(l.until)
    if (l.kind === 'address') return `The address ${l.addr} can't sign in until ${until}, after ${l.fails} wrong tries.`
    if (l.kind === 'account') return `“${l.name}” can't sign in from outside your home until ${until}, after ${l.fails} wrong tries. It still works at home.`
    return `“${l.name}” from ${l.addr} can't try again until ${until}.`
  }
  const rows: SpecRow[] = [
    R('Wrong sign-ins in the last day', String(o.failures)),
    { ...R('Passwords', o.weak.length ? `Weak: ${o.weak.join(', ')}` : o.unknown.length ? `Not checked yet: ${o.unknown.join(', ')}` : 'All fine', o.weak.length > 0, 'How strong they are is only known when someone signs in with them, and they are never stored.') },
    { ...R('Two-factor sign-in', o.adminsWithout2fa > 0 ? 'Set it up' : 'On for administrators', false, 'A code from an app on your phone, on top of your password. Each adult turns it on for themselves.'), id: 'twofa', to: '/settings/security' },
  ]
  const sections: SpecSection[] = [secR('', rows)]
  if (o.lockouts.length) {
    sections.push(secR('Locked out', o.lockouts.map((l, i) => ({ ...R(lockText(l)), id: 'lockout:' + i }))))
    sections.push(secP('', [{ ...Cc('Let everyone try again'), id: 'clear' }]))
  }
  return mk('remoteaccess', 'Settings · Remote access', 'Sign-in protection', 'Always on. Repeated wrong passwords make whoever is guessing wait longer and longer. Nobody on the internet can lock you out at home.', sections)
}

// Advanced addresses; field ids publicUrl, trustedProxies, homeNets, awayNets; action save.
export function advancedSpec(v: { publicUrl: string; trustedProxies: string; homeNets: string; awayNets: string }, dirty: boolean, busy: boolean): SpecPageData {
  const f = (id: keyof typeof v, label: string, hint: string, placeholder: string): SpecField => ({ id, kind: 'text', label, hint, placeholder, value: v[id], autoComplete: 'off' })
  return mk('remoteaccess', 'Settings · Remote access', 'Advanced', '', [secF('', [
    f('publicUrl', 'Outside address', 'Where visitors from the internet are sent when they use the unencrypted port.', 'https://media.example.com'),
    f('trustedProxies', 'Trusted reverse proxies', 'Addresses or networks, separated by commas. X-Forwarded-For is believed from these only, and from nobody by default.', '127.0.0.1, 10.0.0.2'),
    f('homeNets', 'Also count as my home network', 'Private addresses already count. Devices here can see profile names and sign in without a password where allowed, so list only what you control.', '203.0.113.0/24'),
    f('awayNets', 'Count as away from home', 'A VPN of your own that uses private addresses, for example WireGuard. Tailscale and NetBird are recognised by themselves.', '10.8.0.0/24'),
  ])], [{ id: 'save', title: 'Save', kind: 'primary', disabled: busy || !dirty }])
}
