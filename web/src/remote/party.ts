// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A watch party, from this browser's side: the room as the server last described it, the
// server's clock, and keeping a player in step with both. docs/SYNCPLAY.md has the rules;
// the drift arithmetic is the live channels' own, in player/sync.ts.
import { useEffect, useRef, useSyncExternalStore } from 'react'
import { ApiError } from '../api'
import { clockOffset, syncStep, type ClockSample } from '../player/sync'
import { toast } from '../state'
import { call, readEvents, sleep, steadyNow } from './stream'

export type PartyMember = { id: string; userId: number; name: string; host: boolean; you?: boolean; state: 'ready' | 'loading' | 'buffering' | 'lagging' | 'away' | 'blocked' | 'time-up'; rttMs: number }
export type PartyState = { rev: number; seq: number; itemId: number; index: number; positionSec: number; atMs: number; paused: boolean; waiting: boolean; rate: number }
export type PartyRoom = {
  id: string; code: string; name: string; policy: 'host' | 'everyone'; waitForAll: boolean; hostId: string; you: string; canControl: boolean; blocked: boolean
  // This profile's screen time is over: it sits out, keeping its place, until it has more.
  timeUp?: boolean
  members: PartyMember[]; queue: number[]; items: { id: number; title?: string; kind?: string; runtimeSec?: number }[]; state: PartyState; serverTimeMs: number; leadMs: number
}
export type PartyListing = { id: string; name: string; hostName: string; members: number; memberId?: string; invited?: boolean }
// canJoin is false when watch parties are turned off for this profile. An older server doesn't say.
export type PartyInfo = { enabled: boolean; canCreate: boolean; canJoin?: boolean; rooms: PartyListing[] }

export const MEMBER_WORDS: Record<PartyMember['state'], string> = {
  ready: 'Ready', loading: 'Loading', buffering: 'Buffering', lagging: 'Catching up', away: 'Away', blocked: 'Can’t watch this one', 'time-up': 'Out of screen time',
}

const ROOM_KEY = 'wms.party.room'
const TAB_KEY = 'wms.party.tab'

// What tells this window's seat from another window's of the same sign-in.
function tab() {
  try {
    let t = sessionStorage.getItem(TAB_KEY)
    if (!t) { t = Math.random().toString(36).slice(2, 10); sessionStorage.setItem(TAB_KEY, t) }
    return t
  } catch { return 'tab' }
}
const remember = (id: string) => { try { if (id) sessionStorage.setItem(ROOM_KEY, id); else sessionStorage.removeItem(ROOM_KEY) } catch { /* this visit only */ } }
const remembered = () => { try { return sessionStorage.getItem(ROOM_KEY) || '' } catch { return '' } }

let room: PartyRoom | null = null
let offset = 0 // how far the server's clock is ahead of this one, ms
let rtt = 0
let samples: ClockSample[] = []
let run: AbortController | null = null
const listeners = new Set<() => void>()

const publish = () => listeners.forEach((l) => l())
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export const useParty = () => useSyncExternalStore(subscribe, () => room)
export const currentParty = () => room
export const serverNow = () => steadyNow() + offset

// Where the room is at a server time: the one sum every client does.
export function positionAt(s: PartyState, serverMs: number) {
  if (s.paused || s.waiting || serverMs <= s.atMs) return s.positionSec
  return s.positionSec + ((serverMs - s.atMs) / 1000) * s.rate
}

function take(r: PartyRoom) {
  // The stream and the answers to commands both carry the room; the newest wins.
  if (room && room.id === r.id && r.state.rev < room.state.rev) return
  room = r
  publish()
}

function gone(why?: string) {
  run?.abort()
  run = null
  room = null
  samples = []
  remember('')
  publish()
  if (why) toast(why)
}

