// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secN, secP } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// The activity panel's own notices and the history's filter (canvas page "activity"); the running tasks stay the
// panel's live widgets. Ids: show:all, show:failed, clear.
export interface ActivityView { tab: 'now' | 'history'; connected: boolean; loaded: boolean; readOnly: boolean; update: string; isAdmin: boolean; failedOnly: boolean; canClear: boolean }

export function activitySpec(v: ActivityView): SpecPageData {
  const s: SpecSection[] = []
  if (!v.connected && v.loaded) s.push(secN('', 'Reconnecting to the server. This may be out of date.', true))
  if (v.readOnly) s.push(secN('', 'You can see what the server is doing here. Only an administrator can change it.'))
  if (v.update && v.isAdmin && v.tab === 'now') s.push(secN('Update available', `WittyMediaServer ${v.update} can be installed.`, true))
  if (v.tab === 'history') s.push(secP('Show', [{ ...Cc('All', !v.failedOnly), id: 'show:all' }, { ...Cc('Failed', v.failedOnly), id: 'show:failed' }, ...(v.canClear ? [{ ...Cc('Clear history'), id: 'clear' }] : [])]))
  return mk('activity', 'Top bar · Activity', 'Activity', '', s, [], '')
}
