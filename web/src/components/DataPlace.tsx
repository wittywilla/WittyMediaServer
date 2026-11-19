// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { checkDataLocation, size, syncedWords, type DataChoice, type DataLocation, type MoveState, type PathCheck } from '../dataLocation'
import { ChoiceCards, type Card } from '../pages/setup/parts'
import { Button, Field, Input, Notice, Progress } from '../ui'
import { ChooseFolderButton } from './FolderPicker'
import './dataPlace.css'

// Choosing where the server keeps its settings and data: Documents, the install folder, or a folder
// someone types or browses to. The same in first-time setup and Settings, Data folder. Every choice
// is checked on the server before it can be made: room, whether the server can write there, and
// whether a sync program keeps it in the cloud.

export type Place = { choice: DataChoice; path: string }

// The place to start from: Documents when it can be had, as setup suggests it, else a folder on this computer.
export function firstPlace(v: DataLocation): Place {
  const docs = v.options.find((o) => o.choice === 'documents')
  if (docs?.available && !docs.current) return { choice: 'documents', path: '' }
  return { choice: 'custom', path: v.local }
}

// usePlaceCheck asks the server about the place chosen, a moment after typing stops.
export function usePlaceCheck(v: DataLocation | null, p: Place): { check: PathCheck | null; checking: boolean } {
  const [check, setCheck] = useState<PathCheck | null>(null)
  const [checking, setChecking] = useState(false)
  useEffect(() => {
    if (!v) return
    let gone = false
    setChecking(true)
    setCheck(null) // what was said about the last place isn't about this one
    const t = setTimeout(() => {
      checkDataLocation(p.choice, p.path)
        .then((c) => { if (!gone) setCheck(c) })
        .catch((e) => { if (!gone) setCheck({ path: '', problem: (e as Error).message }) })
        .finally(() => { if (!gone) setChecking(false) })
    }, p.choice === 'custom' ? 400 : 0)
    return () => { gone = true; clearTimeout(t) }
  }, [v, p.choice, p.path])
  return { check, checking }
}

export function DataPlace({ view, place, onChange, check, checking }: { view: DataLocation; place: Place; onChange: (p: Place) => void; check: PathCheck | null; checking: boolean }) {
  const docs = view.options.find((o) => o.choice === 'documents')
  const inst = view.options.find((o) => o.choice === 'install')
  const free = (n?: number) => (n ? `${size(n)} free` : '')
  const cards: Card<DataChoice>[] = [
    { value: 'documents', title: 'Documents' + (docs?.current ? ' (now)' : ''), text: docs?.available ? `${docs.path}${docs.synced ? `. Synced by ${docs.synced}` : ''}` : docs?.why || 'Not available' },
    { value: 'install', title: 'The install folder' + (inst?.current ? ' (now)' : ''), text: inst?.available ? `${inst.path}. Everything stays together, to copy or carry as one folder.` : 'Not available here' },
    { value: 'custom', title: 'Somewhere else', text: 'A folder you choose, such as one on a bigger or faster drive.' },
  ]
  const chosen = place.choice === 'documents' ? docs : place.choice === 'install' ? inst : null
  const synced = check?.synced || (chosen?.synced ?? '')
  return (
    <div className="dp">
      <ChoiceCards label="Where to keep it" value={place.choice} onChange={(c) => onChange({ choice: c, path: c === 'custom' ? place.path || view.local : '' })} cards={cards} />
      {place.choice === 'custom' && (
        <div className="dp-custom">
          <Field label="Folder" hint={`The full path. A folder that isn’t there yet is made. It should be empty, on this computer, and not in a folder that’s synced to the cloud.`}>
            <Input value={place.path} onChange={(e) => onChange({ choice: 'custom', path: e.target.value })} spellCheck={false} autoComplete="off" placeholder={view.local} />
          </Field>
          <div className="dp-browse">
            <ChooseFolderButton browseOnly label="Browse folders" onPick={(p) => onChange({ choice: 'custom', path: p.replace(/[\\/]+$/, '') + view.sep + 'WittyMediaServer' })} />
            {view.local && place.path !== view.local && <Button variant="ghost" onClick={() => onChange({ choice: 'custom', path: view.local })}>Use {view.local}</Button>}
          </div>
        </div>
      )}
      <div className="dp-status" aria-live="polite">
        {chosen && !chosen.available && <Notice tone="warn" title="Not available.">{' '}{chosen.why}</Notice>}
        {(chosen?.available || place.choice === 'custom') && checking && !check && <p className="muted">Checking the folder…</p>}
        {(chosen?.available || place.choice === 'custom') && check && (
          check.problem
            ? <Notice tone="warn" title="Can’t use this folder.">{' '}{check.problem}</Notice>
            : <p className="dp-ok">{check.current ? 'Your data is in this folder now.' : `${check.path}${check.free ? ` · ${free(check.free)}` : ''}${view.bytes ? ` · your data takes ${size(view.bytes)}` : ''}`}</p>
        )}
        {synced && !check?.problem && (
          <Notice tone="warn" title={`Synced by ${synced}.`} action={view.local && !(place.choice === 'custom' && place.path === view.local) ? <Button onClick={() => onChange({ choice: 'custom', path: view.local })}>Use a local folder</Button> : undefined}>
            {' '}{syncedWords(synced)}{view.local ? ` ${view.local} is on this computer and isn’t synced.` : ''}
          </Notice>
        )}
      </div>
    </div>
  )
}

// MoveProgress says what a move is doing while the server starts again and copies.
export function MoveProgress({ state }: { state: MoveState }) {
  const words: Record<MoveState['phase'], string> = {
    asking: 'Asking the server…',
    restarting: 'The server is starting again to move your data…',
    moving: 'Copying your data and checking the copy…',
    done: 'Done. Your data is in its new folder.',
    failed: 'Your data couldn’t be moved, and is still where it was.',
  }
  return (
    <div className="dp-progress" role="status">
      <p>{words[state.phase]}</p>
      {state.phase === 'moving' && state.total ? <Progress label="How much is copied" value={state.done || 0} max={state.total} showValue /> : null}
      {(state.phase === 'asking' || state.phase === 'restarting') && <Progress label="Starting again" />}
      {state.phase === 'failed' && state.error && <Notice tone="danger">{state.error}</Notice>}
    </div>
  )
}
