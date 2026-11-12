// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// The top of Settings, WittyLog (canvas page "logs"): what logging is doing right now, and the two things to do
// about a problem: send the logs, or keep what just happened. The tabs and the viewer are the page's own.
export interface LogsTop { recordingUntil: string; problems: string[]; flight: boolean; showDownload: boolean; saving: boolean }

export function logsSpec(t: LogsTop): SpecPageData {
  const sections: SpecSection[] = []
  if (t.recordingUntil) sections.push(secN('Recording everything until ' + t.recordingUntil, 'Try to make the problem happen now. Logging goes back to your usual settings by itself.', true), secR('', [{ id: 'stop', title: 'Stop now' }]))
  if (t.problems.length) sections.push(secN('Logging has a problem', t.problems.join(' '), true))
  const rows = []
  if (t.showDownload) rows.push({ title: 'Download logs as ZIP', sub: 'Someone helping you? Send them the logs, sanitized for sharing.', to: '?tab=bundle' })
  rows.push({ id: 'save', title: t.saving ? 'Saving…' : 'Save what just happened', sub: t.flight ? 'Keep the last 10 minutes, in full detail, before they’re gone.' : 'Keep what every part of the server is doing right now.', disabled: t.saving })
  sections.push(secR('Logs', rows))
  return mk('logs', 'Settings · Logs', 'Logs', 'Read, choose how much, download.', sections)
}
