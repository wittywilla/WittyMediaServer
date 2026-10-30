// WittyMediaClient for Windows and Linux: a frameless window with two views.
// The title bar is our own local page. Below it, the content view shows the
// connect screen and then the server's web UI, which has no access to Node or
// to the app beyond reporting its theme colours.
const { app, BaseWindow, WebContentsView, ipcMain, net, session, shell } = require('electron')
const path = require('path')
const { normalizeAddress } = require('./connect/address.js')
const state = require('./state.js')

const TITLE_BAR = 32
const CONNECT_PAGE = path.join(__dirname, 'connect', 'index.html')
const STATUS_TIMEOUT_MS = 5000
const COLOUR = /^(#[0-9a-fA-F]{3,8}|rgba?\([\d\s.,%/]+\))$/

const stateFile = () => path.join(app.getPath('userData'), 'state.json')
let saved = {}
let win, bar, content
let server = '' // the origin the content view may show

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

function showConnect(query) {
  server = ''
  bar.webContents.send('server', '')
  content.webContents.loadFile(CONNECT_PAGE, { query })
}

function showServer(url) {
  server = new URL(url).origin
  bar.webContents.send('server', url)
  content.webContents.loadURL(url)
}

// Asks an address whether it is a WittyMediaServer. In the main process, so no
// page's security rules decide the answer and no page sees more than the result.
async function probe(url) {
  const r = normalizeAddress(url)
  if (r.error) return { error: r.error }
  try {
    const res = await net.fetch(r.url + '/api/status', {
      credentials: 'omit', redirect: 'error', signal: AbortSignal.timeout(STATUS_TIMEOUT_MS),
    })
    const data = await res.json()
    if (data.name !== 'WittyMediaServer') return { error: 'not-server' }
    return { ok: true, url: r.url, version: String(data.version || ''), name: String(data.brandName || data.name) }
  } catch { return { error: 'unreachable' } }
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
  win.on('closed', () => { state.save(stateFile(), saved); app.quit() })

  bar.webContents.loadFile(path.join(__dirname, 'titlebar', 'index.html')).then(() => {
    const start = saved.server && !normalizeAddress(saved.server).error ? saved.server : ''
    if (start) showServer(start); else showConnect()
  })
}

ipcMain.handle('probe', (e, url) => (fromConnect(e) ? probe(String(url)) : { error: 'invalid' }))
ipcMain.handle('recent', (e) => (fromConnect(e) ? saved.recent || [] : []))
ipcMain.handle('open-server', async (e, s) => {
  if (!fromConnect(e)) return false
  const found = await probe(String(s && s.url))
  if (found.error) return false
  saved = state.remember(saved, { url: found.url, name: found.name })
  state.save(stateFile(), saved)
  showServer(found.url)
  return true
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
