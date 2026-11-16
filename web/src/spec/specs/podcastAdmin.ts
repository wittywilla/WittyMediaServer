// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secP } from '../model.ts'
import type { SpecPageData } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// The controls on a followed podcast's page (canvas page "podcasts"): check for new episodes, how many of the
// newest to keep downloaded, and stop following. Ids: check, auto.<n>, unfollow.
export const AUTO_STEPS = [0, 1, 3, 5, 10, 25]
export const autoWords = (n: number) => (n === 0 ? t('None, stream them') : tn('Newest episode', 'Newest {count} episodes', n))

export function podcastAdminSpec(autoDownload: number): SpecPageData {
  return mk('podcasts', t('Podcasts'), t('Podcasts'), t('Follow a show by search or by an RSS address.'), [
    secP('', [{ ...Cc(t('Check for new episodes')), id: 'check' }]),
    secP(t('Keep downloaded'), AUTO_STEPS.map((n) => ({ ...Cc(autoWords(n), n === autoDownload), id: `auto.${n}` }))),
    secP('', [{ ...Cc(t('Unfollow')), id: 'unfollow' }]),
  ])
}
