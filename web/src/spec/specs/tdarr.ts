// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, mk, secF, secN, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Connecting one's own Tdarr (canvas page "tdarr": Connection and nodes). Field ids: url, key, enabled, and
// map:<libraryId>:<tdarr|from|to> for each library; action ids: test, forget, save, send:<libraryId>.
export interface TdarrView {
  url: string; key: string; keySet: boolean; enabled: boolean; dirty: boolean; busy: string
  libs: { id: number; name: string; tdarrId: string; from: string; to: string; canSend: boolean }[]
  known: { id: string; name: string }[]
  report: { ok: boolean; error?: string; version?: string; nodes: { id: string; name: string; paused: boolean; doing: string }[]; libraries: number } | null
}

export function tdarrSpec(v: TdarrView): SpecPageData {
  const sections: SpecSection[] = [
    secN('', t('Tdarr is a separate program, and it isn’t open source. It’s made by other people and comes under its own licence, with free and paid tiers. It isn’t included here, and this server doesn’t install it: you download it from tdarr.io and agree to its terms yourself. A Tdarr node connects to a Tdarr server, and that connection isn’t documented for anything else to use, so nodes can’t connect directly to this server: attach them to your Tdarr server as usual, and connect that here. Tdarr’s own plugins and flows decide what happens to a file, not the recipes above. If it changes a file’s extension, this server sees a new file on its next scan, and what’s been watched won’t carry over to it.'), true),
    secF('Tdarr', [
      { id: 'url', kind: 'text', label: t('Tdarr server address'), value: v.url, placeholder: 'http://192.168.1.20:8266', autoComplete: 'off', hint: t('Its server port, which is 8266 unless you’ve changed it. Not the web page port.') },
      { id: 'key', kind: 'password', label: t('API key'), value: v.key, autoComplete: 'new-password', placeholder: v.keySet ? t('Saved') : '', hint: v.keySet ? t('A key is saved. Type a new one to replace it.') : t('In Tdarr, under Tools, API Keys. Leave empty if its sign-in is off.') },
      { id: 'enabled', kind: 'switch', label: t('Use this connection'), value: v.enabled, hint: t('Shows what Tdarr is doing in the activity panel, asking it every 20 seconds while something’s going on or someone has the panel open, and every 2 minutes otherwise. Off, nothing contacts it.') },
    ]),
  ]
  const r = v.report
  if (r && !r.ok) sections.push(secN(t('That didn’t work'), r.error || '', true))
  if (r?.ok) {
    const nodes: SpecRow[] = r.nodes.map((n) => ({ title: n.paused ? t('{name}, paused', { name: n.name }) : n.name, value: n.doing }))
    sections.push(secR(r.version ? t('Connected to Tdarr {version}', { version: r.version }) : t('Connected to Tdarr'), [
      { title: t('Nodes'), value: String(r.nodes.length), on: true, sub: r.nodes.length ? undefined : t('No nodes are attached to it right now.') },
      ...nodes,
      { title: t('Libraries set up there'), value: String(r.libraries) },
    ]))
    if (r.error) sections.push(secN('', t('It answered, but not everything worked: {error}', { error: r.error }), true))
  }
  if (v.enabled && v.libs.length) {
    sections.push(secN(t('Handing files to Tdarr'), v.known.length
      ? t('Choose which of Tdarr’s libraries each one goes to. If the two machines call a folder by different names, say so, so a file can be named the way Tdarr knows it.')
      : t('Test the connection to list Tdarr’s libraries. If the two machines call a folder by different names, say so, so a file can be named the way Tdarr knows it.')))
    for (const l of v.libs) {
      const opts = v.known.some((k) => k.id === l.tdarrId) || !l.tdarrId ? v.known : [{ id: l.tdarrId, name: l.tdarrId }, ...v.known]
      const f: SpecField[] = [
        { id: `map:${l.id}:tdarr`, kind: 'select', label: t('Tdarr library'), value: l.tdarrId, options: [{ value: '', label: t('Not connected') }, ...opts.map((k) => ({ value: k.id, label: k.name }))] },
        { id: `map:${l.id}:from`, kind: 'text', label: t('Folder name here'), value: l.from, placeholder: '/srv/films' },
        { id: `map:${l.id}:to`, kind: 'text', label: t('Folder name in Tdarr'), value: l.to, placeholder: '/media' },
      ]
      sections.push(secF(l.name, f), secR('', [{ id: 'send:' + l.id, title: t('Ask Tdarr to look through this library'), disabled: !l.canSend }]))
    }
  }
  return mk('tdarr', t('Settings · Transcoding'), t('Connect your own Tdarr'), t('If you already use Tdarr, or want spare computers doing the work, this server can watch it and hand it files. It’s optional: everything above works without it.'), sections, [
    { ...A1(v.busy === 'save' ? t('Saving…') : t('Save the connection')), id: 'save', disabled: !v.dirty || !!v.busy },
    { title: v.busy === 'test' ? t('Testing…') : t('Test the connection'), kind: 'secondary', id: 'test', disabled: !!v.busy },
    ...(v.keySet ? [{ title: t('Forget the saved key'), kind: 'secondary' as const, id: 'forget', disabled: !!v.busy }] : []),
  ])
}
