// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where the server keeps its settings and data: what first-time setup asks, and Settings, Data folder
// changes. The server moves the folder while it starts again (internal/datahome).
import { api } from './api'
import { t, tk, tn } from './i18n'

export type DataChoice = 'documents' | 'install' | 'custom'

export type DataOption = { choice: 'documents' | 'install'; path: string; available: boolean; why?: string; free: number; synced?: string; current?: boolean }

export type MoveResult = { ok: boolean; from: string; to: string; error?: string; files: number; bytes: number; at: string }

export type DataLocation = {
  dataDir: string
  from: string
  movable: boolean
  why?: string
  ask: boolean
  options: DataOption[]
  local: string
  synced?: string
  files: number
  bytes: number
  sep: string
  canRestart: boolean
  apart?: string[]
  pending?: { to: string; choice: string; keepOld: boolean; at: string }
  last?: MoveResult
  old?: { from: string; entries: string[]; kept: boolean; at: string }
}

export type PathCheck = { path: string; problem: string; free?: number; synced?: string; current?: boolean }

export const loadDataLocation = () => api.get<DataLocation>('/data-location')
export const checkDataLocation = (choice: DataChoice, path = '') => api.post<PathCheck>('/data-location/check', { choice, path })

export function size(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`
  if (n >= 1 << 20) return `${Math.round(n / (1 << 20))} MB`
  if (n >= 1 << 10) return `${Math.round(n / (1 << 10))} KB`
  return tn('{count} byte', '{count} bytes', n)
}

// The words for a sync program's warning: a database that's open while it's synced can be damaged.
export const syncedWords = (who: string) =>
  t("{program} keeps this folder in step with the cloud. The server's database is open all the time it runs, and a sync program copying it, or another computer changing it, can damage it. Choose a folder on this computer that isn't synced.", { program: who })

// What the server says about places, exactly as it sends them (internal/datahome, internal/server/api_datalocation.go),
// marked here so they're translated where shown with tx(). Messages it fills in with a path or a size stay in English.
export const SERVER_WORDS = [
  tk('This account has no Documents folder.'),
  tk("A Mac app can't keep files inside itself: changing what's in it breaks its signature, and the Mac would refuse to open it."),
  tk('This copy of WittyMediaServer is one read-only file, so nothing can be kept inside it.'),
  tk("The system's package manager looks after the install folder, and nothing else belongs in it."),
  tk("The data folder is set with -data where the server is started (the service or a shortcut), so it's changed there."),
  tk("The data folder is set by the WMS_DATA_DIR environment variable, so it's changed there."),
  tk("The data folder is set in the config file, wms.json, so it's changed there."),
  tk('In a container the data folder is the /config volume: change it where the container is set up.'),
  tk("This is the system service's data folder, which the package set up."),
  tk("The data folder's place isn't known."),
  tk('Choose a folder, not the top of a drive.'),
  tk('Your data is already in this folder.'),
  tk('That folder is inside the current data folder. Choose one outside it.'),
  tk('The current data folder is inside that folder. Choose an empty folder of its own.'),
  tk("That's the program's own folder. Choose the install folder option, or a folder outside it."),
  tk("There's a file with that name. Choose a folder."),
  tk('That folder already holds a WittyMediaServer data folder. Choose an empty folder, or a new one.'),
  tk('That folder has files in it. Choose an empty folder, or type a new one to make.'),
  tk("Type the folder's full path."),
  tk('Choose Documents, the install folder or a folder of your own.'),
  tk("This server can't restart itself, and the data moves while it starts. Stop it, move the folder, and start it with -data."),
  tk("there's no old copy"),
  tk('the old copy overlaps the data folder, so it was left alone'),
]

export type MoveState = { phase: 'asking' | 'restarting' | 'moving' | 'done' | 'failed'; done?: number; total?: number; error?: string; dataDir?: string }

// moveData asks the server to keep its data at the folder chosen, then waits while it starts again
// and copies it, saying how far it is. It ends when the server answers from the new folder, or
// when it's back in the old one, saying why.
export async function moveData(choice: DataChoice, path: string, onState: (s: MoveState) => void): Promise<MoveState> {
  onState({ phase: 'asking' })
  const r = await api.post<{ restarting: boolean; dataDir: string }>('/data-location', { choice, path })
  if (!r.restarting) {
    const s: MoveState = { phase: 'done', dataDir: r.dataDir }
    onState(s)
    return s
  }
  const target = r.dataDir
  const until = Date.now() + 60 * 60 * 1000
  let wentAway = false
  onState({ phase: 'restarting' })
  while (Date.now() < until) {
    await new Promise((ok) => setTimeout(ok, 800))
    const ctl = new AbortController()
    const timer = setTimeout(() => ctl.abort(), 4000)
    try {
      const p = await api.get<{ moving: boolean; done?: number; total?: number; dataDir?: string; last?: MoveResult }>('/data-location/progress', ctl.signal)
      if (p.moving) {
        wentAway = true
        onState({ phase: 'moving', done: p.done, total: p.total })
        continue
      }
      if (p.dataDir === target) {
        const s: MoveState = { phase: 'done', dataDir: p.dataDir }
        onState(s)
        return s
      }
      // Back, but in the old folder: the move didn't happen, and the server says why.
      if (wentAway) {
        const s: MoveState = { phase: 'failed', dataDir: p.dataDir, error: p.last && !p.last.ok ? p.last.error : t('The server started again in the old folder.') }
        onState(s)
        return s
      }
    } catch {
      wentAway = true // stopped, or starting
    } finally {
      clearTimeout(timer)
    }
  }
  const s: MoveState = { phase: 'failed', error: t('The server hasn’t come back. Look at it on the computer it runs on.') }
  onState(s)
  return s
}
