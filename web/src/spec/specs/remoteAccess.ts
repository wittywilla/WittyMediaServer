// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// Settings, Remote Access (canvas "remoteaccess"): who can connect, limits for viewers away, who is connected from
// outside, sign-in protection, advanced addresses. Real data from GET /remote and GET /sessions?all=1.
export type Choice = 'home' | 'private' | 'internet'
// Names, tags and lines are marked with tk: shown with t().
export const CHOICES: { id: Choice; name: string; tag?: string; who: string }[] = [
  { id: 'home', name: tk('Home network only'), tag: tk('Safest'), who: tk('Only devices on your Wi-Fi or cable at home. A phone on mobile data can’t connect.') },
  { id: 'private', name: tk('Home and away, through a private network'), tag: tk('Recommended'), who: tk('Devices at home, and your own devices anywhere once they’ve joined your private network with Tailscale or NetBird. Nothing is opened on your router, so the internet can’t see this server.') },
  { id: 'internet', name: tk('Home and away, through the internet'), who: tk('Devices at home, and anyone anywhere who has the address: they reach the sign-in page. Through a port on your router, a reverse proxy you already run, or a rented server, always encrypted with HTTPS.') },
]

// Row ids "choice:<id>". The one chosen is `on`. Home is not offered when the server's configuration forces access open.
export function chooserSpec(choice: Choice, forced: boolean, busy: boolean): SpecPageData {
  const rows: SpecRow[] = CHOICES.map((c) => {
    const locked = c.id === 'home' && forced
    return {
      id: 'choice:' + c.id, title: t(c.name), value: c.tag ? t(c.tag) : undefined, on: choice === c.id, disabled: locked || busy,
      sub: locked ? t('{who} Can’t be chosen here: the server’s configuration (the remoteAccess or publicUrl setting) lets devices away from home in.', { who: t(c.who) }) : t(c.who),
    }
  })
  return mk('remoteaccess', t('Settings · Remote access'), t('Who can connect'), t('Choose one. You can change your mind at any time, and only what your choice needs is shown below it.'), [secR('', rows)])
}

// "Limits for viewers away from home": ids maxHeight and maxBitrate.
export function limitsSpec(maxHeight: number, maxBitrate: string): SpecPageData {
  const heights = [[0, t('No limit')], [2160, '4K'], [1080, '1080p'], [720, '720p'], [480, '480p'], [360, '360p']] as const
  const rates = ['20M', '12M', '8M', '6M', '4M', '3M', '2M', '1M']
  return mk('remoteaccess', t('Settings · Remote access'), t('Limits for viewers away from home'), '', [secF('', [
    { id: 'maxHeight', kind: 'select', label: t('Highest resolution'), hint: t('Larger files are converted first, which takes work from this computer.'), value: String(maxHeight), options: heights.map(([v, l]) => ({ value: String(v), label: l })) },
    { id: 'maxBitrate', kind: 'select', label: t('Most data per stream'), hint: t('Video and sound together. Keep it below your connection’s upload speed, divided by how many people may watch at once.'), value: maxBitrate, options: [{ value: '', label: t('No limit') }, ...rates.map((r) => ({ value: r, label: t('{rate} Mb/s', { rate: r.replace('M', '') }) }))] },
  ])])
}

export interface OutsideSession { id: string; userName: string; device: string; where: string; via: string; ip: string; lastSeen: number; streaming: boolean; current: boolean; plainAway?: boolean }
const SHORT: Record<string, string> = { machine: tk('This computer'), home: tk('At home'), vpn: tk('VPN'), internet: tk('Internet') }

// People connected from outside: a row each, and a "Sign out <name> on <device>" chip each (ids out:<id>).
export function outsideSpec(list: OutsideSession[], viaLabel: (where: string, via: string) => string, ago: (at: number) => string): SpecPageData {
  const rows: SpecRow[] = list.map((v) => ({
    id: 'who:' + v.id, title: `${v.userName} · ${v.device || t('Unknown device')}`,
    value: [v.current ? t('This device') : '', v.streaming ? t('Streaming') : '', v.plainAway ? t('Signed in unencrypted') : ''].filter(Boolean).join(' · ') || undefined,
    sub: `${SHORT[v.where] ? t(SHORT[v.where]) : v.where} · ${viaLabel(v.where, v.via)} · ${v.ip} · ${ago(v.lastSeen)}`, on: v.current,
  }))
  const chips: SpecChip[] = list.filter((v) => !v.current).map((v) => ({ ...Cc(v.device ? t('Sign {name} out on {device}', { name: v.userName, device: v.device }) : t('Sign {name} out on this device', { name: v.userName })), id: 'out:' + v.id }))
  const sections: SpecSection[] = rows.length ? [secR('', rows)] : [secN('', t('Nobody yet.'))]
  if (chips.length) sections.push(secP('', chips))
  return mk('remoteaccess', t('Settings · Remote access'), t('People connected from outside'), '', sections)
}

