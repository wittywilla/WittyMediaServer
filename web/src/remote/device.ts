// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// This browser as something other devices can play on: the device's side of the Remote
// Protocol (docs/REMOTE-PROTOCOL.md). It announces itself, keeps the channel open, hands
// each command to whichever player is open, and says what is playing.
import { ApiError } from '../api'
import { getTvMode } from '../tvmode/mode'
import { call, readEvents, sleep } from './stream'

export type RemoteTrack = { id: string; label?: string; lang?: string }
export type RemoteState = {
  v?: number; status: 'idle' | 'playing' | 'paused' | 'buffering'; itemId?: number; positionSec?: number; durationSec?: number; rate?: number
  volume?: number; muted?: boolean; audio?: string; subtitle?: string; audioTracks?: RemoteTrack[]; subtitleTracks?: RemoteTrack[]
  queue?: number[]; queueIndex?: number; screen?: string
}
export type RemoteCommand = {
  v?: number; id?: string; type: string; itemId?: number; queue?: number[]; queueIndex?: number; positionSec?: number; bySec?: number
  audio?: string; subtitle?: string; level?: number; muted?: boolean; rate?: number; key?: string; from?: string; sender?: { userName: string }
}

// What the open player gives this file: how to obey, and what to report. A command it
// can't carry out throws, with words for the person holding the other device.
export type RemotePlayer = { itemId: number; command: (cmd: RemoteCommand) => void | Promise<void>; state: () => RemoteState }

// What this player can do. The web player plays videos; music has its own bar and discs
// their own player, neither of which listens yet, so they aren't claimed.
const CAPABILITIES = { mediaKinds: ['video'], features: ['seek', 'volume', 'audio', 'subtitles', 'rate'] }

const ID_KEY = 'wms.remote.device'
const OFF_KEY = 'wms.remote.target'

let player: RemotePlayer | null = null
let navigate: ((to: string) => void) | null = null
let deviceId = ''
let online = false
let wanted = false
let run: AbortController | null = null
let lastReport = 0
let reportTimer: ReturnType<typeof setTimeout> | null = null
let beat: ReturnType<typeof setInterval> | null = null

const stored = () => { try { return sessionStorage.getItem(ID_KEY) || '' } catch { return '' } }
const store = (id: string) => { try { sessionStorage.setItem(ID_KEY, id) } catch { /* it gets a new id next time */ } }

// Each browser chooses whether it can be played on. On unless turned off here.
export const targetEnabled = () => { try { return localStorage.getItem(OFF_KEY) !== '0' } catch { return true } }
export function setTargetEnabled(on: boolean) {
  try { if (on) localStorage.removeItem(OFF_KEY); else localStorage.setItem(OFF_KEY, '0') } catch { /* for this visit only */ }
  if (on) sync(); else stop(true)
}

// This device's id while its channel is open, for handing over what it's playing.
export const myDeviceId = () => (online ? deviceId : '')

export function setRemoteNavigator(fn: ((to: string) => void) | null) { navigate = fn }

const idle = (): RemoteState => ({ v: 1, status: 'idle', screen: location.pathname.startsWith('/play/') ? 'player' : 'browse' })

function report() {
  reportTimer = null
  if (!online || !deviceId) return
  lastReport = Date.now()
  let st: RemoteState
  try { st = player ? { v: 1, ...player.state() } : idle() } catch { st = idle() }
  call('POST', `/remote-control/devices/${deviceId}/state`, st).catch(() => { /* the next one says it */ })
}

// reportSoon says what's playing, shortly: at once if nothing was said lately, and
// otherwise once the server's limit allows, with whatever is true by then.
export function reportSoon() {
  if (!online || reportTimer) return
  reportTimer = setTimeout(report, Math.max(0, 1000 - (Date.now() - lastReport)))
}