async function ping(ctl: AbortController) {
  const r = room
  if (!r) return
  const sent = steadyNow()
  try {
    const a = await call<{ t1: number; t2: number }>('POST', `/syncplay/rooms/${r.id}/ping`, { member: r.you, t0: sent, rttMs: rtt }, ctl.signal)
    const got = steadyNow()
    // The server's two times are a fraction of a millisecond apart: their middle is "the server's time".
    samples = [...samples.slice(-7), { sent, server: (a.t1 + a.t2) / 2, got }]
    offset = clockOffset(samples)
    rtt = Math.min(...samples.map((s) => s.got - s.sent))
  } catch { /* the next one will do */ }
}

async function pings(ctl: AbortController) {
  // A few quickly, to have a good offset before anything plays; then a steady heartbeat.
  for (let i = 0; i < 5 && !ctl.signal.aborted; i++) { await ping(ctl); await sleep(300, ctl.signal) }
  while (!ctl.signal.aborted) { await sleep(5000, ctl.signal); if (!ctl.signal.aborted) await ping(ctl) }
}

const CLOSED: Record<string, string> = {
  closed: 'The watch party has ended.', removed: 'You were removed from the watch party.', 'signed-out': 'The watch party ended for this device.',
  'server-stopping': 'The server is restarting, which ends the watch party.',
  'feature-off': 'Watch parties have been turned off for this profile.',
}

async function listen(ctl: AbortController) {
  pings(ctl)
  let wait = 1000
  while (!ctl.signal.aborted && room) {
    let closed = ''
    try {
      await readEvents(`/syncplay/rooms/${room.id}/events?member=${encodeURIComponent(room.you)}`, (event, data) => {
        if (event === 'room') { wait = 1000; take(data) }
        else if (event === 'closed') closed = data?.reason || 'closed'
      }, ctl.signal)
    } catch (e) {
      if (e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 429) closed = 'closed'
    }
    if (ctl.signal.aborted) return
    // Another window took this seat: this one steps back without a word.
    if (closed === 'replaced') { gone(); return }
    if (closed) { gone(CLOSED[closed] || CLOSED.closed); return }
    // The connection dropped. The seat is kept for two minutes: take it again.
    await sleep(wait, ctl.signal)
    wait = Math.min(wait * 2, 15000)
    if (ctl.signal.aborted || !room) return
    try { take(await call<PartyRoom>('POST', '/syncplay/join', { roomId: room.id, device: tab() }, ctl.signal)) } catch (e) {
      if (e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 429) { gone(CLOSED.closed); return }
    }
  }
}

function enter(r: PartyRoom) {
  run?.abort()
  room = r
  samples = []
  remember(r.id)
  publish()
  const ctl = new AbortController()
  run = ctl
  listen(ctl)
}

export const partyInfo = () => call<PartyInfo>('GET', `/syncplay?device=${encodeURIComponent(tab())}`)

export async function createParty(itemId: number, everyone: boolean, positionSec = 0) {
  enter(await call<PartyRoom>('POST', '/syncplay/rooms', { itemId, positionSec, policy: everyone ? 'everyone' : 'host', device: tab() }))
}

export async function joinParty(key: { code: string } | { roomId: string }) {
  enter(await call<PartyRoom>('POST', '/syncplay/join', { ...key, device: tab() }))
}

// A page reload keeps the seat: the room is taken up again where it was.
export async function resumeParty() {
  const id = remembered()
  if (!id || room) return
  try { await joinParty({ roomId: id }) } catch { remember('') }
}

export function leaveParty() {
  const r = room
  if (!r) return
  call('POST', `/syncplay/rooms/${r.id}/leave`, { member: r.you }).catch(() => {})
  gone()
}

// Signing out: the stream stops, and the server gives the seat up when it notices.
export function dropParty() { if (room) gone() }

export async function endParty() {
  const r = room
  if (!r) return
  await call('DELETE', `/syncplay/rooms/${r.id}?member=${encodeURIComponent(r.you)}`)
  gone()
}

