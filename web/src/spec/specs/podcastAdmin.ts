// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secP } from '../model.ts'
import type { SpecPageData } from '../types.ts'

// The controls on a followed podcast's page (canvas page "podcasts"): check for new episodes, how many of the
// newest to keep downloaded, and stop following. Ids: check, auto.<n>, unfollow.
export const AUTO_STEPS = [0, 1, 3, 5, 10, 25]
export const autoWords = (n: number) => (n === 0 ? 'None, stream them' : n === 1 ? 'Newest episode' : `Newest ${n} episodes`)

export function podcastAdminSpec(autoDownload: number): SpecPageData {
  return mk('podcasts', 'Podcasts', 'Podcasts', 'Follow a show by search or by an RSS address.', [
    secP('', [{ ...Cc('Check for new episodes'), id: 'check' }]),
    secP('Keep downloaded', AUTO_STEPS.map((n) => ({ ...Cc(autoWords(n), n === autoDownload), id: `auto.${n}` }))),
    secP('', [{ ...Cc('Unfollow'), id: 'unfollow' }]),
  ])
}
