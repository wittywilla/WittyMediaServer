import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, isNativeShell, serverBase, setToken, type Library, type User } from './api'
import { setLogVersion } from './errorReport'
import { applyBrand, applyTheme, deviceTheme } from './theme'

type Status = { name: string; version: string; setupRequired: boolean; onboarding: boolean; pickerLocal: boolean; user: User | null; brandName?: string; brandLogo?: string; theme?: string; update?: string; legalAck?: boolean }

type AppState = {
  user: User | null
  setupRequired: boolean
  libraries: Library[]
  version: string
  refreshLibraries: () => Promise<void>
  setUser: (u: User | null) => void
  loading: boolean
  // Name shown in the top-left corner ('' = WittyMediaServer) and the
  // server's default theme.
  brandName: string
  brandLogo: string // the custom logo's version, '' = the app icon
  setBrandLogo: (v: string) => void
  serverTheme: string
  setBrandName: (n: string) => void
  setServerTheme: (id: string) => void
  // The setup wizard is still running (after the admin account exists).
  onboarding: boolean
  setOnboarding: (v: boolean) => void
  // The browser runs on the server itself, so the system folder picker can open.
  pickerLocal: boolean
  // The mobile app has no server yet, or can't reach it.
  needServer: boolean
  // A newer version found by the server's update check (admins only; '' = none).
  update: string
  setUpdate: (v: string) => void
  // False until an administrator has seen the first-run legal notice.
  legalAck: boolean
  setLegalAck: (v: boolean) => void
}

const Ctx = createContext<AppState>(null!)
export const useApp = () => useContext(Ctx)

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [user, setUserState] = useState<User | null>(null)
  const [setupRequired, setSetup] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])
  const [version, setVersion] = useState('')
  const [loading, setLoading] = useState(true)
  const [brandName, setBrand] = useState('')
  const [brandLogo, setBrandLogo] = useState('')
  const [serverTheme, setServerThemeState] = useState('default')
  const [onboarding, setOnboarding] = useState(false)
  const [pickerLocal, setPickerLocal] = useState(false)
  const [update, setUpdate] = useState('')
  const [legalAck, setLegalAck] = useState(true)
  const [needServer, setNeedServer] = useState(isNativeShell() && !serverBase())
  const setBrandName = (n: string) => { setBrand(n); applyBrand(n) }
  const setServerTheme = (id: string) => { setServerThemeState(id); if (!deviceTheme()) applyTheme(id) }

  const refreshLibraries = useCallback(async () => {
    try { setLibraries(await api.get<Library[]>('/libraries')) } catch { setLibraries([]) }
  }, [])

  useEffect(() => {
    if (needServer) { setLoading(false); return }
    api.get<Status>('/status').then(async (s) => {
      setSetup(s.setupRequired)
      setOnboarding(!!s.onboarding)
      setPickerLocal(!!s.pickerLocal)
      setUpdate(s.update || '')
      setLegalAck(s.legalAck !== false)
      setVersion(s.version)
      setLogVersion(s.version)
      setBrandName(s.brandName || '')
      setBrandLogo(s.brandLogo || '')
      setServerThemeState(s.theme || 'default')
      applyTheme(deviceTheme() || s.theme || 'default')
      setUserState(s.user)
      if (s.user) await refreshLibraries()
    }).catch(() => { if (isNativeShell()) setNeedServer(true) }).finally(() => setLoading(false))
  }, [refreshLibraries])

  const setUser = (u: User | null) => {
    setUserState(u)
    if (!u) setToken('')
    if (u) { setSetup(false); refreshLibraries() } else setLibraries([])
  }

  return <Ctx.Provider value={{ user, setupRequired, libraries, version, refreshLibraries, setUser, loading, brandName, brandLogo, setBrandLogo, serverTheme, setBrandName, setServerTheme, onboarding, setOnboarding, pickerLocal, needServer, update, setUpdate, legalAck, setLegalAck }}>{children}</Ctx.Provider>
}

// Tiny toast helper.
let toastFn: ((msg: string) => void) | null = null
export const toast = (msg: string) => toastFn?.(msg)
export function ToastHost() {
  const [msg, setMsg] = useState<string | null>(null)
  useEffect(() => {
    toastFn = (m) => { setMsg(m); setTimeout(() => setMsg(null), 3000) }
    return () => { toastFn = null }
  }, [])
  // Always mounted so screen readers announce new messages.
  return <div className={msg ? 'toast' : 'sr-only'} role="status" aria-live="polite">{msg || ''}</div>
}
