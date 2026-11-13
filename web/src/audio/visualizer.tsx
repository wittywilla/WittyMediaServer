// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense } from 'react'
import { api } from '../api'
import type { Place, VisualizerHost } from '@wittywilla/wittyvisualizer'
import { bundledModule } from '../moduleBundles'
import { useApp } from '../state'
import { soundGraph } from './sound'

// WittyVisualizer in this app. It costs nothing until someone opens it: this small file is
// all that's in the main script, and the plugin (a newer bundle if the server has one, or
// the copy in the app) arrives with the first press.

// Kept per person as the module's own profile settings (the keys of its sdui.json: effect, speedPct,
// colour ...), through the module settings routes, so the Settings page and the visualizer's own
// panel agree. Whether the notice about flashing was read, and whether it shows with the player,
// are the player's own and kept with the person's other interface choices.
let saving = Promise.resolve()
type Player = { on?: boolean; notice?: boolean }
const SETTINGS = '/sdui/modules/wittyvisualizer/settings'
export const visualizerHost: VisualizerHost = {
  loadSettings: () => Promise.all([
    api.get<{ values?: Record<string, unknown> }>(SETTINGS),
    api.get<{ visualizerPlayer?: Player }>('/me/ui'),
  ]).then(([m, ui]) => ({ ...(m.values ?? {}), on: ui.visualizerPlayer?.on === true, notice: ui.visualizerPlayer?.notice === true })),
  saveSettings: (settings) => {
    saving = saving.then(async () => {
      const p = await loadPlugin()
      if (!p) return
      const s = settings as Parameters<Plugin['moduleSettingValues']>[0] & Player
      await api.put(SETTINGS + '/personal', { values: p.moduleSettingValues(s) })
      const ui = await api.get<Record<string, unknown>>('/me/ui')
      await api.put('/me/ui', { ...ui, visualizerPlayer: { on: s.on === true, notice: s.notice === true } })
    }).catch(() => { /* the next change tries again */ })
  },
}

// Whether this person's saved choice is to show it with the full player.
export const visualizerWantedWithPlayer = () => visualizerHost.loadSettings().then((v) => {
  const s = v as { on?: boolean; notice?: boolean }
  return s.on === true && s.notice === true
}).catch(() => false)

// Whether they've read the notice about flashing, which comes before any of the shaders.
export const acceptedFlashNotice = () => visualizerHost.loadSettings().then((v) => (v as { notice?: boolean }).notice === true).catch(() => false)

type Plugin = typeof import('@wittywilla/wittyvisualizer')
let plugin: Plugin | null = null
const loadPlugin = async (): Promise<Plugin | null> => {
  if (plugin) return plugin
  const bundled = await bundledModule<Plugin>('wittyvisualizer', (m) => typeof m.VisualizerStage === 'function' && typeof m.currentSession === 'function')
  plugin = bundled
  return plugin
}
const Stage = lazy(async () => ({ default: (await loadPlugin())?.VisualizerStage ?? ((() => null) as unknown as Plugin['VisualizerStage']) }))

export type ShaderEffect = { id: string; name: string; motion: 'gentle' | 'lively' }

// The plugin's effects, for choosing between. It's a press away from being used, so the
// list arriving here is what brings the plugin in, a moment after the full player opens.
export const shaderEffects = async (): Promise<ShaderEffect[]> => {
  const p = await loadPlugin()
  // Once it's running the session knows them all, a person's own among them.
  return (p?.currentSession()?.effects ?? p?.BUILTIN_EFFECTS ?? []).map((e) => ({ id: e.id, name: e.name, motion: e.motion }))
}

// The running visualizer, to change its effect from the player's own controls.
export const shaderSession = () => plugin?.currentSession() ?? null

// The player has two elements that take turns, and the plugin listens to one. So it's given
// a stand-in that hands over a copy of whichever element is playing and keeps the copy up to
// date as they swap: the plugin's own check for a new track picks up each change. Where the
// sound has already been routed through the sound graph, that's where the copy comes from,
// as a browser with no copy to offer couldn't route the element a second time.
type Captureable = HTMLMediaElement & { captureStream?: () => MediaStream }
export function trackingMedia(el: () => HTMLMediaElement | null): () => HTMLMediaElement | null {
  const standIn = {
    crossOrigin: 'anonymous',
    captureStream() {
      const out = new MediaStream()
      let watched: HTMLMediaElement | null = null
      let copy: MediaStream | null = null
      let song = '' // the source the track in out was taken from
      const sync = () => {
        const now = el() as Captureable | null
        if (!now?.captureStream) return
        // Captured again for each song: after a skip the old capture's track can still say it is live
        // while carrying only silence.
        if (now !== watched || now.currentSrc !== song) { watched = now; song = now.currentSrc; copy = now.captureStream() }
        const track = copy?.getAudioTracks().find((x) => x.readyState === 'live')
        const have = out.getAudioTracks()[0]
        if (track && track !== have) {
          if (have) out.removeTrack(have)
          out.addTrack(track)
        }
      }
      sync()
      const t = setInterval(() => {
        const have = out.getAudioTracks()[0]
        // Ended with the same song still loaded on the element that is still playing: the plugin hung up.
        // Ended because the song changed, or because the next song started on the other deck (no gaps
        // between songs plays them on two elements in turn): carry on with the one now playing.
        if (have && have.readyState === 'ended' && el() === watched && watched?.currentSrc === song) { clearInterval(t); return }
        sync()
      }, 500)
      return out
    },
  } as unknown as HTMLMediaElement
  return () => {
    const now = el() as Captureable | null
    if (now?.captureStream) return standIn
    const g = soundGraph()
    if (now && g?.routed(now)) return { crossOrigin: 'anonymous', captureStream: () => g.outputCopy() } as unknown as HTMLMediaElement
    return now
  }
}

// A simple way to show it anywhere there's a media element: WittyCD's player uses this.
export function VisualizerView({ media, place, onPlace, onClose }: { media: () => HTMLMediaElement | null; place: Place; onPlace?: (p: Place) => void; onClose: () => void }) {
  const { user } = useApp()
  return (
    <Suspense fallback={null}>
      <Stage host={visualizerHost} owner={user?.id} media={media} place={place} onPlace={onPlace} onClose={onClose} />
    </Suspense>
  )
}
