import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { IconClose, IconFull, IconMute, IconVolume } from '../../icons'
import { useSource } from '../../player/hooks'
import { PREVIEW_MIN, PREVIEW_SIZES, previewSizeName, setLook, type Look } from './look'
import { leaveChannel, makeViewer, preflight, tune, type TuneInfo } from './tvApi'

// The little TV on the guide. It waits for the highlight to settle before
// tuning, because each channel it shows runs a stream, and says when it leaves,
// so a channel that was only flicked past stops at once. With the tab in the
// background it stops altogether.
//
// It sits in the page's layout, not over it, so it can't cover anything. Its
// size is any width from small up to what the page can spare (`max`), set by
// dragging its handle or with the keys on it, and snaps to the named sizes.

const SETTLE_MS = 1500
const STEP = 24
const SNAP_WITHIN = 18

const snap = (w: number) => PREVIEW_SIZES.find((s) => Math.abs(s.w - w) <= SNAP_WITHIN)?.w ?? w

export default function Preview({ channel, label, look, max }: { channel: string; label: string; look: Look; max: number }) {
  const nav = useNavigate()
  const video = useRef<HTMLVideoElement | null>(null)
  const box = useRef<HTMLDivElement | null>(null)
  const viewer = useRef(makeViewer())
  const [url, setUrl] = useState<string | null>(null)
  const [info, setInfo] = useState<TuneInfo | null>(null)
  const [state, setState] = useState<'idle' | 'tuning' | 'playing' | 'failed'>('idle')
  const [msg, setMsg] = useState('')
  const [hidden, setHidden] = useState(() => document.hidden)

  useEffect(() => {
    const on = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', on)
    return () => document.removeEventListener('visibilitychange', on)
  }, [])

  useEffect(() => {
    setUrl(null); setInfo(null); setMsg('')
    if (!channel || hidden) { setState('idle'); return }
    setState('tuning')
    const ac = new AbortController()
    const v = viewer.current
    const t = setTimeout(() => {
      tune(channel, v, ac.signal)
        .then(async (i) => {
          if (ac.signal.aborted) return
          setInfo(i)
          const u = i.url + '?viewer=' + v
          await preflight(u, ac.signal)
          if (!ac.signal.aborted) setUrl(u)
        })
        .catch((e) => { if (!ac.signal.aborted) { setState('failed'); setMsg(e.message) } })
    }, SETTLE_MS)
    // Closing the tab counts as leaving, too.
    const gone = () => leaveChannel(channel, v)
    window.addEventListener('pagehide', gone)
    return () => { clearTimeout(t); ac.abort(); window.removeEventListener('pagehide', gone); gone() }
  }, [channel, hidden])

  useSource(video, url, {
    onReady: () => {
      const v = video.current
      if (!v) return
      v.muted = !look.previewSound
      v.play().then(() => setState('playing')).catch(() => { v.muted = true; v.play().then(() => setState('playing')).catch(() => setState('failed')) })
    },
    onError: (m) => { setState('failed'); setMsg(m) },
  })
  useEffect(() => { if (video.current) video.current.muted = !look.previewSound }, [look.previewSound])

  // Below what the controls need, a preview can't be used at all.
  const least = Math.min(PREVIEW_MIN, max)
  const width = Math.max(least, Math.min(max, look.previewW))
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
        <video ref={video} playsInline muted preload="auto" aria-hidden="true" tabIndex={-1} />
        {state !== 'playing' && (
          <div className="ctv-preview-msg" aria-live="polite">
            {state === 'failed' ? <small>{msg.length > 90 ? 'Not available' : msg || 'Not available'}</small> : state === 'tuning' ? <small>{info ? 'Tuning…' : label}</small> : <small>{hidden ? 'Paused while this page is in the background' : 'No preview for this channel'}</small>}
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
