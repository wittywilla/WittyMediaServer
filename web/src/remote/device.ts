// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// This browser as something other devices can play on: the device's side of the Remote
// Protocol (docs/REMOTE-PROTOCOL.md). It announces itself with what it can really play,
// keeps the channel open, carries out `play` and `navigate` itself, hands every other
// command to whichever player is open, and says what that player is doing.
import { api, ApiError, type Item } from '../api'
import type { AudioMode } from '../audio/types'
import { press } from '../tv'
import { getTvMode } from '../tvmode/mode'
import { accepts, checkQueue, deviceFeatures, isNavKey, keyboardKey, mediaKindsFor, playerFor, queueFamily, refusal, routeFor, screenFor, type LibraryKindLike, type PlayerKind } from './capabilities'
import { makeQueue, nextId, reported } from './queue'
import { call, readEvents, sleep } from './stream'
import { queuedBefore, queueFor, setVideoQueue } from './videoQueue'

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

// What an open player gives this file: which player it is, how to obey, and what to report.
// A command it can't carry out throws, with words for the person holding the other device.
// background: it plays under the pages (the music bar), so a player that fills the screen
// is the one a remote steers while both are open.
export type RemotePlayer = { kind: PlayerKind; itemId: number; background?: boolean; command: (cmd: RemoteCommand) => void | Promise<void>; state: () => RemoteState }

// What the app gives this file so that `play` can open things: the router, and the music
// player, which has no page of its own.
export type RemoteOpener = {
  go: (to: string, replace?: boolean) => void
  playAudio: (items: Item[], index: number, mode: AudioMode, at?: number) => void
  libraryType: (libraryId: number) => string | undefined
}

const ID_KEY = 'wms.remote.device'
const OFF_KEY = 'wms.remote.target'

const players: RemotePlayer[] = []
let opener: RemoteOpener | null = null
// What was announced: what this browser can play against this server, and so what it can do.
let caps = { mediaKinds: mediaKindsFor(null), features: deviceFeatures(mediaKindsFor(null)) }
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
// And whether or not it is: how this browser knows itself in a list of devices.
export const ownDeviceId = () => deviceId || stored()

export function setRemoteOpener(o: RemoteOpener | null) { opener = o }

// The player a remote is steering: the one that fills the screen, opened last; else the one
// playing underneath.
function active(): RemotePlayer | null {
  for (let i = players.length - 1; i >= 0; i--) if (!players[i].background) return players[i]
  return players[players.length - 1] || null
}

const idle = (): RemoteState => ({ v: 1, status: 'idle', screen: screenFor(location.pathname) })

function stateOf(p: RemotePlayer): RemoteState {
  const st: RemoteState = { v: 1, ...p.state() }
  // The video player's queue is kept here, not by the player.
  if (p.kind === 'video' || p.kind === 'disc') Object.assign(st, reported(queueFor(p.itemId)))
  return st
}

function report() {
  reportTimer = null
  if (!online || !deviceId) return
  lastReport = Date.now()
  const p = active()
  let st: RemoteState
  try { st = p ? stateOf(p) : idle() } catch { st = idle() }
  call('POST', `/remote-control/devices/${deviceId}/state`, st).catch(() => { /* the next one says it */ })
}

// reportSoon says what's playing, shortly: at once if nothing was said lately, and
// otherwise once the server's limit allows, with whatever is true by then.
export function reportSoon() {
  sync()
  if (!online || reportTimer) return
  reportTimer = setTimeout(report, Math.max(0, 1000 - (Date.now() - lastReport)))
}

// A player opens: commands go to it and its state is what's reported, until it closes.
export function registerPlayer(p: RemotePlayer): () => void {
  players.push(p)
  reportSoon()
  return () => {
    const i = players.indexOf(p)
    if (i < 0) return
    players.splice(i, 1)
    // The video player closing for good ends its queue; closing to open the next item doesn't.
    // The player's page is still there for the next one, so look once this change has settled.
    if (p.kind === 'video' || p.kind === 'disc') setTimeout(() => { if (!document.querySelector('.player:not(.ctv-player)')) setVideoQueue(null) }, 0)
    reportSoon()
  }
}

// Items are asked for a few at a time: a queue can be 200 long, and the answer to `play` is due in seconds.
async function loadItems(ids: number[], have: Item): Promise<Item[]> {
  const out = new Array<Item>(ids.length)
  let at = 0
  const one = async () => {
    for (let i = at++; i < ids.length; i = at++) out[i] = ids[i] === have.id ? have : await api.get<Item>(`/items/${ids[i]}`)
  }
  await Promise.all(Array.from({ length: Math.min(8, ids.length) }, one))
  return out
}

