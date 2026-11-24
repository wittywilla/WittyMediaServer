// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Whether a module works on this server, in plain words: a badge for its card, one sentence on what
// that means, and what to do about it. The server decides whether something fits (the item's fit);
// this only says it, following the table "Every state the module store shows" in the 0.9 plan
// (0.FuturePlans/0.9.0/MODULE-API-0.9.md, 3a): one badge, one sentence, one main action, and never
// an API number, a version range or the word "compatible". Installed state wins over store state.
// When the server sends its own sentence (fit.why), that's the one shown.
//
// From 0.9.0-alpha only modules built for 0.9 or newer load: one built for an older server is never
// offered as "install anyway". The 0.9 server says so with the kinds needs-update and not-ready;
// until it does, an older server's kinds (too-old, older-server) are read the same way here.
// Kept apart from the pages so it can be tested on its own.
import { series } from './olderServer.ts'
import { t } from './words.ts'

export type CompatState = 'works' | 'needs-update' | 'not-ready' | 'not-yet' | 'needs-newer-server' | 'not-here' | 'withdrawn' | 'blocked' | 'comes-with' | 'nothing-yet' | 'unreadable' | 'older-ok'
export type Compat = { state: CompatState; ok: boolean; tag: string; tone: 'ok' | 'warn' | 'bad' | 'off'; line: string; todo?: string; hidden?: boolean }

type ItemLike = {
  name: string; version?: string; fit: { ok: boolean; kind: string; why?: string }; minServer?: string; madeFor?: string
  status?: { installed?: boolean; version?: string; olderServer?: boolean; madeFor?: string; latest?: { version: string }; withdrawn?: { version?: string; reason?: string; severity?: string } }
}

const num = (v: string) => series(v).split('.').map((x) => parseInt(x, 10) || 0)
// strictMinimum: whether this server takes only modules built for its own series or newer (0.9 on).
export function strictMinimum(server: string): boolean {
  const [major, minor] = num(server || '0.0')
  return major > 0 || minor >= 9
}

