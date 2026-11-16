// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fill, type Vars } from '../i18n/core.ts'

// The app's words for the parts that also run under node's tests without a browser: the layout step (model.ts) and
// the page specs (spec/specs). They can't load the catalogues themselves (that needs Vite), so the app hands them its
// t() and tn() when it starts drawing (setWords, called by SduiPage and SpecPage); until then, and in the tests, the
// English is used as it is. Use them exactly as the ones in i18n/index.ts: the English is the key.

type T = (english: string, vars?: Vars) => string
type TN = (one: string, other: string, count: number, vars?: Vars) => string

let own: T = (english, vars) => fill(english, vars, 'en')
let ownN: TN = (one, other, count, vars) => fill(count === 1 ? one : other, { count, ...vars }, 'en')

/** Gives this file the app's translations (i18n's t and tn). */
export function setWords(t: T, tn: TN): void { own = t; ownN = tn }

export const t: T = (english, vars) => own(english, vars)
export const tn: TN = (one, other, count, vars) => ownN(one, other, count, vars)
/** Marks English kept in a list made when the file loads: translated where it is shown, with t(label). */
export const tk = <S extends string>(english: S): S => english
/** Text from the server: translated when the catalogue knows it, else as it is. */
export const tx = (text: string | undefined | null): string => (text ? own(text) : '')
