// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { IconClose } from '../icons'
import type { PlayerView } from './types'
import { date, t } from '../i18n'

// Stop playing after: the board's calm column, a row of pills and a note (canvas page "sleep").

const MINUTES = [15, 30, 45, 60, 90]

export function SleepPanel({ p, onClose }: { p: PlayerView; onClose: () => void }) {
  const at = typeof p.sleepAt === 'number' ? p.sleepAt : null
  const stopsAt = at !== null ? date(at, { hour: '2-digit', minute: '2-digit' }) : ''
  const set = (v: number | 'chapter' | null) => { p.setSleepAt(v); onClose() }
  return (
    <div className="np-side">
      <div className="np-side-head">
        <h3>{t('Stop playing after')}</h3>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Close the sleep timer')}><IconClose /></button>
      </div>
      <p className="np-hint np-lede">{t('Music, audiobooks, podcasts and radio.')}</p>
      <div className="np-pills" role="group" aria-label={t('Sleep timer')}>
        {MINUTES.map((m) => <button type="button" key={m} className="np-chip small" onClick={() => set(Date.now() + m * 60000)}>{t('{count} min', { count: m })}</button>)}
        {!p.live && <button type="button" className={'np-chip small' + (p.sleepAt === 'chapter' ? ' on' : '')} aria-pressed={p.sleepAt === 'chapter'} onClick={() => set('chapter')}>{p.books ? t('End of chapter') : t('After this song')}</button>}
        <button type="button" className={'np-chip small' + (p.sleepAt === null ? ' on' : '')} aria-pressed={p.sleepAt === null} onClick={() => set(null)}>{t('Off')}</button>
      </div>
      <p className="np-note np-callout" role="status">
        {p.sleepAt === null ? t('The timer is off. Choose a time and playback stops by itself.') : at !== null ? t('Playback stops at {time}.', { time: stopsAt }) : p.books ? t('Playback stops at the end of this chapter.') : t('Playback stops after this song.')}
      </p>
    </div>
  )
}
