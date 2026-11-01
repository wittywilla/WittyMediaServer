import { lazy, Suspense } from 'react'
import { api } from '../api'
import type { Place, VisualizerHost } from '@wittywilla/wittyvisualizer'
import { bundledModule } from '../moduleBundles'
import { useApp } from '../state'
import { soundGraph } from './sound'

// WittyVisualizer in this app. It costs nothing until someone opens it: this small file is
// all that's in the main script, and the plugin (a newer bundle if the server has one, or
// the copy in the app) arrives with the first press.

// Kept per person with their other interface choices, which the server stores whole, so
// what's there is read first and only this part changes.
//
// Effects changed by themselves every 30 seconds until now, whether anyone asked or not.
// They don't any more, so a timer saved from those days (and nobody chose that) is dropped
// once. From then on what's saved is someone's own choice.
let saving = Promise.resolve()
export const visualizerHost: VisualizerHost = {
  loadSettings: () => api.get<{ visualizer?: Record<string, unknown>; visualizerStays?: boolean }>('/me/ui').then((u) => {
    const v = u.visualizer ?? {}
    return u.visualizerStays ? v : { ...v, cycleSec: 0 }
  }),
  saveSettings: (settings) => {
    saving = saving.then(async () => {
      const ui = await api.get<Record<string, unknown>>('/me/ui')
      await api.put('/me/ui', { ...ui, visualizer: settings, visualizerStays: true })
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
const loadPlugin = async (): Promise<Plugin> => {
  if (plugin) return plugin
  const bundled = await bundledModule<Plugin>('wittyvisualizer', (m) => typeof m.VisualizerStage === 'function' && typeof m.currentSession === 'function')
  plugin = bundled ?? await import('@wittywilla/wittyvisualizer')
  return plugin
}
const Stage = lazy(async () => ({ default: (await loadPlugin()).VisualizerStage }))

export type ShaderEffect = { id: string; name: string; motion: 'gentle' | 'lively' }

// The plugin's effects, for choosing between. It's a press away from being used, so the
// list arriving here is what brings the plugin in, a moment after the full player opens.
export const shaderEffects = async (): Promise<ShaderEffect[]> => (await loadPlugin()).BUILTIN_EFFECTS.map((e) => ({ id: e.id, name: e.name, motion: e.motion }))

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
      const sync = () => {
        const now = el() as Captureable | null
        if (!now?.captureStream) return
        if (now !== watched) { watched = now; copy = now.captureStream() }
        const track = copy?.getAudioTracks()[0]
        const have = out.getAudioTracks()[0]
        if (track && track !== have) {
          if (have) out.removeTrack(have)
          out.addTrack(track)
        }
      }
      sync()
      const t = setInterval(() => {
        const have = out.getAudioTracks()[0]
        if (have && have.readyState === 'ended') { clearInterval(t); return } // the plugin hung up
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
