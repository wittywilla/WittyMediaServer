import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { IconClose, IconFull, IconMute, IconVolume } from '../../icons'
import { PREVIEW_SIZES, previewSizeName, setLook, type Look } from './look'
import { PREVIEW_MIN } from './tvThemes'
import { liveNow, liveVideo, useLive } from './live'
import Slate from './Slate'

// The little TV on the guide. It waits for the highlight to settle before
// tuning, because each channel it shows runs a stream, and moving on leaves the
// last at once, so a channel that was only flicked past stops. With the tab in the
// background it stops altogether. The picture is the TV section's one player
// (live.ts): the channel that was being watched carries on here without tuning
// again, and carries on into the watch view the same way.
//
// It sits in the page's layout, not over it, so it can't cover anything. It starts at
// a size that suits the theme and the window (`start`). Someone can make it any width
// from small up to what the page can spare (`max`), by dragging its handle or with the
// keys on it, and it snaps to the named sizes. A size someone chose is theirs to keep.

const SETTLE_MS = 1500
const STEP = 24
const SNAP_WITHIN = 18

const snap = (w: number) => PREVIEW_SIZES.find((s) => Math.abs(s.w - w) <= SNAP_WITHIN)?.w ?? w

export default function Preview({ channel, label, look, max, start }: { channel: string; label: string; look: Look; max: number; start: number }) {
  const nav = useNavigate()
  const box = useRef<HTMLDivElement | null>(null)
  const screen = useRef<HTMLDivElement | null>(null)
  const [hidden, setHidden] = useState(() => document.hidden)
  // The channel this asks the player for: the highlighted one once it has settled, or at once
  // when it's the one already playing.
  const [want, setWant] = useState(() => (channel && channel === liveNow().key ? channel : ''))
  const live = useLive(want, screen)
  const mine = !!want && live.key === want
  const [shown, setShown] = useState(false)

  useEffect(() => {
    const on = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', on)
    return () => document.removeEventListener('visibilitychange', on)
  }, [])

  useEffect(() => {
    if (!channel || hidden) { setWant(''); return }
    if (channel === liveNow().key) { setWant(channel); return }
    setWant('')
    const t = setTimeout(() => setWant(channel), SETTLE_MS)
    return () => clearTimeout(t)
  }, [channel, hidden])

  useLayoutEffect(() => {
    const v = liveVideo()
    v.setAttribute('aria-hidden', 'true')
    v.tabIndex = -1
  }, [])
  // Sound only when asked for; a browser that won't start it with sound starts it silent.
  useEffect(() => {
    setShown(false)
    if (!mine || !live.ready) return
    const v = liveVideo()
    v.muted = !look.previewSound
    let on = true
    v.play().then(() => { if (on) setShown(true) })
      .catch(() => { v.muted = true; v.play().then(() => { if (on) setShown(true) }).catch(() => {}) })
    return () => { on = false }
  }, [mine, live.ready]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (mine) liveVideo().muted = !look.previewSound }, [mine, look.previewSound])
  const failed = mine && !!live.err
  const state = !channel || hidden ? 'idle' : failed ? 'failed' : mine && shown && live.playing ? 'playing' : 'tuning'
  const msg = failed ? live.err : ''
  const info = mine ? live.info : null

  // Below what the controls need, a preview can't be used at all.
  const least = Math.min(PREVIEW_MIN, max)
  const width = Math.max(least, Math.min(max, look.previewW ?? start))
  const setWidth = (w: number) => setLook({ previewW: Math.round(Math.max(least, Math.min(max, w))) })

  // Dragging changes only this element, so nothing else redraws on every move.
  const onDrag = (e: React.PointerEvent) => {
    const el = box.current
    if (!el) return
    e.preventDefault()
    const startX = e.clientX, startW = el.getBoundingClientRect().width
    const sign = look.previewSide === 'right' ? -1 : 1
    let now = startW
    const move = (ev: PointerEvent) => {
      now = snap(Math.max(least, Math.min(max, startW + (ev.clientX - startX) * sign)))
      el.style.width = now + 'px'
    }
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('pointercancel', up)
      setWidth(now)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
    window.addEventListener('pointercancel', up)
  }
  // The named sizes that fit, for jumping between with Page keys and the buttons.
  const sizes = PREVIEW_SIZES.map((s) => s.w).filter((w) => w <= max)
  const up = () => { const n = sizes.find((w) => w > width + 1); setWidth(n ?? max) }
  const down = () => { const n = [...sizes].reverse().find((w) => w < width - 1); setWidth(n ?? least) }
  const onKey = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowRight': setWidth(width + STEP); break
      case 'ArrowLeft': setWidth(width - STEP); break
      case 'PageUp': up(); break
      case 'PageDown': down(); break
      case 'Home': setWidth(least); break
      case 'End': setWidth(max); break
      default: return
    }
    e.preventDefault(); e.stopPropagation()
  }
  return (
    <div className={'ctv-preview side-' + look.previewSide} ref={box} style={{ width }} role="group" aria-label={`Preview: ${label || 'no channel'}`}>
      <div className="ctv-preview-screen">
        <div className="ctv-preview-slot" ref={screen} />
        {state === 'playing' && info?.now?.kind === 'slate' && <Slate name={info.name} next={info.next} />}
        {state !== 'playing' && (
          <div className="ctv-preview-msg" aria-live="polite">
            {/* Only a failure is said aloud: moving through the guide already says each channel, and "Tuning…" on every move is noise. */}
            {state === 'failed' ? <small>{msg.length > 90 ? 'Not available' : msg || 'Not available'}</small> : state === 'tuning' ? <small aria-hidden="true">{info ? 'Tuning…' : label}</small> : <small aria-hidden="true">{hidden ? 'Paused while this page is in the background' : 'No preview for this channel'}</small>}
          </div>
        )}
        <span className="ctv-preview-handle" role="slider" tabIndex={0} aria-label="Preview size" aria-orientation="horizontal"
          aria-valuemin={Math.round(least)} aria-valuemax={Math.round(max)} aria-valuenow={Math.round(width)} aria-valuetext={previewSizeName(width)}
          title="Drag to resize, or use the arrow keys" onPointerDown={onDrag} onKeyDown={onKey} />
      </div>
      <div className="ctv-preview-bar">
        <span className="ctv-preview-name">{label}</span>
        <button type="button" onClick={down} disabled={width <= least} aria-label="Smaller preview" title="Smaller">−</button>
        <button type="button" onClick={up} disabled={width >= max} aria-label="Larger preview" title="Larger">+</button>
        <button type="button" onClick={() => setLook({ previewSound: !look.previewSound })} aria-pressed={look.previewSound} aria-label="Preview sound" title={look.previewSound ? 'Sound on' : 'Sound off'}>{look.previewSound ? <IconVolume size={15} /> : <IconMute size={15} />}</button>
        <button type="button" disabled={!channel} onClick={() => nav(`/tv/watch/${channel}`, { state: { fromGuide: true } })} aria-label={`Watch ${label} full screen`} title="Full screen"><IconFull size={15} /></button>
        <button type="button" onClick={() => setLook({ preview: 'off' })} aria-label="Hide the preview" title="Hide the preview"><IconClose size={15} /></button>
      </div>
    </div>
  )
}
