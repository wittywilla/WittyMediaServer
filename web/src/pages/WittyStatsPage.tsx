import { lazy, Suspense, useMemo } from 'react'
import { api, getToken, serverBase } from '../api'
import { useApp } from '../state'

// WittyStats, shown here until the module system can load a page by itself: one lazy import and
// the small `host` a module's page is given. This file goes away when that lands. Nothing of the
// module costs anything until this page opens.
const Stats = lazy(async () => ({ default: (await import('@wittywilla/wittystats')).pages.stats }))

// The account's interface choices are stored whole, so what's there is read first and only this
// module's part changes, one write at a time.
let saving: Promise<unknown> = Promise.resolve()

const store = {
  get: (key: string) => api.get<{ wittystats?: Record<string, unknown> }>('/me/ui').then((u) => u.wittystats?.[key]),
  set: (key: string, value: unknown) => {
    saving = saving.catch(() => {}).then(async () => {
      const ui = await api.get<Record<string, unknown>>('/me/ui')
      await api.put('/me/ui', { ...ui, wittystats: { ...(ui.wittystats as Record<string, unknown> | undefined), [key]: value } })
    })
    return saving
  },
}

function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  switch (method) {
    case 'GET': return api.get<T>(path)
    case 'POST': return api.post<T>(path, body)
    case 'PUT': return api.put<T>(path, body)
    case 'PATCH': return api.patch<T>(path, body)
    case 'DELETE': return api.del<T>(path)
    default: return Promise.reject(new Error('unknown method ' + method))
  }
}

export default function WittyStatsPage() {
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const host = useMemo(() => ({ request, store, user: { admin }, token: getToken, base: serverBase }), [admin])
  return <Suspense fallback={null}><Stats host={host} /></Suspense>
}
