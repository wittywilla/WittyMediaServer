// Runs before every page in the main view, in its own isolated world. The
// connect screen, opened from a file, gets a small bridge. A server's page gets
// nothing: all it can do is report its colours, so the title bar matches.
const { contextBridge, ipcRenderer } = require('electron')

if (location.protocol === 'file:') {
  contextBridge.exposeInMainWorld('wittyClient', {
    supportsDiscovery: false,
    probe: (url) => ipcRenderer.invoke('probe', url),
    recent: () => ipcRenderer.invoke('recent'),
    open: (server) => ipcRenderer.invoke('open-server', server),
  })
} else {
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
  })
}
