// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fill, type Vars } from '../../../i18n/core.ts'

// t and tn for the store's files that are tested on their own, under node, where the app's i18n
// (which loads catalogues the browser's way) can't come along. Until the app plugs its own in
// (storeApi.ts and OlderServerDialog.tsx do, as they load), they give the English, which is what
// the tests read.

type T = (english: string, vars?: Vars) => string
type TN = (one: string, other: string, count: number, vars?: Vars) => string

let tNow: T = (english, vars) => fill(english, vars, 'en')
let tnNow: TN = (one, other, count, vars) => fill(count === 1 ? one : other, { count, ...vars }, 'en')

export const t: T = (english, vars) => tNow(english, vars)
export const tn: TN = (one, other, count, vars) => tnNow(one, other, count, vars)
export const tk = <S extends string>(english: S): S => english

export function plugAppWords(appT: T, appTn: TN) { tNow = appT; tnNow = appTn }
