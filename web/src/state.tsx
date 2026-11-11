// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, isNativeShell, serverBase, setToken, type Library, type User } from './api'
import { setLogVersion } from './errorReport'
import { applyBrand, applyTheme, deviceTheme } from './theme'

type Status = { name: string; version: string; setupRequired: boolean; onboarding: boolean; pickerLocal: boolean; user: User | null; brandName?: string; brandColouring?: string; brandLogo?: string; theme?: string; update?: string; legalAck?: boolean; plainConnection?: boolean }

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
  // How it is coloured: 'auto', 'one' or the chosen words (brandName.ts).
  brandColouring: string
  brandLogo: string // the custom logo's version, '' = the app icon
  setBrandLogo: (v: string) => void
  serverTheme: string
  setBrandName: (n: string, colouring?: string) => void
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
  // This page came over the internet unencrypted, which the owner allowed: a
  // password typed here can be read on the way.
  plainConnection: boolean
}

// One context for the life of the page. While developing, a hot update of this file would otherwise make a
// second one, and the parts of the page still holding the first find no app: useApp() answered null, and
// Shell crashed on it ("Cannot destructure property 'user'"). A built server has no hot updates.
const Ctx: React.Context<AppState> = import.meta.hot
  ? ((import.meta.hot.data.appContext as React.Context<AppState> | undefined) ??= createContext<AppState>(null!))
  : createContext<AppState>(null!)
export const useApp = () => useContext(Ctx)

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [user, setUserState] = useState<User | null>(null)
  const [setupRequired, setSetup] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])
  const [version, setVersion] = useState('')
  const [loading, setLoading] = useState(true)
  const [brandName, setBrand] = useState('')
  const [brandColouring, setBrandColouring] = useState('auto')
  const [brandLogo, setBrandLogo] = useState('')
  const [serverTheme, setServerThemeState] = useState('default')
  const [onboarding, setOnboarding] = useState(false)
  const [pickerLocal, setPickerLocal] = useState(false)
  const [update, setUpdate] = useState('')
  const [legalAck, setLegalAck] = useState(true)
  const [plainConnection, setPlainConnection] = useState(false)
  const [needServer, setNeedServer] = useState(isNativeShell() && !serverBase())
  const setBrandName = (n: string, colouring = 'auto') => { setBrand(n); setBrandColouring(colouring); applyBrand(n) }
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
      setPlainConnection(!!s.plainConnection)
      setVersion(s.version)
      setLogVersion(s.version)
      setBrandName(s.brandName || '', s.brandColouring || 'auto')
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

  return <Ctx.Provider value={{ user, setupRequired, libraries, version, refreshLibraries, setUser, loading, brandName, brandColouring, brandLogo, setBrandLogo, serverTheme, setBrandName, setServerTheme, onboarding, setOnboarding, pickerLocal, needServer, update, setUpdate, legalAck, setLegalAck, plainConnection }}>{children}</Ctx.Provider>
}

// Toasts live with the other shared parts; every page imports them from here.
export { toast, ToastHost } from './ui/Toast'
