// The connect screen: choose a server found on the network or type an address,
// then prove it with a security code. It works from a TV remote, a keyboard or a
// mouse. Platforms that can do more than a web page (the desktop app) provide
// window.wittyClient: a web page can't search the network or check a signature.
(function () {
  const S = self.WittyStrings
  const app = document.getElementById('app')
  const CODE_LENGTH = 6
  const NONE_AFTER_MS = 6000

  const ERRORS = {
    empty: S.errEmpty, scheme: S.errScheme, credentials: S.errCredentials, invalid: S.errInvalid,
    'public-http': S.errPublicHttp, unreachable: S.errUnreachable, 'not-server': S.errNotServer,
    identity: S.identityBad,
  }

  // Without a native bridge: a plain web page on a smart TV.
  const webBridge = {
    supportsDiscovery: false,
    async probe(url) {
      try {
        const res = await fetch(url + '/api/status', { credentials: 'omit' })
        const data = await res.json()
        if (data.name !== 'WittyMediaServer') return { error: 'not-server' }
        return { ok: true, url, version: data.version, name: data.brandName || data.name, identity: { state: 'none' } }
      } catch (e) { return { error: 'unreachable' } }
    },
    async recent() {
      try { return JSON.parse(localStorage.getItem('wmc.recent') || '[]') } catch (e) { return [] }
    },
    open(server) {
      try { localStorage.setItem('wmc.recent', JSON.stringify([{ url: server.url, name: server.name }])) } catch (e) { /* private mode */ }
      location.href = server.url
    },
  }
  const bridge = self.wittyClient || webBridge

  let screenBack = null
  let found = []
  let renderFound = null // set while the server list is on screen
  let userMoved = false

  function el(tag, props, ...kids) {
    const n = Object.assign(document.createElement(tag), props)
    for (const k of kids) n.appendChild(typeof k === 'string' ? document.createTextNode(k) : k)
    return n
  }

  function button(label, onclick, opts) {
    const b = el('button', { type: 'button', onclick, className: (opts && opts.className) || '' }, label)
    if (opts && opts.sub) b.appendChild(el('span', { className: 'sub', textContent: opts.sub }))
    if (opts && opts.url) b.dataset.url = opts.url
    return b
  }

  // Written for the older browsers built into smart TVs.
  function show(...kids) {
    renderFound = null
    userMoved = false
    app.textContent = ''
    for (const k of kids) app.appendChild(k)
    const first = app.querySelector('input, button.primary, button')
    if (first) first.focus()
  }

  const serverButton = (s) => button(s.name || s.url, () => choose(s.url), { sub: s.url, url: s.url })

  async function listScreen(message) {
    const recent = await bridge.recent()
    screenBack = null
    if (!bridge.supportsDiscovery && !recent.length) return addressScreen(message)

    const foundList = el('div', { className: 'list' })
    const status = el('p', { role: 'status', textContent: S.findSearching })
    const children = [el('h1', { textContent: S.findTitle })]
    if (message) children.push(el('p', { className: 'error', role: 'alert', textContent: message }))
    if (bridge.supportsDiscovery) {
      children.push(el('h2', { textContent: S.foundServers }), status, foundList)
    } else {
      children.push(el('p', { textContent: S.findNotSupported }))
    }
    if (recent.length) {
      children.push(el('h2', { textContent: S.recentServers }), el('div', { className: 'list' }, ...recent.map(serverButton)))
    }
    children.push(el('div', { className: 'row' }, button(S.enterAddress, () => addressScreen())))
    if (bridge.openLogs) children.push(button(S.openLogs, () => bridge.openLogs(), { className: 'link' }))
    children.push(el('p', { className: 'legal', textContent: S.legalConnect }))
    show(...children)

    // Servers come and go while this is on screen: keep focus on the same one,
    // and offer the first one found until the person has moved themselves.
    renderFound = () => {
      const focusedUrl = document.activeElement && document.activeElement.dataset ? document.activeElement.dataset.url : ''
      const hadFocus = foundList.contains(document.activeElement)
      const focusedIndex = Array.from(foundList.children).indexOf(document.activeElement)
      foundList.textContent = ''
      for (const s of found) foundList.appendChild(serverButton(s))
      status.textContent = found.length ? '' : (Date.now() - startedAt > NONE_AFTER_MS ? S.findNone : S.findSearching)
      const same = Array.from(foundList.children).find((b) => b.dataset.url === focusedUrl)
      if (same && hadFocus) same.focus()
      else if (hadFocus) {
        // The focused server went away. Without this, a remote has nothing to move from.
        const neighbour = foundList.children[Math.min(focusedIndex, foundList.children.length - 1)]
        const fallback = neighbour || app.querySelector('button')
        if (fallback) fallback.focus()
      } else if (!userMoved && foundList.firstChild) foundList.firstChild.focus()
    }
    const startedAt = Date.now()
    renderFound()
    if (bridge.supportsDiscovery) setTimeout(() => { if (renderFound) renderFound() }, NONE_AFTER_MS + 100)
  }

  function addressScreen(message, value) {
    const input = el('input', {
      id: 'address', type: 'text', value: value || '', autocapitalize: 'off', spellcheck: false,
      autocomplete: 'off', inputMode: 'url', placeholder: '192.168.1.20',
    })
    const error = el('p', { className: 'error', role: 'alert', textContent: message || '' })
    const go = button(S.connect, () => submit(input, error, go), { className: 'primary' })
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); go.click() } })
    show(
      el('h1', { textContent: S.addressTitle }),
      el('p', { textContent: S.addressHint }),
      el('label', { htmlFor: 'address', textContent: S.addressLabel }),
      input, error,
      el('div', { className: 'row' }, go),
      el('p', { className: 'legal', textContent: S.legalConnect }),
    )
    screenBack = () => listScreen()
  }

  async function submit(input, error, go) {
    const r = self.WittyAddress.normalizeAddress(input.value)
    if (r.error) { error.textContent = ERRORS[r.error]; input.focus(); return }
    go.disabled = true
    go.textContent = S.connecting
    const server = await bridge.probe(r.url)
    go.disabled = false
    go.textContent = S.connect
    if (server.error) { error.textContent = ERRORS[server.error]; input.focus(); return }
    next(server)
  }

  async function choose(url) {
    const server = await bridge.probe(url)
    if (server.error) return addressScreen(ERRORS[server.error], url)
    next(server)
  }

  // What this server needs before it can be opened.
  function next(server) {
    const state = server.identity ? server.identity.state : 'none'
    if (state === 'different') return identityScreen(server)
    if (state === 'bad') return addressScreen(S.identityBad, server.url)
    if (state === 'new' && bridge.redeem && !server.setupRequired) return codeScreen(server)
    bridge.open({ url: server.url, name: server.name })
  }

  function identityScreen(server) {
    show(
      el('h1', { textContent: S.identityTitle }),
      el('p', { textContent: S.identityHint }),
      el('p', { className: 'fingerprint', textContent: `${server.url} · ${S.fingerprintLabel}: ${(server.identity && server.identity.fingerprint) || ''}` }),
      button(S.back, () => listScreen(), { className: 'primary' }),
      el('div', { className: 'row' }, button(S.identityTrust, () => bridge.open({ url: server.url, name: server.name, trust: true }))),
    )
    screenBack = () => listScreen()
  }

  function codeScreen(server) {
    let code = ''
    let busy = false
    const display = el('div', { className: 'code', role: 'status', 'aria-label': S.codeLabel })
    const error = el('p', { className: 'error', role: 'alert' })
    const paint = () => {
      display.textContent = '•'.repeat(code.length) + '–'.repeat(CODE_LENGTH - code.length)
      display.setAttribute('aria-label', `${S.codeLabel}: ${code.length} ${S.codeEntered}`)
    }
    const send = async () => {
      busy = true
      error.textContent = S.connecting
      const r = await bridge.redeem(server.url, code)
      busy = false
      if (r.ok) return
      code = ''
      paint()
      error.textContent = r.message || ERRORS[r.error] || S.errInvalid
    }
    const type = (digit) => {
      if (busy || code.length >= CODE_LENGTH) return
      code += digit
      error.textContent = ''
      paint()
      if (code.length === CODE_LENGTH) send()
    }
    const erase = () => {
      if (busy) return
      code = code.slice(0, -1)
      paint()
    }
    const pad = el('div', { className: 'pad' })
    for (const d of ['1', '2', '3', '4', '5', '6', '7', '8', '9']) pad.appendChild(button(d, () => type(d)))
    pad.appendChild(button(S.codeDelete, erase))
    pad.appendChild(button('0', () => type('0')))
    paint()
    const usePassword = button(S.usePassword, () => bridge.open({ url: server.url, name: server.name }))
    show(
      el('h1', { textContent: S.codeTitle }),
      el('p', { textContent: S.codeHint }),
      display, error, pad,
      el('div', { className: 'row' }, usePassword),
      el('p', { className: 'fingerprint', textContent: `${S.fingerprintLabel}: ${(server.identity && server.identity.fingerprint) || ''}` }),
    )
    screenBack = () => listScreen()
    onScreenKey = (e) => {
      if (/^[0-9]$/.test(e.key)) { type(e.key); return true }
      if (e.key === 'Backspace') { erase(); return true }
      return false
    }
  }

  let onScreenKey = null

  // Arrow keys move to the nearest control in that direction. Inside a text box
  // left and right belong to the text, so only up and down leave it.
  function moveFocus(dir) {
    const cur = document.activeElement
    const all = Array.from(app.querySelectorAll('button:not(:disabled), input'))
    if (!all.length) return false
    if (!all.includes(cur)) { all[0].focus(); return true }
    const a = cur.getBoundingClientRect()
    const ax = a.left + a.width / 2, ay = a.top + a.height / 2
    let best = null, bestScore = Infinity
    for (const n of all) {
      if (n === cur) continue
      const b = n.getBoundingClientRect()
      const dx = b.left + b.width / 2 - ax, dy = b.top + b.height / 2 - ay
      const along = dir === 'left' ? -dx : dir === 'right' ? dx : dir === 'up' ? -dy : dy
      const across = dir === 'left' || dir === 'right' ? Math.abs(dy) : Math.abs(dx)
      if (along <= 1) continue
      const score = along + across * 2
      if (score < bestScore) { bestScore = score; best = n }
    }
    if (best) best.focus()
    return !!best
  }

  const DIRS = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }
  // Back on a remote: GoBack on many boxes, 461 on webOS, 10009 on Tizen.
  const isBack = (e) => e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.keyCode === 461 || e.keyCode === 10009

  document.addEventListener('keydown', (e) => {
    if (isBack(e)) {
      if (screenBack) { e.preventDefault(); const f = screenBack; screenBack = null; onScreenKey = null; f() }
      return
    }
    if (onScreenKey && document.querySelector('.pad') && onScreenKey(e)) { e.preventDefault(); return }
    const dir = DIRS[e.key]
    if (!dir) return
    if (document.activeElement && document.activeElement.tagName === 'INPUT' && (dir === 'left' || dir === 'right')) return
    userMoved = true
    if (moveFocus(dir)) e.preventDefault()
  })

  if (bridge.supportsDiscovery) {
    bridge.startDiscovery((servers) => {
      found = servers
      if (renderFound) renderFound()
    })
  }

  // A message from the app, such as "the saved server has gone away".
  const params = new URLSearchParams(location.search)
  const mismatch = params.get('mismatch')
  if (mismatch) choose(mismatch)
  else listScreen(params.get('lost') ? S.errLost : '')
})()
