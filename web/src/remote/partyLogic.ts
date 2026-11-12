// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The rules a watch party's player goes by, apart from any player or page: where the room
// is, what a player should do about it this moment, when a seek may be tried again, how a
// queue is edited around what is playing, and which invitations still need an answer.
// Nothing here touches the page or the clock, so partyLogic.test.mjs can walk through it.
// docs/SYNCPLAY.md has the protocol; the drift numbers are the live channels' own (player/sync.ts).
import { syncStep } from '../player/sync.ts'

export type PartyState = { rev: number; seq: number; itemId: number; index: number; positionSec: number; atMs: number; paused: boolean; waiting: boolean; rate: number }

// Where the room is at a server time: the one sum every client does.
export function positionAt(s: PartyState, serverMs: number) {
  if (s.paused || s.waiting || serverMs <= s.atMs) return s.positionSec
  return s.positionSec + ((serverMs - s.atMs) / 1000) * s.rate
}

// ---------------------------------------------------------------- what the web app keeps in step

// A room can hold any item its members may see. This app keeps two kinds in step: videos
// (films, episodes, home videos) in the video player and songs in the music player. A disc
// with menus has no one timeline to agree on, and a book or a game has none at all.
export type PartyKind = 'video' | 'music' | 'none'
export function partyKind(kind: string | undefined): PartyKind {
  return kind === 'video' ? 'video' : kind === 'audio' ? 'music' : 'none'
}

// ---------------------------------------------------------------- one look at the player

// A paused picture on a stream that seeks for nothing (a file, a stream listed whole) is
// put on the very frame; one that starts again from wherever it's sent is left within half
// a second, which costs nothing once it plays.
export const HOLD_EXACT = 0.08
export const HOLD_NEAR = 0.5
// "Ready" means loaded and within this of the room.
export const READY_WITHIN = 2
// The room's position this close to the end of the title, or past it, is the end: there is
// nothing there to load.
export const END_SLACK = 0.25

export type PlayerNow = {
  pos: number // where the player is in the title, seconds
  duration: number // of the title; 0 when not known
  ended: boolean
  seeking: boolean
  canPlay: boolean // has enough to play from where it is
}

export type PartyStep = {
  target: number // where the room is
  play: boolean // false: hold, paused, at the room's place
  seekTo: number | null // a place to go to first
  rate: number // the speed to play at: the room's, or a little either side while catching up
  nudging: boolean
  startInMs: number | null // the room starts this far in the future: look again exactly then
  over: boolean // the room has run past the end of this title
  ready: boolean // what to tell the room
}

// partyStep: what a player should be doing, given the room and where the player is.
// `nudging`: it is already playing a little fast or slow. `cheapSeek`: a seek is quick.
export function partyStep(s: PartyState, serverMs: number, p: PlayerNow, o: { nudging: boolean; cheapSeek: boolean }): PartyStep {
  const target = positionAt(s, serverMs)
  const going = !s.paused && !s.waiting
  const playing = going && serverMs >= s.atMs
  const startInMs = going && serverMs < s.atMs ? s.atMs - serverMs : null
  const over = p.duration > 0 && target >= p.duration - END_SLACK
  const behind = target - p.pos
  const loaded = p.canPlay && !p.seeking
  if (over) {
    // Nothing to load past the end: the player rests where its own end is, and isn't waited for.
    return { target, play: false, seekTo: p.ended || p.pos >= p.duration - READY_WITHIN ? null : Math.max(0, p.duration - 1), rate: s.rate, nudging: false, startInMs: null, over, ready: true }
  }
  const ready = loaded && Math.abs(behind) < READY_WITHIN
  if (!playing) {
    const tolerance = o.cheapSeek ? HOLD_EXACT : HOLD_NEAR
    return { target, play: false, seekTo: Math.abs(behind) > tolerance ? target : null, rate: s.rate, nudging: false, startInMs, over, ready }
  }
  const step = syncStep(behind, o.nudging)
  if ('seekBy' in step) return { target, play: true, seekTo: target, rate: s.rate, nudging: false, startInMs: null, over, ready }
  return { target, play: true, seekTo: null, rate: s.rate * step.rate, nudging: step.rate !== 1, startInMs: null, over, ready }
}

