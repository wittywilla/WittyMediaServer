// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { GameConsole } from '../../api'
import { useMemo } from 'react'
import SpecPage from '../../spec/SpecPage'
import { coresSpec } from '../../spec/specs/consoleCores'
import { drawsWords, platformName, type CoreSupport, type Draws, type PlatformSupport } from './platforms'

// A console's cores on its collection page: how each draws on this server, and which other server platforms it
// runs on (docs/GAMING-API.md, "Which platforms a core runs on"). The words come from platforms.tsx, as on a
// game's page and in the core choices. Every device plays what the server can run, so that's said once, never
// per device. A server that doesn't send coreSupport shows none of this.
type WithSupport = GameConsole & { coreSupport?: Record<string, CoreSupport> }

const hereWords = (h: PlatformSupport) =>
  !h.build ? h.note || 'not published for this server.'
    : h.draws === 'none' ? h.note || 'can’t draw.'
      : `drawn ${drawsWords[h.draws]}.${h.note && h.draws !== 'cpu' ? ` ${h.note}` : ''}`

const and = (l: string[]) => (l.length < 2 ? l.join('') : `${l.slice(0, -1).join(', ')} and ${l[l.length - 1]}`)

// "Runs on Windows and Linux, drawn on a graphics card; macOS (Apple silicon), drawn by the processor, slower.
// Not on FreeBSD." Platforms that draw the same way go together.
function runsOnLine(s: CoreSupport) {
  const groups = new Map<Draws, string[]>()
  for (const p of s.platforms) {
    if (!p.build || p.draws === 'none') continue
    const names = groups.get(p.draws) || []
    if (!names.includes(platformName(p))) names.push(platformName(p))
    groups.set(p.draws, names)
  }
  const not = s.platforms.filter((p) => !p.build || p.draws === 'none').map(platformName)
  const runs = [...groups].map(([d, names]) => `${and(names)}, drawn ${drawsWords[d]}`).join('; ')
  return `${runs ? `Runs on ${runs}.` : 'No server platform yet.'}${not.length ? ` Not on ${and(not)}.` : ''}`
}

export function ConsoleCores({ c }: { c: GameConsole }) {
  const support = (c as WithSupport).coreSupport
  const page = useMemo(() => {
    if (!support || Object.keys(support).length === 0) return null
    const names = new Map([...c.cores, ...(c.customCores || [])].map((x) => [x.id, x.name]))
    return coresSpec(c.short, Object.entries(support).map(([id, s]) => ({ id, name: names.get(id) || id, installed: c.installed.some((x) => x.id === id), here: hereWords(s.here), runs: runsOnLine(s) })))
  }, [c, support])
  if (!page) return null
  return (
    <section className="gm-cores gm-section" aria-labelledby="gm-cores-h">
      <h2 id="gm-cores-h">Where {c.short} games run</h2>
      <SpecPage bare page={page} />
    </section>
  )
}
