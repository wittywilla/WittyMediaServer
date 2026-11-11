// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Suspense, useMemo, type ComponentType, type LazyExoticComponent } from 'react'
import { useParams } from 'react-router-dom'
import { api, getToken, serverBase } from '../api'
import { lazyModule } from '../moduleBundles'
import { useApp } from '../state'
import { Spinner } from '../components/common'

// A page a module adds to the app: /m/<module>/<page>. Its bundle exports `pages`, components by
// id, and each gets one prop: `host`, which is all it knows of the app. It calls the API with the
// viewer's own access, so a page can't see more than they could.
export type PageHost = {
  request: <T = unknown>(method: string, path: string, body?: unknown) => Promise<T>
  user: { admin: boolean }
  store: { get: (key: string) => Promise<unknown>; set: (key: string, value: unknown) => Promise<void> }
  token: () => string
  base: () => string
}

const WORD = /^[a-z0-9-]+$/i

// Only the server's own API: a page can't be made to send the viewer's sign-in anywhere else.
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  if (!path.startsWith('/') || path.startsWith('//')) throw new Error('A module can only ask this server.')
  switch (method.toUpperCase()) {
    case 'GET': return api.get<T>(path)
    case 'POST': return api.post<T>(path, body)
    case 'PUT': return api.put<T>(path, body)
    case 'PATCH': return api.patch<T>(path, body)
    case 'DELETE': return api.del<T>(path)
    default: throw new Error('A module asked in a way this server has no use for: ' + method)
  }
}

const loaded = new Map<string, LazyExoticComponent<ComponentType<{ host: PageHost }>>>()
function pageOf(module: string, page: string) {
  const key = module + '/' + page
  let c = loaded.get(key)
  if (!c) {
    c = lazyModule<{ host: PageHost }>(module, (m) => (m.pages as Record<string, unknown> | undefined)?.[page])
    loaded.set(key, c)
  }
  return c
}

export default function ModulePage() {
  const { module = '', page = '' } = useParams()
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const host = useMemo<PageHost>(() => ({
    request,
    user: { admin },
    store: {
      get: (key) => api.get(`/module-store/${module}/${encodeURIComponent(key)}`),
      set: async (key, value) => { await api.put(`/module-store/${module}/${encodeURIComponent(key)}`, value) },
    },
    token: getToken,
    base: serverBase,
  }), [module, admin])
  if (!WORD.test(module) || !WORD.test(page)) return null
  const Page = pageOf(module, page)
  return <Suspense fallback={<Spinner />}><Page host={host} /></Suspense>
}
