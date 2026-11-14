// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The console a games library holds, in the add-library flow and the library editor. WittyPlay scans games
// libraries and knows the consoles (GET /modules/wittyplay/games/consoles); without it there's no list, and a
// games library can't be made anyway.
import { useEffect, useState } from 'react'
import { api } from '../api'

type Console = { id: string; name: string; short: string; unavailable?: string; bios?: { file: string; for: string; required: boolean }[] }

let asked: Promise<Console[]> | null = null
function getConsoles(): Promise<Console[]> {
  if (!asked) {
    // A failure isn't kept: the next one to ask asks again.
    const p = api.get<Console[]>('/modules/wittyplay/games/consoles')
    asked = p
    p.catch(() => { if (asked === p) asked = null })
  }
  return asked
}

function useConsoles() {
  const [list, setList] = useState<Console[]>([])
  useEffect(() => { getConsoles().then(setList).catch(() => {}) }, [])
  return list
}

export function ConsolePicker({ value, onChange }: { value: string; onChange: (id: string, name: string) => void }) {
  const consoles = useConsoles()
  return (
    <label className="oobe-field">Console
      <select className="select" value={value} onChange={(e) => {
        const id = e.target.value
        onChange(id, id === 'auto' ? 'Games' : consoles.find((c) => c.id === id)?.short || id)
      }}>
        <option value="auto">Many consoles in one folder (sorted by folder and file type)</option>
        {consoles.map((c) => <option key={c.id} value={c.id}>{c.name}{c.unavailable ? ' (listed only, not playable yet)' : ''}</option>)}
      </select>
    </label>
  )
}

// Where BIOS files go, shown wherever someone is choosing a games folder. Nothing
// downloads one, so this is the only way people find out.
export function BiosHint({ console: id }: { console: string }) {
  const consoles = useConsoles()
  const c = consoles.find((x) => x.id === id)
  const required = c?.bios?.find((b) => b.required)
  return (
    <p className="muted game-bios-hint">
      {required
        ? <><b>{c!.name} games need a BIOS</b>: a file such as <code>{required.file}</code>, from a console you own.{' '}</>
        : <>Some consoles need a BIOS file from a console you own.{' '}</>}
      Put BIOS files in a folder named <code>BIOS</code> inside {c ? <>your {c.short} games folder</> : <>each console’s games folder</>}, such as <code>{c ? c.short : 'PS2'}/BIOS</code>.
      Folders inside it are fine. It’s never listed as a game, and the server picks up what it needs when you press Play.
    </p>
  )
}
