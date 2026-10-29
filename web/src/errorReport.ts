import { api, getPrefs, getToken } from './api'

// Sends uncaught errors of the web app to the server's log, but only when
// the server owner turned on "App errors" in Settings → Logs & privacy.
// Errors go to the owner's own server, never anywhere else.

let sending = false
let sent = 0

async function report(message: string, stack?: string) {
  if (sending || sent >= 20 || !getToken()) return // never loop, never flood
  sending = true
  try {
    const p = await getPrefs()
    if (!p.clientLogs) return
    sent++
    await api.post('/logs/client', { level: 'error', message: message.slice(0, 500), stack: (stack || '').slice(0, 2000), page: location.pathname, app: 'web' })
  } catch { /* the reporter must never cause errors itself */ } finally { sending = false }
}

export function installErrorReporter() {
  window.addEventListener('error', (e) => { void report(e.message || 'error', e.error?.stack) })
  window.addEventListener('unhandledrejection', (e) => {
    const r = e.reason
    void report(r?.message ? String(r.message) : String(r), r?.stack)
  })
}
