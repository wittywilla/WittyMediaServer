// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secF, secP } from '../model.ts'
import type { SpecPageData } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Creator videos (canvas "creators"): the topic chips over a library, and the channel details dialog.

// Topics: All, then each tag. Ids: tag: (all) and tag:<name>.
export function topicsSpec(tags: string[], tag: string): SpecPageData {
  return mk('creators', t('Creator videos'), t('Creators'), '', [secP(t('Topics'), [{ ...Cc(t('All'), !tag), id: 'tag:' }, ...tags.map((name) => ({ ...Cc(name, tag === name), id: 'tag:' + name }))])])
}

export interface ChannelForm { name: string; handle: string; platform: string; url: string }

// A channel's details (the description box is the dialog's own, after the handle). Fields: name, handle, platform, url.
export function channelSpec(f: ChannelForm, err: string): SpecPageData {
  return mk('creators', t('Creator videos'), t('Channel details'), '', [secF('', [
    { id: 'name', kind: 'text', label: t('Name'), value: f.name, required: true },
    { id: 'handle', kind: 'text', label: t('Handle'), value: f.handle, placeholder: t('@name') },
    { id: 'platform', kind: 'text', label: t('Platform of origin'), value: f.platform },
    { id: 'url', kind: 'text', label: t('Web address'), value: f.url, placeholder: 'https://…', autoComplete: 'url', error: err || undefined },
  ])])
}
