// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef } from 'react'
import { CanvasVideo } from '../canvas/CanvasVideo'
import type { CanvasInfo } from '../canvas/rules'
import type { Look } from './look'
import { SoundListener } from './sound'
import './look.css'

// ArtView: a song's cover in the look chosen for it (look.ts), the same in the full player, the bar, karaoke and
// the Look panel's previews. With a Canvas video given, the video takes the cover's place, cropped to its shape.
// Everything that moves (a record turning, the drift, the lean, the pulse) moves only while the music plays, and
// not at all for someone who asked for less motion. The pulse listens to the sound only while it is on screen.

type Size = 'full' | 'bar' | 'tile' | 'kar'

export default function ArtView({ look, src, alt = '', caption, canvas, playing = false, size = 'full', media, fallback }: {
  look: Pick<Look, 'art' | 'pulse'>; src: string; alt?: string; caption?: string
  canvas?: { info: CanvasInfo | null; blocked: string } | null
  playing?: boolean; size?: Size
  media?: () => HTMLMediaElement | null // for the pulse
  fallback?: React.ReactNode // drawn when there's no cover
}) {
  const box = useRef<HTMLDivElement>(null)
  const style = look.art
  const video = !!canvas?.info?.available && !canvas.blocked
  usePulse(box, look.pulse && playing && size !== 'tile' && !!media, media)
  useTilt(box, style === 'tilt' && size === 'full')
  const face = video
    ? <CanvasVideo info={canvas!.info} blocked={canvas!.blocked} paused={!playing && size !== 'tile'} className="av-video" />
    : src ? <img src={src} alt={alt} draggable={false} decoding="async" /> : fallback
  return (
    <div ref={box} className={`art-view s-${style} z-${size}` + (playing ? ' playing' : '') + (video ? ' has-video' : '')}>
      <div className="av-body">
        <div className="av-face">{face}</div>
        {(style === 'vinyl' || style === 'cd') && <span className="av-hole" aria-hidden="true" />}
      </div>
      {(style === 'vinyl' || style === 'cd') && <span className="av-sheen" aria-hidden="true" />}
      {style === 'polaroid' && caption && size !== 'bar' && <span className="av-caption">{caption}</span>}
    </div>
  )
}

const reduced = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches

// The pulse: the low end of the sound, read once a frame while it shows, sets --beat (0 to 1) on the art.
function usePulse(box: React.RefObject<HTMLDivElement | null>, on: boolean, media?: () => HTMLMediaElement | null) {
  useEffect(() => {
    const el = box.current
    if (!on || !el || !media || reduced()) return
    const ear = new SoundListener(media)
    const freq = new Uint8Array(1024)
    let raf = 0, level = 0, seen = true
    const io = typeof IntersectionObserver !== 'undefined' ? new IntersectionObserver((e) => { seen = e.some((x) => x.isIntersecting) }) : null
    io?.observe(el)
    const tick = () => {
      raf = requestAnimationFrame(tick)
      if (!seen || document.visibilityState === 'hidden') return
      if (!ear.read(freq)) return
      // The first bins hold the bass: their average, eased so the cover breathes rather than shakes.
      let sum = 0
      for (let i = 1; i < 8; i++) sum += freq[i]
      const now = Math.max(0, (sum / 7 - 120) / 135)
      level = now > level ? level + (now - level) * 0.55 : level * 0.88
      el.style.setProperty('--beat', level.toFixed(3))
    }
    raf = requestAnimationFrame(tick)
    return () => { cancelAnimationFrame(raf); io?.disconnect(); ear.close(); el.style.removeProperty('--beat') }
  }, [box, on, media])
}

// The lean: the art turns a little towards the pointer, anywhere over the player, eased.
function useTilt(box: React.RefObject<HTMLDivElement | null>, on: boolean) {
  useEffect(() => {
    const el = box.current
    if (!on || !el || reduced()) return
    let raf = 0, tx = 0, ty = 0, x = 0, y = 0
    const move = (e: PointerEvent) => {
      const r = el.getBoundingClientRect()
      tx = Math.max(-1, Math.min(1, (e.clientX - (r.left + r.width / 2)) / (window.innerWidth / 2)))
      ty = Math.max(-1, Math.min(1, (e.clientY - (r.top + r.height / 2)) / (window.innerHeight / 2)))
      if (!raf) raf = requestAnimationFrame(step)
    }
    const step = () => {
      x += (tx - x) * 0.12; y += (ty - y) * 0.12
      el.style.setProperty('--ry', (x * 14).toFixed(2) + 'deg')
      el.style.setProperty('--rx', (-y * 10).toFixed(2) + 'deg')
      el.style.setProperty('--gx', (50 + x * 30).toFixed(1) + '%')
      el.style.setProperty('--gy', (50 + y * 30).toFixed(1) + '%')
      raf = Math.abs(tx - x) + Math.abs(ty - y) > 0.002 ? requestAnimationFrame(step) : 0
    }
    window.addEventListener('pointermove', move, { passive: true })
    return () => { window.removeEventListener('pointermove', move); cancelAnimationFrame(raf) }
  }, [box, on])
}
