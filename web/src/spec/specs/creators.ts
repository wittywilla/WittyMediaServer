// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secF, secP } from '../model.ts'
import type { SpecPageData } from '../types.ts'

// Creator videos (canvas "creators"): the topic chips over a library, and the channel details dialog.

// Topics: All, then each tag. Ids: tag: (all) and tag:<name>.
export function topicsSpec(tags: string[], tag: string): SpecPageData {
  return mk('creators', 'Creator videos', 'Creators', '', [secP('Topics', [{ ...Cc('All', !tag), id: 'tag:' }, ...tags.map((t) => ({ ...Cc(t, tag === t), id: 'tag:' + t }))])])
}

export interface ChannelForm { name: string; handle: string; platform: string; url: string }

// A channel's details (the description box is the dialog's own, after the handle). Fields: name, handle, platform, url.
export function channelSpec(f: ChannelForm, err: string): SpecPageData {
  return mk('creators', 'Creator videos', 'Channel details', '', [secF('', [
    { id: 'name', kind: 'text', label: 'Name', value: f.name, required: true },
    { id: 'handle', kind: 'text', label: 'Handle', value: f.handle, placeholder: '@name' },
    { id: 'platform', kind: 'text', label: 'Platform of origin', value: f.platform },
    { id: 'url', kind: 'text', label: 'Web address', value: f.url, placeholder: 'https://…', autoComplete: 'url', error: err || undefined },
  ])])
}
