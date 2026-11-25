// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the live player saw lately, kept for Auto quality (liveAuto.ts) and the playback report: the
// last fragments (when each was asked for, how long it took, its size and length in time) and the stalls.

export type FragRecord = { at: number; sn: number; cc: number; bytes: number; durationSec: number; loadMs: number; firstByteMs: number }
export type StallRecord = { at: number }
export type AutoRecord = { at: number; from: number; to: number; why: string }

const MAX = 40
export const frags: FragRecord[] = []
export const stalls: StallRecord[] = []
export const autoChanges: AutoRecord[] = []

export function noteFrag(f: FragRecord) { frags.push(f); if (frags.length > MAX) frags.shift() }
export function noteStall(at: number) { stalls.push({ at }); if (stalls.length > MAX) stalls.shift() }
export function noteAuto(r: AutoRecord) { autoChanges.push(r); if (autoChanges.length > 10) autoChanges.shift() }
export function clearStats() { frags.length = 0; stalls.length = 0 }
