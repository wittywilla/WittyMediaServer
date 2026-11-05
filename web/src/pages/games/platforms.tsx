import { Link } from 'react-router-dom'
import type { GameConsole } from '../../api'
import { IconCheck, IconInfo } from '../../icons'
import type { GameCheck } from './data'

// Where each core runs: the server platforms it has a build for, how it draws there, and how fast,
// as GET /games/consoles says in coreSupport (docs/GAMING-API.md). The server runs every game and
// streams it, so every device plays whatever the server can run: that's said once, never per
// device. A server that doesn't send coreSupport yet shows none of this.

export type CoreDraws = 'gpu' | 'cpu' | 'cpu-fallback' | 'none'
export type CorePlace = { build?: boolean; draws: CoreDraws; api?: string; note?: string }
export type CorePlatform = CorePlace & { os: string; arch: string }
export type CoreSupport = { here?: CorePlace; platforms?: CorePlatform[] }
export type WithSupport = GameConsole & { coreSupport?: Record<string, CoreSupport> }
// What /games/{id}/check adds when a game can't play on this server.
export type CheckWhere = GameCheck & { runsOn?: CorePlatform[]; alternative?: { id: string; name: string } }

const OS: Record<string, string> = { windows: 'Windows', linux: 'Linux', macos: 'Mac', freebsd: 'FreeBSD' }
const API: Record<string, string> = { metal: 'Metal', opengl: 'OpenGL', vulkan: 'Vulkan' }

// A platform as people say it: a Mac with Apple silicon or an Intel Mac, Linux on Arm.
export function platformName(os: string, arch: string) {
  if (os === 'macos') return arch === 'x64' ? 'Intel Mac' : 'Mac with Apple silicon'
  const name = OS[os] || os
  return arch === 'arm64' ? `${name} on Arm` : arch === 'arm32' ? `${name} on 32-bit Arm` : name
}

export function drawsWords(p: CorePlace) {
  if (p.draws === 'gpu') return `the graphics card${p.api && API[p.api] ? ` (${API[p.api]})` : ''}`
  if (p.draws === 'cpu') return 'the processor'
  if (p.draws === 'cpu-fallback') return 'the processor, slower than a graphics card'
  return 'nothing here: it can’t draw'
}

// "Windows and Linux (graphics card), Mac with Apple silicon (processor, slower)", grouping
// platforms that draw the same way.
export function runsOnWords(list: CorePlatform[]) {
  const usable = list.filter((p) => p.build !== false && p.draws !== 'none')
  const groups = new Map<string, string[]>()
  for (const p of usable) {
    const how = p.draws === 'gpu' ? 'graphics card' : p.draws === 'cpu' ? 'processor' : 'processor, slower'
    const names = groups.get(how) || []
    const n = platformName(p.os, p.arch)
    if (!names.includes(n)) names.push(n)
    groups.set(how, names)
  }
  return [...groups].map(([how, names]) => `${joinAnd(names)} (${how})`).join('; ')
}

const joinAnd = (l: string[]) => l.length < 2 ? l.join('') : `${l.slice(0, -1).join(', ')} and ${l[l.length - 1]}`

const EVERY_DEVICE = 'Every device plays it: the server runs the game and streams it.'

// One core's line on a game's page: how it draws here, where else it runs.
export function CoreWhere({ c, coreId }: { c?: GameConsole; coreId: string }) {
  const s = (c as WithSupport | undefined)?.coreSupport?.[coreId]
  if (!s) return null
  return (
    <small className="gm-core-where">
      {s.here && <>Here it draws on {drawsWords(s.here)}{s.here.note ? `: ${s.here.note}` : ''}. </>}
      {s.platforms && s.platforms.length > 0 && <>Runs on {runsOnWords(s.platforms)}. </>}
      {EVERY_DEVICE}
    </small>
  )
}

// A console's cores and where each runs, on its collection page.
export function ConsoleCores({ c }: { c: GameConsole }) {
  const support = (c as WithSupport).coreSupport
  if (!support || Object.keys(support).length === 0) return null
  const names = new Map(c.cores.map((x) => [x.id, x.name]))
  return (
    <section className="gm-cores gm-section" aria-labelledby="gm-cores-h">
      <h2 id="gm-cores-h">Where {c.short} games run</h2>
      <ul>
        {Object.entries(support).map(([id, s]) => {
          const installed = c.installed.some((x) => x.id === id)
          return (
            <li key={id}>
              <b>{names.get(id) || id}{installed && <span className="gm-core-on"><IconCheck size={14} /> On this server</span>}</b>
              {s.here && <small>Here: {s.here.build === false ? 'no download for this server' : drawsWords(s.here)}{s.here.note ? `. ${s.here.note}` : ''}</small>}
              {s.platforms && s.platforms.length > 0 && <small>Runs on {runsOnWords(s.platforms)}</small>}
            </li>
          )
        })}
      </ul>
      <p className="muted">{EVERY_DEVICE}</p>
    </section>
  )
}

// A game this server can't play, saying which servers could, and the core that would, if any.
export function NotOnThisServer({ check, isAdmin }: { check: CheckWhere | null; isAdmin: boolean }) {
  if (!check || check.ready || !check.runsOn || check.runsOn.length === 0) return null
  return (
    <div className="gm-elsewhere" role="status">
      <IconInfo />
      <div>
        <strong>Not on this server</strong>
        <p>This game plays on a server running {runsOnWords(check.runsOn)}. {EVERY_DEVICE}</p>
        {check.alternative && (isAdmin
          ? <p><Link to="/settings/gaming">Install {check.alternative.name}</Link>, which plays it here.</p>
          : <p>{check.alternative.name} would play it here: ask whoever runs this server.</p>)}
      </div>
    </div>
  )
}
