// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secR } from '../model.ts'
import type { SpecPageData, SpecRow } from '../types.ts'
import { t, tx } from '../../sdui/words.ts'

// Help's list of guides (canvas page "help"): the three ways in, then each group of guides, every row a link. The
// groups' and guides' names come from help/guides.ts: shown through tx, so they're translated where a catalogue has them.
export interface HelpGroup { title: string; guides: { file: string; title: string; about: string }[] }

export function helpSpec(groups: HelpGroup[], version: string, slugOf: (file: string) => string): SpecPageData {
  const link = (title: string, sub: string, slug: string): SpecRow => ({ title, sub, to: `/help/${slug}` })
  return mk('help', t('Help'), t('Guides'),
    version ? t('Everything about this server, built in. It works with no internet connection, and it matches the version you’re running: {version}.', { version })
      : t('Everything about this server, built in. It works with no internet connection, and it matches the version you’re running.'),
    [
      secR(t('Start'), [
        link(t('New here?'), t('Start with Getting started.'), 'getting-started'),
        link(t('Something’s wrong?'), t('Look it up by what you’re seeing.'), 'troubleshooting'),
        link(t('Looking for a setting?'), t('Every settings page, one by one.'), 'settings'),
      ]),
      ...groups.map((g) => secR(tx(g.title), g.guides.map((it) => link(tx(it.title), tx(it.about), slugOf(it.file))))),
    ])
}
