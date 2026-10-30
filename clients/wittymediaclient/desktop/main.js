// WittyMediaClient for Windows and Linux: a frameless window with two views.
// The title bar is our own local page. Below it, the content view shows the
// connect screen and then the server's web UI, which has no access to Node or
// to the app beyond reporting its theme colours.
const { app, BaseWindow, WebContentsView, ipcMain, net, session, shell } = require('electron')
const os = require('os')
const path = require('path')
const { normalizeAddress } = require('./connect/address.js')
const { judge, newNonce } = require('./identity.js')
const { startDiscovery } = require('./discover.js')
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
let stopDiscovery = null

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
  bar.webContents.send('server', url)
  content.webContents.loadURL(url)
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
    if (res.status === 404) return { state: 'none' } // a server older than this check
    if (!res.ok) return { state: 'bad' }
    const answer = await res.json()
    const result = judge(answer, nonce, url, saved.recent)
    if (result === 'bad') return { state: 'bad' }
    return { state: result, serverId: answer.serverId, publicKey: answer.publicKey, fingerprint: String(answer.fingerprint || '') }
  } catch {
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
    if (data.name !== 'WittyMediaServer') return { error: 'not-server' }
    const identity = await identify(r.url)
    return {
      ok: true, url: r.url, version: String(data.version || ''), name: String(data.brandName || data.name),
      setupRequired: data.setupRequired === true,
      identity: { state: identity.state, fingerprint: identity.fingerprint || '' },
    }
  } catch { return { error: 'unreachable' } }
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
  } catch { return { error: 'unreachable' } }
  if (!res.ok || !data.token) return { message: String(data.error || ''), reason: String(data.reason || '') }
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
    try { if (server && new URL(url).origin === server) return } catch { /* falls through */ }
    e.preventDefault()
    openOutside(url)
  }
  wc.on('will-navigate', stay)
  wc.on('will-redirect', stay)
  wc.on('will-attach-webview', (e) => e.preventDefault())
  // A server that can't be reached goes back to the connect screen and says so.
  wc.on('did-fail-load', (_e, code, _desc, _url, isMainFrame) => {
    if (isMainFrame && code !== -3 && server) showConnect({ lost: '1' })
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
  if (identity.state === 'different' && !(s && s.trust === true)) return false
  rememberServer(found.url, found.name, identity)
  showServer(found.url)
  return true
})

ipcMain.on('discover-start', (e) => {
  if (!fromConnect(e) || process.env.WMC_NO_DISCOVERY === '1') return
  endDiscovery()
  const page = e.sender
  stopDiscovery = startDiscovery((servers) => { if (!page.isDestroyed()) page.send('servers', servers) })
})

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
})

// One window only: a second launch brings the first to the front.
if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', () => { if (win) { if (win.isMinimized()) win.restore(); win.focus() } })
  app.whenReady().then(() => {
    saved = state.load(stateFile())
    // Pages may go fullscreen and copy text. Camera, location and the rest stay off.
    const allow = new Set(['fullscreen', 'clipboard-sanitized-write'])
    session.defaultSession.setPermissionRequestHandler((_wc, perm, done) => done(allow.has(perm)))
    session.defaultSession.setPermissionCheckHandler((_wc, perm) => allow.has(perm))
    createWindow()
  })
  app.on('window-all-closed', () => app.quit())
}
