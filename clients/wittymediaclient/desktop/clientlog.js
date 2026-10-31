// The app's own log: a small file on this computer, and errors reported to the
// server this app is connected to, which is the only place they ever go. Nothing
// in here may throw, block the window or cause more errors of its own: a report
// that fails is dropped, and writing is queued and bounded. There's no console
// output either, as a closed console raises an error that would log another.
const fs = require('fs')
const os = require('os')
const path = require('path')

const FILE = 'client.log'
const MAX_FILE_BYTES = 2 * 1024 * 1024
const KEEP_FILES = 3
const LINES_PER_MINUTE = 120
const MAX_UNWRITTEN_BYTES = 256 * 1024
const RECENT_LINES = 20

// The route's own limits are 64 KB and 25 events, and 60 events a minute for an
// account. The web app shares those, so this takes half.
const FLUSH_MS = 5000
const MAX_EVENTS = 25
const MAX_BODY_BYTES = 60 * 1024
const SEND_PER_MINUTE = 30
const MAX_WAITING = 100
const HOLD_AFTER_FAILURE_MS = 60 * 1000
const MAX_HOLD_MS = 10 * 60 * 1000

const MAX_INPUT = 16 * 1024 // looked at for secrets; anything longer isn't kept anyway
const MAX_MESSAGE = 1000
const MAX_STACK = 4000
const MAX_FIELD = 200
const MINUTE = 60 * 1000

const HIDDEN = '[hidden]'