// A command to the room. The answer is the room after it, which everyone's player then
// follows, this one's included.
export async function partyCommand(type: string, more: Record<string, unknown> = {}) {
  const r = room
  if (!r) return
  try {
    take(await call<PartyRoom>('POST', `/syncplay/rooms/${r.id}/command`, { member: r.you, type, ...more }))
  } catch (e) {
    // Someone else got there first: their change is the room now.
    if (e instanceof ApiError && e.data?.code === 'stale' && e.data.room) { take(e.data.room); return }
    throw e
  }
}

export const partySet = (s: { policy?: 'host' | 'everyone'; waitForAll?: boolean }) => {
  const r = room
  return r ? call('PUT', `/syncplay/rooms/${r.id}`, { member: r.you, ...s }) : Promise.resolve()
}
export const partyHost = (to: string) => { const r = room; return r ? call('POST', `/syncplay/rooms/${r.id}/host`, { member: r.you, to }) : Promise.resolve() }
export const partyKick = (whom: string) => { const r = room; return r ? call('POST', `/syncplay/rooms/${r.id}/kick`, { member: r.you, whom }) : Promise.resolve() }

function reportState(state: 'ready' | 'buffering', rev: number) {
  const r = room
  if (r) call('POST', `/syncplay/rooms/${r.id}/report`, { member: r.you, state, rev }).catch(() => {})
}

// Which item this window's player is following the party with, 0 for none: so that the
// app knows whether the party's item is already open, full screen or in the corner.
let following = 0
export const partyPlayerItem = () => following

type PlayerHooks = {
  // seek moves the player; offset turns the video's own time into the title's; loaded says
  // whether the player has a stream open at all.
  seek: (t: number) => void
  offset: () => number
  loaded: () => boolean
  // blocked: the browser won't start playback without a tap, so the player should ask for one.
  blocked?: () => void
  // cheapSeek: a seek is quick (a file, or a stream listed whole), so a paused picture can be
  // put on the exact frame. False for a stream that starts again from wherever it's sent.
  cheapSeek?: () => boolean
}

