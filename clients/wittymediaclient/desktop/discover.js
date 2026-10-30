// Finds WittyMediaServers on the local network: mDNS first, with the server's
// small UDP question as an extra (docs/CLIENT-API.md, "Finding a server").
// Everything here comes from the network and is treated as untrusted: being
// announced opens nothing, and the app checks the server's identity afterwards.
const dgram = require('dgram')
const { normalizeAddress, isPrivateHost } = require('./connect/address.js')

const SERVICE = '_wittymedia._tcp.local'
const UDP_QUESTION = 'WMS-DISCOVER/1'
const UDP_PORT = 9797
const QUERY_EVERY_MS = 5000
const FORGET_AFTER_MS = 20000
const MAX_SERVERS = 32
const MAX_NAME = 80
const ID = /^[0-9a-f]{32}$/

function cleanName(name) {
  const clean = String(name == null ? '' : name).replace(/[\u0000-\u001f\u007f]/g, '').trim().slice(0, MAX_NAME)
  return clean || 'WittyMediaServer'
}

// A server as the connect screen may show it, or null if any part is unusable.
function makeServer({ id, name, host, port, tls }) {
  if (!ID.test(String(id)) || !Number.isInteger(port) || port < 1 || port > 65535) return null
  if (!/^\d{1,3}(\.\d{1,3}){3}$/.test(String(host)) || !isPrivateHost(host)) return null
  const scheme = tls ? 'https' : 'http'
  const r = normalizeAddress(`${scheme}://${host}:${port}`)
  return r.url ? { id, name: cleanName(name), url: r.url } : null
}

function txtValues(data) {
  const out = {}
  for (const entry of Array.isArray(data) ? data : [data]) {
    const text = Buffer.isBuffer(entry) ? entry.toString('utf8') : String(entry)
    const at = text.indexOf('=')
    if (at > 0) out[text.slice(0, at)] = text.slice(at + 1)
  }
  return out
}

// Reads servers from the records of one mDNS answer. A record names its
// instance or host, so they are joined by those names.
function serversFromRecords(records) {
  const txt = new Map()
  const srv = new Map()
  const addresses = new Map()
  for (const r of records) {
    if (r.type === 'TXT') txt.set(r.name, txtValues(r.data))
    else if (r.type === 'SRV' && r.data) srv.set(r.name, r.data)
    else if (r.type === 'A') addresses.set(r.name, r.data)
  }
  const found = []
  for (const [instance, service] of srv) {
    if (!instance.endsWith(`.${SERVICE}`)) continue
    const values = txt.get(instance)
    if (!values) continue
    const server = makeServer({
      id: values.id, name: values.name, host: addresses.get(service.target), port: service.port, tls: values.tls === '1',
    })
    if (server) found.push(server)
  }
  return found
}

// The direct answer to the UDP question.
function serverFromUdpReply(message, fromAddress) {
  let d
  try { d = JSON.parse(message.toString('utf8')) } catch { return null }
  if (!d || d.service !== 'wittymediaserver') return null
  return makeServer({ id: d.id, name: d.name, host: fromAddress, port: d.port, tls: d.tls === true })
}

// Keeps the list. Servers drop out when they stop answering, and the oldest
// goes when too many turn up, so a noisy network can't grow it without limit.
class ServerList {
  constructor(now = Date.now) {
    this.now = now
    this.servers = new Map()
  }

  add(server) {
    const known = this.servers.has(server.id)
    if (!known && this.servers.size >= MAX_SERVERS) this.servers.delete(this.servers.keys().next().value)
    const before = this.servers.get(server.id)
    this.servers.set(server.id, { ...server, seen: this.now() })
    return !before || before.url !== server.url || before.name !== server.name
  }

  expire() {
    let changed = false
    for (const [id, s] of this.servers) {
      if (this.now() - s.seen > FORGET_AFTER_MS) { this.servers.delete(id); changed = true }
    }
    return changed
  }

  list() {
    return [...this.servers.values()].map(({ id, name, url }) => ({ id, name, url })).sort((a, b) => a.name.localeCompare(b.name))
  }
}

// Runs only while the connect screen is open. A socket that can't open, such
// as when a firewall refuses it, just means nothing gets found this way.
function startDiscovery(onChange) {
  const servers = new ServerList()
  const changed = () => onChange(servers.list())
  const add = (s) => { if (s && servers.add(s)) changed() }

  const mdns = require('multicast-dns')()
  mdns.on('error', () => {})
  mdns.on('response', (packet) => {
    for (const s of serversFromRecords([...(packet.answers || []), ...(packet.additionals || [])])) add(s)
  })

  const udp = dgram.createSocket('udp4')
  udp.on('error', () => {})
  udp.on('message', (message, from) => add(serverFromUdpReply(message, from.address)))
  udp.bind(() => { try { udp.setBroadcast(true) } catch { /* no broadcast here */ } })

  const ask = () => {
    if (servers.expire()) changed()
    mdns.query([{ name: SERVICE, type: 'PTR' }])
    udp.send(UDP_QUESTION, UDP_PORT, '255.255.255.255', () => {})
  }
  ask()
  const timer = setInterval(ask, QUERY_EVERY_MS)

  return function stop() {
    clearInterval(timer)
    try { mdns.destroy() } catch { /* already closed */ }
    try { udp.close() } catch { /* already closed */ }
  }
}

module.exports = { startDiscovery, serversFromRecords, serverFromUdpReply, makeServer, ServerList, FORGET_AFTER_MS, MAX_SERVERS }
