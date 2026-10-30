import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { IconFull, IconMute, IconVolume } from '../../icons'
import { useSource } from '../../player/hooks'
import { setLook, type Look } from './look'
import { leaveChannel, makeViewer, preflight, tune, type TuneInfo } from './tvApi'

// The little TV on the guide. It waits for the highlight to settle before
// tuning, because each channel it shows runs a stream, and says when it leaves,
// so a channel that was only flicked past stops at once.

const SETTLE_MS = 1500

export default function Preview({ channel, label, look }: { channel: string; label: string; look: Look }) {
  const nav = useNavigate()
  const video = useRef<HTMLVideoElement | null>(null)
  const box = useRef<HTMLDivElement | null>(null)
  const viewer = useRef(makeViewer())
  const [url, setUrl] = useState<string | null>(null)
  const [info, setInfo] = useState<TuneInfo | null>(null)
  const [state, setState] = useState<'idle' | 'tuning' | 'playing' | 'failed'>('idle')
  const [msg, setMsg] = useState('')

  useEffect(() => {
    setUrl(null); setInfo(null); setMsg('')
    if (!channel) { setState('idle'); return }
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
  }, [channel])

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

  // Moving it: drag with a pointer, or the arrow keys on the handle.
  const place = (x: number, y: number) => {
    setLook({ previewPos: 'custom', previewX: Math.max(0, Math.min(100, x)), previewY: Math.max(0, Math.min(100, y)) })
  }
  const onDrag = (e: React.PointerEvent) => {
    const el = box.current, page = el?.parentElement
    if (!el || !page) return
    e.preventDefault()
    const pr = page.getBoundingClientRect(), r = el.getBoundingClientRect()
    const dx = e.clientX - r.left, dy = e.clientY - r.top
    const move = (ev: PointerEvent) => {
      const room = { w: Math.max(1, pr.width - r.width), h: Math.max(1, pr.height - r.height) }
      place(((ev.clientX - pr.left - dx) / room.w) * 100, ((ev.clientY - pr.top - dy) / room.h) * 100)
    }
    const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  const onGripKey = (e: React.KeyboardEvent) => {
    const step = { ArrowLeft: [-3, 0], ArrowRight: [3, 0], ArrowUp: [0, -3], ArrowDown: [0, 3] }[e.key]
    if (!step) return
    e.preventDefault(); e.stopPropagation()
    let x = look.previewX, y = look.previewY
    if (look.previewPos !== 'custom') {
      const el = box.current, page = el?.parentElement
      if (el && page) {
        const pr = page.getBoundingClientRect(), r = el.getBoundingClientRect()
        x = ((r.left - pr.left) / Math.max(1, pr.width - r.width)) * 100
        y = ((r.top - pr.top) / Math.max(1, pr.height - r.height)) * 100
      }
    }
    place(x + step[0], y + step[1])
  }
  const style: React.CSSProperties = look.previewPos === 'custom'
    ? { left: `calc((100% - var(--ctv-pv-w)) * ${look.previewX / 100})`, top: `calc((100% - var(--ctv-pv-h)) * ${look.previewY / 100})` }
    : {}
  return (
    <div className={'ctv-preview pos-' + look.previewPos} ref={box} style={style} role="group" aria-label={`Preview: ${label || 'no channel'}`}>
      <video ref={video} playsInline muted preload="auto" aria-hidden="true" tabIndex={-1} />
      {state !== 'playing' && (
        <div className="ctv-preview-msg" aria-live="polite">
          {state === 'failed' ? <small>{msg.length > 90 ? 'Not available' : msg || 'Not available'}</small> : state === 'tuning' ? <small>{info ? 'Tuning…' : label}</small> : <small>No preview for this channel</small>}
        </div>
      )}
      <div className="ctv-preview-bar">
        <button type="button" className="ctv-preview-grip" title="Drag to move, or use the arrow keys" aria-label="Move the preview. Use the arrow keys." onPointerDown={onDrag} onKeyDown={onGripKey}>⠿</button>
        <span className="ctv-preview-name">{label}</span>
        <button type="button" onClick={() => setLook({ previewSound: !look.previewSound })} aria-pressed={look.previewSound} aria-label="Preview sound" title="Preview sound">{look.previewSound ? <IconVolume size={15} /> : <IconMute size={15} />}</button>
        <button type="button" disabled={!channel} onClick={() => nav(`/tv/watch/${channel}`)} aria-label={`Watch ${label} full screen`} title="Full screen"><IconFull size={15} /></button>
      </div>
    </div>
  )
}