export interface Lockout { kind: string; addr?: string; name?: string; until: number; fails: number }
// Sign-in protection. Ids: lockout:<i> rows, clear chip, twofa link.
export function protectionSpec(o: { failures: number; lockouts: Lockout[]; weak: string[]; unknown: string[]; adminsWithout2fa: number; fmtTime: (at: number) => string }): SpecPageData {
  const lockText = (l: Lockout) => {
    const until = o.fmtTime(l.until)
    if (l.kind === 'address') return tn('The address {address} can\'t sign in until {time}, after {count} wrong try.', 'The address {address} can\'t sign in until {time}, after {count} wrong tries.', l.fails, { address: l.addr, time: until })
    if (l.kind === 'account') return tn('“{name}” can\'t sign in from outside your home until {time}, after {count} wrong try. It still works at home.', '“{name}” can\'t sign in from outside your home until {time}, after {count} wrong tries. It still works at home.', l.fails, { name: l.name, time: until })
    return t('“{name}” from {address} can\'t try again until {time}.', { name: l.name, address: l.addr, time: until })
  }
  const rows: SpecRow[] = [
    R(t('Wrong sign-ins in the last day'), String(o.failures)),
    { ...R(t('Passwords'), o.weak.length ? t('Weak: {names}', { names: o.weak.join(', ') }) : o.unknown.length ? t('Not checked yet: {names}', { names: o.unknown.join(', ') }) : t('All fine'), o.weak.length > 0, t('How strong they are is only known when someone signs in with them, and they are never stored.')) },
    { ...R(t('Two-factor sign-in'), o.adminsWithout2fa > 0 ? t('Set it up') : t('On for administrators'), false, t('A code from an app on your phone, on top of your password. Each adult turns it on for themselves.')), id: 'twofa', to: '/settings/security' },
  ]
  const sections: SpecSection[] = [secR('', rows)]
  if (o.lockouts.length) {
    sections.push(secR(t('Locked out'), o.lockouts.map((l, i) => ({ ...R(lockText(l)), id: 'lockout:' + i }))))
    sections.push(secP('', [{ ...Cc(t('Let everyone try again')), id: 'clear' }]))
  }
  return mk('remoteaccess', t('Settings · Remote access'), t('Sign-in protection'), t('Always on. Repeated wrong passwords make whoever is guessing wait longer and longer. Nobody on the internet can lock you out at home.'), sections)
}

// Advanced addresses; field ids publicUrl, trustedProxies, homeNets, awayNets; action save.
export function advancedSpec(v: { publicUrl: string; trustedProxies: string; homeNets: string; awayNets: string }, dirty: boolean, busy: boolean): SpecPageData {
  const f = (id: keyof typeof v, label: string, hint: string, placeholder: string): SpecField => ({ id, kind: 'text', label, hint, placeholder, value: v[id], autoComplete: 'off' })
  return mk('remoteaccess', t('Settings · Remote access'), t('Advanced'), '', [secF('', [
    f('publicUrl', t('Outside address'), t('Where visitors from the internet are sent when they use the unencrypted port.'), 'https://media.example.com'),
    f('trustedProxies', t('Trusted reverse proxies'), t('Addresses or networks, separated by commas. X-Forwarded-For is believed from these only, and from nobody by default.'), '127.0.0.1, 10.0.0.2'),
    f('homeNets', t('Also count as my home network'), t('Private addresses already count. Devices here can see profile names and sign in without a password where allowed, so list only what you control.'), '203.0.113.0/24'),
    f('awayNets', t('Count as away from home'), t('A VPN of your own that uses private addresses, for example WireGuard. Tailscale and NetBird are recognised by themselves.'), '10.8.0.0/24'),
  ])], [{ id: 'save', title: t('Save'), kind: 'primary', disabled: busy || !dirty }])
}
