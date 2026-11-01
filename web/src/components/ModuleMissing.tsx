import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useApp } from '../state'

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
      setErr(e instanceof ApiError ? e.message : 'That module could not be added.')
    } finally { setAdding(false) }
  }

  const name = mods?.[0]?.name || ids[0]
  const busy = mods?.find((m) => m.phase && m.phase !== 'idle' && m.phase !== 'failed')
  const noBuild = mods?.find((m) => m.unavailable)
  return (
    <div className="module-missing">
      <h2>{what || `This needs ${name}`}</h2>
      <p>{name} isn't on this server, so this can't open yet. Everything else works as usual.</p>
      {mods?.map((m) => m.summary ? <p key={m.id}><strong>{m.name}</strong>: {m.summary}</p> : null)}
      {!admin && <p>An admin of this server can add it in Settings, Modules.</p>}
      {noBuild && <p>{noBuild.unavailable}</p>}
      {admin && !busy && !noBuild && (mods || ids.map((id) => ({ id, name: id }))).map((m) => (
        <button key={m.id} type="button" className="btn primary" disabled={adding} onClick={() => add(m.id)}>Add {m.name}</button>
      ))}
      {busy && <p>Adding {busy.name}{busy.total ? `: ${Math.round((busy.done || 0) * 100 / busy.total)}%` : '…'} There's no need to restart anything.</p>}
      {err && <p className="module-missing-error">{err}</p>}
    </div>
  )
}

// isModuleMissing finds out whether a request failed because a module isn't here.
export function missingModule(e: unknown): string | null {
  return e instanceof ApiError && e.status === 424 && e.data?.module ? String(e.data.module) : null
}
