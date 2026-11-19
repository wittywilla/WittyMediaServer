// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { api } from '../../api'
import { toast } from '../../state'
import { ask } from '../../ui'
import { forgetLineup } from './tvApi'
import './reminders.css'
import { list, t, tn } from '../../i18n'

// Reset channel: asks first, in plain words, then makes the channel's schedule
// again from its settings and the files that match them now. Shared by the
// channel's editor and its details in the guide. Resolves true once it's done.

type Reminders = { kept: number; moved: number; dropped: number; droppedTitles?: string[] }

export async function resetChannel(id: number, name: string): Promise<boolean> {
  let startOver = false
  const yes = await ask({
    title: t('Reset “{name}”?', { name }),
    // The dialog's description is a paragraph already, so these are spans laid out as paragraphs.
    body: (
      <>
        <span className="tv-reset-para">{t('What’s on now stops, and the guide is rebuilt from the channel’s current settings and the files that match them now. Anyone watching moves straight to the new schedule.')}</span>
        <span className="tv-reset-para">{t('Reminders follow their programme to its new time. A reminder for something that no longer airs is removed, and you’ll be told which.')}</span>
        <label className="check-row"><input type="checkbox" onChange={(e) => { startOver = e.target.checked }} /> {t('Also start every show from its first episode, and shuffle again')}</label>
      </>
    ),
    confirm: t('Reset channel'),
    danger: true,
  })
  if (!yes) return false
  try {
    const r = await api.post<{ watching: boolean; reminders: Reminders }>(`/channels/${id}/reset`, { startOver })
    forgetLineup()
    toast(resetNote(name, r.reminders, r.watching))
    return true
  } catch (e) {
    toast((e as Error).message)
    return false
  }
}

export function resetNote(name: string, r: Reminders, watching: boolean) {
  const parts = [watching ? t('{name} has a new schedule, and everyone watching is on it.', { name }) : t('{name} has a new schedule.', { name })]
  if (r.kept) parts.push(tn('{count} reminder kept.', '{count} reminders kept.', r.kept))
  if (r.moved) parts.push(tn('{count} reminder moved to a new time.', '{count} reminders moved to a new time.', r.moved))
  if (r.dropped) {
    const shown = r.droppedTitles?.slice(0, 5) || []
    const titles = shown.length ? (r.droppedTitles!.length > 5 ? t('{titles} and others', { titles: shown.join(', ') }) : list(shown)) : ''
    parts.push(titles
      ? tn('{count} reminder was removed, as that programme no longer airs: {titles}.', '{count} reminders were removed, as those programmes no longer air: {titles}.', r.dropped, { titles })
      : tn('{count} reminder was removed, as that programme no longer airs.', '{count} reminders were removed, as those programmes no longer air.', r.dropped))
  }
  return parts.join(' ')
}
