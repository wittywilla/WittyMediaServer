// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { api, type Library } from '../api'
import { tx } from '../i18n'
import { emptyLibraryState } from '../profileWords'
import { useApp } from '../state'
import { EmptyState } from './common'
import type { ScanProgress } from './ScanControl'

// LibraryEmpty: what a library page with nothing on it shows. A library waiting for its turn to
// be scanned says so, one being scanned says titles come as they're found, and only one whose
// scan found nothing says "Nothing here yet" with the owner's advice (forOwner).
//
// The library list the app holds is read once, so the scan's state is asked here: once, and
// every few seconds while a scan waits or runs. When it ends, onScanned asks the page again.
export function LibraryEmpty({ lib, icon, forOwner, action, onScanned }: { lib: Library; icon?: ReactNode; forOwner: string; action?: ReactNode; onScanned?: () => void }) {
  const { user, refreshLibraries } = useApp()
  const [scan, setScan] = useState<{ scanning: boolean; progress: ScanProgress | null } | null>(null)
  const done = useRef(onScanned)
  done.current = onScanned
  useEffect(() => {
    let live = true, timer = 0, was = lib.scanning
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: ScanProgress | null }>(`/libraries/${lib.id}/scan`)
        if (!live) return
        setScan(r)
        if (was && !r.scanning) { void refreshLibraries(); done.current?.() }
        was = r.scanning
        if (r.scanning) timer = window.setTimeout(poll, 3000)
      } catch { if (live && was) timer = window.setTimeout(poll, 5000) }
    }
    void poll()
    return () => { live = false; window.clearTimeout(timer) }
  }, [lib.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const scanning = scan ? scan.scanning : lib.scanning
  const w = emptyLibraryState(user, { scanning, lastScanAt: lib.lastScanAt }, scan?.progress?.phase, forOwner)
  return <EmptyState icon={icon} title={tx(w.title)} text={tx(w.text)} action={w.state === 'empty' || w.state === 'unscanned' ? action : undefined} />
}