// ---------------------------------------------------------------- seeks take time

// A seek on a file lands in a moment. On a stream the server is converting it asks for a
// new stream from that place, which can take seconds to arrive. Asking again before it has
// arrived starts it over, and a slow server would never get there; so one seek is given its
// time, and how long it took is remembered: the next seek while the room is playing aims
// that far ahead, to land where the room will be.
export type SeekMemo = {
  at: number // when the last seek was asked for (this device's ms), 0 for none in flight
  to: number // where it was sent
  tries: number // how many times in a row the player has been sent to that same place
  lag: number // how long the last one took to land, seconds
}
export const newSeekMemo = (): SeekMemo => ({ at: 0, to: -1, tries: 0, lag: 0 })

export const SEEK_SETTLE_MS = 400 // before this the player hasn't even begun, and still looks "ready" where it was
export const CHEAP_SEEK_MS = 5000
export const RESTART_SEEK_MS = 25000
export const MAX_LAG = 12
// A paused player is sent to a place twice at most: once to get the stream there (a
// converted stream may start a little before, on a whole frame), once more to step to the
// place within it. If it still isn't there, asking again won't help.
export const HOLD_TRIES = 2

// seekLanded: the seek in flight has arrived (or there was none). Updates the memo's lag.
export function seekLanded(m: SeekMemo, nowMs: number, p: { seeking: boolean; canPlay: boolean; loaded: boolean }): boolean {
  if (!m.at) return true
  if (nowMs - m.at < SEEK_SETTLE_MS || !p.loaded || p.seeking || !p.canPlay) return false
  m.lag = Math.min(MAX_LAG, (nowMs - m.at) / 1000)
  m.at = 0
  return true
}

// maySeek: whether to send the player to `to` now. `playing`: the room is playing, so the
// place moves on while the seek is made.
export function maySeek(m: SeekMemo, nowMs: number, to: number, playing: boolean, p: { seeking: boolean; canPlay: boolean; loaded: boolean }, cheapSeek: boolean): boolean {
  if (!p.loaded) return false
  if (!seekLanded(m, nowMs, p) && nowMs - m.at < (cheapSeek ? CHEAP_SEEK_MS : RESTART_SEEK_MS)) return false
  if (p.seeking && m.at === 0) return false // the viewer's own seek, or the player's: let it finish
  if (!playing) {
    if (to === m.to && m.tries >= HOLD_TRIES) return false
    m.tries = to === m.to ? m.tries + 1 : 1
  } else m.tries = 0
  m.to = to
  m.at = nowMs
  return true
}

// seekAim: where to actually send the player. While the room plays, ahead by as long as
// the last seek took, but never past the end.
export function seekAim(m: SeekMemo, to: number, playing: boolean, duration: number): number {
  const aim = playing ? to + m.lag : to
  return duration > 0 ? Math.min(aim, Math.max(0, duration - 1)) : aim
}

// ---------------------------------------------------------------- moving on at the end

export const ADVANCE_AFTER = 2

// shouldAdvance: the title has run out and the room hasn't moved on. Every player that may
// control says "next" with the seq it saw, and the room moves once. A player's own "ended"
// is the usual sign; `target` past the end by a couple of seconds covers a player that
// never got there (it joined late, or stalled).
export function shouldAdvance(room: { canControl: boolean; queue: number[]; state: PartyState }, serverMs: number, p: { ended: boolean; duration: number }): boolean {
  const s = room.state
  if (!room.canControl || s.index >= room.queue.length - 1 || s.paused || s.waiting) return false
  if (p.ended) return true
  return p.duration > 0 && positionAt(s, serverMs) >= p.duration + ADVANCE_AFTER
}

// ---------------------------------------------------------------- the queue

export type QueueEdit = { items: number[]; index: number }

// An entry of 0 is something this account may not see. It can't send such a queue back
// (the server takes only items the sender can see), so it can't edit one.
export const queueEditable = (queue: number[]) => queue.length > 0 && queue.every((id) => id > 0)

