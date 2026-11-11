// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A watch party, from this browser's side: the room as the server last described it, the
// server's clock, and keeping a player in step with both. docs/SYNCPLAY.md has the rules;
// the drift arithmetic is the live channels' own, in player/sync.ts.
import { useEffect, useRef, useSyncExternalStore } from 'react'
import { ApiError } from '../api'
import { clockOffset, type ClockSample } from '../player/sync'
import { toast } from '../state'
import { maySeek, newSeekMemo, partyStep, positionAt, seekAim, seekLanded, shouldAdvance, type Listing, type PartyState, type QueueEdit } from './partyLogic'
import { call, readEvents, sleep, steadyNow } from './stream'

export { positionAt, type PartyState }
export type PartyMember = { id: string; userId: number; name: string; host: boolean; you?: boolean; state: 'ready' | 'loading' | 'buffering' | 'lagging' | 'away' | 'blocked'; rttMs: number }
export type PartyQueueItem = { id: number; title?: string; kind?: string; runtimeSec?: number }
export type PartyRoom = {
  id: string; code: string; name: string; policy: 'host' | 'everyone'; waitForAll: boolean; hostId: string; you: string; canControl: boolean; blocked: boolean
  members: PartyMember[]; queue: number[]; items: PartyQueueItem[]; state: PartyState; serverTimeMs: number; leadMs: number
}
export type PartyListing = Listing
export type PartyInfo = { enabled: boolean; canCreate: boolean; rooms: PartyListing[]; limits?: { queue?: number; members?: number; invites?: number; codeLength?: number } }

