import { Suspense, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, getToken, mediaUrl, posterUrl, serverBase, type Item } from '../api'
import { LoadError, Spinner } from '../components/common'
import { reportClient, setLogSession } from '../errorReport'
import { useApp } from '../state'
import { couchOn } from './games/data'
import { getConsoles, knownConsoles, useConsoles, useSettingsHost } from './games/consoles'
import type { Background, ControllerData, ControlsData, PlayHost, PrepareStatus, Profile, ProfileHost, SessionInfo } from '@wittywilla/wittyplay'
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
      // Version 2 of the stream, the same for every device: docs/GAMING-STREAM.md.
      stream: (sid, hello, sdp) => api.post(`/play/${sid}/stream`, { hello, sdp }),
      streamUrl: (sid) => {
        const url = mediaUrl(`/api/play/${sid}/stream`)
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