// Each of these keeps what is playing playing: `index` follows it to its new place, and the
// server leaves a room alone whose item at `index` is the one it was.
export function queueAdd(queue: number[], index: number, ids: number[], where: 'end' | 'next', max: number): QueueEdit | null {
  const fresh = ids.filter((id, i) => id > 0 && ids.indexOf(id) === i)
  if (!fresh.length || queue.length + fresh.length > max) return null
  const at = where === 'next' ? index + 1 : queue.length
  return { items: [...queue.slice(0, at), ...fresh, ...queue.slice(at)], index }
}

// Playing something now: it goes in after what is playing, and the room moves to it.
export function queuePlayNow(queue: number[], index: number, ids: number[], max: number): QueueEdit | null {
  const e = queueAdd(queue, index, ids, 'next', max)
  return e && { items: e.items, index: index + 1 }
}

export function queueRemove(queue: number[], index: number, i: number): QueueEdit | null {
  if (i < 0 || i >= queue.length || queue.length < 2) return null
  const items = queue.filter((_, x) => x !== i)
  // Taking out what is playing moves on to what came after it, or back to the last.
  return { items, index: i < index ? index - 1 : Math.min(index, items.length - 1) }
}

export function queueMove(queue: number[], index: number, from: number, to: number): QueueEdit | null {
  if (from === to || from < 0 || to < 0 || from >= queue.length || to >= queue.length) return null
  const items = [...queue]
  const [it] = items.splice(from, 1)
  items.splice(to, 0, it)
  const now = from === index ? to : from < index && to >= index ? index - 1 : from > index && to <= index ? index + 1 : index
  return { items, index: now }
}

// The rest of a season from one episode, in order: what a party started on an episode
// plays through. Episodes with no number go by their place in the list.
export function seasonQueue(episodes: { id: number; season?: number | null; episode?: number | null }[], id: number, max: number): number[] {
  const from = episodes.find((e) => e.id === id)
  if (!from) return [id]
  const same = episodes.map((e, i) => ({ e, i })).filter((x) => (x.e.season ?? null) === (from.season ?? null))
  same.sort((a, b) => (a.e.episode ?? Infinity) - (b.e.episode ?? Infinity) || a.i - b.i)
  const at = same.findIndex((x) => x.e.id === id)
  return same.slice(at, at + max).map((x) => x.e.id)
}

// ---------------------------------------------------------------- codes and invitations

// A code as typed: capitals, and no spaces or dashes, which people add when reading one out.
export const tidyCode = (s: string) => s.toUpperCase().replace(/[^A-Z0-9]/g, '')

export type Listing = { id: string; name: string; hostName: string; members: number; memberId?: string; invited?: boolean }

// The invitations still waiting for an answer: not ones declined on this device, not the
// room this window is in, not ones this sign-in already has a seat in.
export function openInvites(rooms: Listing[], declined: string[], inRoom: string): Listing[] {
  return rooms.filter((r) => r.invited && !r.memberId && r.id !== inRoom && !declined.includes(r.id))
}

// What is remembered as declined is kept only while the server still lists it, so the
// list can't grow for ever: a party that has ended is forgotten.
export function keepDeclined(declined: string[], rooms: Listing[]): string[] {
  return declined.filter((id) => rooms.some((r) => r.id === id && r.invited))
}

// How long to wait before asking about invitations again. Half a minute while watch
// parties are on: an invitation is something people also say out loud, and a small server
// answers this for every open tab. Much longer while they're off, and longer after each
// failure, so a server that is away isn't knocked on.
export const INVITE_POLL_MS = 30000
export const INVITE_POLL_OFF_MS = 5 * 60000
export function invitePollDelay(enabled: boolean, failures: number): number {
  if (failures > 0) return Math.min(INVITE_POLL_OFF_MS, INVITE_POLL_MS * 2 ** Math.min(failures, 4))
  return enabled ? INVITE_POLL_MS : INVITE_POLL_OFF_MS
}
