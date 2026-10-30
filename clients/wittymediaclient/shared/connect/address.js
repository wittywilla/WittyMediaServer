// Turns what someone typed into a server address, or says why it can't be one.
// The rules and their test cases are shared by every client: see
// shared/data/connection-cases.json. Loaded as a plain script and by Node's tests.
(function (root) {
  const DEFAULT_PORT = 9797

  function ipv4Private(host) {
    const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host)
    if (!m) return null
    const [a, b] = [Number(m[1]), Number(m[2])]
    return a === 127 || a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) ||
      (a === 169 && b === 254) || (a === 100 && b >= 64 && b <= 127)
  }

  // IPv6 hosts arrive in brackets, in the URL parser's compressed form.
  function ipv6Private(host) {
    if (!host.startsWith('[')) return null
    const h = host.slice(1, -1)
    return h === '::1' || /^f[cd]/.test(h) || /^fe[89ab]/.test(h)
  }

  // Is this address on the local network? A name with no dots or ending in
  // .local can only be resolved locally; anything else may be public.
  function isPrivateHost(host) {
    const v4 = ipv4Private(host)
    if (v4 !== null) return v4
    const v6 = ipv6Private(host)
    if (v6 !== null) return v6
    return host === 'localhost' || !host.includes('.') || host.endsWith('.local')
  }

  function normalizeAddress(input) {
    let text = String(input == null ? '' : input).trim()
    if (!text) return { error: 'empty' }
    const scheme = /^([a-zA-Z][a-zA-Z0-9+.-]*):\/\//.exec(text) || /^(javascript|file|data|mailto):/i.exec(text)
    let addPort = false
    if (scheme) {
      if (!/^https?$/i.test(scheme[1])) return { error: 'scheme' }
    } else {
      // A bare IPv6 address needs its brackets before it can be parsed.
      if ((text.match(/:/g) || []).length >= 2 && !text.startsWith('[')) text = `[${text}]`
      addPort = !/(\]|^[^[\]:]+):\d*$/.test(text)
      text = 'http://' + text
    }
    let u
    try { u = new URL(text) } catch { return { error: 'invalid' } }
    if (u.username || u.password) return { error: 'credentials' }
    if (!u.hostname) return { error: 'invalid' }
    if (addPort) u.port = String(DEFAULT_PORT)
    if (u.protocol === 'http:' && !isPrivateHost(u.hostname)) return { error: 'public-http' }
    return { url: u.origin }
  }

  const api = { normalizeAddress, isPrivateHost, DEFAULT_PORT }
  if (typeof module !== 'undefined' && module.exports) module.exports = api
  else root.WittyAddress = api
})(typeof self !== 'undefined' ? self : this)
