// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../api'
import { IconDown, IconRefresh } from '../icons'
import { Button, Menu, ask, btnClass, toast, type ButtonSize, type ButtonVariant } from '../ui'
import './scancontrol.css'

// Every place a library can be scanned offers the same two choices, drawn the same way: Scan for
// changes (the button itself: only new, changed or removed files) and, from the menu beside it,
// Full rescan (every file and its details again, from scratch, after asking). A scan never times
// out, and one a restart interrupted carries on when the server starts again.

export type ScanMode = 'changes' | 'full'
// What GET /api/libraries/{id}/scan says of a scan going on.
export type ScanProgress = { done?: number; found?: number; mode?: ScanMode; resumed?: boolean; phase?: string }

// scanProgressText: "Scanning: 12,400 of 16,780 files", or "Full rescan: …".
export function scanProgressText(p: ScanProgress | null | undefined): string {
  const what = p?.mode === 'full' ? 'Full rescan' : 'Scanning'
  if (p?.found && p.found > 0) return `${what}: ${(p.done ?? 0).toLocaleString()} of ${p.found.toLocaleString()} files`
  return `${what}…`
}

// startScan asks for a scan, after asking the person first for a full rescan. True when one began.
export async function startScan(lib: { id: number; name: string }, mode: ScanMode): Promise<boolean> {
  if (mode === 'full' && !await ask({
    title: `Full rescan of “${lib.name}”?`,
    body: 'Every file and its details are read again from scratch, changed or not. On a big library this can take a long time. The library stays usable meanwhile, and the scan carries on after a restart.',
    confirm: 'Full rescan',
  })) return false
  try {
    const r = await api.post<{ started: boolean; mode: ScanMode }>(`/libraries/${lib.id}/scan`, { mode })
    if (!r.started) toast(`${lib.name} is already being scanned`)
    else toast(mode === 'full' ? `Full rescan of ${lib.name} started` : `Scanning ${lib.name} for changes`)
    return r.started
  } catch (e) {
    toast((e as Error).message, { tone: 'danger' })
    return false
  }
}

// ScanControl: Scan for changes, with Full rescan in the menu beside it; while a scan goes on, how far
// it has got. For administrators: callers show it only to them.
export function ScanControl({ lib, progress, scanning, onStarted, size = 'md', variant = 'ghost', className }: {
  lib: { id: number; name: string }
  progress?: ScanProgress | null // a scan going on, as its route says
  scanning?: boolean // a scan going on, without its progress
  onStarted?: (mode: ScanMode) => void
  size?: ButtonSize
  variant?: ButtonVariant
  className?: string
}) {
  if (progress || scanning) {
    return <Button size={size} variant={variant} icon={<IconRefresh />} disabled className={className} aria-live="polite">{scanProgressText(progress)}</Button>
  }
  const run = (mode: ScanMode) => { void startScan(lib, mode).then((ok) => { if (ok) onStarted?.(mode) }) }
  return (
    <span className={'scan-ctl' + (className ? ' ' + className : '')} role="group" aria-label={`Scan ${lib.name}`}>
      <Button size={size} variant={variant} icon={<IconRefresh />} onClick={() => run('changes')} title="Looks only at new, changed or removed files">Scan for changes</Button>
      <Menu label={`Ways to scan ${lib.name}`} align="end" items={[
        { key: 'changes', label: 'Scan for changes', hint: 'Only new, changed or removed files', onSelect: () => run('changes') },
        { key: 'full', label: 'Full rescan…', hint: 'Every file and its details again, from scratch', onSelect: () => run('full') },
      ]} trigger={(props) => (
        <button type="button" {...props} className={btnClass(variant, size, 'square scan-ctl-more')} aria-label={`More ways to scan ${lib.name}`} title="More ways to scan"><IconDown /></button>
      )} />
    </span>
  )
}

// useScanProgress asks how far a library's scan has got, every couple of seconds while active.
export function useScanProgress(libraryId: number, active: boolean): ScanProgress | null {
  const [progress, setProgress] = useState<ScanProgress | null>(null)
  useEffect(() => {
    if (!active) { setProgress(null); return }
    let live = true
    let t = 0
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: ScanProgress | null }>(`/libraries/${libraryId}/scan`)
        if (!live) return
        setProgress(r.scanning ? r.progress || {} : null)
      } catch { /* asked again in a moment */ }
      if (live) t = window.setTimeout(poll, 2000)
    }
    void poll()
    return () => { live = false; window.clearTimeout(t) }
  }, [libraryId, active])
  return progress
}
