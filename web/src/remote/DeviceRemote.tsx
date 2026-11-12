// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A live remote for one device: what it is playing, a scrubber, the transport keys, volume,
// speed, sound and subtitle tracks, its queue, and a D-pad that drives its interface. It
// shows only what the device claimed and what applies to what it is playing
// (capabilities.ts), and nothing about an item this account may not see.
import { useEffect, useRef, useState } from 'react'
import { api, ApiError, episodeLabel, fmtTime, posterUrl, sized, type Item } from '../api'
import { IconBack, IconDown, IconFf, IconHome, IconLeft, IconMenu, IconMute, IconNext, IconPause, IconPlay, IconPrev, IconRew, IconRight, IconStop, IconUp, IconVolume } from '../icons'
import { toast } from '../state'
import { Field, IconButton, Select } from '../ui'
import { errorText, remoteControls, statusWords, stepWords, watchingTv, type NavKey } from './capabilities'
import type { RemoteCommand } from './device'
import { partDone, positionNow, seekTarget } from './position'
import { sendCommand, type RemoteDevice, type ServerClock } from './useDevices'

// The speeds the protocol carries (0.25 to 4), as the players offer them.
const RATES = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 3.5, 4]

// Items are looked up once each: a queue names the same ones again and again.
const known = new Map<number, Promise<Item | null>>()
const lookUp = (id: number) => {
  let p = known.get(id)
  if (!p) {
    if (known.size > 400) known.clear()
    p = api.get<Item>(`/items/${id}`).catch(() => null)
    known.set(id, p)
  }
  return p
}
function useItem(id: number): Item | null {
  const [it, setIt] = useState<Item | null>(null)
  useEffect(() => {
    setIt(null)
    if (!id) return
    let on = true
    lookUp(id).then((x) => { if (on) setIt(x) })
    return () => { on = false }
  }, [id])
  return it
}

const itemLine = (it: Item) => (it.showTitle ? [it.showTitle, episodeLabel(it)].filter(Boolean).join(' · ') : it.artist || it.albumArtist || (it.year ? String(it.year) : ''))

// The latest value, sent at most every so often, and once more when the hand lets go:
// a slider being dragged mustn't run into the server's limit on commands.
function useSpaced(fn: (v: number) => void, everyMs: number) {
  const st = useRef<{ last: number; timer: ReturnType<typeof setTimeout> | null; value: number }>({ last: 0, timer: null, value: 0 })
  const now = useRef(fn)
  now.current = fn
  useEffect(() => () => { if (st.current.timer) clearTimeout(st.current.timer) }, [])
  return (v: number) => {
    const s = st.current
    s.value = v
    if (s.timer) return
    s.timer = setTimeout(() => { s.timer = null; s.last = Date.now(); now.current(s.value) }, Math.max(0, everyMs - (Date.now() - s.last)))
  }
}

function Scrubber({ d, serverNow, onSeek }: { d: RemoteDevice; serverNow: ServerClock; onSeek: (sec: number) => void }) {
  // Where the thumb is being held, and where it was let go until the device says where it is now.
  const [held, setHeld] = useState<number | null>(null)
  const [sent, setSent] = useState<{ to: number; at: number } | null>(null)
  const [, setTick] = useState(0)
  const playing = d.state.status === 'playing'
  useEffect(() => {
    if (!playing) return
    const t = setInterval(() => setTick((n) => n + 1), 500)
    return () => clearInterval(t)
  }, [playing])
  useEffect(() => {
    if (!sent) return
    if (d.stateAt !== sent.at) { setSent(null); return }
    const t = setTimeout(() => setSent(null), 3000)
    return () => clearTimeout(t)
  }, [sent, d.stateAt])
  const dur = d.state.durationSec || 0
  const pos = held ?? sent?.to ?? positionNow(d.state, d.stateAt, serverNow())
  const release = () => {
    if (held == null) return
    setSent({ to: held, at: d.stateAt })
    onSeek(held)
    setHeld(null)
  }
  return (
    <div className="rr-scrub">
      <span className="rr-time" aria-hidden="true">{fmtTime(pos)}</span>
      <input type="range" className="rr-range" min={0} max={Math.max(1, Math.floor(dur))} step={1} value={Math.min(Math.floor(pos), Math.floor(dur))}
        aria-label={`Position on ${d.name}`} aria-valuetext={`${fmtTime(pos)} of ${fmtTime(dur)}`}
        onChange={(e) => setHeld(Number(e.target.value))} onPointerUp={release} onTouchEnd={release} onKeyUp={release} onBlur={release} />
      <span className="rr-time" aria-hidden="true">{fmtTime(dur)}</span>
    </div>
  )
}

