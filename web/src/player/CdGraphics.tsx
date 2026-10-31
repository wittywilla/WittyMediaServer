import { useEffect, useRef } from 'react'
import { mediaUrl } from '../api'
import type { CdDisc, CdNow } from '../audio/cdEngine'

// CD+G: the pictures on a karaoke disc. 300 packets a second, each drawing a tile
// or changing the palette. The screen is 300 by 216, and only ever redrawn while
// there are new packets, so this costs almost nothing.

const WIDTH = 300, HEIGHT = 216, PACKET = 24, PER_SECOND = 300

class Screen {
  px = new Uint8Array(WIDTH * HEIGHT)
  palette = new Uint32Array(16)
  dirty = true
  clear(color: number) { this.px.fill(color & 15); this.dirty = true }
  packet(b: Uint8Array, o: number) {
    if ((b[o] & 0x3f) !== 9) return
    const op = b[o + 1] & 0x3f
    const data = o + 4
    if (op === 1) this.clear(b[data])
    else if (op === 2) this.border(b[data] & 15)
    else if (op === 6 || op === 38) this.tile(b, data, op === 38)
    else if (op === 30 || op === 31) this.colors(b, data, op === 30 ? 0 : 8)
  }
  border(c: number) {
    for (let y = 0; y < HEIGHT; y++) for (let x = 0; x < WIDTH; x++) {
      if (x < 6 || x >= 294 || y < 12 || y >= 204) this.px[y * WIDTH + x] = c
    }
    this.dirty = true
  }
  tile(b: Uint8Array, d: number, xor: boolean) {
    const c0 = b[d] & 15, c1 = b[d + 1] & 15, row = b[d + 2] & 31, col = b[d + 3] & 63
    const x0 = col * 6, y0 = row * 12
    if (x0 + 6 > WIDTH || y0 + 12 > HEIGHT) return
    for (let y = 0; y < 12; y++) {
      const bits = b[d + 4 + y] & 0x3f
      for (let x = 0; x < 6; x++) {
        const c = bits & (0x20 >> x) ? c1 : c0
        const at = (y0 + y) * WIDTH + x0 + x
        this.px[at] = xor ? this.px[at] ^ c : c
      }
    }
    this.dirty = true
  }
  colors(b: Uint8Array, d: number, from: number) {
    for (let i = 0; i < 8; i++) {
      const hi = b[d + i * 2] & 0x3f, lo = b[d + i * 2 + 1] & 0x3f
      const r = (hi >> 2) * 17, g = (((hi & 3) << 2) | (lo >> 4)) * 17, bl = (lo & 15) * 17
      this.palette[from + i] = 0xff000000 | (bl << 16) | (g << 8) | r
    }
    this.dirty = true
  }
}

export function CdGraphics({ disc, now, el }: { disc: CdDisc; now: CdNow; el: HTMLAudioElement | null }) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const state = useRef({ track: 0, data: null as Uint8Array | null, from: 0, done: 0, screen: new Screen(), hasDrawn: false })
  const number = now.track?.number ?? 0

  // One track's packets at a time.
  useEffect(() => {
    const t = disc.tracks.find((x) => x.number === number)
    if (!t) return
    const s = state.current
    s.track = number; s.data = null; s.from = t.pregap; s.done = 0; s.screen = new Screen(); s.hasDrawn = false
    const sectors = Math.min(45000, t.end - t.pregap)
    let live = true
    fetch(mediaUrl(`/api/cd/${disc.id}/graphics?from=${t.pregap}&sectors=${sectors}`), { credentials: 'include' }).then((r) => (r.ok ? r.arrayBuffer() : null)).then((b) => {
      if (live && b && s.track === number) s.data = new Uint8Array(b)
    }).catch(() => {})
    return () => { live = false }
  }, [disc.id, disc.tracks, number])

  // Catch up with the music. Going backwards plays the packets again from the top,
  // which is how the picture is rebuilt: a tile alone doesn't know what's behind it.
  const secRef = useRef(now.trackSec)
  secRef.current = now.trackSec
  useEffect(() => {
    let raf = 0
    const ctx = canvas.current?.getContext('2d')
    const image = ctx?.createImageData(WIDTH, HEIGHT)
    const view = image ? new Uint32Array(image.data.buffer) : null
    const tick = () => {
      raf = requestAnimationFrame(tick)
      const s = state.current
      if (!s.data || !ctx || !image || !view || !el || el.paused) return
      const t = disc.tracks.find((x) => x.number === s.track)
      if (!t) return
      const sec = secRef.current + (t.start - t.pregap) / 75
      const want = Math.min(Math.floor(sec * PER_SECOND), Math.floor(s.data.length / PACKET))
      if (want < s.done) { s.screen = new Screen(); s.done = 0 }
      for (; s.done < want; s.done++) s.screen.packet(s.data, s.done * PACKET)
      if (s.screen.dirty) {
        s.screen.dirty = false
        s.hasDrawn = true
        for (let i = 0; i < view.length; i++) view[i] = s.screen.palette[s.screen.px[i]] || 0xff000000
        ctx.putImageData(image, 0, 0)
        canvas.current?.classList.add('on')
      }
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [disc, el])

  return <canvas ref={canvas} className="cdp-cdg" width={WIDTH} height={HEIGHT} role="img" aria-label="Pictures from this disc" />
}
