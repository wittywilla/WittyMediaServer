// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout editor wherever it is asked for (Settings, Appearance, or Ctrl+Shift+L): one host in
// the app. The editor itself (with the brand dialog and the layout pictures) is fetched only when it
// opens, so it isn't in every page's first download (#643).
import { lazy, Suspense, useEffect, useState } from 'react'
import { useTvMode } from '../tvmode/mode'
import { LAYOUT_EVENT } from './layoutEvent'

const ShellEditor = lazy(() => import('../pages/ShellEditor').then((m) => ({ default: m.ShellEditor })))
const OnAComputerDialog = lazy(() => import('../tvmode/OnAComputer').then((m) => ({ default: m.OnAComputerDialog })))

export function ShellEditorHost() {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    const on = () => setOpen(true)
    window.addEventListener(LAYOUT_EVENT, on)
    return () => window.removeEventListener(LAYOUT_EVENT, on)
  }, [])
  // On a television the layout is edited on a computer (docs/TV-MODE.md): the button says where.
  const tv = useTvMode()
  if (!open) return null
  return <Suspense fallback={null}>{tv ? <OnAComputerDialog part="shell.editor" onClose={() => setOpen(false)} /> : <ShellEditor onClose={() => setOpen(false)} />}</Suspense>
}
