// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// The administrator's "Fix this game" panel on a game's page (canvas page "fixgame"), from GET /games/<id>/versions
// and the game's details. Ids: open.<item>, split.<item>, play.<archive entry>, console, notgame.
export interface FixInput {
  how: string; sure?: number; why?: string
  discs?: string[]
  others: { id: number; label: string }[]
  inside?: { entry: string; label: string; altered?: boolean; current?: boolean }[]
  choosing: boolean; busy: boolean
}

export function fixGameSpec(f: FixInput): SpecPageData {
  const s: SpecSection[] = []
  if (f.how) s.push(secN('', `${f.how}${f.sure ? `, ${f.sure}% sure` : ''}.${f.why ? ' ' + f.why : ''}`))
  if (f.discs && f.discs.length > 1) s.push(secN('', `${f.discs.length} discs. Change discs from the game’s menu while it’s running. ${f.discs.join(', ')}`))
  if (f.others.length) {
    s.push(secR('Other versions of this game', f.others.map((x) => ({ ...R(x.label || 'Another version'), id: `open.${x.id}`, to: `/item/${x.id}` }))))
    s.push(secP('', f.others.map((x) => ({ ...Cc(`Make ${x.label || 'this version'} a game of its own`), id: `split.${x.id}`, disabled: f.busy }))))
  }
  if (f.inside) {
    s.push(secR('Which version in this archive to play', f.inside.map((x) => ({
      ...R(`${x.label}${x.altered ? ' (altered)' : ''}`, x.current ? 'Plays now' : 'Play this one', !!x.current), id: `play.${x.entry}`, disabled: !!x.current || f.busy,
    }))))
  }
  s.push(secP('', [{ ...Cc('Wrong console', f.choosing), id: 'console', disabled: f.busy }, { ...Cc('It isn’t a game'), id: 'notgame', disabled: f.busy }]))
  return mk('fixgame', 'Game · Fix this game', 'Fix this game', 'Match, files and settings.', s)
}

// The same versions for anyone who is not an administrator: each is a game of its own, so all there is to do is open it.
export function versionLinksSpec(others: { id: number; label: string }[]): SpecPageData {
  return mk('fixgame', 'Game · Other versions', 'Other versions of this game', '', [
    secR('', others.map((x) => ({ ...R(x.label || 'Another version'), id: `open.${x.id}`, to: `/item/${x.id}` }))),
  ])
}
