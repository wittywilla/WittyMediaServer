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

let win
function createWindow() {
  const state = loadState()
  win = new BrowserWindow({
    width: state.width || 1400, height: state.height || 900, backgroundColor: '#0f1115', title: 'WittyMedia',
    autoHideMenuBar: true, webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true },
  })
  win.on('resize', () => { const [width, height] = win.getSize(); saveState({ ...loadState(), width, height }) })
  win.webContents.setWindowOpenHandler(({ url }) => { shell.openExternal(url); return { action: 'deny' } })
  if (state.server) win.loadURL(state.server); else win.loadFile('connect.html')
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    { label: 'Server', submenu: [
      { label: 'Change server…', click: () => win.loadFile('connect.html') },
      { label: 'Reload', role: 'reload' }, { type: 'separator' }, { role: 'quit' }] },
    { label: 'View', submenu: [{ role: 'togglefullscreen' }, { role: 'zoomIn' }, { role: 'zoomOut' }, { role: 'resetZoom' }, { role: 'toggleDevTools' }] },
  ]))
}

ipcMain.handle('connect', (_e, url) => {
  url = url.replace(/\/+$/, '')
  saveState({ ...loadState(), server: url })
  win.loadURL(url)
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
