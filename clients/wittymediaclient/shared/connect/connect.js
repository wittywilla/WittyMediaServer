// The connect screen: choose a server found on the network or type an address.
// It works from a TV remote, a keyboard or a mouse. Platforms that can do more
// than a web page (the desktop app) provide window.wittyClient.
(function () {
  const S = self.WittyStrings
  const app = document.getElementById('app')

  const ERRORS = {
    empty: S.errEmpty, scheme: S.errScheme, credentials: S.errCredentials, invalid: S.errInvalid,
    'public-http': S.errPublicHttp, unreachable: S.errUnreachable, 'not-server': S.errNotServer,
  }

  // Without a native bridge: a plain web page on a smart TV.
  const webBridge = {
    supportsDiscovery: false,
    async probe(url) {
      try {
        const res = await fetch(url + '/api/status', { credentials: 'omit' })
        const data = await res.json()
        if (data.name !== 'WittyMediaServer') return { error: 'not-server' }
        return { ok: true, url, version: data.version, name: data.brandName || data.name }
      } catch { return { error: 'unreachable' } }
    },
    async recent() {
      try { return JSON.parse(localStorage.getItem('wmc.recent') || '[]') } catch { return [] }
    },
    open(server) {
      try { localStorage.setItem('wmc.recent', JSON.stringify([server])) } catch { /* private mode */ }
      location.href = server.url
    },
  }
  const bridge = self.wittyClient || webBridge

  function el(tag, props, ...kids) {
    const n = Object.assign(document.createElement(tag), props)
    for (const k of kids) n.append(k)
    return n
  }

  function button(label, onclick, opts) {
    const b = el('button', { type: 'button', onclick, className: (opts && opts.className) || '' }, label)
    if (opts && opts.sub) b.append(el('span', { className: 'sub', textContent: opts.sub }))
    return b
  }

  function show(...kids) {
    app.replaceChildren(...kids)
    const first = app.querySelector('input, button.primary, button')
    if (first) first.focus()
  }

  const serverButton = (s) => button(s.name || s.url, () => choose(s.url), { sub: s.url })

  async function listScreen(message) {
    const recent = await bridge.recent()
    const children = [el('h1', { textContent: S.findTitle })]
    if (message) children.push(el('p', { className: 'error', role: 'alert', textContent: message }))
    if (!bridge.supportsDiscovery && !recent.length) return addressScreen(message)
    if (bridge === webBridge) children.push(el('p', { textContent: S.findNotSupported }))
    if (recent.length) {
      children.push(el('h2', { textContent: S.recentServers }), el('div', { className: 'list' }, ...recent.map(serverButton)))
    }
    children.push(el('div', { className: 'row' }, button(S.enterAddress, () => addressScreen())))
    show(...children)
  }

  function addressScreen(message, value) {
    const input = el('input', {
      id: 'address', type: 'text', value: value || '', autocapitalize: 'off', spellcheck: false,
      autocomplete: 'off', inputMode: 'url', placeholder: '192.168.1.20',
    })
    const error = el('p', { className: 'error', role: 'alert', textContent: message || '' })
    const go = button(S.connect, () => submit(input, error, go), { className: 'primary' })
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); go.click() } })
    screenBack = () => listScreen()
    show(
      el('h1', { textContent: S.addressTitle }),
      el('p', { textContent: S.addressHint }),
      el('label', { htmlFor: 'address', textContent: S.addressLabel }),
      input, error,
      el('div', { className: 'row' }, go),
    )
  }

  async function submit(input, error, go) {
    const r = self.WittyAddress.normalizeAddress(input.value)
    if (r.error) { error.textContent = ERRORS[r.error]; input.focus(); return }
    go.disabled = true
    go.textContent = S.connecting
    const found = await bridge.probe(r.url)
    go.disabled = false
    go.textContent = S.connect
    if (found.error) { error.textContent = ERRORS[found.error]; input.focus(); return }
    bridge.open({ url: r.url, name: found.name })
  }

  async function choose(url) {
    const found = await bridge.probe(url)
    if (found.error) return addressScreen(ERRORS[found.error], url)
    bridge.open({ url, name: found.name })
  }

  let screenBack = null

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
      if (screenBack) { e.preventDefault(); const f = screenBack; screenBack = null; f() }
      return
    }
    const dir = DIRS[e.key]
    if (!dir) return
    if (document.activeElement && document.activeElement.tagName === 'INPUT' && (dir === 'left' || dir === 'right')) return
    if (moveFocus(dir)) e.preventDefault()
  })

  // A message from the app, such as "the saved server has gone away".
  const params = new URLSearchParams(location.search)
  listScreen(params.get('lost') ? S.errLost : '')
})()
