// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Item } from '../../api'
import { IconPlay, IconTune } from '../../icons'
import { useGameLinks } from './data'
import { emulatorsApi } from './emulatorsApi'
import type { GameEmulator } from './emulatorsLogic'
import './emulators.css'

// Most people never open these, so nobody downloads them until they do.
const EmulatorSettingsDialog = lazy(() => import('./EmulatorSettings'))

// On a game's page, under Play: the standalone emulators that could play this game, where the
// server has any. Each can start the game, or says why it can't right now, and has its own
// settings: someone's own for all their games, or for this one.
//
// Most servers have none, and then this is nothing at all: asked for once the page is up, and
// silent if the answer is empty or doesn't come.
export function EmulatorChoices({ it }: { it: Item }) {
  const links = useGameLinks()
  const [list, setList] = useState<GameEmulator[]>([])
  const [settings, setSettings] = useState<GameEmulator | null>(null)
  useEffect(() => {
    let live = true
    setList([])
    emulatorsApi.forGame(it.id).then((l) => { if (live) setList(Array.isArray(l) ? l : []) }).catch(() => {})
    return () => { live = false }
  }, [it.id])
  if (!list.length) return null
  return (
    <div className="gm-emus" role="group" aria-label="Other ways to play this game">
      {list.map((e) => {
        const whyId = `gm-emu-why-${e.id}`
        return (
          <div key={e.id} className="gm-emu">
            {e.ready
              ? <Link className="btn" to={links.play(it.id, 'emulator=' + encodeURIComponent(e.id))}><IconPlay size={16} /> Play with {e.name}</Link>
              // Not something to press yet, but still something to land on and hear the reason for.
              : <button type="button" className="btn" aria-disabled="true" aria-describedby={e.why ? whyId : undefined} onClick={(ev) => ev.preventDefault()}><IconPlay size={16} /> Play with {e.name}</button>}
            <button type="button" className="btn ghost" onClick={() => setSettings(e)} aria-label={`${e.name} settings`}><IconTune size={16} /> Emulator settings</button>
            {e.experimental && <span className="gm-pill">Experimental</span>}
            {!e.ready && e.why && <p className="gm-emu-why" id={whyId}>{e.why}</p>}
          </div>
        )
      })}
      {settings && <Suspense fallback={null}><EmulatorSettingsDialog emulator={{ id: settings.id, name: settings.name }} game={{ id: it.id, title: it.title }} onClose={() => setSettings(null)} /></Suspense>}
    </div>
  )
}
