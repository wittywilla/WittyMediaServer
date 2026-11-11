// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useMemo, useState, type ComponentType } from 'react'
import { api, ApiError, type Item } from '../../api'
import { Spinner } from '../../components/common'
import { ModuleMissing } from '../../components/ModuleMissing'
import { reportClient } from '../../errorReport'
import { IconGamepad } from '../../icons'
import { bundledModule } from '../../moduleBundles'
import { EmptyState } from '../../ui'
import type { ControllerData, ControlsData, EmulatorHost, EmulatorSession } from '@wittywilla/wittyplay'
import { emulatorsApi, sessionHost } from './emulatorsApi'
import './emulators.css'

// A game played with a standalone emulator: the page that starts it and holds its player. The
// player is WittyPlay's, from the same bundle as the one for a core's game, loaded when a game
// is opened. The game's own page sends people here (EmulatorChoices), as /game/{id}?emulator=.

type PlayerProps = { host: EmulatorHost; session: EmulatorSession; title: string; onExit: () => void }

// Any WittyPlay bundle is accepted, so one from before standalone emulators isn't taken for a
// broken one and reported: it just has no player for this, and the page says so.
const Player = lazy(async () => {
  const m = await bundledModule<Record<string, unknown>>('wittyplay', (x) => !!x.default)
  const found = m?.EmulatorPlayer as ComponentType<PlayerProps> | undefined
  if (found) return { default: found }
  const Missing = ({ onExit }: PlayerProps) => (m
    ? <div className="emu-start"><EmptyState icon={<IconGamepad />} title="WittyPlay needs updating" text="This server’s WittyPlay is from before standalone emulators. An administrator can update it in Settings, under Modules." action={<button type="button" className="btn" onClick={onExit}>Back</button>} /></div>
    : <ModuleMissing ids={['wittyplay']} onReady={() => window.location.reload()} />)
  return { default: Missing }
})

// Starting a game takes a container, so one press starts one game: a page drawn twice (as React
// does while developing) shares the first request, and a game that arrives after its page has
// gone is stopped again rather than left running with nobody playing it.
const starting = new Map<string, { session: Promise<EmulatorSession>; wanted: number }>()
function start(itemId: number, emulator: string) {
  const key = `${itemId}:${emulator}`
  let s = starting.get(key)
  if (!s) {
    const made = { session: emulatorsApi.play(itemId, emulator), wanted: 0 }
    s = made
    starting.set(key, made)
    made.session.then((session) => {
      starting.delete(key)
      if (made.wanted <= 0) void sessionHost(session).stop().catch(() => {})
    }, () => { starting.delete(key) })
  }
  const mine = s
  mine.wanted++
  return { session: mine.session, release: () => { mine.wanted-- } }
}

export default function EmulatorPlay({ it, emulator, onClose }: { it: Item; emulator: string; onClose: () => void }) {
  const [session, setSession] = useState<EmulatorSession | null>(null)
  const [error, setError] = useState<{ text: string; again: boolean } | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let live = true
    setError(null)
    const s = start(it.id, emulator)
    s.session.then((got) => { if (live) setSession(got) }, (e) => {
      if (!live) return
      // Busy is worth trying again in a while; anything else the server refused stays refused.
      const status = e instanceof ApiError ? e.status : 0
      setError({ text: e?.message || 'The server didn’t say why.', again: status === 503 || status === 0 || status >= 500 })
    })
    return () => { live = false; s.release() }
  }, [it.id, emulator, attempt])

  const host = useMemo<EmulatorHost | null>(() => session && sessionHost(session, {
    // The console's own controller, as for a core's game. The player has a usual one without it.
    controls: (device) => api.get<ControlsData>(`/consoles/${encodeURIComponent(it.console || '')}/controls?device=${encodeURIComponent(device)}&game=${it.id}`),
    controllers: () => api.get<ControllerData>('/game-controllers'),
    report: (level, message) => reportClient({ level, kind: 'game', message, module: 'wittyplay' }),
  }), [session, it.id, it.console])

  if (error) {
    return (
      <div className="emu-start" role="alert">
        <EmptyState icon={<IconGamepad />} title={`${it.title} didn’t start`} text={error.text}
          action={<div className="emu-start-actions">
            {error.again && <button type="button" className="btn primary" onClick={() => setAttempt((n) => n + 1)}>Try again</button>}
            <button type="button" className={'btn' + (error.again ? '' : ' primary')} autoFocus={!error.again} onClick={onClose}>Back</button>
          </div>} />
      </div>
    )
  }
  if (!session || !host) {
    return (
      <div className="emu-start" role="status">
        <EmptyState icon={<IconGamepad />} title={`Starting ${it.title}…`} text="The emulator is starting on the server. The first start can take a little while."
          action={<button type="button" className="btn" onClick={onClose}>Cancel</button>} />
      </div>
    )
  }
  return <Suspense fallback={<Spinner />}><Player host={host} session={session} title={it.title} onExit={onClose} /></Suspense>
}
