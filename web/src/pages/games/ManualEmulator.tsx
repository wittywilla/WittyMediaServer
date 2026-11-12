// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Spinner } from '../../components/common'
import { WebLink } from '../../components/WebLink'
import { toast } from '../../state'
import { Dialog, Notice } from '../../ui'
import SpecPage from '../../spec/SpecPage'
import { manualSpec } from '../../spec/specs/manualEmulator'
import { emulatorsApi } from './emulatorsApi'
import type { CatalogueEntry, ManualView } from './emulatorsLogic'
import './emulators.css'

// Add an emulator you installed yourself: one WittyPlay never downloads (Ryujinx for the Switch,
// Lichii for the Switch 2). The administrator installs it from its own project, and here
// points the server at it, or lets it be found where it's usually installed or in the module's
// drop folder (the way in for a server in a container). The server checks it really is that
// emulator before adding it, and says its version.
export default function ManualEmulatorDialog({ emulator, onClose, onAdded }: { emulator: CatalogueEntry; onClose: () => void; onAdded: () => void }) {
  const [view, setView] = useState<ManualView | null>(null)
  const [failed, setFailed] = useState('')
  const [program, setProgram] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    let live = true
    emulatorsApi.manual(emulator.id).then((v) => { if (live) setView(v) }).catch((e) => { if (live) setFailed(e?.message || 'This couldn’t be loaded.') })
    return () => { live = false }
  }, [emulator.id])
  const add = async (path: string) => {
    setBusy(true)
    setError('')
    try {
      const v = await emulatorsApi.addManual(emulator.id, path)
      setView(v)
      toast(`${emulator.name} ${v.installed?.version && v.installed.version !== 'unknown' ? v.installed.version + ' ' : ''}added`)
      onAdded()
      onClose()
    } catch (e: any) {
      setError(e?.message || 'That couldn’t be added.')
    } finally { setBusy(false) }
  }
  return (
    <Dialog title={`Add ${emulator.name} you installed yourself`} size="wide" onClose={onClose}
      actions={<><button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
        <button type="button" className="btn primary" disabled={busy || (!program.trim() && !view?.detected)} onClick={() => { void add(program.trim()) }}>{busy ? 'Checking…' : 'Add'}</button></>}>
      <div className="emu-dialog-body">
        {failed ? <Notice tone="danger">{failed}</Notice> : !view ? <Spinner /> : (
          <>
            <p>WittyPlay never downloads {emulator.name}. Install it yourself from its own project: <WebLink href={view.download}>{view.download}</WebLink>. Then add it here. The server checks it really is {emulator.name} before using it.</p>
            <SpecPage embedded page={manualSpec({ name: emulator.name, id: emulator.id, detected: view.detected || undefined, dropFolder: view.dropFolder, notes: view.notes, programHint: view.programs?.[0] ? `…/${view.programs[0]}` : '', system: view.system, programs: view.programs || [], file: view.file }, program, error)}
              onField={(_id, v) => setProgram(String(v))} />
          </>
        )}
      </div>
    </Dialog>
  )
}
