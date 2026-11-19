// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { IconPause, IconPlay } from '../icons'
import { num, t } from '../i18n'

// The loop editor of the Canvas studio: a picture, a timeline with a filmstrip, and the part to loop between two
// handles. Drag a handle to move one end, drag the part to move both, click the strip to look there. Each handle is a
// slider for the keyboard too (arrows a tenth of a second, Shift a second, Page keys five). "Play the loop" plays the
// part over and over; the seam shows the part's last frame beside its first, the join a loop has to hide.
//
// The source is either a video the browser can play (a clip from this device, or a canvas already made) or, for a
// video on the server, pictures of it taken by the server (frame), as a film on the server may be in a form no browser
// plays. Nothing here plays sound.

export type Range = { start: number; end: number }
type Source = { kind: 'video'; url: string } | { kind: 'frames'; frame: (sec: number) => string }

const STRIP = 12 // pictures along the timeline
const clock = (t: number) => { const m = Math.floor(t / 60), s = t - m * 60; return `${m}:${s.toFixed(1).padStart(4, '0')}` }

export default function LoopEditor({ source, duration, value, onChange, min, max, crossfade }: {
  source: Source; duration: number; value: Range; onChange: (r: Range) => void
  min: number; max: number // the shortest and longest part allowed, in seconds
  crossfade?: boolean // the cut will blend the seam: the seam preview shows it blended
}) {
  const track = useRef<HTMLDivElement>(null)
  const video = useRef<HTMLVideoElement>(null)
  const [head, setHead] = useState(value.start) // where the picture is
  const [playing, setPlaying] = useState(false)
  const [strip, setStrip] = useState<string[]>([])
  const [seam, setSeam] = useState<{ a: string; b: string }>({ a: '', b: '' })
  const dur = Math.max(0.1, duration)
  // What the source is, as a string: the page may hand over a new object each time it draws, the same video.
  const srcKey = source.kind === 'video' ? source.url : 'frames:' + source.frame(0)
  const src = useRef(source)
  src.current = source
  // A long video gets two tracks: the whole of it, to go anywhere, and a minute around the loop, to set its ends.
  const zoomed = dur > 90
  const span = zoomed ? 60 : dur
  const centre = (value.start + value.end) / 2
  const from = zoomed ? Math.max(0, Math.min(dur - span, centre - span / 2)) : 0

  // Keep a range inside the rules: within the video, no shorter than min, no longer than max.
  const fit = useCallback((r: Range, moving: 'start' | 'end' | 'both'): Range => {
    let { start, end } = r
    if (moving === 'both') { const len = end - start; start = Math.max(0, Math.min(dur - len, start)); return { start, end: start + len } }
    start = Math.max(0, Math.min(start, dur)); end = Math.max(0, Math.min(end, dur))
    if (moving === 'start') { start = Math.min(start, end - min); start = Math.max(start, end - max, 0) } else { end = Math.max(end, start + min); end = Math.min(end, start + max, dur) }
    return { start: +start.toFixed(2), end: +end.toFixed(2) }
  }, [dur, min, max])

  // The filmstrip and the seam: from the server's pictures, or drawn from the video by a second, hidden copy of it.
  const grab = useGrabber(source)
  const grabSeam = useGrabber(source) // its own copy: the seam answers at once, not after the whole strip
  useEffect(() => {
    let live = true
    const times = Array.from({ length: STRIP }, (_, i) => from + (span * (i + 0.5)) / STRIP)
    const source = src.current
    if (source.kind === 'frames') { setStrip(times.map((t) => source.frame(Math.floor(t)))); return }
    ;(async () => { const out: string[] = []; for (const t of times) { const u = await grab(t); if (!live) return; out.push(u); setStrip([...out]) } })()
    return () => { live = false }
  }, [srcKey, span, Math.round(from / 5), grab]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    let live = true
    const t = setTimeout(async () => {
      const end = Math.max(value.start, value.end - 0.05)
      const source = src.current
      const [a, b] = source.kind === 'frames' ? [source.frame(Math.floor(end)), source.frame(Math.floor(value.start))] : [await grabSeam(end), await grabSeam(value.start)]
      if (live) setSeam({ a, b })
    }, 250) // after a drag settles
    return () => { live = false; clearTimeout(t) }
  }, [value.start, value.end, srcKey, grabSeam]) // eslint-disable-line react-hooks/exhaustive-deps

  // Playing the loop: the video between the handles, over and over; the server's pictures as a flipbook.
  useEffect(() => {
    if (!playing) { video.current?.pause(); return }
    if (src.current.kind === 'video') {
      const v = video.current
      if (!v) return
      if (v.currentTime < value.start || v.currentTime >= value.end) v.currentTime = value.start
      void v.play().catch(() => setPlaying(false))
      let raf = 0
      const watch = () => { if (v.currentTime >= value.end || v.currentTime < value.start - 0.25) v.currentTime = value.start; setHead(v.currentTime); raf = requestAnimationFrame(watch) }
      raf = requestAnimationFrame(watch)
      return () => cancelAnimationFrame(raf)
    }
    const step = Math.max(0.5, (value.end - value.start) / 12)
    const t = setInterval(() => setHead((h) => (h + step >= value.end ? value.start : h + step)), 420)
    return () => clearInterval(t)
  }, [playing, srcKey, value.start, value.end]) // eslint-disable-line react-hooks/exhaustive-deps
  // The picture follows the playhead when nothing plays.
  useEffect(() => { if (!playing && src.current.kind === 'video' && video.current && Math.abs(video.current.currentTime - head) > 0.04) video.current.currentTime = head }, [head, playing, srcKey])

  // Dragging on the timeline.
  const drag = useRef<{ what: 'start' | 'end' | 'both'; from: number; r0: Range } | null>(null)
  const at = (clientX: number) => { const r = track.current!.getBoundingClientRect(); return from + Math.max(0, Math.min(1, (clientX - r.left) / r.width)) * span }
  const down = (what: 'start' | 'end' | 'both') => (e: React.PointerEvent) => {
    e.stopPropagation(); e.preventDefault()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    drag.current = { what, from: at(e.clientX), r0: value }
    setPlaying(false)
  }
  const move = (e: React.PointerEvent) => {
    const d = drag.current
    if (!d) return
    const t = at(e.clientX), dt = t - d.from
    const next = d.what === 'start' ? fit({ ...d.r0, start: d.r0.start + dt }, 'start') : d.what === 'end' ? fit({ ...d.r0, end: d.r0.end + dt }, 'end') : fit({ start: d.r0.start + dt, end: d.r0.end + dt }, 'both')
    onChange(next)
    setHead(d.what === 'end' ? next.end : next.start)
  }
  const up = () => { drag.current = null }
  const onTrack = (e: React.PointerEvent) => { if (e.target === e.currentTarget || (e.target as HTMLElement).closest('.le-strip')) { setPlaying(false); setHead(at(e.clientX)) } }
  const keys = (what: 'start' | 'end') => (e: React.KeyboardEvent) => {
    const step = e.key.startsWith('Page') ? 5 : e.shiftKey ? 1 : 0.1
    const dir = e.key === 'ArrowRight' || e.key === 'ArrowUp' || e.key === 'PageUp' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowDown' || e.key === 'PageDown' ? -1 : 0
    if (!dir) return
    e.preventDefault()
    const next = fit({ ...value, [what]: value[what] + dir * step }, what)
    onChange(next); setHead(next[what]); setPlaying(false)
  }
  const pct = (t: number) => `${Math.max(0, Math.min(100, ((t - from) / span) * 100))}%`
  const whole = (t: number) => `${(t / dur) * 100}%`
  // The overview: a press puts the loop there, keeping its length.
  const jump = (e: React.PointerEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    const t = (Math.max(0, Math.min(1, (e.clientX - r.left) / r.width))) * dur
    const len = value.end - value.start
    const next = fit({ start: t - len / 2, end: t + len / 2 }, 'both')
    onChange(next); setHead(next.start); setPlaying(false)
  }
  const len = value.end - value.start
  const still = useMemo(() => (source.kind === 'frames' ? source.frame(Math.floor(head)) : ''), [srcKey, Math.floor(head)]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="le">
      <div className="le-view">
        {source.kind === 'video'
          ? <video ref={video} src={source.url} muted playsInline preload="auto" onLoadedMetadata={() => { if (video.current) video.current.currentTime = value.start }} />
          : <img src={still} alt="" />}
        <span className="le-time" aria-hidden="true">{clock(head)}</span>
      </div>
      <div className="le-bar">
        <button type="button" className="le-play" onClick={() => setPlaying(!playing)} aria-pressed={playing}>{playing ? <IconPause size={18} /> : <IconPlay size={18} />} {playing ? t('Stop') : t('Play the loop')}</button>
        <span className="le-facts" aria-live="polite"><b>{t('{n} s', { n: num(len, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) })}</b> {t('from {start} to {end}', { start: clock(value.start), end: clock(value.end) })}</span>
      </div>
      {zoomed && (
        <div className="le-overview" onPointerDown={jump} title={t('The whole video: press to put the loop there')}>
          <span className="le-ov-win" style={{ left: whole(from), width: whole(span) }} aria-hidden="true" />
          <span className="le-ov-sel" style={{ left: whole(value.start), width: `max(3px, ${whole(value.end - value.start)})` }} aria-hidden="true" />
          <span className="le-ov-t">{clock(0)}</span><span className="le-ov-t end">{clock(dur)}</span>
        </div>
      )}
      <div className="le-track" ref={track} onPointerDown={onTrack} onPointerMove={move} onPointerUp={up} onPointerCancel={up}>
        <div className="le-strip" aria-hidden="true">{Array.from({ length: STRIP }, (_, i) => <span key={i} style={strip[i] ? { backgroundImage: `url("${strip[i]}")` } : undefined} />)}</div>
        <div className="le-shade" style={{ left: 0, width: pct(value.start) }} aria-hidden="true" />
        <div className="le-shade" style={{ left: pct(value.end), right: 0 }} aria-hidden="true" />
        <div className="le-sel" style={{ left: pct(value.start), width: `calc(${pct(value.end)} - ${pct(value.start)})` }} onPointerDown={down('both')} title={t('Drag to move the loop')} />
        <div className="le-head" style={{ left: pct(head) }} aria-hidden="true" />
        <span className="le-handle start" style={{ left: pct(value.start) }} role="slider" tabIndex={0} aria-label={t('Loop start')} aria-valuemin={0} aria-valuemax={Math.round(dur * 10) / 10} aria-valuenow={value.start} aria-valuetext={clock(value.start)} onPointerDown={down('start')} onKeyDown={keys('start')} />
        <span className="le-handle end" style={{ left: pct(value.end) }} role="slider" tabIndex={0} aria-label={t('Loop end')} aria-valuemin={0} aria-valuemax={Math.round(dur * 10) / 10} aria-valuenow={value.end} aria-valuetext={clock(value.end)} onPointerDown={down('end')} onKeyDown={keys('end')} />
      </div>
      <div className="le-seam">
        <span className="le-seam-l">{t('The seam')}</span>
        <figure><span style={seam.a ? { backgroundImage: `url("${seam.a}")` } : undefined} /><figcaption>{t('Last frame')}</figcaption></figure>
        <span className={'le-join' + (crossfade ? ' fade' : '')} aria-hidden="true">{crossfade ? '⇄' : '→'}</span>
        <figure><span style={seam.b ? { backgroundImage: `url("${seam.b}")` } : undefined} /><figcaption>{t('First frame')}</figcaption></figure>
        <p className="le-seam-note">{crossfade ? t('The last second is blended into the first, so the join doesn’t show.') : t('The closer these two look, the smoother the loop.')}</p>
      </div>
    </div>
  )
}