// A player opens: commands go to it and its state is what's reported, until it closes.
export function registerPlayer(p: RemotePlayer): () => void {
  player = p
  sync()
  reportSoon()
  return () => {
    if (player !== p) return
    player = null
    reportSoon()
    sync()
  }
}

async function obey(cmd: RemoteCommand) {
  let ok = true, error = ''
  try {
    if (cmd.type === 'play') {
      const id = cmd.itemId || cmd.queue?.[cmd.queueIndex || 0]
      if (!id || !navigate) throw new Error('This device can’t open that.')
      const q = new URLSearchParams()
      if (cmd.positionSec != null) q.set('start', String(cmd.positionSec))
      if (cmd.audio) q.set('audio', cmd.audio)
      if (cmd.subtitle) q.set('subtitle', cmd.subtitle)
      // Named for this command, so that asking for what's already open starts it again there.
      q.set('remote', cmd.id || String(Date.now()))
      navigate(`/play/${id}?${q}`)
    } else if (cmd.type === 'stop') {
      await player?.command(cmd)
    } else if (!player) {
      throw new Error('Nothing is playing there.')
    } else {
      await player.command(cmd)
    }
  } catch (e) {
    ok = false
    error = (e as Error).message || 'That didn’t work.'
  }
  if (cmd.id) call('POST', `/remote-control/devices/${deviceId}/ack`, { id: cmd.id, ok, error }).catch(() => { /* the controller is told it timed out */ })
  reportSoon()
}

async function loop(ctl: AbortController) {
  let wait = 2000
  while (!ctl.signal.aborted) {
    let ended = ''
    try {
      const a = await call<{ deviceId: string }>('POST', '/remote-control/devices', {
        v: 1, name: '', kind: getTvMode() ? 'tv' : 'web', capabilities: CAPABILITIES, deviceId: deviceId || stored() || undefined,
      }, ctl.signal)
      deviceId = a.deviceId
      store(deviceId)
      await readEvents(`/remote-control/devices/${deviceId}/channel`, (event, data) => {
        if (event === 'hello') { online = true; wait = 2000; report() }
        else if (event === 'command') obey(data)
        else if (event === 'bye') ended = data?.reason || ''
      }, ctl.signal)
    } catch (e) {
      // Turned off on the server, or not signed in: nothing to keep trying for.
      if (e instanceof ApiError && (e.status === 401 || e.status === 403)) { online = false; return }
    }
    online = false
    if (ctl.signal.aborted || ended === 'signed-out') return
    await sleep(wait, ctl.signal)
    wait = Math.min(wait * 2, 30000)
  }
}

function stop(forget: boolean) {
  run?.abort()
  run = null
  if (beat) { clearInterval(beat); beat = null }
  if (reportTimer) { clearTimeout(reportTimer); reportTimer = null }
  if (forget && deviceId && online) call('DELETE', `/remote-control/devices/${deviceId}`).catch(() => {})
  online = false
}

// The channel is open while someone could want it: the page is showing, or something is
// playing in it. A tab left in the background with nothing playing holds no connection.
function sync() {
  const want = wanted && targetEnabled() && (document.visibilityState !== 'hidden' || !!player)
  if (want && !run) {
    const ctl = new AbortController()
    run = ctl
    loop(ctl).finally(() => { if (run === ctl) run = null })
    // The heartbeat, and while something plays, where it has got to.
    beat = setInterval(() => { if (Date.now() - lastReport >= (player ? 5000 : 25000)) report() }, 2500)
  } else if (!want && run) stop(false)
}

// startDevice and stopDevice follow the sign-in.
export function startDevice() {
  wanted = true
  document.addEventListener('visibilitychange', sync)
  sync()
}
export function stopDevice() {
  wanted = false
  document.removeEventListener('visibilitychange', sync)
  stop(true)
  // Signed out, or another profile: whoever is here next is a new device, not this one.
  deviceId = ''
  try { sessionStorage.removeItem(ID_KEY) } catch { /* nothing was kept */ }
}
