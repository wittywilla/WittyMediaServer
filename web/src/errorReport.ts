import { ApiError, getPrefs, getToken, serverBase } from './api'

// Sends what goes wrong in the web app to the log of this server, where it can
// be read beside what the server itself was doing. It goes to the owner's own
// server and nowhere else, and the owner can turn it off ("App errors" in
// Settings → WittyLog).
//
// Reporting must never make anything worse: a report that fails is dropped and
// not reported, repeats are grouped, the queue is bounded and there is a limit
// per minute. Nothing here may throw into the page.

type Level = 'error' | 'warn' | 'info'
export type ClientReport = {
  level: Level
  kind: string // js, request, network, player, game, csp...
  message: string
  stack?: string
  session?: string | null // leave out for the session in play
  request?: string // the X-Request-Id of a failed request
  module?: string // the web module it happened in
}
type Queued = { level: Level; kind: string; message: string; stack?: string; session?: string; request?: string; module?: string; page: string; time: string; count: number }

const FLUSH_MS = 5000
const ERROR_FLUSH_MS = 800 // soon, but long enough to group a burst
const MAX_QUEUE = 50
const MAX_PER_MINUTE = 20
const BATCH_EVENTS = 25 // what the server takes in one report
const BATCH_CHARS = 40000 // under its limit on size, and the browser's for a request sent while leaving
const BACK_OFF_MS = 60000
const OFF_MS = 5 * 60000 // turned off by the owner: ask again after a while
const STALL_MS = 60000

let queue: Queued[] = []
let timer = 0
let timerAt = 0
let sending = false
let quietUntil = 0 // told to wait: reports queue up, within the limit
let offUntil = 0 // switched off: nothing is kept
let windowStart = 0
let windowCount = 0
let session: string | null = null
let version = ''
let leftAt = 0

