// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The parts of the catalogue (GET /sdui/catalog) that are data for the player: WittyRemote's keys, groups, pages and
// which keys each kind of disc dims, and the music player's controls and visualizer presets. Read once per sign-in.
import { api } from '../api'
import { setRemoteCatalogue, type RemoteCatalogue } from '../player/remoteKeys'

export type MusicCatalogue = {
  miniPlayer: string[]
  visualizer: { presets: { id: string; name: string }[]; controls: { id: string; type: string; min?: number; max?: number; default?: number | string | boolean; options?: string[] }[]; defaultOn: boolean }
  karaoke: { controls: { id: string; type: string; min?: number; max?: number; default?: number | boolean; note?: string; unit?: string }[] }
}

let music: MusicCatalogue | null = null
const listeners = new Set<() => void>()
export const musicCatalogue = () => music
export const subscribeCatalogue = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export async function loadCatalogue() {
  try {
    const c = await api.get<{ remote?: RemoteCatalogue; music?: MusicCatalogue }>('/sdui/catalog')
    if (c.remote) setRemoteCatalogue(c.remote)
    music = c.music || null
    listeners.forEach((l) => l())
  } catch { /* the keys then follow the player's own state alone */ }
}
