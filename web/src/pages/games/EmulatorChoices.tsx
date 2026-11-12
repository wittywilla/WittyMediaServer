// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import SpecPage from '../../spec/SpecPage'
import { emulatorChoicesSpec } from '../../spec/specs/emulatorChoices'
import type { Item } from '../../api'
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
  const page = useMemo(() => emulatorChoicesSpec(list, (id) => links.play(it.id, 'emulator=' + encodeURIComponent(id))), [list, links, it.id])
  if (!list.length) return null
  const handled = new Set(list.map((e) => `settings.${e.id}`))
  const act = (id: string) => { const e = list.find((x) => id === `settings.${x.id}`); if (e) setSettings(e) }
  return (
    <div className="gm-emus" role="group" aria-label="Other ways to play this game">
      <SpecPage bare page={page} handled={handled} onAction={act} />
      {settings && <Suspense fallback={null}><EmulatorSettingsDialog emulator={{ id: settings.id, name: settings.name }} game={{ id: it.id, title: it.title }} onClose={() => setSettings(null)} /></Suspense>}
    </div>
  )
}
