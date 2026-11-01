// Big screen mode: this screen is across the room. Larger type and the stronger focus
// outline stay on whether or not a controller is connected. It's per device, and
// `?tv=1` in the address turns it on for a TV browser with no easy way to reach Settings.

const KEY = 'wms.tenfoot'

export function getTenFoot(): boolean {
  try { return localStorage.getItem(KEY) === '1' } catch { return false }
}

function apply(on: boolean) {
  const html = document.documentElement
  html.classList.toggle('ten-foot', on)
  if (on) html.classList.add('using-controller')
  else html.classList.remove('using-controller')
}

export function setTenFoot(on: boolean) {
  try { localStorage.setItem(KEY, on ? '1' : '0') } catch { /* for this visit only */ }
  apply(on)
}

export function installTenFoot() {
  const asked = new URLSearchParams(location.search).get('tv')
  if (asked === '1' || asked === '0') setTenFoot(asked === '1')
  else if (getTenFoot()) apply(true)
}
