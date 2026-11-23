// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secN, secP } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// The activity panel's own notices and the history's filter (canvas page "activity"); the running tasks stay the
// panel's live widgets. Ids: show:all, show:failed, clear. The panel draws the notices above its tabs and the
// history draws its own filter, so each asks for its part; the canvas page shows both.
export interface ActivityView { tab: 'now' | 'history'; connected: boolean; loaded: boolean; readOnly: boolean; update: string; isAdmin: boolean; failedOnly: boolean; canClear: boolean; part?: 'notices' | 'filter' }

export function activitySpec(v: ActivityView): SpecPageData {
  const s: SpecSection[] = []
  const notices = v.part !== 'filter', filter = v.part !== 'notices'
  if (notices && !v.connected && v.loaded) s.push(secN('', t('Reconnecting to the server. This may be out of date.'), true))
  if (notices && v.readOnly) s.push(secN('', t('You can see what the server is doing here. Only an administrator can change it.')))
  if (notices && v.update && v.isAdmin && v.tab === 'now') s.push(secN(t('Update available'), t('WittyMediaServer {version} can be installed.', { version: v.update }), true))
  if (filter && v.tab === 'history') s.push(secP(t('Show'), [{ ...Cc(t('All'), !v.failedOnly), id: 'show:all' }, { ...Cc(t('Failed'), v.failedOnly), id: 'show:failed' }, ...(v.canClear ? [{ ...Cc(t('Clear history')), id: 'clear' }] : [])]))
  return mk('activity', t('Top bar · Activity'), t('Activity'), '', s, [], '')
}
