// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secR } from '../model.ts'
import type { SpecPageData, SpecRow } from '../types.ts'

// Help's list of guides (canvas page "help"): the three ways in, then each group of guides, every row a link.
export interface HelpGroup { title: string; guides: { file: string; title: string; about: string }[] }

export function helpSpec(groups: HelpGroup[], version: string, slugOf: (file: string) => string): SpecPageData {
  const link = (title: string, sub: string, slug: string): SpecRow => ({ title, sub, to: `/help/${slug}` })
  return mk('help', 'Help', 'Guides',
    `Everything about this server, built in. It works with no internet connection, and it matches the version you’re running${version ? `: ${version}` : ''}.`,
    [
      secR('Start', [
        link('New here?', 'Start with Getting started.', 'getting-started'),
        link('Something’s wrong?', 'Look it up by what you’re seeing.', 'troubleshooting'),
        link('Looking for a setting?', 'Every settings page, one by one.', 'settings'),
      ]),
      ...groups.map((g) => secR(g.title, g.guides.map((it) => link(it.title, it.about, slugOf(it.file))))),
    ])
}
