import { lazy, Suspense } from 'react'
import { api } from '../api'
import type { Place, VisualizerHost } from '@wittywilla/wittyvisualizer'
import { bundledModule } from '../moduleBundles'

// WittyVisualizer in this app. It costs nothing until someone opens it: this small file is
// all that's in the main script, and the plugin (a newer bundle if the server has one, or
// the copy in the app) arrives with the first press.

// Kept per person with their other interface choices, which the server stores whole, so
// what's there is read first and only this part changes.
let saving = Promise.resolve()
export const visualizerHost: VisualizerHost = {
  loadSettings: () => api.get<{ visualizer?: unknown }>('/me/ui').then((u) => u.visualizer ?? {}),
  saveSettings: (settings) => {
    saving = saving.then(async () => {
      const ui = await api.get<Record<string, unknown>>('/me/ui')
      await api.put('/me/ui', { ...ui, visualizer: settings })
    }).catch(() => { /* the next change tries again */ })
  },
}

// Whether this person's saved choice is to show it with the full player.
export const visualizerWantedWithPlayer = () => visualizerHost.loadSettings().then((v) => {
  const s = v as { on?: boolean; notice?: boolean }
  return s.on === true && s.notice === true
}).catch(() => false)

type Plugin = typeof import('@wittywilla/wittyvisualizer')
const load = async (): Promise<{ default: Plugin['VisualizerStage'] }> => {
  const bundled = await bundledModule<Plugin>('wittyvisualizer', (m) => typeof m.VisualizerStage === 'function')
  if (bundled) return { default: bundled.VisualizerStage }
  const own = await import('@wittywilla/wittyvisualizer')
  return { default: own.VisualizerStage }
}
const Stage = lazy(load)

// A simple way to show it anywhere there's a media element: WittyCD's player uses this.
export function VisualizerView({ media, place, onPlace, onClose }: { media: () => HTMLMediaElement | null; place: Place; onPlace?: (p: Place) => void; onClose: () => void }) {
  return (
    <Suspense fallback={null}>
      <Stage host={visualizerHost} media={media} place={place} onPlace={onPlace} onClose={onClose} />
    </Suspense>
  )
}