function Queue({ d, send, open, setOpen }: { d: RemoteDevice; send: (cmd: RemoteCommand) => Promise<unknown>; open: boolean; setOpen: (on: boolean) => void }) {
  const ids = d.state.queue || []
  const at = d.state.queueIndex || 0
  const [titles, setTitles] = useState<Record<number, string>>({})
  // Entries this account may not see come as 0: they can't be named, or sent back.
  const whole = ids.every((id) => id > 0)
  useEffect(() => {
    if (!open) return
    let on = true
    const todo = [...new Set(ids.filter((id) => id > 0))]
    let next = 0
    const one = async () => {
      while (on && next < todo.length) {
        const id = todo[next++]
        const it = await lookUp(id)
        if (on && it) setTitles((t) => (t[id] ? t : { ...t, [id]: [it.title, itemLine(it)].filter(Boolean).join(' · ') }))
      }
    }
    for (let i = 0; i < 4; i++) void one()
    return () => { on = false }
  }, [open, ids.join(',')]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <section className="rr-queue" aria-label={`Queue on ${d.name}`}>
      <button type="button" className="rr-queue-head" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span>Queue</span><span className="rr-queue-count">{at + 1} of {ids.length}</span><IconDown />
      </button>
      {open && (
        <>
          <ol className="rr-queue-list">
            {ids.map((id, i) => (
              <li key={i}>
                <button type="button" className={'rr-queue-item' + (i === at ? ' now' : '')} aria-current={i === at ? 'true' : undefined} disabled={!whole || i === at}
                  onClick={() => send({ type: 'play', queue: ids, queueIndex: i })}>
                  <span className="rr-queue-n" aria-hidden="true">{i === at ? <IconPlay /> : i + 1}</span>
                  <span className="rr-queue-title">{id > 0 ? titles[id] || 'Loading…' : 'Something you can’t see'}</span>
                </button>
              </li>
            ))}
          </ol>
          {!whole && <p className="rr-note">This queue holds things you can’t see, so it can’t be jumped through from here. Next and Previous still work.</p>}
        </>
      )}
    </section>
  )
}

const PAD: { key: NavKey; label: string; icon?: React.ReactNode; text?: string; place: string }[] = [
  { key: 'back', label: 'Back', icon: <IconBack />, place: 'a' }, { key: 'up', label: 'Up', icon: <IconUp />, place: 'u' }, { key: 'home', label: 'Home', icon: <IconHome />, place: 'h' },
  { key: 'left', label: 'Left', icon: <IconLeft />, place: 'l' }, { key: 'select', label: 'Select', text: 'OK', place: 'o' }, { key: 'right', label: 'Right', icon: <IconRight />, place: 'r' },
  { key: 'menu', label: 'Menu', icon: <IconMenu />, place: 'm' }, { key: 'down', label: 'Down', icon: <IconDown />, place: 'd' }, { key: 'playpause', label: 'Play or pause', icon: <><IconPlay /><IconPause /></>, place: 'p' },
]