// usePartySync keeps a video element in step with the room while the room is playing
// this item, and turns what the viewer does (play, pause, a seek, reaching the end) into
// commands for everyone. `active` says whether it is doing so; `seek` is what the
// player's own seeking should go through while it is.
export function usePartySync(video: React.RefObject<HTMLVideoElement | null>, itemId: number, hooks: PlayerHooks) {
  const r = useParty()
  const active = !!r && !r.blocked && r.state.itemId === itemId
  const latest = useRef(hooks)
  latest.current = hooks
  const roomId = active ? r!.id : ''

  useEffect(() => {
    if (!roomId) return
    const v = video.current
    if (!v) return
    following = itemId
    let quietUntil = 0 // what this file does to the player isn't the viewer doing it
    let nudging = false
    let said = 0, toldOff = 0, stalledSince = 0
    // A seek costs time, most of all on a stream the server is converting, which starts
    // again from the new place. So a seek while playing aims ahead by as long as the last
    // one took, and another isn't tried until this one has had its chance.
    let seekAt = 0, lag = 0
    const quiet = (fn: () => void) => { quietUntil = performance.now() + 700; fn() }
    let lastStill = -1
    const jump = (target: number, playing: boolean) => {
      if (v.seeking || (seekAt && performance.now() - seekAt < 5000)) return
      // A paused picture is sent to a place once: if it can't get there (past the end), asking again won't help.
      if (!playing) { if (target === lastStill) return; lastStill = target } else lastStill = -1
      seekAt = performance.now()
      quiet(() => latest.current.seek(target + (playing ? lag : 0)))
    }
    const mine = () => { const cur = currentParty(); return cur && cur.id === roomId && !cur.blocked && cur.state.itemId === itemId ? cur : null }
    const notYours = () => {
      if (Date.now() - toldOff > 8000) { toldOff = Date.now(); toast('Only the host controls this watch party.') }
    }

    let startTimer: ReturnType<typeof setTimeout> | undefined
    const tick = () => {
      const cur = mine()
      if (!cur || !latest.current.loaded()) return
      const s = cur.state
      const now = serverNow()
      const target = positionAt(s, now)
      const pos = v.currentTime + latest.current.offset()
      const playing = !s.paused && !s.waiting && now >= s.atMs
      if (!playing) {
        if (!v.paused) quiet(() => v.pause())
        if (Math.abs(target - pos) > (latest.current.cheapSeek?.() ? 0.08 : 0.5)) jump(target, false)
        // A start set for a moment from now is kept to the millisecond, not to the next tick.
        if (!s.paused && !s.waiting && now < s.atMs) { clearTimeout(startTimer); startTimer = setTimeout(tick, Math.max(0, s.atMs - now)) }
        if (nudging) { nudging = false; v.playbackRate = s.rate }
      } else if (!v.ended) {
        if (v.paused) quiet(() => { v.play().catch((e) => { if (e?.name === 'NotAllowedError') latest.current.blocked?.() }) })
        const step = syncStep(target - pos, nudging)
        if ('seekBy' in step) {
          jump(target, true)
          nudging = false
          v.playbackRate = s.rate
        } else {
          nudging = step.rate !== 1
          v.playbackRate = s.rate * step.rate
        }
      }
      // Telling the room how this player is doing, when the room has it wrong.
      const me = cur.members.find((m) => m.id === cur.you)
      const ready = v.readyState >= 3 && !v.seeking && Math.abs(target - (v.currentTime + latest.current.offset())) < 2
      if (ready) {
        stalledSince = 0
        if (seekAt) { lag = Math.min(10, (performance.now() - seekAt) / 1000); seekAt = 0 }
      }
      else if (playing && !stalledSince) stalledSince = Date.now()
      if (Date.now() - said < 1000 || !me) return
      if (ready && me.state !== 'ready') { said = Date.now(); reportState('ready', s.rev) }
      else if (!ready && playing && me.state === 'ready' && Date.now() - stalledSince > 800) { said = Date.now(); reportState('buffering', s.rev) }
    }

    // Play and pause reach the video from many places (the controls, keys, the remote,
    // the system's media keys), so they are noticed here rather than at each.
    const onPause = () => {
      const cur = mine()
      if (!cur || performance.now() < quietUntil || v.ended || cur.state.paused || cur.state.waiting) return
      if (cur.canControl) partyCommand('pause').catch(() => {})
      else notYours() // the next tick sets it going again
    }
    const onPlay = () => {
      const cur = mine()
      if (!cur || performance.now() < quietUntil || !cur.state.paused) return
      if (cur.canControl) partyCommand('play').catch(() => {})
      else notYours()
    }
    const onEnded = () => {
      const cur = mine()
      // Every player reaches the end together and says "next"; the seq lets one of them count.
      if (cur?.canControl && cur.state.index < cur.queue.length - 1) partyCommand('next', { ifSeq: cur.state.seq }).catch(() => {})
    }
    v.addEventListener('pause', onPause)
    v.addEventListener('play', onPlay)
    v.addEventListener('ended', onEnded)
    const t = setInterval(tick, 500)
    // And at once when the room changes, rather than up to half a second later.
    const unsubscribe = subscribe(tick)
    tick()
    return () => {
      clearInterval(t)
      clearTimeout(startTimer)
      unsubscribe()
      v.removeEventListener('pause', onPause)
      v.removeEventListener('play', onPlay)
      v.removeEventListener('ended', onEnded)
      v.playbackRate = v.defaultPlaybackRate || 1
      if (following === itemId) following = 0
      // Closing the player is leaving the party; the party moving on to something else,
      // which opens another player, is not.
      setTimeout(() => {
        const cur = currentParty()
        if (cur && cur.id === roomId && !following && cur.state.itemId === itemId && !location.pathname.startsWith('/play/')) leaveParty()
      }, 1500)
    }
  }, [roomId, itemId, video])

  const seek = (t: number) => {
    const cur = currentParty()
    if (!cur) return
    if (!cur.canControl) { toast('Only the host controls this watch party.'); return }
    partyCommand('seek', { positionSec: Math.max(0, t) }).catch((e) => toast((e as Error).message))
  }
  return { active, room: active ? r : null, seek }
}
