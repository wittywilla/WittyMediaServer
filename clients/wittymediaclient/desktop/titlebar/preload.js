const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('wittyBar', {
  send: (action) => ipcRenderer.send('window', action),
  onServer: (fn) => ipcRenderer.on('server', (_e, url) => fn(url)),
  onMaximized: (fn) => ipcRenderer.on('maximized', (_e, on) => fn(on)),
  onTheme: (fn) => ipcRenderer.on('theme', (_e, t) => fn(t)),
})
