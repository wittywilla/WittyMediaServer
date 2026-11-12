// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Settings, Modules, the store (canvas "modstore", "modfile") and Other stores. The module cards under the search
// are the page's own (they carry artwork and their own buttons). Ids: field q, publisher, platform, kind, status;
// chips cat:<id>, show:installed, show:updates, check.
export interface StoreSearch {
  q: string; cats: { id: string; title: string; n: number }[]; cat: string; show: string; updates: number
  publishers: string[]; publisher: string; platforms: { value: string; label: string }[]; platform: string
  kinds: { value: string; label: string }[]; moduleKind: string; statuses: { value: string; label: string }[]; status: string
  checking: boolean
}
const opts = (first: string, list: { value: string; label: string }[]) => [{ value: '', label: first }, ...list]

export function storeSearchSpec(v: StoreSearch): SpecPageData {
  const s: SpecSection[] = [
    secF('Find a module', [{ id: 'q', kind: 'text', label: 'Search modules', value: v.q, placeholder: 'Search modules', hint: 'By name, description or category.', autoComplete: 'off' }]),
    secP('Category', [{ id: 'cat:', title: 'All', on: !v.cat }, ...v.cats.map((c) => ({ id: 'cat:' + c.id, title: `${c.title} · ${c.n}`, label: `${c.title}, ${c.n}`, on: v.cat === c.id }))]),
    secP('Show', [
      { id: 'show:installed', title: 'Installed', on: v.show === 'installed' },
      { id: 'show:updates', title: `Updates available${v.updates ? ` · ${v.updates}` : ''}`, on: v.show === 'updates' },
      { id: 'check', title: v.checking ? 'Checking…' : 'Check now', label: 'Check for updates now', disabled: v.checking },
    ]),
    secF('Narrow by', [
      { id: 'publisher', kind: 'select' as const, label: 'Publisher', value: v.publisher, options: opts('Anyone', v.publishers.map((p) => ({ value: p, label: p }))) },
      { id: 'platform', kind: 'select' as const, label: 'Platform', value: v.platform, options: opts('Any', v.platforms) },
      ...(v.kinds.length ? [{ id: 'kind', kind: 'select' as const, label: 'Kind', value: v.moduleKind, options: opts('Any', v.kinds) }] : []),
      { id: 'status', kind: 'select' as const, label: 'Status', value: v.status, options: opts('Any', v.statuses) },
    ]),
  ]
  return mk('modstore', 'Settings · Modules', 'Module store', 'Add to what this server can do. Everything here is signed, and nothing runs until you choose it.', s)
}

// Other stores: the stores added to this server, one section of rows each (facts, then buttons).
export interface StoreLine {
  id: number | string; name: string; host: string; enabled: boolean; url: string; fingerprint: string; modules: number
  added: string; checked: string; private: boolean; inUse: string[]; working: boolean; problem?: string
}
export function otherStoresSpec(stores: StoreLine[]): SpecPageData {
  const sections: SpecSection[] = []
  if (!stores.length) sections.push(secN('Stores on this server', 'You haven’t added any. This server only uses this project’s store.'))
  for (const s of stores) {
    const k = 'store' + s.id
    sections.push(secF(s.name + (s.enabled ? '' : ' (off)'), [{ id: k + ':on', kind: 'switch', label: `Use ${s.name}`, value: s.enabled, disabled: s.working, hint: s.host }]))
    if (s.problem) sections.push(secN('', s.problem, true))
    const rows = [
      { title: 'Address', value: s.url }, { title: 'Pinned key fingerprint', value: s.fingerprint }, { title: 'Modules', value: String(s.modules) },
      { title: 'Added', value: s.added }, { title: 'Last checked', value: s.checked || 'Not yet' },
      ...(s.private ? [{ title: 'Network', value: 'Allowed on your own network' }] : []),
      ...(s.inUse.length ? [{ title: 'In use by', value: s.inUse.join(', ') }] : []),
    ]
    sections.push(secR('', rows))
    sections.push(secP('', [{ id: k + ':check', title: 'Check now', disabled: s.working || !s.enabled }, { id: k + ':remove', title: 'Remove store', label: `Remove ${s.name}`, disabled: s.working }]))
  }
  return mk('otherstores', 'Settings · Modules · Other stores', 'Other stores', 'Add a store run by someone else, and see the modules it offers alongside this project’s. Everything from one is labelled, and whether to trust it is up to you.', sections)
}

// Add a store: the address and its key, then (after the server has looked) what it found and the two agreements.
export interface AddStoreState {
  url: string; key: string; allowPrivate: boolean; needKey: boolean; working: boolean; failure?: { title: string; message: string }
  preview?: { name: string; host: string; count: number; warning: string[]; fingerprint: string; keySource: string; modules: { id: string; name: string; tag: string; summary?: string }[] }
  trust: boolean
}
export function addStoreSpec(v: AddStoreState): SpecPageData {
  const s: SpecSection[] = [
    secF('Add a store', [
      { id: 'url', kind: 'text', label: 'Address', required: true, value: v.url, placeholder: 'https://example.com/modules/catalog.json', hint: 'The address of its catalog.json, from whoever runs the store. It starts with https://.', autoComplete: 'off' },
      { id: 'key', kind: 'text', label: 'The store’s public key', value: v.key, placeholder: '44 characters', autoComplete: 'off', hint: v.needKey ? 'This store doesn’t offer one, so ask its owner.' : 'Optional, and better: if its owner gave you a key, paste it, so the first answer doesn’t have to be taken on trust. Otherwise the store’s own suggestion is shown for you to check.' },
      { id: 'private', kind: 'switch', label: 'Allow an address on my own network', value: v.allowPrivate, hint: 'Only for a store you run yourself. It lets this server fetch from your own network for this one store, and over plain http there.' },
    ]),
  ]
  if (v.failure) s.push(secN(v.failure.title, v.failure.message, true))
  if (!v.preview) s.push(secP('', [{ id: 'look', title: 'Look at this store', disabled: v.working || !v.url.trim() }]))
  else {
    const p = v.preview
    s.push(secN(`Before you add ${p.name}`, `From ${p.host}. ${p.count} ${p.count === 1 ? 'module' : 'modules'}. ${p.warning.join(' ')}`, true))
    s.push(secF('', [{ id: 'fingerprint', kind: 'code', label: p.keySource === 'offered' ? 'The key this store offers. Nothing has checked it for you yet.' : 'The key you pasted, which this store’s signature matches.', value: p.fingerprint }]))
    if (p.modules.length) s.push(secR('What it offers', p.modules.map((m) => ({ title: m.name, value: m.tag, sub: m.summary }))))
    s.push(secF('', [{ id: 'trust', kind: 'switch', label: 'I understand, and I trust whoever runs this store', value: v.trust }]))
    s.push(secP('', [{ id: 'add', title: 'Add this store', disabled: !v.trust }, { id: 'cancel', title: 'Cancel' }]))
  }
  return mk('addstore', 'Settings · Modules · Other stores', 'Add a store', '', s)
}
