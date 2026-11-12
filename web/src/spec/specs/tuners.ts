// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secN, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Settings, Channels and tuners, the Tuners and IPTV section (canvas "tuner"): devices found on the network, and the
// tuners added. A tuner's row goes to its channels; Remove is a button on the row (id remove:<id>); a found device's
// button is set:<index>.
export interface TunerLine { id: number; name: string; driverName: string; channels: number; clear: number; encrypted: number; lastError?: string; detail?: string }
export interface FoundDevice { name: string; detail: string; canSetUp: boolean }

const n = (c: number) => `${c} ${c === 1 ? 'channel' : 'channels'}`

export function tunersSpec(list: TunerLine[] | null, found: FoundDevice[] | null): SpecPageData {
  const sections: SpecSection[] = []
  if (found && !found.length) sections.push(secN('', 'No new tuners found on your network or this machine, or the search didn’t finish. Press Add live TV to add one by its address.'))
  if (found && found.length) {
    sections.push(secR('Found on your network', found.map((f, i): SpecRow => ({ id: 'found:' + i, title: f.name, sub: f.detail, icon: 'Tv', actions: f.canSetUp ? [{ id: 'set:' + i, title: 'Set up', label: `Set up ${f.name}` }] : undefined }))))
  }
  if (list && !list.length && !found) sections.push(secN('', 'None yet. Add live TV walks you through it: a provider’s playlist, one stream, a tuner box on your network, SAT>IP or a TV card.'))
  if (list && list.length) {
    sections.push(secR('Your tuners', list.map((t): SpecRow => ({
      id: 'tuner:' + t.id, title: t.name, to: `tuner/${t.id}`, icon: 'Tv', tint: t.lastError ? 'd' : 'a',
      sub: [`${t.driverName} · ${n(t.channels)}${t.clear > 0 ? ` · ${t.clear} clear` : ''}${t.encrypted > 0 ? ` · ${t.encrypted} encrypted` : ''}`, t.lastError].filter(Boolean).join(' · '),
      value: 'Channels', actions: [{ id: 'remove:' + t.id, title: 'Remove', label: `Remove ${t.name}`, danger: true }],
    }))))
  }
  return mk('tuner', 'Settings · Channels and tuners', 'Tuners and IPTV', '', sections)
}

export interface GuideLine { id: number; name: string; programmes: number; read: string; lastError?: string }

// Guide data (canvas "tuner"): the XMLTV sources read, each with Read again and Remove (ids again:<id>, drop:<id>),
// and the field to add one (field "url", chip "add").
export function guideSourcesSpec(list: GuideLine[] | null, url: string): SpecPageData {
  const sections: SpecSection[] = []
  if (list && list.length) {
    sections.push(secR('Read', list.map((s): SpecRow => ({
      id: 'src:' + s.id, title: s.name, icon: 'Refresh', sub: [s.read ? `${s.programmes} listings · read ${s.read}` : 'Not read yet', s.lastError].filter(Boolean).join(' · '),
      actions: [{ id: 'again:' + s.id, title: 'Read again', label: `Read ${s.name} again` }, { id: 'drop:' + s.id, title: 'Remove', label: `Remove ${s.name}`, danger: true }],
    }))))
  }
  sections.push({ kind: 'fields', h: 'Add a guide', items: [{ id: 'url', kind: 'text', label: 'XMLTV address, or a file on the server', value: url }] })
  sections.push({ kind: 'chips', h: '', items: [{ id: 'add', title: 'Add', disabled: !url.trim() }] })
  return mk('tuner', 'Settings · Channels and tuners', 'Guide data', '', sections)
}
