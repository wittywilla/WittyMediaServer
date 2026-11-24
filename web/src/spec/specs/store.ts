// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// Settings, Module store (canvas "modstore", "modfile") and Other stores. The module cards under the search
// are the page's own (they carry artwork and their own buttons). Ids: field q, publisher, platform, kind, status;
// chips cat:<id>. What's installed and what can be updated are the store's own tabs, not filters here.
export interface StoreSearch {
  q: string; cats: { id: string; title: string; n: number }[]; cat: string
  publishers: string[]; publisher: string; platforms: { value: string; label: string }[]; platform: string
  kinds: { value: string; label: string }[]; moduleKind: string; statuses: { value: string; label: string }[]; status: string
}
const opts = (first: string, list: { value: string; label: string }[]) => [{ value: '', label: first }, ...list]

export function storeSearchSpec(v: StoreSearch): SpecPageData {
  const s: SpecSection[] = [
    secF(t('Find a module'), [{ id: 'q', kind: 'text', label: t('Search modules'), value: v.q, placeholder: t('Search modules'), hint: t('By name, description or category.'), autoComplete: 'off' }]),
    secP(t('Category'), [{ id: 'cat:', title: t('All'), on: !v.cat }, ...v.cats.map((c) => ({ id: 'cat:' + c.id, title: `${c.title} · ${c.n}`, label: `${c.title}, ${c.n}`, on: v.cat === c.id }))]),
    secF(t('Narrow by'), [
      { id: 'publisher', kind: 'select' as const, label: t('Publisher'), value: v.publisher, options: opts(t('Anyone'), v.publishers.map((p) => ({ value: p, label: p }))) },
      { id: 'platform', kind: 'select' as const, label: t('Platform'), value: v.platform, options: opts(t('Any'), v.platforms) },
      ...(v.kinds.length ? [{ id: 'kind', kind: 'select' as const, label: t('Kind'), value: v.moduleKind, options: opts(t('Any'), v.kinds) }] : []),
      { id: 'status', kind: 'select' as const, label: t('Status'), value: v.status, options: opts(t('Any'), v.statuses) },
    ]),
  ]
  return mk('modstore', t('Settings · Modules'), t('Module store'), t('Add to what this server can do. Everything here is signed, and nothing runs until you choose it.'), s)
}

// Other stores: the stores added to this server, one section of rows each (facts, then buttons).
export interface StoreLine {
  id: number | string; name: string; host: string; enabled: boolean; url: string; fingerprint: string; modules: number
  added: string; checked: string; private: boolean; inUse: string[]; working: boolean; problem?: string
}
export function otherStoresSpec(stores: StoreLine[]): SpecPageData {
  const sections: SpecSection[] = []
  if (!stores.length) sections.push(secN(t('Stores on this server'), t('You haven’t added any. This server only uses this project’s store.')))
  for (const s of stores) {
    const k = 'store' + s.id
    sections.push(secF(s.enabled ? s.name : t('{name} (off)', { name: s.name }), [{ id: k + ':on', kind: 'switch', label: t('Use {name}', { name: s.name }), value: s.enabled, disabled: s.working, hint: s.host }]))
    if (s.problem) sections.push(secN('', s.problem, true))
    const rows = [
      { title: t('Address'), value: s.url }, { title: t('Pinned key fingerprint'), value: s.fingerprint }, { title: t('Modules'), value: String(s.modules) },
      { title: t('Added'), value: s.added }, { title: t('Last checked'), value: s.checked || t('Not yet') },
      ...(s.private ? [{ title: t('Network'), value: t('Allowed on your own network') }] : []),
      ...(s.inUse.length ? [{ title: t('In use by'), value: s.inUse.join(', ') }] : []),
    ]
    sections.push(secR('', rows))
    sections.push(secP('', [{ id: k + ':check', title: t('Check now'), disabled: s.working || !s.enabled }, { id: k + ':remove', title: t('Remove store'), label: t('Remove {name}', { name: s.name }), disabled: s.working }]))
  }
  return mk('otherstores', t('Settings · Modules · Other stores'), t('Other stores'), t('Add a store run by someone else, and see the modules it offers alongside this project’s. Everything from one is labelled, and whether to trust it is up to you.'), sections)
}

// Add a store: the address and its key, then (after the server has looked) what it found and the two agreements.
export interface AddStoreState {
  url: string; key: string; allowPrivate: boolean; needKey: boolean; working: boolean; failure?: { title: string; message: string }
  preview?: { name: string; host: string; count: number; warning: string[]; fingerprint: string; keySource: string; modules: { id: string; name: string; tag: string; summary?: string }[] }
  trust: boolean
}
export function addStoreSpec(v: AddStoreState): SpecPageData {
  const s: SpecSection[] = [
    secF(t('Add a store'), [
      { id: 'url', kind: 'text', label: t('Address'), required: true, value: v.url, placeholder: 'https://example.com/modules/catalog.json', hint: t('The address of its catalog.json, from whoever runs the store. It starts with https://.'), autoComplete: 'off' },
      { id: 'key', kind: 'text', label: t('The store’s public key'), value: v.key, placeholder: t('44 characters'), autoComplete: 'off', hint: v.needKey ? t('This store doesn’t offer one, so ask its owner.') : t('Optional, and better: if its owner gave you a key, paste it, so the first answer doesn’t have to be taken on trust. Otherwise the store’s own suggestion is shown for you to check.') },
      { id: 'private', kind: 'switch', label: t('Allow an address on my own network'), value: v.allowPrivate, hint: t('Only for a store you run yourself. It lets this server fetch from your own network for this one store, and over plain http there.') },
    ]),
  ]
  if (v.failure) s.push(secN(v.failure.title, v.failure.message, true))
  if (!v.preview) s.push(secP('', [{ id: 'look', title: t('Look at this store'), disabled: v.working || !v.url.trim() }]))
  else {
    const p = v.preview
    s.push(secN(t('Before you add {name}', { name: p.name }), `${tn('From {host}. {count} module.', 'From {host}. {count} modules.', p.count, { host: p.host })} ${p.warning.join(' ')}`, true))
    s.push(secF('', [{ id: 'fingerprint', kind: 'code', label: p.keySource === 'offered' ? t('The key this store offers. Nothing has checked it for you yet.') : t('The key you pasted, which this store’s signature matches.'), value: p.fingerprint }]))
    if (p.modules.length) s.push(secR(t('What it offers'), p.modules.map((m) => ({ title: m.name, value: m.tag, sub: m.summary }))))
    s.push(secF('', [{ id: 'trust', kind: 'switch', label: t('I understand, and I trust whoever runs this store'), value: v.trust }]))
    s.push(secP('', [{ id: 'add', title: t('Add this store'), disabled: !v.trust }, { id: 'cancel', title: t('Cancel') }]))
  }
  return mk('addstore', t('Settings · Modules · Other stores'), t('Add a store'), '', s)
}
