// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Invitations to watch parties, noticed without anyone opening a dialog. The server has
// no stream for these (a room's stream is for its members), so GET /syncplay is asked now
// and then: only while someone is signed in and this tab is showing, every half minute
// while watch parties are on and far less often while they're off or the server is away
// (partyLogic.ts has the numbers). An invitation names the party and its host, never what
// is playing.
//
// The server has no way to decline: an invitation stands until the party ends. Declining
// is remembered here, on this device, for that party only.
import { useSyncExternalStore } from 'react'
import { invitePollDelay, keepDeclined, openInvites } from './partyLogic'
import { currentParty, partyInfo, subscribeParty, type PartyInfo, type PartyListing } from './party'

const KEY = 'wms.party.declined.'

let user = 0
let info: PartyInfo | null = null
let invites: PartyListing[] = []
let timer: ReturnType<typeof setTimeout> | undefined
let failures = 0
let asking = false
let askedAt = 0
let stopParty: (() => void) | null = null
const listeners = new Set<() => void>()

function declined(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY + user) || '[]')
    return Array.isArray(v) ? v.filter((x) => typeof x === 'string').slice(0, 64) : []
  } catch { return [] }
}
function setDeclined(list: string[]) {
  try { if (list.length) localStorage.setItem(KEY + user, JSON.stringify(list)); else localStorage.removeItem(KEY + user) } catch { /* declined for this visit only */ }
}

function work() {
  const next = info ? openInvites(info.rooms, declined(), currentParty()?.id || '') : []
  if (next.length === invites.length && next.every((r, i) => r.id === invites[i].id && r.name === invites[i].name && r.hostName === invites[i].hostName && r.members === invites[i].members)) return
  invites = next
  listeners.forEach((l) => l())
}

const hidden = () => document.visibilityState === 'hidden'

function later() {
  clearTimeout(timer)
  if (!user || hidden()) return
  timer = setTimeout(ask, invitePollDelay(info ? info.enabled : true, failures))
}

// run: which watch this is. Signing out and in again starts a new one, and an answer still
// on its way for the one before is dropped.
let run = 0
async function ask() {
  if (!user || hidden() || asking) return
  const mine = run
  asking = true
  try {
    const got = await partyInfo()
    if (mine !== run) return
    info = got
    failures = 0
    askedAt = Date.now()
    const d = declined(), kept = keepDeclined(d, got.rooms)
    if (kept.length !== d.length) setDeclined(kept)
    work()
  } catch {
    if (mine === run) failures++
  } finally {
    if (mine === run) { asking = false; later() }
  }
}

// Coming back to the tab asks at once, unless it was asked a moment ago.
const onVisibility = () => {
  clearTimeout(timer)
  if (hidden() || !user) return
  if (Date.now() - askedAt > 10000) ask(); else later()
}

// Called when someone signs in, and with 0 when they sign out.
export function watchInvites(userId: number) {
  if (userId === user) return
  clearTimeout(timer)
  stopParty?.()
  stopParty = null
  document.removeEventListener('visibilitychange', onVisibility)
  user = userId
  run++
  asking = false
  info = null
  failures = 0
  askedAt = 0
  invites = []
  listeners.forEach((l) => l())
  if (!userId) return
  document.addEventListener('visibilitychange', onVisibility)
  // Joining or leaving a party changes which invitations are still open.
  stopParty = subscribeParty(work)
  ask()
}

// After something this window did that changes the list (joining by invitation).
export function refreshInvites() { if (user) { clearTimeout(timer); ask() } }

export function declineInvite(roomId: string) {
  const d = declined()
  if (!d.includes(roomId)) setDeclined([...d, roomId])
  work()
}

const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }
export const useInvites = () => useSyncExternalStore(subscribe, () => invites)