export function compat(it: ItemLike, server: string, platformName = ''): Compat {
  const strict = strictMinimum(server)
  const installed = !!it.status?.installed
  const why = it.fit.why
  const have = it.status?.version || it.version || ''
  const fits = it.status?.latest?.version

  // Installed, but made for an older server: on 0.9 it's turned off until it's updated.
  const tooOldInstalled = installed && (it.fit.kind === 'needs-update' || it.fit.kind === 'not-ready' || !!it.status?.olderServer)
  if (tooOldInstalled && (strict || it.fit.kind === 'needs-update' || it.fit.kind === 'not-ready')) {
    if (it.fit.kind !== 'not-ready' && fits) return {
      state: 'needs-update', ok: false, tone: 'warn', tag: t('Needs an update'),
      line: why || t('The version you have ({have}) was made for an older WittyMediaServer, so it’s turned off. Version {fits} is made for this server.', { have, fits }),
      todo: t('Update {name}: nothing it saved is lost.', { name: it.name }),
    }
    return {
      state: 'not-ready', ok: false, tone: 'warn', tag: t('Not ready yet'),
      line: why || t('{name} isn’t ready for this server yet. It’s turned off, and nothing it saved is lost. You’ll get a notification when it’s ready.', { name: it.name }),
      todo: t('Check again later.'),
    }
  }
  if (installed && it.status?.withdrawn) {
    const w = it.status.withdrawn
    return {
      state: 'withdrawn', ok: false, tone: 'bad', tag: t('Withdrawn'),
      line: w.severity === 'security'
        ? t('The publisher took {name} {version} back because of a security problem. It has been turned off.', { name: it.name, version: w.version || have })
        : t('The publisher took {name} {version} back: {reason}', { name: it.name, version: w.version || have, reason: w.reason || t('no reason was given.') }),
      todo: fits ? t('Switch to version {version}.', { version: fits }) : t('Remove it, or keep it until another version is offered.'),
    }
  }
  if (installed) {
    if (it.status?.olderServer) return {
      state: 'older-ok', ok: true, tone: 'warn', tag: t('Made for an older server'), line: t('It runs, but some of what this server expects of it may be missing.'), todo: t('Update it when a newer version is offered.'),
    }
    if (it.fit.kind === 'blocked') return { state: 'blocked', ok: false, tone: 'bad', tag: t('Turned off'), line: why || t('{name} kept failing here, so it was turned off.', { name: it.name }), todo: t('Try again, or choose another version on its page.') }
    return { state: 'works', ok: true, tone: 'ok', tag: t('Works on this server'), line: t('Installed and works on this server.') }
  }
  switch (it.fit.kind) {
    case 'ok': return { state: 'works', ok: true, tone: 'ok', tag: t('Works on this server'), line: t('Works on this server.') }
    case 'too-old': case 'needs-update': case 'not-ready':
      return { state: 'not-yet', ok: false, tone: 'off', tag: t('Not available yet'), line: t('There’s no version of {name} for this server yet.', { name: it.name }), todo: t('Check again later.') }
    case 'older-server': return strict
      ? { state: 'not-yet', ok: false, tone: 'off', tag: t('Not available yet'), line: t('There’s no version of {name} for this server yet.', { name: it.name }), todo: t('Check again later.') }
      : { state: 'older-ok', ok: false, tone: 'warn', tag: t('Made for an older server'), line: why || t('Only a version made for an older server is offered. It may start, but some of what this server expects of it may be missing.'), todo: t('You’ll be asked before it installs.') }
    case 'too-new': return {
      state: 'needs-newer-server', ok: false, tone: 'off', tag: t('Needs a newer server'),
      line: why || (it.minServer ? t('{name} {version} needs WittyMediaServer {min} or newer.', { name: it.name, version: it.version || '', min: it.minServer }).replace('  ', ' ') : t('{name} needs a newer WittyMediaServer.', { name: it.name })),
      todo: t('Update this server from Settings, About.'),
    }
    case 'no-build': return { state: 'not-here', ok: false, tone: 'off', tag: t('Not for this system'), line: platformName ? t('{name} has no version for {platform}.', { name: it.name, platform: platformName }) : t('{name} has no version for this system.', { name: it.name }) }
    case 'withdrawn': return { state: 'withdrawn', ok: false, tone: 'bad', tag: t('Withdrawn'), line: why || t('The publisher took {name} back.', { name: it.name }), todo: t('Check again later for a fixed version.') }
    case 'blocked': return { state: 'blocked', ok: false, tone: 'bad', tag: t('Turned off'), line: why || t('{name} kept failing here, so it was turned off.', { name: it.name }), todo: t('Try again, or choose another version on its page.') }
    case 'library': return { state: 'comes-with', ok: false, tone: 'off', tag: t('Comes with other modules'), line: t('It’s installed by itself with the modules that are built on it.'), hidden: true }
    case 'no-version': return { state: 'nothing-yet', ok: false, tone: 'off', tag: t('Nothing to install yet'), line: why || t('The store has no version of {name} to offer yet.', { name: it.name }), hidden: true }
    case 'unreadable': return { state: 'unreadable', ok: false, tone: 'bad', tag: t('Can’t be read'), line: why || t('The store’s entry for it couldn’t be read.'), todo: t('Check again later, or tell whoever runs that store.') }
    default: return { state: it.fit.ok ? 'works' : 'nothing-yet', ok: it.fit.ok, tone: it.fit.ok ? 'ok' : 'off', tag: it.fit.ok ? t('Works on this server') : t('Not available'), line: why || (it.fit.ok ? t('Works on this server.') : t('It can’t be installed on this server.')) }
  }
}

// What a module may do, in a few words each, for its card. Its page has the whole sentence for each.
const PERMISSIONS: Record<string, () => string> = {
  network: () => t('the internet'), programs: () => t('start programs'), devices: () => t('use devices such as tuners and controllers'), data: () => t('keep its own files'),
  gpu: () => t('use the graphics card'),
}
export const permissionWord = (p: string) => PERMISSIONS[p]?.() || p.replace(/[-_]+/g, ' ')
export function permissionsLine(perms?: string[]): string {
  if (!perms?.length) return t('Asks for nothing extra')
  return t('Asks for: {list}', { list: perms.map(permissionWord).join(', ') })
}
