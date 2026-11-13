// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// A module version made for an older server: it may start here, but the pages, settings and
// features this server expects of it may be missing (WittyPlay 0.3.6, made for 0.6, on a 0.8.0-alpha
// server). The server never installs one by itself: it answers 409 with code "older-server" and
// names each one, and only a request that says confirmOlder installs it. Everything here is what
// the pop-up needs to ask, kept apart from the page so it can be tested on its own.

export type OlderVersion = { module: string; name: string; version: string; madeFor: string; server: string; warning: string }

// What a store item or a refusal carries; only the fields read here.
type ItemLike = { id: string; name: string; official?: boolean; version?: string; madeFor?: string; olderServer?: boolean; fit: { kind: string; why?: string }; status?: { installed?: boolean } }

// series is a version as people say which release it belongs to: "0.6.x" and "0.6.2" are "0.6".
export function series(v: string): string {
  const bare = v.replace(/\.x$/, '').replace(/[-+].*$/, '')
  const parts = bare.split('.')
  return parts.length >= 2 ? `${parts[0]}.${parts[1]}` : bare
}

// olderOf reads the server's refusal of a version made for an older server, or null for any
// other error. It looks at the shape alone, so it works on any error the api module throws.
export function olderOf(e: unknown): OlderVersion[] | null {
  const x = e as { status?: number; data?: { code?: string; older?: unknown } } | null
  if (!x || typeof x !== 'object' || x.status !== 409 || x.data?.code !== 'older-server' || !Array.isArray(x.data.older)) return null
  const list = (x.data.older as Partial<OlderVersion>[]).filter((o) => o && typeof o.module === 'string' && typeof o.version === 'string')
    .map((o) => ({ module: o.module!, name: o.name || o.module!, version: o.version!, madeFor: o.madeFor || '', server: o.server || '', warning: o.warning || '' }))
  return list.length ? list : null
}

// olderFromStore finds, among the modules about to be installed, those whose only version was made
// for an older server, before anything is asked of the server: first-run setup asks about them all
// in one pop-up.
export function olderFromStore(items: ItemLike[], ids: string[], server: string): OlderVersion[] {
  const out: OlderVersion[] = []
  for (const id of ids) {
    const it = items.find((i) => i.id === id && i.official !== false)
    if (!it || it.status?.installed || it.fit.kind !== 'older-server' || !it.version) continue
    out.push({ module: it.id, name: it.name, version: it.version, madeFor: it.madeFor || '', server, warning: it.fit.why || '' })
  }
  return out
}

// One choice for each: true installs it anyway. Every one starts as "Don't install".
export type OlderChoices = Record<string, boolean>
export const startChoices = (list: OlderVersion[]): OlderChoices => Object.fromEntries(list.map((o) => [o.module, false]))
export const confirmedIds = (choices: OlderChoices): string[] => Object.keys(choices).filter((id) => choices[id])

// The pop-up's title.
export function olderTitle(list: OlderVersion[]): string {
  if (list.length === 1) return `${list[0].name} ${list[0].version} was made for an older server`
  return `${list.length} modules were made for an older server`
}

// What each one means, in plain words: what it was made for, what this server is, and what may
// not work. The server's own warning comes first when it sent one.
export function olderWords(o: OlderVersion): string[] {
  const made = o.madeFor ? `WittyMediaServer ${series(o.madeFor)}` : 'an older WittyMediaServer'
  const here = o.server || 'this server'
  const lines = [o.warning || `${o.name} ${o.version} was made for ${made}; this server is ${here}. Pages, settings and features from ${o.server ? series(o.server) : 'this server'} will be missing, and it may not work.`]
  lines.push(`If you install it anyway, parts of ${o.name} may show “This isn’t available”, and it can stop working when this server updates. You can remove it, or update it once a version made for ${o.server ? series(o.server) : 'this server'} is published.`)
  return lines
}

// What a module the person chose not to install says in a list of what happened.
export const notInstalledWords = (o: OlderVersion): string =>
  `No version of ${o.name} for this server is published yet, so it wasn’t installed. Version ${o.version} was made for WittyMediaServer ${series(o.madeFor) || 'an older series'}.`