// play: open the item in the player for its kind (openItem.tsx chooses the same way), at the
// place asked for. A queue goes to the player that keeps one for that kind of thing.
async function play(cmd: RemoteCommand) {
  const ids = cmd.queue?.length ? cmd.queue : cmd.itemId ? [cmd.itemId] : []
  const index = cmd.queue?.length ? cmd.queueIndex || 0 : 0
  const id = ids[index]
  if (!id || !opener) throw new Error('This device can’t open that.')
  const first = await api.get<Item>(`/items/${id}`)
  const player = playerFor(first.kind)
  if (!player || !caps.mediaKinds.includes(first.kind)) throw new Error('This device can’t play that kind of thing.')
  const family = queueFamily(player)
  const items = ids.length > 1 ? await loadItems(ids, first) : [first]
  const wrong = checkQueue(items.map((it) => it.kind))
  if (wrong) throw new Error(wrong)
  const options = { positionSec: cmd.positionSec, audio: cmd.audio, subtitle: cmd.subtitle, remote: cmd.id || String(Date.now()) }
  if (family === 'audio' || family === 'radio') {
    // A station as the radio pages have it: with where its stream is.
    const list = family === 'radio' ? await Promise.all(items.map((it) => api.get<Item>(`/radio/${it.id}`))) : items
    const type = opener.libraryType(first.libraryId)
    const mode: AudioMode = family === 'radio' ? 'radio' : type === 'audiobooks' || type === 'podcasts' ? type : 'music'
    opener.playAudio(list, index, mode, family === 'radio' ? 0 : cmd.positionSec || 0)
    return
  }
  const to = routeFor(first, options)
  if (!to) throw new Error('That can’t be opened in this browser.')
  if (family === 'video') setVideoQueue(makeQueue(ids, index))
  opener.go(to)
}

// navigate: the key is pressed in this browser's interface as a TV remote's or a
// controller's would be (tv.ts), so it does whatever that key does wherever the focus is: in
// TV mode, on a page, in a disc's menu, in a player.
async function pressKey(cmd: RemoteCommand) {
  if (!isNavKey(cmd.key)) throw new Error('This device doesn’t know that key.')
  const p = active()
  if (p?.kind === 'game') throw new Error(refusal('game', 'navigate'))
  if (cmd.key === 'home') { if (!opener) throw new Error('This device can’t go there.'); opener.go('/'); return }
  if (cmd.key === 'playpause') {
    if (p && accepts(p.kind, 'pause')) { await p.command({ type: p.state().status === 'paused' ? 'resume' : 'pause' }); return }
    press('MediaPlayPause')
    return
  }
  // Show where the focus is, as for any remote.
  document.documentElement.classList.add('using-controller')
  press(keyboardKey(cmd.key)!)
}

// The video player's queue: next and previous move along it before they mean anything else.
function stepQueue(p: RemotePlayer, cmd: RemoteCommand): boolean {
  if (p.kind !== 'video' || !opener) return false
  const q = queueFor(p.itemId)
  const to = cmd.type === 'next' ? nextId(q) : cmd.type === 'previous' && (p.state().positionSec || 0) < 5 ? queuedBefore(p.itemId) : null
  if (!to) return false
  opener.go(`/play/${to}?remote=${encodeURIComponent(cmd.id || String(Date.now()))}`, true)
  return true
}

async function obey(cmd: RemoteCommand) {
  let ok = true, error = ''
  try {
    const p = active()
    if (cmd.type === 'play') await play(cmd)
    else if (cmd.type === 'navigate') await pressKey(cmd)
    else if (!p) { if (cmd.type !== 'stop') throw new Error('Nothing is playing there.') }
    else if (cmd.type !== 'stop' && !accepts(p.kind, cmd.type)) throw new Error(refusal(p.kind, cmd.type))
    else if (!stepQueue(p, cmd)) await p.command(cmd)
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
      // What this server has decides what this browser says it plays. Asked each time the
      // device announces itself, so a module added since is claimed on the next connection.
      const kinds = await call<{ libraries: LibraryKindLike[] }>('GET', '/modules/libraries', undefined, ctl.signal).then((r) => r.libraries || [], () => null)
      const mediaKinds = mediaKindsFor(kinds)
      caps = { mediaKinds, features: deviceFeatures(mediaKinds) }
      const a = await call<{ deviceId: string }>('POST', '/remote-control/devices', {
        v: 1, name: '', kind: getTvMode() ? 'tv' : 'web', capabilities: caps, deviceId: deviceId || stored() || undefined,
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
  const want = wanted && targetEnabled() && (document.visibilityState !== 'hidden' || players.some(inUse))
  if (want && !run) {
    const ctl = new AbortController()
    run = ctl
    loop(ctl).finally(() => { if (run === ctl) run = null })
    // The heartbeat, and while something plays, where it has got to.
    beat = setInterval(() => { if (Date.now() - lastReport >= (players.length ? 5000 : 25000)) report() }, 2500)
  } else if (!want && run) stop(false)
}

// A player that keeps a hidden tab connected: one that fills the screen, or music that is sounding.
// Music left paused in a tab nobody is looking at holds nothing open.
function inUse(p: RemotePlayer): boolean {
  if (!p.background) return true
  try { return p.state().status === 'playing' } catch { return false }
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
}
