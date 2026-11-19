// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// The top of Settings, WittyLog (canvas page "logs"): what logging is doing right now, and the two things to do
// about a problem: send the logs, or keep what just happened. The tabs and the viewer are the page's own.
export interface LogsTop { recordingUntil: string; problems: string[]; flight: boolean; showDownload: boolean; saving: boolean }

export function logsSpec(top: LogsTop): SpecPageData {
  const sections: SpecSection[] = []
  if (top.recordingUntil) sections.push(secN(t('Recording everything until {time}', { time: top.recordingUntil }), t('Try to make the problem happen now. Logging goes back to your usual settings by itself.'), true), secR('', [{ id: 'stop', title: t('Stop now') }]))
  if (top.problems.length) sections.push(secN(t('Logging has a problem'), top.problems.join(' '), true))
  const rows = []
  if (top.showDownload) rows.push({ title: t('Download logs as ZIP'), sub: t('Someone helping you? Send them the logs, sanitized for sharing.'), to: '?tab=bundle' })
  rows.push({ id: 'save', title: top.saving ? t('Saving…') : t('Save what just happened'), sub: top.flight ? t('Keep the last 10 minutes, in full detail, before they’re gone.') : t('Keep what every part of the server is doing right now.'), disabled: top.saving })
  sections.push(secR(t('Logs'), rows))
  return mk('logs', t('Settings · Logs'), t('Logs'), t('Read, choose how much, download.'), sections)
}
