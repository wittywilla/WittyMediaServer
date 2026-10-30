const win = window.wittyBar
const change = document.getElementById('change')

for (const action of ['minimize', 'maximize', 'close']) {
  document.getElementById(action).addEventListener('click', () => win.send(action))
}
change.addEventListener('click', () => win.send('change-server'))
document.body.addEventListener('dblclick', (e) => { if (e.target === document.body || e.target.id === 'title') win.send('maximize') })

win.onServer((url) => { change.hidden = !url })
win.onMaximized((on) => {
  const b = document.getElementById('maximize')
  b.setAttribute('aria-label', on ? 'Restore' : 'Maximize')
  b.title = on ? 'Restore' : 'Maximize'
})
win.onTheme((t) => {
  const root = document.documentElement.style
  root.setProperty('--bg', t.bg)
  root.setProperty('--fg', t.fg)
  root.setProperty('--border', t.border)
})
