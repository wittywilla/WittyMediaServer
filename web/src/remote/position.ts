// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a device is now, from what it last said: docs/REMOTE-PROTOCOL.md, "List devices".
// Plain arithmetic, tested in position.test.mjs.

export type Placed = { status: string; positionSec?: number; durationSec?: number; rate?: number }

// clockAhead: how far the server's clock is ahead of this one, from the server's time in an
// answer and this device's clock when the answer arrived.
export const clockAhead = (serverTimeMs: number, localNowMs: number): number => serverTimeMs - localNowMs

// positionNow: positionSec, carried forward by the time since the report at the speed it was
// playing. Only a playing device moves; nothing goes before the start or past the end.
export function positionNow(st: Placed, stateAtMs: number, serverNowMs: number): number {
  let pos = Number.isFinite(st.positionSec) ? st.positionSec! : 0
  if (st.status === 'playing' && stateAtMs > 0) {
    const rate = Number.isFinite(st.rate) && st.rate! > 0 ? st.rate! : 1
    // A report that seems to come from the future (clocks a little apart) counts as now.
    pos += (Math.max(0, serverNowMs - stateAtMs) / 1000) * rate
  }
  const dur = st.durationSec || 0
  return Math.max(0, dur > 0 ? Math.min(pos, dur) : pos)
}

// seekTarget: where a seek by so many seconds lands, kept inside the item.
export function seekTarget(nowSec: number, bySec: number, durationSec: number): number {
  const to = nowSec + bySec
  return Math.max(0, durationSec > 0 ? Math.min(to, durationSec) : to)
}

// partDone: how far through a book a reader is, as a whole percentage, or null when it
// hasn't said. Readers count in their own units (pages, or chapters), so only the share means
// the same thing for all of them.
export function partDone(positionSec: number | undefined, durationSec: number | undefined): number | null {
  if (!durationSec || durationSec <= 0 || positionSec == null || !Number.isFinite(positionSec)) return null
  return Math.round(Math.max(0, Math.min(1, positionSec / durationSec)) * 100)
}
