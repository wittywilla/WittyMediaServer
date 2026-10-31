// WittyMedia desktop client: a thin shell around the server's web UI with a
// server picker, remembered connection and media-key support. The same code
// runs on Windows, macOS, Linux and FreeBSD (Electron is in FreeBSD ports:
// `pkg install electron33`, then `electron .`).
const { app, BrowserWindow, Menu, globalShortcut, ipcMain, shell } = require('electron')
const path = require('path')
const fs = require('fs')

const statePath = () => path.join(app.getPath('userData'), 'state.json')
const loadState = () => { try { return JSON.parse(fs.readFileSync(statePath(), 'utf8')) } catch { return {} } }
const saveState = (s) => fs.writeFileSync(statePath(), JSON.stringify(s))

// Only web addresses are servers, and only web and mail links leave the
// app: anything else (file:, custom schemes) could open programs or files.
const webURL = (u) => { try { const x = new URL(u); return x.protocol === 'http:' || x.protocol === 'https:' ? x : null } catch { return null } }
const openOutside = (u) => { try { const x = new URL(u); if (['http:', 'https:', 'mailto:'].includes(x.protocol)) shell.openExternal(x.href) } catch { /* not a link */ } }

let win
function createWindow() {
  const state = loadState()
  win = new BrowserWindow({
    width: state.width || 1400, height: state.height || 900, backgroundColor: '#0f1115', title: 'WittyMedia',
    autoHideMenuBar: true, webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, sandbox: true, nodeIntegration: false, webSecurity: true, spellcheck: false }, // spelling dictionaries would be fetched from outside
  })
  win.on('resize', () => { const [width, height] = win.getSize(); saveState({ ...loadState(), width, height }) })
  win.webContents.setWindowOpenHandler(({ url }) => { openOutside(url); return { action: 'deny' } })
  // The window stays on the chosen server (and the connect page); other
  // sites open in the browser, without this app's bridge.
  win.webContents.on('will-navigate', (e, url) => {
    const server = loadState().server
    const to = webURL(url)
    if (url.startsWith('file:') && url.endsWith('/connect.html')) return
    if (to && server && to.origin === new URL(server).origin) return
    e.preventDefault()
    openOutside(url)
  })
  if (state.server && webURL(state.server)) win.loadURL(state.server); else win.loadFile('connect.html')
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    { label: 'Server', submenu: [
      { label: 'Change server…', click: () => win.loadFile('connect.html') },
      { label: 'Reload', role: 'reload' }, { type: 'separator' }, { role: 'quit' }] },
    { label: 'View', submenu: [{ role: 'togglefullscreen' }, { role: 'zoomIn' }, { role: 'zoomOut' }, { role: 'resetZoom' }, { role: 'toggleDevTools' }] },
  ]))
}

// Only the connect page may choose the server.
ipcMain.handle('connect', (e, url) => {
  if (!e.senderFrame.url.startsWith('file:')) return false
  const u = webURL(String(url).trim())
  if (!u) return false
  const server = u.href.replace(/\/+$/, '')
  saveState({ ...loadState(), server })
  win.loadURL(server)
  return true
})

app.whenReady().then(() => {
  createWindow()
  // Forward media keys to the page as keyboard shortcuts the web player understands.
  globalShortcut.register('MediaPlayPause', () => win.webContents.sendInputEvent({ type: 'keyDown', keyCode: 'Space' }))
  globalShortcut.register('MediaNextTrack', () => win.webContents.sendInputEvent({ type: 'keyDown', keyCode: 'n' }))
  globalShortcut.register('MediaPreviousTrack', () => win.webContents.sendInputEvent({ type: 'keyDown', keyCode: 'p' }))
  app.on('activate', () => { if (BrowserWindow.getAllWindows().length === 0) createWindow() })
})
app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit() })
