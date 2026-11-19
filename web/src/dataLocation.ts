// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where the server keeps its settings and data: what first-time setup asks, and Settings, Data folder
// changes. The server moves the folder while it starts again (internal/datahome).
import { api } from './api'

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
  return `${n} bytes`
}

// The words for a sync program's warning: a database that's open while it's synced can be damaged.
export const syncedWords = (who: string) =>
  `${who} keeps this folder in step with the cloud. The server's database is open all the time it runs, and a sync program copying it, or another computer changing it, can damage it. Choose a folder on this computer that isn't synced.`

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
    const t = setTimeout(() => ctl.abort(), 4000)
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
        const s: MoveState = { phase: 'failed', dataDir: p.dataDir, error: p.last && !p.last.ok ? p.last.error : 'The server started again in the old folder.' }
        onState(s)
        return s
      }
    } catch {
      wentAway = true // stopped, or starting
    } finally {
      clearTimeout(t)
    }
  }
  const s: MoveState = { phase: 'failed', error: 'The server hasn’t come back. Look at it on the computer it runs on.' }
  onState(s)
  return s
}