// useGrabber: a picture of the video at a time, drawn by a hidden copy of it (kept for the editor's life), as a
// small JPEG. For the browser's own videos only.
function useGrabber(source: Source) {
  const url = source.kind === 'video' ? source.url : ''
  const copy = useRef<{ v: HTMLVideoElement; c: HTMLCanvasElement; busy: Promise<unknown> } | null>(null)
  useEffect(() => {
    if (!url) return
    const v = document.createElement('video')
    v.muted = true; v.preload = 'auto'; v.src = url
    copy.current = { v, c: document.createElement('canvas'), busy: Promise.resolve() }
    return () => { v.removeAttribute('src'); v.load(); copy.current = null }
  }, [url])
  return useCallback((t: number): Promise<string> => {
    const k = copy.current
    if (!k) return Promise.resolve('')
    const job = k.busy.then(() => new Promise<string>((done) => {
      const { v, c } = k
      const draw = () => {
        try {
          const w = 160, h = Math.round((w * (v.videoHeight || 9)) / (v.videoWidth || 16))
          c.width = w; c.height = h
          c.getContext('2d')!.drawImage(v, 0, 0, w, h)
          done(c.toDataURL('image/jpeg', 0.6))
        } catch { done('') }
      }
      const go = () => {
        const to = Math.min(Math.max(0, t), Math.max(0, (v.duration || t) - 0.05))
        if (Math.abs(v.currentTime - to) < 0.02 && v.readyState >= 2) { draw(); return } // already there: no seek, no 'seeked'
        v.addEventListener('seeked', draw, { once: true }); v.currentTime = to
      }
      if (v.readyState >= 1) go(); else v.addEventListener('loadedmetadata', go, { once: true })
      setTimeout(() => { v.removeEventListener('seeked', draw); done('') }, 3000) // a frame that never comes doesn't hold up the rest
    }))
    k.busy = job
    return job
  }, [])
}
