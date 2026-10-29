const { contextBridge, ipcRenderer } = require('electron')
contextBridge.exposeInMainWorld('wittyDesktop', {
  connect: (url) => ipcRenderer.invoke('connect', url),
  platform: process.platform,
})
