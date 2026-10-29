import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, setToken, type Library, type User } from './api'

type Status = { name: string; version: string; setupRequired: boolean; user: User | null }

type AppState = {
  user: User | null
  setupRequired: boolean
  libraries: Library[]
  version: string
  refreshLibraries: () => Promise<void>
  setUser: (u: User | null) => void
  loading: boolean
}

const Ctx = createContext<AppState>(null!)
export const useApp = () => useContext(Ctx)

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [user, setUserState] = useState<User | null>(null)
  const [setupRequired, setSetup] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])
  const [version, setVersion] = useState('')
  const [loading, setLoading] = useState(true)

  const refreshLibraries = useCallback(async () => {
    try { setLibraries(await api.get<Library[]>('/libraries')) } catch { setLibraries([]) }
  }, [])

  useEffect(() => {
    api.get<Status>('/status').then(async (s) => {
      setSetup(s.setupRequired)
      setVersion(s.version)
      setUserState(s.user)
      if (s.user) await refreshLibraries()
    }).catch(() => {}).finally(() => setLoading(false))
  }, [refreshLibraries])

  const setUser = (u: User | null) => {
    setUserState(u)
    if (!u) setToken('')
    if (u) { setSetup(false); refreshLibraries() } else setLibraries([])
  }

  return <Ctx.Provider value={{ user, setupRequired, libraries, version, refreshLibraries, setUser, loading }}>{children}</Ctx.Provider>
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
  return msg ? <div className="toast">{msg}</div> : null
}
