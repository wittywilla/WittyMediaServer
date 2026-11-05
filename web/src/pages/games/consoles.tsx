import { Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, mediaUrl, uploadFile, type GameConsole } from '../../api'
import { Spinner } from '../../components/common'
import { toast, useApp } from '../../state'
import { lazyModule } from '../../moduleBundles'
import type { SettingsHost } from '@wittywilla/wittyplay'

// What other pages need from games: the list of consoles, and the settings screen. The page that
// holds the player is Games.tsx, loaded only when a game is opened, and this keeps it that way.

export const SettingsScreen = lazyModule<any>('wittyplay', (m) => m.GameSettingsScreen) // eslint-disable-line @typescript-eslint/no-explicit-any

// How the module's settings screens reach the API.
export const useSettingsHost = (isAdmin: boolean) => useMemo<SettingsHost>(() => ({
  isAdmin,
  mediaUrl,
  upload: (path, body) => uploadFile(path, body),
  request: (method, path, body) => (method === 'GET' ? api.get(path) : method === 'DELETE' ? api.del(path) : method === 'PUT' ? api.put(path, body) : api.post(path, body)),
}), [isAdmin])

// Settings for all of someone's games, or a console's, on the settings page.
export function GameSettingsPage() {
  const { user } = useApp()
  const host = useSettingsHost(!!user?.isAdmin)
  const consoles = useConsoles()
  const say = useCallback((m: string) => toast(m), [])
  // ?console=nes, from a console's page: its settings, not every game's.
  const consoleId = useSearchParams()[0].get('console') || undefined
  return <Suspense fallback={<Spinner />}><SettingsScreen key={consoleId} host={host} consoles={consoles} consoleId={consoleId} onError={say} /></Suspense>
}

let consolesCache: Promise<GameConsole[]> | null = null
export const getConsoles = (fresh = false): Promise<GameConsole[]> => {
  if (!consolesCache || fresh) consolesCache = api.get<GameConsole[]>('/games/consoles')
  return consolesCache
}

export const knownConsoles = new Map<string, string>()

export function useConsoles() {
  const [list, setList] = useState<GameConsole[]>([])
  useEffect(() => { getConsoles().then((l) => { for (const c of l) knownConsoles.set(c.id, c.short); setList(l) }).catch(() => {}) }, [])
  return list
}

// The console picker in the add-library flow and the library editor.
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
