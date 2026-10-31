// WittyMediaClient for Windows and Linux: a frameless window with two views.
// The title bar is our own local page. Below it, the content view shows the
// connect screen and then the server's web UI, which has no access to Node or
// to the app beyond reporting its theme colours.
const { app, BaseWindow, Menu, WebContentsView, clipboard, ipcMain, net, session, shell } = require('electron')
const os = require('os')
const path = require('path')
const { normalizeAddress } = require('./connect/address.js')
const { judge, judgeMissing, fingerprintOf, newNonce } = require('./identity.js')
const { startDiscovery } = require('./discover.js')
const { createClientLog } = require('./clientlog.js')
const state = require('./state.js')

const TITLE_BAR = 32
const CONNECT_PAGE = path.join(__dirname, 'connect', 'index.html')
const STATUS_TIMEOUT_MS = 5000
const API = '/api/v1' // the contract: docs/CLIENT-API.md
const SESSION_COOKIE = 'wms_token'
const SESSION_DAYS = 90
const COLOUR = /^(#[0-9a-fA-F]{3,8}|rgba?\([\d\s.,%/]+\))$/

const stateFile = () => path.join(app.getPath('userData'), 'state.json')
let saved = {}
let win, bar, content
let server = '' // the origin the content view may show
let lastServer = ''
let stopDiscovery = null

const OS_NAMES = { win32: 'Windows', linux: 'Linux', darwin: 'macOS' }
const DEVICE = `${OS_NAMES[process.platform] || process.platform} ${os.release()} ${process.arch}, Electron ${process.versions.electron}`

// The sign-in stays in the session's cookie, which no page script can read. Only
// the main process reads it here, to send this app's own reports to that server.
async function reportTarget() {
  if (!server) return null
  const origin = server
  const cookies = await session.defaultSession.cookies.get({ url: origin, name: SESSION_COOKIE })
  const token = cookies.length ? cookies[0].value : ''
  return token ? { origin, token } : null
}

async function postReport(to, body) {
  const res = await fetchJson(to.origin + API + '/logs/client', {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${to.token}` }, body,
  })
  res.text().catch(() => {})
  return { status: res.status, retryAfter: res.headers.get('retry-after') }
}

// Only what the main process sees: the server's page reports its own errors.
const log = createClientLog({
  dir: path.join(app.getPath('userData'), 'logs'), app: 'WittyMedia desktop', version: app.getVersion(), device: DEVICE,
  target: reportTarget, post: postReport,
})

// Carrying on is safe here: a failed window or request is reported where it
// happens, and these are the ones nobody caught. A failed start ends the app below.
process.on('uncaughtException', (err) => log.noteError('error', 'crash', 'Uncaught error in the main process', err, { page: 'main process' }))
process.on('unhandledRejection', (err) => log.noteError('error', 'crash', 'Unhandled rejection in the main process', err, { page: 'main process' }))

function openLogs() {
  shell.openPath(log.dir)
    .then((problem) => { if (problem) log.info('app', `The logs folder could not be opened: ${problem}`) })
    .catch((err) => log.noteError('info', 'app', 'The logs folder could not be opened', err))
}

function copyDiagnostics() {
  clipboard.writeText(log.diagnostics([`Connected to a server: ${server ? 'yes' : 'no'}`]))
}

function showHelpMenu() {
  Menu.buildFromTemplate([
    { label: 'Open logs folder', click: openLogs },
    { label: 'Copy diagnostic info', click: copyDiagnostics },
  ]).popup({ window: win })
}

// Everything that can go wrong with a view that the view can't report itself.
function watchContents(wc, name) {
  wc.on('render-process-gone', (_e, details) => {
    if (details.reason === 'clean-exit') return
    log.error('crash', `The ${name} stopped: ${details.reason}, exit code ${details.exitCode}`, { page: name })
  })
  wc.on('unresponsive', () => log.warn('crash', `The ${name} stopped responding`, { page: name }))
  wc.on('responsive', () => log.info('crash', `The ${name} is responding again`, { page: name }))
  wc.on('preload-error', (_e, _preload, err) => log.noteError('error', 'crash', `The ${name} failed to prepare`, err, { page: name }))
}

const isConnectPage = (url) => {
  try { return new URL(url).protocol === 'file:' && path.normalize(decodeURIComponent(new URL(url).pathname)).endsWith(path.join('connect', 'index.html')) } catch { return false }
}
const senderIs = (e, check) => { try { return check(e.senderFrame.url) } catch { return false } }
const fromConnect = (e) => senderIs(e, isConnectPage)
const fromBar = (e) => e.sender === bar.webContents

// Only web and mail links leave the app: anything else could open programs or files.
function openOutside(url) {
  try {
    const u = new URL(url)
    if (['http:', 'https:', 'mailto:'].includes(u.protocol)) shell.openExternal(u.href)
  } catch { /* not a link */ }
}

function layout() {
  const { width, height } = win.getContentBounds()
  const full = win.isFullScreen() || htmlFullScreen
  bar.setVisible(!full)
  bar.setBounds({ x: 0, y: 0, width, height: TITLE_BAR })
  const top = full ? 0 : TITLE_BAR
  content.setBounds({ x: 0, y: top, width, height: Math.max(0, height - top) })
}

let htmlFullScreen = false

function endDiscovery() {
  if (stopDiscovery) stopDiscovery()
  stopDiscovery = null
}

function showConnect(query) {
  endDiscovery()
  server = ''
  bar.webContents.send('server', '')
  content.webContents.loadFile(CONNECT_PAGE, { query })
}

function showServer(url) {
  endDiscovery()
  server = new URL(url).origin
  lastServer = server
  bar.webContents.send('server', url)
  content.webContents.loadURL(url)
  log.kick()
}

const fetchJson = (url, options) =>
  net.fetch(url, { credentials: 'omit', redirect: 'error', signal: AbortSignal.timeout(STATUS_TIMEOUT_MS), ...options })

const postJson = (url, body) =>
  fetchJson(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })

// Has the server at this address proved it holds the key we remember for it?
async function identify(url) {
  const nonce = newNonce()
  try {
    const res = await postJson(url + API + '/identity', { nonce })
    if (res.status === 404) return { state: judgeMissing(url, saved.recent) }
    if (!res.ok) {
      log.warn('network', `The identity check was answered with ${res.status}`, { where: url })
      return { state: 'bad' }
    }
    const answer = await res.json()
    const result = judge(answer, nonce, url, saved.recent)
    if (result === 'bad' || result === 'different') log.warn('network', `The server's identity check came back as ${result}`, { where: url })
    if (result === 'bad') return { state: 'bad' }
    return { state: result, serverId: answer.serverId, publicKey: answer.publicKey, fingerprint: fingerprintOf(answer.publicKey) }
  } catch (err) {
    log.noteError('warn', 'network', 'The identity check failed', err, { where: url })
    return { state: 'unreachable' }
  }
}

// Asks an address whether it is a WittyMediaServer. In the main process, so no
// page's security rules decide the answer and no page sees more than the result.
async function probe(url) {
  const r = normalizeAddress(url)
  if (r.error) return { error: r.error }
  try {
    const res = await fetchJson(r.url + '/api/status')
    const data = await res.json()
    if (data.name !== 'WittyMediaServer') {
      log.info('network', 'Something answered that is not a WittyMediaServer', { where: r.url })
      return { error: 'not-server' }
    }
    const identity = await identify(r.url)
    return {
      ok: true, url: r.url, version: String(data.version || ''), name: String(data.brandName || data.name),
      setupRequired: data.setupRequired === true,
      identity: { state: identity.state, fingerprint: identity.fingerprint || '' },
    }
  } catch (err) {
    log.noteError('warn', 'network', 'A server could not be reached', err, { where: r.url })
    return { error: 'unreachable' }
  }
}

// Sign-ins made for the old key must never reach whoever holds the new one.
async function forgetSignIn(url) {
  const { origin } = new URL(url)
  await session.defaultSession.cookies.remove(origin, SESSION_COOKIE)
  await session.defaultSession.clearStorageData({ origin, storages: ['cookies', 'localstorage'] })
}

function rememberServer(url, name, identity) {
  saved = state.remember(saved, { url, name, serverId: identity.serverId, publicKey: identity.publicKey, fingerprint: identity.fingerprint })
  state.save(stateFile(), saved)
}

// A code typed on this device. The sign-in goes straight into the browser
// session as the server's own HttpOnly cookie, so no page script can read it.
async function redeem(url, code) {
  const found = await probe(url)
  if (found.error) return { error: 'unreachable' }
  const identity = await identify(found.url)
  if (identity.state === 'different' || identity.state === 'bad') return { error: 'identity' }
  let res, data
  try {
    res = await postJson(found.url + API + '/pair/redeem', {
      code: String(code), device: `${os.hostname()} (WittyMedia)`.slice(0, 60), platform: process.platform === 'win32' ? 'Windows' : 'Linux',
    })
    data = await res.json()
  } catch (err) {
    log.noteError('warn', 'network', 'Signing in with a code failed', err, { where: found.url })
    return { error: 'unreachable' }
  }
  if (!res.ok || !data.token) {
    log.warn('network', `Signing in with a code was refused: ${res.status} ${String(data.reason || '')}`, { where: found.url })
    return { message: String(data.error || ''), reason: String(data.reason || '') }
  }
  log.addSecret(data.token)
  await session.defaultSession.cookies.set({
    url: found.url, name: SESSION_COOKIE, value: data.token, httpOnly: true, sameSite: 'lax',
    secure: found.url.startsWith('https:'), expirationDate: Date.now() / 1000 + SESSION_DAYS * 86400,
  })
  rememberServer(found.url, found.name, identity)
  showServer(found.url)
  return { ok: true }
}

// What the last run was using, if it still is who it was.
async function startUp() {
  const url = saved.server && !normalizeAddress(saved.server).error ? saved.server : ''
  if (!url) return showConnect()
  const identity = await identify(url)
  if (identity.state === 'unreachable') return showConnect({ lost: '1' })
  if (identity.state === 'different' || identity.state === 'bad') return showConnect({ mismatch: url })
  if (identity.state === 'new') {
    const known = (saved.recent || []).find((s) => s.url === url)
    rememberServer(url, known ? known.name : 'WittyMediaServer', identity)
  }
  return showServer(url)
}

function guardContent(wc) {
  wc.setWindowOpenHandler(({ url }) => { openOutside(url); return { action: 'deny' } })
  const stay = (e, url) => {
    if (isConnectPage(url)) return
    let origin = ''
    try { origin = new URL(url).origin } catch { /* not a web address */ }
    if (server && origin === server) return
    e.preventDefault()
    // A load still under way when someone pressed Change server isn't a link to open elsewhere.
    if (origin !== lastServer) openOutside(url)
  }
  wc.on('will-navigate', stay)
  wc.on('will-redirect', stay)
  wc.on('will-attach-webview', (e) => e.preventDefault())
  // A server that can't be reached goes back to the connect screen and says so.
  wc.on('did-fail-load', (_e, code, desc, url, isMainFrame) => {
    if (!isMainFrame || code === -3) return
    log.warn('network', `A page failed to load: ${desc} (${code})`, { where: url })
    if (server) showConnect({ lost: '1' })
  })
  wc.on('enter-html-full-screen', () => { htmlFullScreen = true; layout() })
  wc.on('leave-html-full-screen', () => { htmlFullScreen = false; layout() })
  wc.on('did-finish-load', () => wc.focus())
}

function createWindow() {
  const size = saved.window || {}
  win = new BaseWindow({
    width: size.width || 1280, height: size.height || 800, minWidth: 480, minHeight: 360,
    frame: false, backgroundColor: '#0f1115', title: 'WittyMedia', show: true,
  })
  win.setMenu(null)

  const secure = { contextIsolation: true, sandbox: true, nodeIntegration: false, webSecurity: true, spellcheck: false }
  bar = new WebContentsView({ webPreferences: { ...secure, preload: path.join(__dirname, 'titlebar', 'preload.js') } })
  content = new WebContentsView({ webPreferences: { ...secure, preload: path.join(__dirname, 'preload.js'), autoplayPolicy: 'no-user-gesture-required' } })
  win.contentView.addChildView(bar)
  win.contentView.addChildView(content)
  guardContent(content.webContents)
  watchContents(content.webContents, 'page')
  watchContents(bar.webContents, 'title bar')
  bar.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  bar.webContents.on('will-navigate', (e) => e.preventDefault())

  layout()
  win.on('resize', () => { layout(); const [width, height] = win.getSize(); saved.window = { width, height } })
  win.on('enter-full-screen', layout)
  win.on('leave-full-screen', layout)
  win.on('maximize', () => bar.webContents.send('maximized', true))
  win.on('unmaximize', () => bar.webContents.send('maximized', false))
  win.on('closed', () => { endDiscovery(); state.save(stateFile(), saved); app.quit() })

  bar.webContents.loadFile(path.join(__dirname, 'titlebar', 'index.html')).then(startUp)
}

ipcMain.handle('probe', (e, url) => (fromConnect(e) ? probe(String(url)) : { error: 'invalid' }))
ipcMain.handle('recent', (e) => (fromConnect(e) ? (saved.recent || []).map(({ url, name }) => ({ url, name })) : []))
ipcMain.handle('redeem', (e, url, code) => (fromConnect(e) ? redeem(String(url), String(code)) : { error: 'invalid' }))
ipcMain.handle('open-server', async (e, s) => {
  if (!fromConnect(e)) return false
  const found = await probe(String(s && s.url))
  if (found.error) return false
  const identity = await identify(found.url)
  if (identity.state === 'bad') return false
  // Another key at a known server is only ever accepted because the person said so.
  if (identity.state === 'different') {
    if (!(s && s.trust === true)) return false
    await forgetSignIn(found.url)
  }
  rememberServer(found.url, found.name, identity)
  showServer(found.url)
  return true
})

ipcMain.on('discover-start', (e) => {
  if (!fromConnect(e)) return
  endDiscovery()
  const page = e.sender
  stopDiscovery = startDiscovery((servers) => { if (!page.isDestroyed()) page.send('servers', servers) })
})

ipcMain.on('open-logs', (e) => { if (fromConnect(e)) openLogs() })

// Theme colours from the server's page: checked strictly, since a page decides them.
ipcMain.on('theme', (e, t) => {
  if (e.sender !== content.webContents || !t) return
  const ok = (c) => typeof c === 'string' && COLOUR.test(c)
  // A hairline in a form this check doesn't know mustn't stop the rest applying.
  if (ok(t.bg) && ok(t.fg)) bar.webContents.send('theme', { bg: t.bg, fg: t.fg, border: ok(t.border) ? t.border : 'transparent' })
})

ipcMain.on('window', (e, action) => {
  if (!fromBar(e)) return
  if (action === 'minimize') win.minimize()
  else if (action === 'maximize') { if (win.isMaximized()) win.unmaximize(); else win.maximize() }
  else if (action === 'close') win.close()
  else if (action === 'change-server') { saved.server = ''; state.save(stateFile(), saved); showConnect() }
  else if (action === 'help') showHelpMenu()
})

// One window only: a second launch brings the first to the front.
if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', () => { if (win) { if (win.isMinimized()) win.restore(); win.focus() } })
  log.info('app', `Started, version ${app.getVersion()}, ${DEVICE}`)
  // Listening doesn't change the answer: without preventDefault a bad certificate stays refused.
  app.on('certificate-error', (_e, _wc, url, error, _cert, _done, isMainFrame) => {
    log.warn('network', `A certificate was refused: ${error}${isMainFrame ? ', for the page itself' : ''}`, { where: url })
  })
  app.on('child-process-gone', (_e, details) => {
    if (details.reason === 'clean-exit') return
    log.error('crash', `A ${details.type} process stopped: ${details.reason}, exit code ${details.exitCode}${details.name ? `, ${details.name}` : ''}`, { page: 'main process' })
  })
  app.whenReady().then(() => {
    saved = state.load(stateFile())
    // Pages may go fullscreen and copy text. Camera, location and the rest stay off.
    const allow = new Set(['fullscreen', 'clipboard-sanitized-write'])
    session.defaultSession.setPermissionRequestHandler((_wc, perm, done) => done(allow.has(perm)))
    session.defaultSession.setPermissionCheckHandler((_wc, perm) => allow.has(perm))
    // Someone signed in through the server's page: reports waiting can go now.
    session.defaultSession.cookies.on('changed', (_e, cookie, _cause, removed) => { if (!removed && cookie.name === SESSION_COOKIE) log.kick() })
    createWindow()
  }).catch((err) => {
    // There's no window and never will be, and an app with none can't be closed.
    log.noteError('error', 'crash', 'The app could not start', err, { page: 'main process' })
    log.drainSync()
    app.exit(1)
  })
  app.on('window-all-closed', () => app.quit())
  app.on('before-quit', () => { log.flush() }) // one last try, which may not finish
  app.on('will-quit', () => log.drainSync())
}