// Tokens never go in a report, even though the server hides them as well.
const scrub = (s: string) => s.replace(/([?&](?:token|st)=)[^&\s"'#)]*/gi, '$1hidden').replace(/\bBearer\s+[\w.~+/=-]+/gi, 'Bearer hidden')
const pathOnly = (u: string) => u.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/]*/i, '').split(/[?#]/)[0]
const pageNow = () => location.pathname.replace(/^\/s\/[^/]+/, '/s/hidden') // a share link is a key

function device(): string {
  const n = navigator as Navigator & { userAgentData?: { platform?: string } }
  return String(n.userAgentData?.platform || n.platform || 'unknown').slice(0, 60)
}

// The stream, disc or game in play: what goes wrong from now on is filed under it.
export function setLogSession(id: string | null) { session = id || null }

// The server's version, for the reports.
export function setLogVersion(v: string) { version = v }

export function reportClient(r: ClientReport) {
  try {
    const now = Date.now()
    if (now < offUntil || !getToken()) return
    const message = scrub(String(r.message || 'unknown error')).slice(0, 1000)
    const sid = (r.session === undefined ? session : r.session) || undefined
    const same = queue.find((q) => q.message === message && q.kind === r.kind && q.level === r.level && q.session === sid && q.module === r.module)
    if (same) { same.count++; return }
    if (queue.length >= MAX_QUEUE) return
    if (now - windowStart >= 60000) { windowStart = now; windowCount = 0 }
    if (windowCount >= MAX_PER_MINUTE) return
    windowCount++
    queue.push({
      level: r.level, kind: r.kind, message, stack: r.stack ? scrub(String(r.stack)).slice(0, 4000) : undefined, session: sid,
      request: r.request || undefined, module: r.module || undefined, page: pageNow(), time: new Date(now).toISOString(), count: 1,
    })
    schedule(r.level === 'error' ? ERROR_FLUSH_MS : FLUSH_MS)
  } catch { /* never let a report cause an error */ }
}

function schedule(ms: number) {
  const at = Date.now() + ms
  if (timer && timerAt <= at) return
  clearTimeout(timer)
  timerAt = at
  timer = window.setTimeout(() => { timer = 0; void flush() }, ms)
}

function takeBatch(): Queued[] {
  const out: Queued[] = []
  let chars = 0
  while (queue.length && out.length < BATCH_EVENTS) {
    const n = JSON.stringify(queue[0]).length
    if (out.length && chars + n > BATCH_CHARS) break
    chars += n
    out.push(queue.shift()!)
  }
  return out
}

async function post(events: Queued[], keepalive: boolean) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  const body = JSON.stringify({ app: 'web', version, device: device(), events })
  try {
    const res = await fetch(serverBase() + '/api/logs/client', { method: 'POST', headers, body, credentials: 'same-origin', keepalive })
    // Too many, or signed out: it won't be better straight away.
    if (res.status === 429 || res.status === 401 || res.status === 403) quietUntil = Date.now() + BACK_OFF_MS
  } catch { /* dropped: this is not reported */ }
}

async function flush() {
  if (sending || !queue.length) return
  const now = Date.now()
  if (now < quietUntil) { schedule(quietUntil - now); return }
  if (!getToken()) { queue = []; return }
  sending = true
  try {
    const p = await getPrefs()
    if (p.clientLogs === false) { queue = []; offUntil = Date.now() + OFF_MS; return }
    await post(takeBatch(), false)
  } catch { /* dropped */ } finally {
    sending = false
    if (queue.length) schedule(FLUSH_MS)
  }
}

// The page is going away, or may be about to: send what is waiting at once,
// with a request that outlives the page.
function flushLeaving() {
  try {
    if (queue.length && Date.now() >= quietUntil && getToken()) void post(takeBatch(), true)
  } catch { /* nothing to be done */ }
}

// A failed API request. 401, 403, 404 and the like are normal answers; 501 means
// a feature this system lacks. Requests cut off by leaving the page aren't failures.
export function reportRequestFailure(method: string, path: string, status: number, message: string, requestId?: string | null) {
  if (status > 0 && (status < 500 || status === 501)) return
  if (status === 0 && (Date.now() - leftAt < 3000 || navigator.onLine === false)) return
  const where = `${method} /api${pathOnly(path)}`
  reportClient(status
    ? { level: 'error', kind: 'request', message: `${where} failed: ${status} ${message}`, request: requestId || undefined }
    : { level: 'error', kind: 'network', message: `${where} failed: the server could not be reached` })
}

const MEDIA_ERRORS: Record<number, string> = {
  1: 'loading was stopped before it finished (MEDIA_ERR_ABORTED)',
  2: 'a network problem stopped it loading (MEDIA_ERR_NETWORK)',
  3: 'the browser could not decode it (MEDIA_ERR_DECODE)',
  4: 'the browser cannot play this file or format (MEDIA_ERR_SRC_NOT_SUPPORTED)',
}

// A <video> or <audio> element's error, in words. what says which thing it was.
export function reportMediaError(el: HTMLMediaElement, what: string) {
  const e = el.error
  if (!e || e.code === 1) return // aborted: someone moved on
  const detail = e.message ? `: ${e.message}` : ''
  reportClient({ level: 'error', kind: 'player', message: `${what} failed: ${MEDIA_ERRORS[e.code] || `error ${e.code}`}${detail}` })
}

// A fatal error from hls.js.
export function reportHlsError(d: { type?: string; details?: string; reason?: string; response?: { code?: number } }) {
  const code = d.response?.code ? `, HTTP ${d.response.code}` : ''
  reportClient({ level: 'error', kind: 'player', message: `Stream failed: ${d.type || 'error'}, ${d.details || 'unknown'}${code}${d.reason ? `: ${d.reason}` : ''}` })
}

// Watches one source playing in an element, until the function it returns is
// called: its session (from the address, unless the player has one of its own),
// media errors, and stalls that never recover.
export function watchPlayback(el: HTMLMediaElement, url: string, ownSession?: string | null): () => void {
  const hls = !ownSession && /\/api\/hls\/([^/?#]+)\//.exec(url)
  const id = hls ? hls[1] : null
  if (id) setLogSession(id)
  const place = id || ownSession ? 'Video' : `Video ${pathOnly(url)}`
  let stall = 0
  const clear = () => { clearTimeout(stall); stall = 0 }
  const onError = () => reportMediaError(el, place)
  const onWaiting = () => {
    clear()
    stall = window.setTimeout(() => {
      if (!el.paused && el.readyState < 3 && !document.hidden) reportClient({ level: 'error', kind: 'player', message: `${place} stalled: nothing has played for a minute` })
    }, STALL_MS)
  }
  el.addEventListener('error', onError)
  el.addEventListener('waiting', onWaiting)
  for (const ev of ['playing', 'pause', 'emptied'] as const) el.addEventListener(ev, clear)
  return () => {
    clear()
    el.removeEventListener('error', onError)
    el.removeEventListener('waiting', onWaiting)
    for (const ev of ['playing', 'pause', 'emptied'] as const) el.removeEventListener(ev, clear)
    if (id && session === id) setLogSession(null)
  }
}

const fromExtension = (s?: string) => !!s && /^(chrome|moz|safari)(-web)?-extension:/.test(s)

let installed = false

export function installErrorReporter() {
  if (installed) return
  installed = true
  window.addEventListener('error', (e) => {
    const m = e.message || ''
    if (fromExtension(e.filename) || /ResizeObserver loop|^Script error\.?$/.test(m)) return // not this app, or harmless
    reportClient({ level: 'error', kind: 'js', message: m || 'error', stack: e.error?.stack })
  })
  window.addEventListener('unhandledrejection', (e) => {
    const r = e.reason
    if (r instanceof ApiError || r?.name === 'AbortError') return // failed requests are reported when they fail
    let message = 'promise rejected'
    try { message = r?.message ? String(r.message) : String(r) } catch { /* it has no words */ }
    reportClient({ level: 'error', kind: 'js', message, stack: r?.stack })
  })
  document.addEventListener('securitypolicyviolation', (e) => {
    if (fromExtension(e.blockedURI) || fromExtension(e.sourceFile)) return
    reportClient({ level: 'error', kind: 'csp', message: `Blocked by the content security policy: ${e.effectiveDirective} ${pathOnly(e.blockedURI || '')}` })
  })
  window.addEventListener('beforeunload', () => { leftAt = Date.now() })
  window.addEventListener('pagehide', () => { leftAt = Date.now(); flushLeaving() })
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'hidden') flushLeaving() })
}
