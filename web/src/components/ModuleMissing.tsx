// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { api, ApiError } from '../api'
import { useApp } from '../state'
import SpecPage from '../spec/SpecPage'
import { INSTALL, moduleMissingSpec } from '../spec/specs/moduleMissing'

// What a page shows when the module behind it isn't on this server: what's missing and what it
// would add, and for an admin a button that installs it, with no restart. Never a broken screen.

type Status = { id: string; name: string; summary?: string; unavailable?: string; installed?: boolean; enabled?: boolean; phase?: string; done?: number; total?: number; error?: string }
type Report = { modules: Status[]; catalog?: { error?: string } }

const POLL_MS = 1500

export function ModuleMissing({ ids, what, onReady }: { ids: string[]; what?: string; onReady?: () => void }) {
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const [mods, setMods] = useState<Status[] | null>(null)
  const [err, setErr] = useState('')
  const [adding, setAdding] = useState(false)

  useEffect(() => {
    if (!admin) return
    let live = true
    let timer = 0
    const look = () => api.get<Report>('/modules').then((r) => {
      if (!live) return
      const list = ids.map((id) => r.modules.find((m) => m.id === id)).filter(Boolean) as Status[]
      setMods(list)
      if (list.some((m) => m.enabled)) { onReady?.(); return }
      const failed = list.find((m) => m.error)
      if (failed) setErr(failed.error || '')
      if (list.some((m) => m.phase && m.phase !== 'idle' && m.phase !== 'failed')) timer = window.setTimeout(look, POLL_MS)
    }).catch(() => {})
    look()
    return () => { live = false; clearTimeout(timer) }
  }, [admin, adding, ids.join()]) // eslint-disable-line react-hooks/exhaustive-deps

  const add = async (id: string) => {
    setErr('')
    setAdding(true)
    try {
      await api.post(`/modules/${id}/add`)
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : 'That module couldn’t be installed.')
    } finally { setAdding(false) }
  }

  const spec = useMemo(() => moduleMissingSpec({ ids, what, admin, mods, adding, error: err }), [ids.join(), what, admin, mods, adding, err]) // eslint-disable-line react-hooks/exhaustive-deps
  const handled = useMemo(() => new Set(spec.actions.map((a) => a.id || '')), [spec])
  return <SpecPage page={spec} handled={handled} onAction={(id) => { if (id.startsWith(INSTALL)) add(id.slice(INSTALL.length)) }} />
}
