// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { GameConsole } from '../../api'
import { LoadError, Spinner } from '../../components/common'
import { IconCheck, IconInfo, IconRefresh } from '../../icons'
import { useApp } from '../../state'
import { getConsoles } from '../games/consoles'
import { drawsWords } from '../games/platforms'
import { PageHead, Section } from './ui'
import '../games/games-ui.css'

// What this server can play, console by console, and how: a core and how it draws here, a
// standalone emulator, or what's missing and where to set it up. Every device plays whatever
// the server can, since the server runs each game and streams it, so this is the one answer.

type Standing = 'ready' | 'setup' | 'not-here'

function standing(c: GameConsole): Standing {
  if (c.ready) return 'ready'
  if (c.reason === 'gpu' || c.reason === 'unavailable') return 'not-here'
  return 'setup'
}

const HEADINGS: Record<Standing, { title: string; desc: string }> = {
  ready: { title: 'Plays here', desc: 'Every device can play these consoles’ games: this server runs them and streams them.' },
  setup: { title: 'Plays once set up', desc: 'This server can play these after an administrator installs a core or an emulator, or adds a BIOS file of your own.' },
  'not-here': { title: 'Not on this server', desc: 'Something this server lacks, said for each. The games stay listed.' },
}

function how(c: GameConsole): string {
  if (c.standalone) {
    const e = c.standaloneEmulators?.find((x) => x.id === c.standalone)
    return `In ${e?.name || 'a standalone emulator'}${e?.version ? ` ${e.version}` : ''}, on this server.`
  }
  // The core Play would start on, not merely the first installed: on a Mac a PlayStation 2 game
  // plays on Play!, though LRPS2 comes first.
  const core = c.installed.find((x) => x.id === c.playsOn) || c.installed[0]
  if (!core) return c.message || ''
  const here = c.coreSupport?.[core.id]?.here
  const why = c.softwareWhy === 'card-fault'
    ? 'drawn by the processor, as its emulator doesn’t draw right on this server’s graphics card'
    : 'drawn by the processor, as there’s no graphics card games can use'
  const draws = c.software ? why : here ? drawsWords[here.draws] : ''
  const more = (c.standaloneEmulators || []).filter((e) => e.ready).map((e) => e.name)
  return `On ${core.name}${draws ? `, ${draws}` : ''}.${more.length ? ` Also in ${more.join(' or ')}.` : ''}`
}

export default function GamesSupport() {
  const { user } = useApp()
  const [list, setList] = useState<GameConsole[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const load = useCallback((fresh: boolean) => {
    setBusy(true)
    return getConsoles(fresh).then((l) => { setList(l); setFailed(false) }).catch(() => setFailed(true)).finally(() => setBusy(false))
  }, [])
  useEffect(() => { void load(false) }, [load])
  if (failed && !list) return <LoadError what="This page" onRetry={() => { void load(true) }} />
  if (!list) return <Spinner />
  const groups: Standing[] = ['ready', 'setup', 'not-here']
  return (
    <div>
      <PageHead title="What this server plays" desc="Each console, whether its games play on this server, and how. It’s the server that matters, not your device: games run here and stream to whatever you’re holding."
        actions={<button type="button" className="btn sm" disabled={busy} onClick={() => { void load(true) }}><IconRefresh size={16} /> {busy ? 'Checking…' : 'Check again'}</button>} />
      {groups.map((g) => {
        const these = list.filter((c) => standing(c) === g)
        if (!these.length) return null
        return (
          <Section key={g} title={HEADINGS[g].title} desc={HEADINGS[g].desc}>
            <ul className="gm-checks">
              {these.map((c) => (
                <li key={c.id} className={g === 'ready' ? 'ok' : g === 'setup' ? 'info' : 'no'}>
                  {g === 'ready' ? <IconCheck size={16} /> : <IconInfo size={16} />}
                  <span>
                    <b>{c.name}{c.totalGames ? <small className="muted"> · {c.totalGames} {c.totalGames === 1 ? 'game' : 'games'}</small> : null}</b>
                    <small>{g === 'ready' ? how(c) : c.message || ''}{c.notice && g === 'ready' ? ` ${c.notice}` : ''}</small>
                    {c.biosMissing && <small>Needs a BIOS of your own: {c.biosMissing}.</small>}
                  </span>
                </li>
              ))}
            </ul>
          </Section>
        )
      })}
      {user?.isAdmin && (
        <p className="muted">
          Set up consoles in <Link to="/settings/gaming">Consoles &amp; cores</Link>, and the emulators that are whole programs of their own in <Link to="/settings/emulators">Standalone emulators</Link>.
        </p>
      )}
    </div>
  )
}
