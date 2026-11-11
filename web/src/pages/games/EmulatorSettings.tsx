// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { Dialog, Notice, Segmented } from '../../ui'
import { emulatorsApi } from './emulatorsApi'
import { changedValues, inheritedValue, mayEdit, scopesFor, scopeValues, type EmulatorSettingsView, type Field, type SettingsScope, type Values } from './emulatorsLogic'
import { SchemaForm } from './SchemaForm'
import './emulators.css'

// One emulator's settings, drawn from what the emulator says they are. There are three levels,
// each over the one before: an administrator's, for everyone; a person's own, for all their
// games; and theirs for one game. A setting kept for administrators shows at every level, and
// can be changed only at theirs.

const SCOPE_NAMES: Record<SettingsScope, string> = { admin: 'Everyone', user: 'All my games', game: 'This game' }

type Props = {
  emulator: { id: string; name: string }
  game?: { id: number; title: string }
  adminOnly?: boolean // Settings, where an administrator sets the server's own starting point
  onClose: () => void
}

export default function EmulatorSettingsDialog({ emulator, game, adminOnly, onClose }: Props) {
  const [view, setView] = useState<EmulatorSettingsView | null>(null)
  const [failed, setFailed] = useState('')
  const [scope, setScope] = useState<SettingsScope | null>(null)
  // What's been changed and not yet saved, a level at a time.
  const [drafts, setDrafts] = useState<Partial<Record<SettingsScope, Values>>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let live = true
    emulatorsApi.emulatorSettings(emulator.id, game?.id).then((v) => { if (live) setView(v) }).catch((e) => { if (live) setFailed(e?.message || 'These settings couldn’t be loaded.') })
    return () => { live = false }
  }, [emulator.id, game?.id])

  const scopes = useMemo(() => (view ? scopesFor(view.canAdmin, !!game, adminOnly) : []), [view, game, adminOnly])
  // It opens on the narrowest level that already has something in it, or on someone's own.
  useEffect(() => {
    if (!view || scope) return
    const start: SettingsScope = adminOnly ? 'admin' : game && Object.keys(view.game || {}).length ? 'game' : 'user'
    setScope(scopes.includes(start) ? start : scopes[0] || null)
  }, [view, scope, scopes, adminOnly, game])

  const valuesAt = (s: SettingsScope): Values => (view ? drafts[s] || scopeValues(view, s) : {})
  const changesAt = (s: SettingsScope) => (view && drafts[s] ? changedValues(view.schema, scopeValues(view, s), drafts[s]!) : {})
  const dirty = scopes.some((s) => Object.keys(changesAt(s)).length > 0)

  const change = (key: string, value: unknown) => {
    if (!view || !scope) return
    setError('')
    setDrafts((d) => {
      const next = { ...(d[scope] || scopeValues(view, scope)) }
      if (value === undefined) delete next[key]
      else next[key] = value
      return { ...d, [scope]: next }
    })
  }

  const save = async () => {
    if (!view) return
    setBusy(true)
    setError('')
    let now = view
    try {
      // A level at a time, as each is its own request. What was saved stays saved if a later
      // one is refused, and the form shows where things stand.
      for (const s of scopes) {
        const values = changedValues(now.schema, scopeValues(now, s), drafts[s] || scopeValues(now, s))
        if (!Object.keys(values).length) continue
        now = await emulatorsApi.saveEmulatorSettings(emulator.id, s, values, game?.id)
        setView(now)
        setDrafts((d) => ({ ...d, [s]: undefined }))
      }
      toast('Saved')
      onClose()
    } catch (e: any) {
      setError(e?.message || 'That couldn’t be saved.')
    } finally { setBusy(false) }
  }

  const locked = (f: Field) => {
    if (!view || !scope || mayEdit(f, scope, view.canAdmin)) return false
    return scope === 'admin' ? 'Only an administrator can change this' : 'Set by an administrator, for everyone'
  }
  // A setting that can't be changed here shows what it is, which is what applies.
  const shown = useMemo<Values>(() => {
    if (!view || !scope) return {}
    const mine = { ...valuesAt(scope) }
    for (const f of view.schema) if (!mayEdit(f, scope, view.canAdmin) && view.effective[f.key] !== undefined) mine[f.key] = view.effective[f.key]
    return mine
  }, [view, scope, drafts]) // eslint-disable-line react-hooks/exhaustive-deps

  const about = scope === 'admin' ? `Where ${emulator.name} starts for everyone on this server. People can change what’s theirs to change, for themselves.`
    : scope === 'game' ? `Yours, for ${game?.title || 'this game'} only.`
    : `Yours, for every game ${emulator.name} plays.`

  return (
    <Dialog title={`${view?.name || emulator.name} settings`} size="wide" onClose={onClose}
      actions={view && scopes.length > 0
        ? <><button type="button" className="btn ghost" onClick={onClose}>{dirty ? 'Cancel' : 'Close'}</button><button type="button" className="btn primary" disabled={!dirty || busy} onClick={save}>{busy ? 'Saving…' : 'Save'}</button></>
        : <button type="button" className="btn" onClick={onClose}>Close</button>}>
      <div className="emu-dialog-body">
        {failed ? <Notice tone="danger">{failed}</Notice>
          : !view || !scope ? (view && !scopes.length ? <p className="muted">Only an administrator can change these.</p> : <Spinner />)
          : (
            <>
              <div className="emu-scope">
                {scopes.length > 1 && <Segmented label="Who these settings are for" value={scope} onChange={setScope} options={scopes.map((s) => ({ value: s, label: SCOPE_NAMES[s] }))} />}
                <p className="muted">{about}</p>
              </div>
              <SchemaForm schema={view.schema} values={shown} onChange={change} under={(f) => inheritedValue(f, scope, view)} locked={locked} canClear disabled={busy} />
              {error && <div className="emu-form-error"><Notice tone="danger">{error}</Notice></div>}
            </>
          )}
      </div>
    </Dialog>
  )
}
