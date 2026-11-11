// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "Play on…": the other devices this account may control, what each is playing, and a
// small remote for each. The controller's side of the Remote Protocol.
import { useEffect, useRef, useState } from 'react'
import { ApiError, fmtTime } from '../api'
import { IconBack, IconDevices, IconFf, IconPause, IconPlay, IconStop, IconTv } from '../icons'
import { Button, Dialog, EmptyState, IconButton, LoadError, Notice, Spinner, Switch } from '../ui'
import { toast } from '../state'
import { myDeviceId, setTargetEnabled, targetEnabled, type RemoteCommand, type RemoteState } from './device'
import { call, readEvents, sleep, steadyNow } from './stream'
import './remote.css'

export type RemoteDevice = {
  id: string; name: string; kind: string; userId: number; userName: string; online: boolean; mine: boolean; thisSession: boolean
  capabilities: { mediaKinds: string[]; features: string[] }; state: RemoteState; stateAt: number; title?: string; itemKind?: string; hidden?: boolean
}

// What is open here and could be sent elsewhere: a title from its page, or what the
// player is showing now, which is handed over at its place.
export type PlayOnSource =
  | { kind: 'item'; itemId: number; title: string }
  | { kind: 'playing'; itemId: number; title: string; positionSec: () => number; audio?: string; subtitle?: string; stop: () => void }

export const sendCommand = (deviceId: string, cmd: RemoteCommand) => call<{ ok: boolean; sourceStopped?: boolean }>('POST', `/remote-control/devices/${encodeURIComponent(deviceId)}/commands`, { v: 1, ...cmd })

// useDevices keeps the list live for as long as the dialog is open, and no longer.
function useDevices() {
  const [devices, setDevices] = useState<RemoteDevice[] | null>(null)
  const [error, setError] = useState('')
  const [off, setOff] = useState(false)
  const [again, setAgain] = useState(0)
  // How far the server's clock is ahead of this one, near enough for a progress line.
  const ahead = useRef(0)
  useEffect(() => {
    const ctl = new AbortController()
    setError('')
    const merge = (d: RemoteDevice) => setDevices((list) => {
      const rest = (list || []).filter((x) => x.id !== d.id)
      return [...rest, d].sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
    })
    ;(async () => {
      let wait = 1500
      while (!ctl.signal.aborted) {
        try {
          await readEvents('/remote-control/events', (event, data) => {
            if (event === 'snapshot') { ahead.current = data.serverTimeMs - steadyNow(); setDevices(data.devices); setError(''); wait = 1500 }
            else if (event === 'device') merge(data)
            else if (event === 'gone') setDevices((list) => (list || []).filter((x) => x.id !== data.id))
          }, ctl.signal)
        } catch (e) {
          if (e instanceof ApiError && e.data?.code === 'disabled') { setOff(true); return }
          if (!ctl.signal.aborted) setError((e as Error).message)
        }
        await sleep(wait, ctl.signal)
        wait = Math.min(wait * 2, 15000)
      }
    })()
    return () => ctl.abort()
  }, [again])
  return { devices, error, off, retry: () => setAgain((n) => n + 1), ahead }
}

// Where a playing device has got to since it last said.
function positionNow(d: RemoteDevice, ahead: number) {
  const st = d.state
  let pos = st.positionSec || 0
  if (st.status === 'playing') pos += ((steadyNow() + ahead - d.stateAt) / 1000) * (st.rate || 1)
  return Math.max(0, st.durationSec ? Math.min(pos, st.durationSec) : pos)
}

