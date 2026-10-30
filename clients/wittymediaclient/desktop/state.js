// What the app remembers between runs: the current server, recent servers with
// the identity key each one proved, and the window size. No passwords or
// tokens: the sign-in stays in the browser session, as it does in a browser.
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

// The most recent first, once per address and once per server.
function remember(state, server) {
  const entry = { url: server.url, name: server.name }
  for (const key of ['serverId', 'publicKey', 'fingerprint']) if (server[key]) entry[key] = server[key]
  const others = (state.recent || []).filter((s) => s.url !== server.url && (!server.serverId || s.serverId !== server.serverId))
  return { ...state, server: server.url, recent: [entry, ...others].slice(0, MAX_RECENT) }
}

module.exports = { load, save, remember, MAX_RECENT }
