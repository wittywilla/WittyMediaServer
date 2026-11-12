// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secP } from '../model.ts'
import type { SpecField, SpecPageData } from '../types.ts'

// Settings, Library transcoding: "How it runs on this server" (the section keeps its card and title in the page).
// Fields: pause, low, hoursOn, from, to, settle, threads, jobs, hardware, free.
export interface RunDraft {
  pauseWhileStreaming: boolean; lowPriority: boolean; hoursOn: boolean; hoursFrom: number; hoursTo: number
  settleSeconds: number; threads: number; maxJobs: number; useHardware: boolean; minFreeGb: number
}
const num = (n: number[], cur: number) => [...new Set([...n, cur])].sort((a, b) => a - b)
export const settleLabel = (n: number) => (n < 3600 ? `${Math.round(n / 60)} min` : `${Math.round(n / 3600)} hour`)
const hours = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

export function runSettingsSpec(d: RunDraft, threadChoices: { value: string; label: string }[]): SpecPageData {
  const f: SpecField[] = [
    { id: 'pause', kind: 'switch', label: 'Pause while people are streaming', value: d.pauseWhileStreaming, hint: 'Conversions hold still, using no processor, whenever anyone is playing something or a game, and carry on where they were afterwards. A library may take much longer to finish.' },
    { id: 'low', kind: 'switch', label: 'Low priority', value: d.lowPriority, hint: 'Everything else on this machine gets the processor first.' },
    { id: 'hoursOn', kind: 'switch', label: 'Only at certain hours', value: d.hoursOn, hint: 'Outside these hours it holds still. Hours are on this server’s clock, and can run past midnight.' },
    ...(d.hoursOn ? [
      { id: 'from', kind: 'select' as const, label: 'From', value: String(d.hoursFrom), options: hours },
      { id: 'to', kind: 'select' as const, label: 'Until', value: String(d.hoursTo), options: hours },
    ] : []),
    { id: 'settle', kind: 'select', label: 'Wait for new files to settle', value: String(d.settleSeconds), options: num([60, 120, 300, 900, 3600], d.settleSeconds).map((n) => ({ value: String(n), label: settleLabel(n) })), hint: 'For libraries that convert new files by themselves: how long a file has to stay the same size before it’s touched, so one still being copied or downloaded is left to finish.' },
    { id: 'threads', kind: 'select', label: 'How much of the processor', value: String(d.threads), options: threadChoices, hint: 'For each file being converted.' },
    { id: 'jobs', kind: 'select', label: 'Files at once', value: String(d.maxJobs), options: [1, 2, 3, 4].map((n) => ({ value: String(n), label: String(n) })), hint: 'More finishes sooner and leaves less for everything else. One is right for most machines.' },
    { id: 'hardware', kind: 'switch', label: 'Use hardware encoders', value: d.useHardware, hint: 'Much faster and easier on the processor, for slightly larger files. It uses the ones ticked under Live transcoding, for ordinary 8-bit pictures. HDR and 10-bit pictures always use the slower software encoder.' },
    { id: 'free', kind: 'select', label: 'Keep this much free on each drive', value: String(d.minFreeGb), options: num([1, 5, 10, 25, 50, 100], d.minFreeGb).map((n) => ({ value: String(n), label: `${n} GB` })), hint: 'Conversions stop short of this, and a file that wouldn’t fit waits.' },
  ]
  return mk('libtranscode', 'Settings · Library transcoding', 'How it runs on this server', 'Converting competes with people watching. These keep it out of their way.', [secF('', f), secP('', [{ title: 'Live transcoding settings', to: '/settings/transcoding' }])])
}

// The field ids, mapped to the setting each one changes (numbers for selects).
export function runPatch(id: string, v: string | number | boolean): Partial<RunDraft> | null {
  switch (id) {
    case 'pause': return { pauseWhileStreaming: !!v }
    case 'low': return { lowPriority: !!v }
    case 'hoursOn': return { hoursOn: !!v }
    case 'from': return { hoursFrom: Number(v) }
    case 'to': return { hoursTo: Number(v) }
    case 'settle': return { settleSeconds: Number(v) }
    case 'threads': return { threads: Number(v) }
    case 'jobs': return { maxJobs: Number(v) }
    case 'hardware': return { useHardware: !!v }
    case 'free': return { minFreeGb: Number(v) }
    default: return null
  }
}
