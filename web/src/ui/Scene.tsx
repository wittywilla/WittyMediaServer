// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { currentLook, sceneLayers, subscribeScene } from '../theme'
import { serverBase } from '../api'
import { safeColour, type SceneLayer } from '../themeOverrides'

// The theme's scene (docs/THEMES.md, "A scene"): ordered layers drawn behind the page and under the menu, bottom first,
// positions in percent of the window: a gradient (the sky), a radial light (the light sweep), shapes (bubbles, bokeh,
// a hill) and a picture of the theme. Drawn with gradients and shapes and no blur, so it costs a paint and no more.
// On modest hardware and with reduced motion or transparency only the first layer is drawn (the sky), as the theme
// document says.
const num = (v: unknown, fallback: number, lo = -200, hi = 400) => (typeof v === 'number' && isFinite(v) ? Math.min(hi, Math.max(lo, v)) : fallback)
const stops = (l: SceneLayer) => (l.stops || []).map((s) => ({ at: num(s.at, 0, 0, 100), color: safeColour(s.color) })).filter((s) => s.color)
const gradient = (angle: number, list: { at: number; color: string }[]) => `linear-gradient(${angle}deg, ${list.map((s) => `${s.color} ${s.at}%`).join(', ')})`

function modest(): boolean {
  if (typeof window === 'undefined') return false
  const n = navigator as Navigator & { deviceMemory?: number }
  const reduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || window.matchMedia?.('(prefers-reduced-transparency: reduce)').matches
  return !!reduced || (n.deviceMemory !== undefined && n.deviceMemory <= 2) || (n.hardwareConcurrency !== undefined && n.hardwareConcurrency <= 2)
}

function Layer({ l, i }: { l: SceneLayer; i: number }) {
  switch (l.type) {
    case 'gradient': {
      const s = stops(l)
      return s.length ? <div className="scene-layer scene-sky" style={{ backgroundImage: gradient(num(l.angle, 180, 0, 360), s) }} /> : null
    }
    case 'radial': {
      const c = safeColour(l.color)
      if (!c) return null
      const x = num(l.x, 50, -50, 150), y = num(l.y, 50, -50, 150)
      return <div className="scene-layer scene-light" style={{ backgroundImage: `radial-gradient(ellipse ${num(l.w, 50, 1, 200)}% ${num(l.h, 50, 1, 200)}% at ${x}% ${y}%, ${c} 0%, transparent ${num(l.fade, 100, 1, 100)}%)`, animationDelay: -i * 3 + 's' }} />
    }
    case 'shape': {
      if (l.kind === 'hill') {
        const s = stops(l), c = safeColour(l.color)
        const size = num(l.size, 60, 5, 150)
        const fill = s.length ? gradient(180, s) : c
        if (!fill) return null
        return <div className="scene-layer scene-hill" style={{ left: num(l.x, 50) + '%', top: num(l.y, 100) + '%', width: size * 2 + '%', height: size * 0.4 + 'vh', background: fill }} />
      }
      const c = safeColour(l.color)
      if (!c) return null
      return <div className="scene-layer scene-bubble" style={{ left: num(l.x, 50) + '%', top: num(l.y, 50) + '%', width: num(l.size, 8, 1, 100) + 'vmin', height: num(l.size, 8, 1, 100) + 'vmin', '--c': c, animationDelay: -i * 1.7 + 's' } as React.CSSProperties} />
    }
    case 'image': {
      const file = typeof l.file === 'string' && /^[\w][\w.-]*$/.test(l.file) ? l.file : ''
      const theme = currentLook()?.theme
      if (!file || !theme || !/^[\w-]+$/.test(theme)) return null
      return <img className="scene-layer scene-image" src={`${serverBase()}/api/themes/${theme}/${file}`} alt="" decoding="async" />
    }
    default: return null
  }
}

export function Scene() {
  const layers = useSyncExternalStore(subscribeScene, sceneLayers)
  if (!layers.length) return null
  const list = modest() ? layers.slice(0, 1) : layers
  return <div className="scene" aria-hidden="true">{list.map((l, i) => <Layer key={i} l={l} i={i} />)}</div>
}