export default function DeviceRemote({ d, serverNow }: { d: RemoteDevice; serverNow: ServerClock }) {
  const st = d.state
  const c = remoteControls(d)
  const words = stepWords(d)
  const shown = !d.hidden && st.status !== 'idle' && !!st.itemId
  const item = useItem(shown ? st.itemId! : 0)
  // Keys stay pressable while a command is on its way: a key that greyed out under a remote's focus would lose it.
  const [busy, setBusy] = useState('')
  // Kept here, so the queue stays open across the moment between two of its items.
  const [queueOpen, setQueueOpen] = useState(false)
  // What was last refused, kept on screen for a screen reader and for anyone who missed the toast.
  const [problem, setProblem] = useState('')

  const send = async (cmd: RemoteCommand, what = '') => {
    if (what) setBusy(what)
    try { const r = await sendCommand(d.id, cmd); setProblem(''); return r } catch (e) {
      const err = e as ApiError
      const text = errorText(err.data?.code, err.message, d.name)
      setProblem(text)
      toast(text, { tone: 'warn' })
      return null
    } finally { if (what) setBusy('') }
  }
  const sendVolume = useSpaced((level) => { void send({ type: 'volume', level }) }, 250)
  // The slider follows the hand while it moves, and the device again a moment after it stops.
  const [vol, setVol] = useState<number | null>(null)
  useEffect(() => {
    if (vol == null) return
    const t = setTimeout(() => setVol(null), 1500)
    return () => clearTimeout(t)
  }, [vol])
  const seekBy = (bySec: number) => send({ type: 'seek', bySec })
  const paused = st.status === 'paused'
  const title = d.hidden ? '' : d.title || item?.title || ''
  const line = item ? itemLine(item) : ''
  const part = c.place === 'part' ? partDone(st.positionSec, st.durationSec) : null
  const status = watchingTv(d) ? 'Live TV' : statusWords(d)

  return (
    <div className="rr" role="group" aria-label={`Remote for ${d.name}`}>
      <div className="rr-now">
        {shown && item?.poster ? <img className="rr-art" src={sized(posterUrl(item), 240)} alt="" /> : null}
        <div className="rr-what">
          {title && <div className="rr-title">{title}</div>}
          {line && <div className="rr-line">{line}</div>}
          <div className="rr-status" role="status" aria-live="polite">{[status, part != null ? `${part}% through` : ''].filter(Boolean).join(' · ')}{d.hidden ? '. What it is isn’t shown to this account.' : ''}</div>
        </div>
      </div>
      {problem && <p className="rr-problem" role="alert">{problem}</p>}

      {c.seek && <Scrubber d={d} serverNow={serverNow} onSeek={(sec) => send({ type: 'seek', positionSec: seekTarget(sec, 0, st.durationSec || 0) })} />}

      {(c.transport || c.steps || st.status !== 'idle' || watchingTv(d)) && d.online && (
        <div className="rr-keys" role="group" aria-label="Playback" aria-busy={!!busy || undefined}>
          {c.steps && <IconButton label={words.previous} onClick={() => send({ type: 'previous' }, 'previous')}><IconPrev /></IconButton>}
          {c.seek && <IconButton label="Back 10 seconds" onClick={() => seekBy(-10)}><IconRew /></IconButton>}
          {c.transport && (paused
            ? <IconButton label="Play" variant="primary" round className="rr-play" onClick={() => send({ type: 'resume' }, 'toggle')}><IconPlay /></IconButton>
            : <IconButton label="Pause" variant="primary" round className="rr-play" onClick={() => send({ type: 'pause' }, 'toggle')}><IconPause /></IconButton>)}
          {c.seek && <IconButton label="Forward 30 seconds" onClick={() => seekBy(30)}><IconFf /></IconButton>}
          {c.steps && <IconButton label={words.next} onClick={() => send({ type: 'next' }, 'next')}><IconNext /></IconButton>}
          <IconButton label="Stop" onClick={() => send({ type: 'stop' }, 'stop')}><IconStop /></IconButton>
        </div>
      )}

      {c.volume && (
        <div className="rr-volume">
          <IconButton label={st.muted ? `Unmute ${d.name}` : `Mute ${d.name}`} aria-pressed={!!st.muted} onClick={() => send({ type: 'mute', muted: !st.muted })}>{st.muted ? <IconMute /> : <IconVolume />}</IconButton>
          <input type="range" className="rr-range" min={0} max={1} step={0.05} aria-label={`Volume on ${d.name}`} aria-valuetext={`${Math.round((vol ?? st.volume ?? 0) * 100)}%`}
            value={vol ?? st.volume ?? 0} onChange={(e) => { const v = Number(e.target.value); setVol(v); if (st.muted && v > 0 && vol == null) void send({ type: 'mute', muted: false }); sendVolume(v) }} />
        </div>
      )}

      {(c.rate || c.audio || c.subtitles) && (
        <div className="rr-picks">
          {c.rate && (
            <Field label="Speed">
              <Select value={String(RATES.includes(st.rate || 1) ? st.rate || 1 : 1)} onChange={(e) => send({ type: 'rate', rate: Number(e.target.value) })}
                options={RATES.map((r) => ({ value: String(r), label: r === 1 ? 'Normal' : `${r}×` }))} />
            </Field>
          )}
          {c.audio && (
            <Field label="Sound">
              <Select value={st.audio || ''} onChange={(e) => send({ type: 'audio', audio: e.target.value })}
                options={(st.audioTracks || []).map((t, i) => ({ value: t.id, label: t.label || `Sound ${i + 1}` }))} />
            </Field>
          )}
          {c.subtitles && (
            <Field label="Subtitles">
              <Select value={st.subtitle || 'off'} onChange={(e) => send({ type: 'subtitle', subtitle: e.target.value })}
                options={[{ value: 'off', label: 'Off' }, ...(st.subtitleTracks || []).map((t, i) => ({ value: t.id, label: t.label || `Subtitles ${i + 1}` }))]} />
            </Field>
          )}
        </div>
      )}

      {c.queue && <Queue d={d} send={send} open={queueOpen} setOpen={setQueueOpen} />}

      {c.dpad && (
        <div className="rr-pad" role="group" aria-label={`Keys for ${d.name}`}>
          {PAD.map((k) => (
            <IconButton key={k.key} label={k.label} hideTip className={'rr-key rr-key-' + k.place} onClick={() => send({ type: 'navigate', key: k.key })}>{k.icon || <b>{k.text}</b>}</IconButton>
          ))}
        </div>
      )}
      {!d.online && <p className="rr-note">{d.name} isn’t connected. It comes back here by itself when it is.</p>}
    </div>
  )
}
