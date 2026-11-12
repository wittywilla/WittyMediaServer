// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Share (canvas keys "share" and "shared"): a link inside this server for other profiles, and, where the owner allows
// it, a link for someone with no account. Field ids: days, download. Action/chip ids: copyInternal, copyLink, make.
export const DAYS: [number, string][] = [[1, '1 day'], [7, '7 days'], [30, '30 days'], [365, '1 year'], [0, 'No end date']]

export interface ShareState { title: string; internal: string; allowed: boolean; link: string; days: number; download: boolean; busy: boolean; error: string }

export function shareSpec(s: ShareState): SpecPageData {
  const sections: SpecSection[] = [
    secN('With other profiles here', 'Anyone signed in to this server who can open this library.'),
    secR('', [{ ...R('Link for other profiles', s.internal), id: 'internal' }]),
    secP('', [{ ...Cc('Copy', true), id: 'copyInternal' }]),
  ]
  const actions: SpecPageData['actions'] = []
  if (s.allowed) {
    if (s.link) {
      sections.push(secN('With someone without an account', 'This is the only time the link is shown: the server keeps just a fingerprint of it. You can end it any time in Settings, Sharing.', true))
      sections.push(secR('', [{ ...R('Link to share', s.link, true), id: 'link' }]))
      sections.push(secP('', [{ ...Cc('Copy', true), id: 'copyLink' }]))
    } else {
      sections.push(secN('With someone without an account', 'They see only this and your profile name, without signing in. Pictures are re-encoded, so camera details and locations stay here. It works wherever your server can be reached.'))
      sections.push(secF('', [
        { id: 'days', kind: 'select', label: 'Ends after', value: String(s.days), options: DAYS.map(([d, l]) => ({ value: String(d), label: l })), disabled: s.busy },
        { id: 'download', kind: 'switch', label: 'Allow downloading the original files', value: s.download, disabled: s.busy },
      ]))
      if (s.error) sections.push(secN('', s.error, true))
      actions.push({ ...A1(s.busy ? 'Making…' : 'Make a link'), id: 'make', disabled: s.busy })
    }
  }
  return mk('share', 'Share', `Share “${s.title}”`, '', sections, actions)
}

// The page someone sees on opening a link they were sent: the title and until when; no way into the server.
export function sharedHeadSpec(title: string, until: string | null): SpecPageData {
  return mk('shared', 'Shared with you', title, until ? `Until ${until}` : '', [])
}

export function sharedEndedSpec(): SpecPageData {
  return mk('shared', 'Shared page', 'This link has ended, or it was never valid.', 'Ask the person who sent it for a new one.', [])
}