function DeviceRow({ d, source, ahead, onDone }: { d: RemoteDevice; source?: PlayOnSource; ahead: number; onDone: () => void }) {
  const [busy, setBusy] = useState('')
  const [, setTick] = useState(0)
  const active = d.state.status !== 'idle'
  useEffect(() => {
    if (d.state.status !== 'playing') return
    const t = setInterval(() => setTick((n) => n + 1), 1000)
    return () => clearInterval(t)
  }, [d.state.status])
  const has = (f: string) => d.capabilities.features.includes(f)
  const send = async (what: string, cmd: RemoteCommand) => {
    setBusy(what)
    try { return await sendCommand(d.id, cmd) } catch (e) { toast((e as Error).message, { tone: 'warn' }); return null } finally { setBusy('') }
  }
  const playHere = async () => {
    if (!source) return
    if (source.kind === 'item') {
      if (await send('play', { type: 'play', itemId: source.itemId })) { toast(`Playing on ${d.name}`, { tone: 'ok' }); onDone() }
      return
    }
    // Handing over what's playing here. As a device, the server takes it from this one's
    // own report and stops it here; otherwise it's a play at this place, stopped by hand.
    const me = myDeviceId()
    const res = me
      ? await send('play', { type: 'cast', from: me })
      : await send('play', { type: 'play', itemId: source.itemId, positionSec: source.positionSec(), audio: source.audio, subtitle: source.subtitle })
    if (!res) return
    if (!me || res.sourceStopped === false) source.stop()
    toast(`Now playing on ${d.name}`, { tone: 'ok' })
    onDone()
  }
  const what = !d.online ? 'Not connected'
    : !active ? 'Nothing playing'
    : d.hidden ? 'Playing something'
    : `${d.title || 'Playing'} · ${fmtTime(positionNow(d, ahead))}${d.state.durationSec ? ' of ' + fmtTime(d.state.durationSec) : ''}${d.state.status === 'paused' ? ' · paused' : d.state.status === 'buffering' ? ' · buffering' : ''}`
  const canPlay = !!source && d.online && d.capabilities.mediaKinds.includes('video')
  return (
    <li className={'rc-device' + (d.online ? '' : ' off')}>
      <span className="rc-icon" aria-hidden="true">{d.kind === 'tv' ? <IconTv /> : <IconDevices />}</span>
      <div className="rc-what">
        <div className="rc-name">{d.name}{!d.mine && <span className="pill">{d.userName}</span>}</div>
        <div className="rc-state">{what}</div>
        {d.online && active && (
          <div className="rc-keys" role="group" aria-label={`Control ${d.name}`}>
            {has('seek') && <IconButton label="Back 10 seconds" size="sm" disabled={!!busy} onClick={() => send('seek', { type: 'seek', bySec: -10 })}><IconBack /></IconButton>}
            {d.state.status === 'paused'
              ? <IconButton label="Play" size="sm" disabled={!!busy} onClick={() => send('resume', { type: 'resume' })}><IconPlay /></IconButton>
              : <IconButton label="Pause" size="sm" disabled={!!busy} onClick={() => send('pause', { type: 'pause' })}><IconPause /></IconButton>}
            {has('seek') && <IconButton label="Forward 30 seconds" size="sm" disabled={!!busy} onClick={() => send('seek', { type: 'seek', bySec: 30 })}><IconFf /></IconButton>}
            <IconButton label="Stop" size="sm" disabled={!!busy} onClick={() => send('stop', { type: 'stop' })}><IconStop /></IconButton>
            {has('volume') && d.state.volume != null && (
              <input className="rc-volume" type="range" min={0} max={1} step={0.05} aria-label={`Volume on ${d.name}`} defaultValue={d.state.volume} key={Math.round(d.state.volume * 20)}
                onChange={(e) => { sendCommand(d.id, { type: 'volume', level: Number(e.target.value) }).catch(() => {}) }} />
            )}
          </div>
        )}
      </div>
      {canPlay && <Button size="sm" variant="primary" busy={busy === 'play'} disabled={!!busy} icon={<IconPlay />} onClick={playHere}>{source!.kind === 'playing' ? 'Move here' : 'Play here'}</Button>}
    </li>
  )
}

export default function PlayOn({ source, onClose }: { source?: PlayOnSource; onClose: () => void }) {
  const { devices, error, off, retry, ahead } = useDevices()
  const [target, setTarget] = useState(targetEnabled)
  const others = (devices || []).filter((d) => !d.thisSession || d.id !== myDeviceId())
  return (
    <Dialog title="Play on another device" onClose={onClose} className="rc-dialog"
      description={source ? (source.kind === 'playing' ? `Move “${source.title}” to another device, at the same place.` : `Start “${source.title}” on another device.`) : undefined}>
      {off ? <Notice tone="info" title="Playing on other devices is turned off">An administrator can turn it on under Settings, Sharing &amp; photos.</Notice>
        : error && !devices ? <LoadError what="Your devices" onRetry={retry} />
        : !devices ? <Spinner />
        : others.length === 0 ? <EmptyState icon={<IconDevices />} title="No other devices right now" text="Open the app on another device signed in as you, and it shows here." />
        : <ul className="rc-list">{others.map((d) => <DeviceRow key={d.id} d={d} source={source} ahead={ahead.current} onDone={onClose} />)}</ul>}
      {!off && (
        <div className="rc-here">
          <div><div className="rc-name" id="rc-here-label">Let other devices play here</div><div className="rc-state">This browser shows in the list on your other devices while the app is open.</div></div>
          <Switch labelledBy="rc-here-label" checked={target} onChange={(v) => { setTargetEnabled(v); setTarget(v) }} />
        </div>
      )}
    </Dialog>
  )
}
