// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// fmtRuntime says how long something runs, the way a card has room for: 45 min, 1 h 52 min, 2 h.
// Whole minutes are rounded first, so 59 min 45 s is 1 h, never "60 min".
export function fmtRuntime(sec: number): string {
  if (!sec) return ''
  if (sec < 60) return `${Math.max(1, Math.round(sec))} s`
  const mins = Math.round(sec / 60)
  const h = Math.floor(mins / 60), m = mins % 60
  if (h === 0) return `${m} min`
  return m ? `${h} h ${m} min` : `${h} h`
}
