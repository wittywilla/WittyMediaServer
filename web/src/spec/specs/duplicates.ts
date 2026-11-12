// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Settings, Channels and tuners, Duplicate channels (canvas "tuner"): the same channel listed twice, each group made into
// one. Row ids keep:<group>:<channel>; the one kept is `on`. Chip ids merge:<group>.
export interface DupeChannel { id: number; number: string; name: string; tuner: string; health?: string }
export interface DupeGroup { reason: string; suggested: number; channels: DupeChannel[] }
const REASON: Record<string, string> = { 'same guide id': 'Same guide id', 'same name': 'Same name' }

export function duplicatesSpec(groups: DupeGroup[] | null, main: Record<number, number>, busy: number, err = ''): SpecPageData {
  const sections: SpecSection[] = []
  if (err) sections.push(secN('', err, true))
  else if (!groups) sections.push(secN('', 'Looking…'))
  else if (!groups.length) sections.push(secN('', 'No duplicates: every channel is listed once.'))
  else {
    groups.forEach((g, i) => {
      const keep = main[i] ?? g.suggested
      sections.push(secR(`${g.channels[0]?.name ?? 'Channel'} · ${REASON[g.reason] || g.reason}`, g.channels.map((c) => ({
        id: `keep:${i}:${c.id}`, title: `${c.number} ${c.name}`, on: keep === c.id, value: keep === c.id ? 'Keep in the guide' : undefined,
        sub: `${c.tuner}${c.health === 'failing' ? ' · failing' : c.health === 'ok' ? ' · opens' : ''}`,
      }))))
      sections.push(secP('', [{ ...Cc(busy === i ? 'Making into one…' : 'Make into one', true), id: 'merge:' + i, disabled: busy === i }]))
    })
  }
  return mk('tuner', 'Settings · Channels and tuners', 'Duplicate channels', 'The same channel from more than one tuner or playlist. Make each into one: the one you keep stays in the guide, and the others become its backups, tried in turn when it fails to open.', sections)
}
