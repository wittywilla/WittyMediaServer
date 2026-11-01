import { Suspense, useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, getToken, mediaUrl, posterUrl, serverBase, uploadFile, type GameConsole, type Item } from '../api'
import { LoadError, Spinner } from '../components/common'
import { reportClient, setLogSession } from '../errorReport'
import { toast, useApp } from '../state'
import { couchOn } from './games/data'
import type { Background, ControllerData, ControlsData, PlayHost, PrepareStatus, Profile, ProfileHost, SessionInfo, SettingsHost } from '@wittywilla/wittyplay'
import { lazyModule } from '../moduleBundles'
import './games.css'

// Games: what the game pages share, and the page that holds the player. The library and a
// game's own page are in games/.
// The player itself is WittyPlay (the web folder of WittyPlay's repository), loaded only when a game
// is opened.

const GamePlayer = lazyModule<any>('wittyplay', (m) => m.default) // eslint-disable-line @typescript-eslint/no-explicit-any
const Controllers = lazyModule<any>('wittyplay', (m) => m.ControllersPage) // eslint-disable-line @typescript-eslint/no-explicit-any

// Controller profiles and the background, through the same routes a TV app uses.
const profileHost: ProfileHost = {
  list: () => api.get<Profile[]>('/game-profiles'),
  create: (p) => api.post<Profile>('/game-profiles', p),
  update: (id, p) => api.put<Profile>(`/game-profiles/${id}`, p),
  remove: async (id) => { await api.del(`/game-profiles/${id}`) },
  copy: (id) => api.post<Profile>(`/game-profiles/${id}/copy`),
  setActive: (id, active) => api.post<Profile>(`/game-profiles/${id}/active`, { active }),
  exportFile: async (ids) => JSON.stringify(await api.get(`/game-profiles/export?${(ids || []).map((i) => `id=${i}`).join('&')}`), null, 2),
  importFile: (text) => sendBytes('POST', '/game-profiles/import', text),
  setBackground: (b) => api.put<Background>('/game-background', b),
  uploadBackground: (f) => sendBytes('PUT', '/game-background/image', f),
  removeBackground: () => api.del<Background>('/game-background'),
  backgroundUrl: (v) => mediaUrl('/api/game-background/image') + (mediaUrl('/api/game-background/image').includes('?') ? '&' : '?') + `v=${v}`,
}

// The body as it is, for a picture or a file. The server looks inside to see what it is.
async function sendBytes<T>(method: string, path: string, body: BodyInit): Promise<T> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(`${serverBase()}/api${path}`, { method, headers, body, credentials: 'same-origin' })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.error || 'That didn’t work')
  return data as T
}

let knownControllers: Promise<ControllerData> | null = null
const getControllers = () => {
  if (!knownControllers) knownControllers = api.get<ControllerData>('/game-controllers')
  knownControllers.catch(() => { knownControllers = null })
  return knownControllers
}

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
  return <Suspense fallback={<Spinner />}><SettingsScreen host={host} consoles={consoles} onError={say} /></Suspense>
}
let consolesCache: Promise<GameConsole[]> | null = null
export const getConsoles = (fresh = false): Promise<GameConsole[]> => {
  if (!consolesCache || fresh) consolesCache = api.get<GameConsole[]>('/games/consoles')
  return consolesCache
}

export function useConsoles() {
  const [list, setList] = useState<GameConsole[]>([])
  useEffect(() => { getConsoles().then((l) => { for (const c of l) knownConsoles.set(c.id, c.short); setList(l) }).catch(() => {}) }, [])
  return list
}

// The full-window page around the player, and the player's way to the API.
export function GamePage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [params] = useSearchParams()
  const [it, setIt] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const { user } = useApp()
  const settingsHost = useSettingsHost(!!user?.isAdmin)
  useEffect(() => { Promise.all([api.get<Item>(`/items/${id}`), getConsoles().then((l) => { for (const c of l) knownConsoles.set(c.id, c.short) }).catch(() => {})]).then(([i]) => setIt(i)).catch((e) => setErr(e.message)) }, [id])
  const host = useMemo<PlayHost | null>(() => {
    if (!it) return null
    return {
      ...settingsHost,
      itemId: it.id, title: it.title, consoleId: it.console || '', consoleName: consoleName(it.console || ''), resume: params.get('resume') === '1', safeMode: params.get('safe') === '1',
      startSlot: Number(params.get('slot') || 0) || undefined, cover: it.poster ? `${posterUrl(it)}&w=480` : undefined,
      openSettings: () => nav('/settings/gaming'),
      prepare: () => api.post<PrepareStatus>(`/games/${it.id}/prepare`),
      start: (resume, safeMode) => api.post<SessionInfo>(`/games/${it.id}/play`, { resume, safeMode, client: 'browser' }).then((s) => { setLogSession(s.id); return s }),
      report: (level, message) => reportClient({ level, kind: 'game', message, module: 'wittyplay' }),
      relaunch: ({ resume, safeMode }) => {
        const q = [resume ? 'resume=1' : '', safeMode ? 'safe=1' : ''].filter(Boolean).join('&')
        window.location.assign(window.location.pathname + (q ? '?' + q : ''))
      },
      controls: (device) => api.get<any>(`/consoles/${it.console}/controls?device=${encodeURIComponent(device)}&game=${it.id}`),
      controllers: getControllers, profiles: profileHost,
      saveControl: async (s) => { await api.put(`/consoles/${it.console}/controls`, s) },
      stop: async (sid) => { setLogSession(null); await api.del(`/play/${sid}`) },
      answer: async (sid, sdp, prefer) => (await api.post<{ sdp: string }>(`/play/${sid}/webrtc`, { sdp, prefer })).sdp,
      // As for pictures and sound: over HTTPS in a browser the sign-in cookie comes along.
      wsUrl: (sid, video) => {
        const url = mediaUrl(`/api/play/${sid}/ws?video=${video}`)
        return (url.startsWith('http') ? url : window.location.origin + url).replace(/^http/, 'ws')
      },
      action: (sid, type, slot, on, extra) => api.post(`/play/${sid}/action`, { type, slot: slot ?? 0, on: !!on, ...extra }),
      close: () => nav(couchOn() ? `/big/game/${it.id}` : `/item/${it.id}`, { replace: true }),
    }
  }, [it, settingsHost])
  if (err) return <div className="wp-page-error" role="alert"><LoadError what="This game" onRetry={() => window.location.reload()} /><p className="muted">{err}</p></div>
  if (!host) return <Spinner />
  return <Suspense fallback={<Spinner />}><GamePlayer host={host} /></Suspense>
}

const consoleName = (id: string) => knownConsoles.get(id) || id.toUpperCase()
const knownConsoles = new Map<string, string>()

// The controller screen on a page of its own: the game menu has the same one.
export function ControllersScreen() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
  const consoles = useConsoles()
  const consoleId = params.get('console') || consoles.find((c) => c.ready)?.id || consoles[0]?.id || ''
  const controls = useMemo(() => (id: string, device: string) => api.get<ControlsData>(`/consoles/${encodeURIComponent(id)}/controls?device=${encodeURIComponent(device)}`), [])
  if (!consoleId) return <Spinner />
  return (
    <Suspense fallback={<Spinner />}>
      <Controllers profiles={profileHost} controls={controls} controllers={getControllers} consoles={consoles} consoleId={consoleId}
        onConsole={(id: string) => setParams({ console: id }, { replace: true })} saveControl={async (id: string, s: unknown) => { await api.put(`/consoles/${id}/controls`, s) }} onClose={() => nav(-1)} />
    </Suspense>
  )
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
