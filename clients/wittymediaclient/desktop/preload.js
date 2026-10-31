// Runs before every page in the main view, in its own isolated world. The
// connect screen, opened from a file, gets a small bridge. A server's page gets
// very little: it can report its colours, so the title bar matches, and ask for
// Moonlight, which the main process checks strictly (see moonlight.js).
const { contextBridge, ipcRenderer } = require('electron')

if (location.protocol === 'file:') {
  contextBridge.exposeInMainWorld('wittyClient', {
    supportsDiscovery: true,
    startDiscovery: (onServers) => {
      ipcRenderer.on('servers', (_e, servers) => onServers(servers))
      ipcRenderer.send('discover-start')
    },
    probe: (url) => ipcRenderer.invoke('probe', url),
    recent: () => ipcRenderer.invoke('recent'),
    open: (server) => ipcRenderer.invoke('open-server', server),
    redeem: (url, code) => ipcRenderer.invoke('redeem', url, code),
    openLogs: () => ipcRenderer.send('open-logs'),
  })
} else {
  contextBridge.exposeInMainWorld('wittyClient', {
    moonlight: {
      status: () => ipcRenderer.invoke('moonlight-status'),
      launch: (args) => ipcRenderer.invoke('moonlight-launch', args),
    },
  })
  const report = () => {
    const css = getComputedStyle(document.documentElement)
    ipcRenderer.send('theme', {
      bg: css.getPropertyValue('--bg-2').trim(),
      fg: css.getPropertyValue('--fg').trim(),
      border: css.getPropertyValue('--border').trim(),
    })
  }
  // The web UI swaps a stylesheet in <head> and sets data-theme when the theme changes.
  window.addEventListener('DOMContentLoaded', () => {
    report()
    let timer = null
    const later = () => { clearTimeout(timer); timer = setTimeout(report, 150) }
    new MutationObserver(later).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    new MutationObserver(later).observe(document.head, { childList: true })
    // A new stylesheet only applies once loaded, which is after it was added.
    document.head.addEventListener('load', later, true)
  })
}
