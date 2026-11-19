// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fill, type Vars } from '../../i18n/core.ts'

// plan.ts and network.ts are tested in node, where the app's translations can't load, so their words come through this:
// the page passes the app's own { t, tn, list } (from i18n), and the tests get English.
export type Words = {
  t: (english: string, vars?: Vars) => string
  tn: (one: string, other: string, count: number, vars?: Vars) => string
  list: (items: string[]) => string
}

export const ENGLISH: Words = {
  t: (s, v) => fill(s, v, 'en'),
  tn: (one, other, count, v) => fill(count === 1 ? one : other, { count, ...v }, 'en'),
  list: (a) => (a.length < 2 ? a[0] || '' : `${a.slice(0, -1).join(', ')} and ${a[a.length - 1]}`),
}

/** Marks English to be translated where it's shown (the same as tk in i18n, which node can't load). */
export const tk = <S extends string>(english: S): S => english
