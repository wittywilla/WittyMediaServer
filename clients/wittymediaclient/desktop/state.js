// What the app remembers between runs: the current server, recent servers and
// the window size. No passwords or tokens: the server's web page keeps its own
// session, as it does in a browser.
const fs = require('fs')

const MAX_RECENT = 8

function load(file) {
  try {
    const s = JSON.parse(fs.readFileSync(file, 'utf8'))
    return s && typeof s === 'object' ? s : {}
  } catch { return {} }
}

function save(file, state) {
  fs.writeFileSync(file, JSON.stringify(state))
}

// The most recent first, once per address.
function remember(state, server) {
  const others = (state.recent || []).filter((s) => s.url !== server.url)
  return { ...state, server: server.url, recent: [{ url: server.url, name: server.name }, ...others].slice(0, MAX_RECENT) }
}

module.exports = { load, save, remember, MAX_RECENT }