export const MEMBER_WORDS: Record<PartyMember['state'], string> = {
  ready: 'Ready', loading: 'Loading', buffering: 'Buffering', lagging: 'Catching up', away: 'Away', blocked: 'Can’t watch this one',
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
export const subscribeParty = subscribe
export const currentParty = () => room
export const serverNow = () => steadyNow() + offset

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
  resumed = false
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

// items: what the party plays through, starting at items[index] (a film alone; an episode
// and the rest of its season; the songs in a queue).
export async function createParty(items: number[], index: number, everyone: boolean, positionSec = 0) {
  enter(await call<PartyRoom>('POST', '/syncplay/rooms', { items, index, positionSec, policy: everyone ? 'everyone' : 'host', device: tab() }))
}

export async function joinParty(key: { code: string } | { roomId: string }) {
  enter(await call<PartyRoom>('POST', '/syncplay/join', { ...key, device: tab() }))
}

// A page reload keeps the seat: the room is taken up again where it was.
export async function resumeParty() {
  const id = remembered()
  if (!id || room) return
  try { resumed = true; await joinParty({ roomId: id }) } catch { resumed = false; remember('') }
}
// True once after a reload has taken the seat back: the page's address still says where
// the party was when the player was first opened, not where it is now.
let resumed = false
export function takeResumed() { const was = resumed; resumed = false; return was }

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
// follows, this one's included. False: someone else changed the room first (the command
// carried the seq this window had seen), and their change is the room now.
export async function partyCommand(type: string, more: Record<string, unknown> = {}): Promise<boolean> {
  const r = room
  if (!r) return false
  try {
    take(await call<PartyRoom>('POST', `/syncplay/rooms/${r.id}/command`, { member: r.you, type, ...more }))
    return true
  } catch (e) {
    if (e instanceof ApiError && e.data?.code === 'stale' && e.data.room) { take(e.data.room); return false }
    throw e
  }
}

// A new queue, worked out from the one this window was looking at: refused if the room
// has changed since, so two people editing at once can't undo each other without seeing it.
export const partyQueue = (e: QueueEdit) => partyCommand('queue', { items: e.items, index: e.index, ifSeq: room?.state.seq })
// Along the queue. With the seq seen: two people pressing Next at once move on once.
export const partyStepQueue = (type: 'next' | 'previous') => partyCommand(type, { ifSeq: room?.state.seq })
export const partySelect = (index: number) => partyCommand('select', { index, ifSeq: room?.state.seq })

export const partySet = (s: { policy?: 'host' | 'everyone'; waitForAll?: boolean }) => {
  const r = room
  return r ? call('PUT', `/syncplay/rooms/${r.id}`, { member: r.you, ...s }) : Promise.resolve()
}
export const partyHost = (to: string) => { const r = room; return r ? call('POST', `/syncplay/rooms/${r.id}/host`, { member: r.you, to }) : Promise.resolve() }
export const partyKick = (whom: string) => { const r = room; return r ? call('POST', `/syncplay/rooms/${r.id}/kick`, { member: r.you, whom }) : Promise.resolve() }
// Those accounts find the party in their own list, and may join it without the code.
export const partyInvite = (userIds: number[]) => { const r = room; return r ? call<{ invited: number }>('POST', `/syncplay/rooms/${r.id}/invite`, { member: r.you, userIds }) : Promise.resolve({ invited: 0 }) }

// One report at a time: while the server is slow to answer, more of them would only queue
// up behind the first and arrive together, as noise.
let reporting = false
function reportState(state: 'ready' | 'buffering', rev: number) {
  const r = room
  if (!r || reporting) return
  reporting = true
  call('POST', `/syncplay/rooms/${r.id}/report`, { member: r.you, state, rev }).catch(() => {}).finally(() => { reporting = false })
}

// Which item this window's player is following the party with, 0 for none: so that the
// app knows whether the party's item is already open, full screen or in the corner.
let following = 0
export const partyPlayerItem = () => following

type PlayerHooks = {
  // seek moves the player; offset turns the element's own time into the title's; loaded
  // says whether the player has a stream open at all.
  seek: (t: number) => void
  offset: () => number
  loaded: () => boolean
  // The title's length in seconds, 0 when not known.
  duration?: () => number
  // blocked: the browser won't start playback without a tap, so the player should ask for one.
  blocked?: () => void
  // cheapSeek: a seek is quick (a file, or a stream listed whole), so a paused picture can be
  // put on the exact frame. False for a stream that starts again from wherever it's sent.
  cheapSeek?: () => boolean
}

// usePartySync keeps a video or audio element in step with the room while the room is
// playing this item, and turns what the person does (play, pause, a seek, reaching the
// end) into commands for everyone. `active` says whether it is doing so; `seek` and `next`
// are what the player's own seeking and "next" should go through while it is. The rules
// are partyLogic.ts's; this is their hands.
export function usePartySync(media: React.RefObject<HTMLMediaElement | null>, itemId: number, hooks: PlayerHooks) {
  const r = useParty()
  const active = !!r && !r.blocked && itemId > 0 && r.state.itemId === itemId
  const latest = useRef(hooks)
  latest.current = hooks
  const roomId = active ? r!.id : ''

  useEffect(() => {
    if (!roomId) return
    const v = media.current
    if (!v) return
    following = itemId
    let quietUntil = 0 // what this file does to the player isn't the person doing it
    let nudging = false
    let said = 0, toldOff = 0, stalledSince = 0, advanced = { seq: -1, at: 0 }
    const memo = newSeekMemo()
    // Opening the stream is a seek in all but name: how long it takes is how far behind a
    // late arrival lands, and so how far ahead its catching-up seek should aim.
    if (!latest.current.loaded() || v.readyState < 3) memo.at = performance.now()
    const quiet = (fn: () => void) => { quietUntil = performance.now() + 700; fn() }
    const mine = () => { const cur = currentParty(); return cur && cur.id === roomId && !cur.blocked && cur.state.itemId === itemId ? cur : null }
    const notYours = () => {
      if (Date.now() - toldOff > 8000) { toldOff = Date.now(); toast('Only the host controls this watch party.') }
    }

    let startTimer: ReturnType<typeof setTimeout> | undefined
    const tick = () => {
      const cur = mine()
      if (!cur) return
      const h = latest.current
      const s = cur.state
      const now = serverNow()
      const local = performance.now()
      const loaded = h.loaded()
      const cheap = h.cheapSeek ? h.cheapSeek() : true
      const duration = h.duration?.() || 0
      const live = { seeking: v.seeking, canPlay: v.readyState >= 3, loaded }
      const st = partyStep(s, now, { pos: v.currentTime + h.offset(), duration, ended: v.ended, seeking: v.seeking, canPlay: live.canPlay }, { nudging, cheapSeek: cheap })
      if (loaded) {
        seekLanded(memo, local, live)
        nudging = st.nudging
        if (!st.play) {
          if (!v.paused) quiet(() => v.pause())
          // A start set for a moment from now is kept to the millisecond, not to the next tick.
          if (st.startInMs != null) { clearTimeout(startTimer); startTimer = setTimeout(tick, st.startInMs) }
        } else if (v.paused && !v.ended) {
          quiet(() => { v.play().catch((e) => { if (e?.name === 'NotAllowedError') h.blocked?.() }) })
        }
        if (st.seekTo != null && maySeek(memo, local, st.seekTo, st.play, live, cheap)) {
          const to = seekAim(memo, st.seekTo, st.play, duration)
          quiet(() => h.seek(to))
        }
        if (v.playbackRate !== st.rate) v.playbackRate = st.rate
      }
      // The title has run out: on to the next in the queue, once for the whole room.
      if (shouldAdvance(cur, now, { ended: loaded && v.ended, duration }) && (advanced.seq !== s.seq || local - advanced.at > 5000)) {
        advanced = { seq: s.seq, at: local }
        partyCommand('next', { ifSeq: s.seq }).catch(() => {})
      }
      // Telling the room how this player is doing, when the room has it wrong. A seek still
      // on its way isn't ready, however near the place the old stream happens to be.
      const me = cur.members.find((m) => m.id === cur.you)
      const ready = st.over || (loaded && st.ready && memo.at === 0)
      if (ready) stalledSince = 0
      else if (st.play && !stalledSince) stalledSince = Date.now()
      if (Date.now() - said < 1000 || !me) return
      if (ready && me.state !== 'ready') { said = Date.now(); reportState('ready', s.rev) }
      else if (!ready && st.play && me.state === 'ready' && Date.now() - stalledSince > 800) { said = Date.now(); reportState('buffering', s.rev) }
    }

    // Play and pause reach the element from many places (the controls, keys, the remote,
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
    v.addEventListener('pause', onPause)
    v.addEventListener('play', onPlay)
    // Reaching the end, or having loaded, is acted on at once rather than at the next tick.
    const soon = ['ended', 'canplay', 'seeked'] as const
    soon.forEach((e) => v.addEventListener(e, tick))
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
      soon.forEach((e) => v.removeEventListener(e, tick))
      v.playbackRate = v.defaultPlaybackRate || 1
      if (following === itemId) following = 0
      // Closing the player is leaving the party; the party moving on to something else,
      // which opens another player, is not.
      setTimeout(() => {
        const cur = currentParty()
        if (cur && cur.id === roomId && !following && cur.state.itemId === itemId && !location.pathname.startsWith('/play/')) leaveParty()
      }, 1500)
    }
  }, [roomId, itemId, media])

  const controls = (): PartyRoom | null => {
    const cur = currentParty()
    if (!cur) return null
    if (!cur.canControl) { toast('Only the host controls this watch party.'); return null }
    return cur
  }
  const seek = (t: number) => {
    if (controls()) partyCommand('seek', { positionSec: Math.max(0, t) }).catch((e) => toast((e as Error).message))
  }
  // The next thing in the party's queue, for everyone.
  const next = () => {
    if (controls()) partyStepQueue('next').catch((e) => toast((e as Error).message))
  }
  const hasNext = active && r!.state.index < r!.queue.length - 1
  return { active, room: active ? r : null, seek, next, hasNext }
}