// The same rules as the server's log, and the same tests on every client
// (shared/data/redaction-cases.json). Runs of letters have a limit here because
// these run on the main process, where a slow pattern would freeze the window.
const PATTERNS = [
  // In an address or a command line: token=..., ?st=..., password=...
  [/(\b(?:token|access_token|refresh_token|id_token|auth|api[_\-]?key|apikey|password|passwd|pwd|pass|secret|pin|key|st|sig|signature|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|otp|totp)=)[^\&\s"'<>]+/gi, `$1${HIDDEN}`],
  // The same with its name percent-encoded: %74oken=... signs in just the same.
  [/([^\s\&?=;"'<>]{0,24}%[0-9a-f]{2}[^\s\&?=;"'<>]{0,24}=)[^\&\s"'<>]+/gi, `$1${HIDDEN}`],
  // A header, as a line of text.
  [/\b(proxy-authorization|authorization|set-cookie|cookie|x-wms-token|x-api-key)(\s*[:=]\s*)[^\r\n"']+/gi, `$1$2${HIDDEN}`],
  [/\b(Bearer\s+)[A-Za-z0-9._~+\/=\-]+/gi, `$1${HIDDEN}`],
  [/\b(Basic\s+)[A-Za-z0-9+\/]*[A-Z0-9+\/][A-Za-z0-9+\/]{10,}={0,2}/g, `$1${HIDDEN}`],
  // JSON, finished or cut short: "password": "..."
  [/("(?:[a-z_\-]{0,40}(?:token|secret)|password|passwd|api[_\-]?key|apikey|pin|code|setup[_\-]?code|recovery[_\-]?code|pair(?:ing)?[_\-]?code|cookie|authorization)"\s*:\s*)"[^"\\]*(?:\\.[^"\\]*)*(?:"|$)/gi, `$1"${HIDDEN}"`],
  // A login in an address: rtsp://user:password@camera/
  [/\b([a-z][a-z0-9+.\-]{0,20}:\/\/)[^\/\s:@]+:[^\/\s@]+@/gi, `$1${HIDDEN}@`],
  // A share link's token.
  [/(\/(?:api\/)?(?:s|share)\/)[A-Za-z0-9_\-]{8,}/g, `$1${HIDDEN}`],
  // The sign-in cookie and stream tokens, however they got there.
  [/\b(wms_token=|st1\.|st_)[A-Za-z0-9._~\-]{6,}/gi, `$1${HIDDEN}`],
  // A long run of hex standing alone is what a session token looks like.
  [/\b[a-fA-F0-9]{48,}\b/g, HIDDEN],
]

// Takes secrets out of free text. If that goes wrong the text isn't kept at all.
function redact(text, secrets = []) {
  try {
    let s = String(text).slice(0, MAX_INPUT)
    for (const v of secrets) s = s.split(v).join(HIDDEN)
    for (const [pattern, replacement] of PATTERNS) s = s.replace(pattern, replacement)
    return s
  } catch {
    return '[left out: it could not be cleaned]'
  }
}

// One line of the log file. JSON already escapes line breaks and control
// characters. This adds what it leaves alone but a terminal or editor acts on.
function toLine(record) {
  return JSON.stringify(record).replace(/[\u007f-\u009f\u2028\u2029\u202a-\u202e\u2066-\u2069]/g,
    (c) => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0')) + '\n'
}

// options: dir, app, version, device, and how reports leave:
//   target()             where reports can go now: { origin, token }, or null for not now
//   post(target, body)   sends one request, answers { status, retryAfter }
// Tests can also give fs, home, maxFileBytes, keepFiles, flushMs, linesPerMinute and now.
function createClientLog(options) {
  const { dir, app = 'desktop', version = '', device = '', target, post } = options
  const files = options.fs || fs
  const home = options.home === undefined ? os.homedir() : options.home
  const maxBytes = options.maxFileBytes || MAX_FILE_BYTES
  const keep = options.keepFiles || KEEP_FILES
  const flushMs = options.flushMs || FLUSH_MS
  const linesPerMinute = options.linesPerMinute || LINES_PER_MINUTE
  const now = options.now || Date.now

  const file = path.join(dir, FILE)
  const secrets = []
  const recent = []

  // ------------------------------------------------------------ the file

  let unwritten = []
  let unwrittenBytes = 0
  let writing = false
  let size = -1 // not known until the first write
  let leftOutLines = 0
  let linesWindow = 0
  let linesInWindow = 0
  let idleWaiters = []

  const nameOf = (i) => (i === 0 ? file : `${file}.${i}`)
  const quiet = () => {}

  function clean(text, limit) {
    if (text === undefined || text === null || text === '') return ''
    let s = redact(text, secrets)
    if (home) s = s.split(home).join('~').split(home.replace(/\\/g, '/')).join('~')
    return s.length > limit ? s.slice(0, limit) + '…' : s
  }

  function queueLine(line) {
    const bytes = Buffer.byteLength(line)
    if (unwrittenBytes + bytes > MAX_UNWRITTEN_BYTES) { leftOutLines++; return }
    unwritten.push(line)
    unwrittenBytes += bytes
  }

  function writeLine(record) {
    const t = now()
    if (t - linesWindow >= MINUTE) {
      if (leftOutLines) queueLine(toLine({ time: new Date(t).toISOString(), level: 'warn', kind: 'log', message: `${leftOutLines} lines left out: too many at once` }))
      leftOutLines = 0
      linesWindow = t
      linesInWindow = 0
    }
    if (linesInWindow >= linesPerMinute) { leftOutLines++; return }
    linesInWindow++
    queueLine(toLine(record))
    pump()
  }

  // Moves each file along one, losing the oldest. A file that can't be moved
  // (something has it open) is emptied instead, so it still can't grow for ever.
  function rotate(done) {
    const move = (i) => {
      if (i < 1) return done()
      files.rename(nameOf(i - 1), nameOf(i), (err) => {
        if (!err || i > 1) return move(i - 1)
        files.truncate(file, 0, () => move(i - 1))
      })
    }
    move(keep - 1)
  }

  function learnSize(done) {
    files.stat(file, (err, st) => {
      size = err ? 0 : st.size
      done()
    })
  }

  const firstFits = () => size === 0 || size + Buffer.byteLength(unwritten[0]) <= maxBytes

  function writeSome(done) {
    let chunk = ''
    let bytes = 0
    let taken = 0
    for (const line of unwritten) {
      const n = Buffer.byteLength(line)
      if (taken && size + bytes + n > maxBytes) break
      chunk += line
      bytes += n
      taken++
    }
    unwritten = unwritten.slice(taken)
    unwrittenBytes -= bytes
    files.appendFile(file, chunk, { mode: 0o600 }, (err) => {
      if (!err) { size += bytes; return done() }
      // Its folder may have gone. Those lines are lost, and the next ones try again.
      size = -1
      files.mkdir(dir, { recursive: true, mode: 0o700 }, () => done())
    })
  }

  function pump() {
    if (writing || !unwritten.length) return
    writing = true
    const done = () => {
      writing = false
      if (unwritten.length) return pump()
      const waiters = idleWaiters
      idleWaiters = []
      for (const resolve of waiters) resolve()
    }
    const write = () => {
      if (firstFits()) return writeSome(done)
      rotate(() => learnSize(() => {
        if (firstFits()) return writeSome(done)
        // Nothing could be made room for: dropped, rather than growing without bound.
        leftOutLines += unwritten.length
        unwritten = []
        unwrittenBytes = 0
        done()
      }))
    }
    if (size < 0) learnSize(write)
    else write()
  }

  // ------------------------------------------------------------ reports to the server

  const waiting = new Map()
  let dropped = 0
  let timer = null
  let sending = false
  let holdUntil = 0
  let sendWindow = 0
  let sentInWindow = 0
  let lastFailure = ''

  function schedule(ms) {
    if (timer) return
    timer = setTimeout(() => { timer = null; flush() }, ms)
    if (timer.unref) timer.unref()
  }

  // Repeats wait as one event with a count, and a full queue drops the newest:
  // the first errors are usually the ones that explain the rest.
  function enqueue(event) {
    const key = [event.level, event.kind, event.message, event.stack, event.page, event.session, event.request].join('\0')
    const same = waiting.get(key)
    if (same) {
      same.count++
    } else if (waiting.size >= MAX_WAITING) {
      dropped++
    } else {
      waiting.set(key, event)
    }
    schedule(flushMs)
  }

  function takeBatch(room) {
    const events = []
    let bytes = 512 // the app, version and device around them
    for (const [key, event] of waiting) {
      if (events.length >= Math.min(MAX_EVENTS, room)) break
      const n = Buffer.byteLength(JSON.stringify(event))
      if (bytes + n > MAX_BODY_BYTES) {
        if (events.length) break
        event.stack = '' // can't happen at today's limits, but one event must not block the rest
      }
      bytes += n
      events.push(event)
      waiting.delete(key)
    }
    return events
  }

  // Whatever went wrong, what was being sent is gone. This is written to the file
  // once per kind of failure, and never sent: a failure can't cause another.
  function failed(count, why, retryAfter) {
    dropped += count
    const seconds = Number(retryAfter)
    const hold = Number.isFinite(seconds) && seconds > 0 ? Math.min(seconds * 1000, MAX_HOLD_MS) : HOLD_AFTER_FAILURE_MS
    holdUntil = now() + Math.max(hold, HOLD_AFTER_FAILURE_MS)
    if (why === lastFailure) return
    lastFailure = why
    writeLine({ time: new Date(now()).toISOString(), level: 'info', kind: 'log', message: clean(`Reports to the server were dropped: ${why}`, MAX_MESSAGE) })
  }

  async function flush() {
    if (timer) { clearTimeout(timer); timer = null }
    if (sending || !post || !target || (!waiting.size && !dropped)) return
    const left = holdUntil - now()
    if (left > 0) return schedule(left)
    sending = true
    let again = 0
    let count = 0
    try {
      const to = await target()
      if (!to) return // kept until there's a server: see kick()
      addSecret(to.token)
      const t = now()
      if (t - sendWindow >= MINUTE) { sendWindow = t; sentInWindow = 0 }
      const room = SEND_PER_MINUTE - sentInWindow
      if (room <= 0) { again = sendWindow + MINUTE - t; return }
      const earlier = dropped > 0 && room > 1 ? dropped : 0
      const events = takeBatch(earlier ? room - 1 : room)
      count = events.length
      if (earlier) {
        dropped = 0
        count += earlier
        events.unshift({ time: new Date(t).toISOString(), level: 'warn', kind: 'network', message: `${earlier} earlier reports from this app were left out`, count: 1 })
      }
      if (!events.length) return
      sentInWindow += events.length
      const answer = await post(to, JSON.stringify({ app, version, device, events }))
      const status = answer && answer.status
      if (status >= 200 && status < 300) lastFailure = ''
      else failed(count, `the server answered ${status}`, answer && answer.retryAfter)
      if (waiting.size) again = flushMs
    } catch (err) {
      failed(count, err && err.message ? err.message : 'no answer')
      if (waiting.size) again = flushMs
    } finally {
      sending = false
      if (again > 0) schedule(Math.max(again, holdUntil - now()))
    }
  }

  // ------------------------------------------------------------ what the app calls

  function addSecret(value) {
    if (typeof value !== 'string' || value.length < 6 || secrets.includes(value)) return
    if (secrets.length >= 8) secrets.shift()
    secrets.push(value)
  }

  // extra: stack, page, session, request, and where: an address, which stays in
  // the file. What one server should know about another is nothing.
  function note(level, kind, message, extra = {}) {
    try {
      const event = { time: new Date(now()).toISOString(), level, kind: String(kind), message: clean(message, MAX_MESSAGE), count: 1 }
      for (const [field, limit] of [['stack', MAX_STACK], ['page', MAX_FIELD], ['session', MAX_FIELD], ['request', MAX_FIELD]]) {
        const value = clean(extra[field], limit)
        if (value) event[field] = value
      }
      const where = clean(extra.where, MAX_FIELD)
      writeLine({ ...event, ...(where ? { where } : {}), ...(level === 'error' ? { version, device } : {}) })
      recent.push(`${event.time} ${level} ${event.kind}: ${event.message}`)
      if (recent.length > RECENT_LINES) recent.shift()
      if (level !== 'info') enqueue(event)
    } catch { /* the log must never be why something else fails */ }
  }

  // Anything can be thrown, and even reading a thrown thing can throw.
  function noteError(level, kind, what, thrown, extra = {}) {
    let message = ''
    let stack = ''
    try {
      message = thrown && thrown.message ? String(thrown.message) : String(thrown)
      stack = thrown && thrown.stack ? String(thrown.stack) : ''
    } catch { message = message || 'something that could not be read' }
    note(level, kind, `${what}: ${message}`, { ...extra, stack })
  }

  // Waiting reports can go now: a server was opened or someone signed in.
  function kick() {
    try { if (waiting.size || dropped) schedule(flushMs) } catch { /* see note */ }
  }

  // What's still queued, written before the app goes. Only for quitting: it blocks.
  function drainSync() {
    try {
      if (!unwritten.length) return
      const chunk = unwritten.join('')
      unwritten = []
      unwrittenBytes = 0
      files.appendFileSync(file, chunk, { mode: 0o600 })
    } catch { /* it's going anyway */ }
  }

  function idle() {
    return new Promise((resolve) => {
      if (!writing && !unwritten.length) return resolve()
      idleWaiters.push(resolve)
    })
  }

  // For someone to paste when asking for help: no address, and nothing from a file.
  function diagnostics(lines = []) {
    try {
      const last = recent.length ? recent.join('\n') : 'nothing logged yet'
      return [`${app} ${version}`, device, `Logs: ${clean(dir, MAX_FIELD)}`, ...lines.map((l) => clean(l, MAX_FIELD)), '', 'Latest in the log:', last].join('\n')
    } catch { return '' }
  }

  try { files.mkdirSync(dir, { recursive: true, mode: 0o700 }) } catch { /* the first write tries again */ }

  return {
    dir,
    file,
    error: (kind, message, extra) => note('error', kind, message, extra),
    warn: (kind, message, extra) => note('warn', kind, message, extra),
    info: (kind, message, extra) => note('info', kind, message, extra),
    noteError,
    addSecret,
    flush,
    kick,
    drainSync,
    idle,
    diagnostics,
  }
}

module.exports = { createClientLog, redact, toLine, MAX_FILE_BYTES, KEEP_FILES, MAX_MESSAGE, MAX_STACK, MAX_EVENTS, MAX_WAITING, SEND_PER_MINUTE, LINES_PER_MINUTE }
