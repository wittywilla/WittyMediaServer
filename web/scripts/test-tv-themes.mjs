// Walks WittyTV's themes in a headless browser, in every theme at phone, desktop and TV sizes, on a
// running test server with channels. No window opens. It measures the room above the first text and
// to the sides of it, checks nothing scrolls sideways and that the preview starts bigger than it
// used to, drives the Look panel and the guide with the keyboard alone, and checks every TV page
// follows the theme. On every TV screen, in every theme, it also checks that everything to press has
// a name, shows when it has focus and is big enough, that all the words on it can be read, and that
// a controller and touch work as well as keys. A picture of each is written to the folder given.
//
// Sign in details come from the environment. It changes that account's look and the server's theme
// for new viewers, so use a test server. Channels have to exist already.
//
//   WMS_SERVER=http://tv.localhost:9811 WMS_USER=tester WMS_PASSWORD=... node scripts/test-tv-themes.mjs out-folder
//
// WMS_ATTACH=127.0.0.1:9333 runs it all in something already running with a debugging port, such
// as the desktop client or an Android web view, rather than a browser of its own. WMS_ONLY=audit
// or WMS_ONLY=sizes runs part of it.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(new URL((process.env.WITTYVISUALIZER || '../../../WittyVisualizer') + '/tests/cdp.mjs', import.meta.url))

const server = process.env.WMS_SERVER || 'http://tv.localhost:9811'
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const THEMES = ['modern', 'classic', 'satellite', 'teletext', 'listings', 'bigpicture', 'outline', 'compact', 'custom']
// The least room above the first line of a TV page, and to either side of it. A TV crops up to
// about 5% off each edge, so across the room it has to clear that.
const SIZES = {
  phone: { w: 390, h: 844, mobile: true, top: 16, side: 12 },
  desktop: { w: 1440, h: 900, top: 22, side: 16 },
  small: { w: 1024, h: 640, top: 20, side: 16 },
  tv: { w: 1920, h: 1080, ten: true, top: 54, side: 90 },
}

const results = []
const expect = (what, ok, detail = '') => { const line = `${ok ? 'ok  ' : 'FAIL'} ${what}${ok ? '' : ' ' + JSON.stringify(detail)}`; results.push(line); console.log(line); if (!ok) process.exitCode = 1 }

// A page in something that's already running, spoken to the same way.
async function attach(hostPort) {
  const list = await (await fetch(`http://${hostPort}/json/list`)).json()
  const target = list.find((t) => t.type === 'page' && t.url.startsWith(server)) || list.find((t) => t.type === 'page' && !/titlebar|devtools/.test(t.url))
  if (!target) throw new Error('nothing to attach to: ' + JSON.stringify(list.map((t) => t.url)))
  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((ok, no) => { ws.onopen = ok; ws.onerror = no })
  let id = 0
  const waiting = new Map(), handlers = new Map()
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data)
    if (d.id && waiting.has(d.id)) { const [ok, no] = waiting.get(d.id); waiting.delete(d.id); d.error ? no(new Error(JSON.stringify(d.error))) : ok(d.result) } else handlers.get(d.method)?.(d.params)
  }
  const send = (method, params = {}) => new Promise((ok, no) => { waiting.set(++id, [ok, no]); ws.send(JSON.stringify({ id, method, params })) })
  await send('Page.enable'); await send('Runtime.enable')
  const page = {
    send, logs: [],
    async go(url) { const done = new Promise((d) => handlers.set('Page.loadEventFired', d)); await send('Page.navigate', { url }); await Promise.race([done, wait(15000)]) },
    async eval(expression) { const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, userGesture: true }); if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text); return r.result.value },
    async until(expression, ms = 15000, what = expression) { const end = Date.now() + ms; for (;;) { if (await page.eval(`!!(${expression})`)) return true; if (Date.now() > end) throw new Error('timed out waiting for ' + what); await wait(100) } },
    async shot() { return Buffer.from((await send('Page.captureScreenshot', { format: 'png' })).data, 'base64') },
  }
  return { page, close: async () => ws.close() }
}

const attached = process.env.WMS_ATTACH ? await attach(process.env.WMS_ATTACH) : null
const browser = attached || await launch({ software: true, width: 1440, height: 900 })
const page = attached ? attached.page : await browser.open(server + '/')
if (attached) await page.go(server + '/')
const only = process.env.WMS_ONLY || ''
const size = ({ w, h, mobile }) => page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: !!mobile })
await size(SIZES.desktop)
const call = (method, path, body) => page.eval(`(async () => { const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
const login = await call('POST', '/login', { name: process.env.WMS_USER, password: process.env.WMS_PASSWORD })
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
await call('POST', '/legal/acknowledge')
const me = login.user.id
const shot = async (name) => writeFileSync(join(out, `tv-${name}.png`), await page.shot())
const KEYS = { ArrowUp: 38, ArrowDown: 40, ArrowLeft: 37, ArrowRight: 39, Enter: 13, Escape: 27 }
const press = async (key) => {
  const windowsVirtualKeyCode = KEYS[key]
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key, code: key, windowsVirtualKeyCode, text: key === 'Enter' ? '\r' : undefined })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key, windowsVirtualKeyCode })
}
// A look put straight on the account and the device, the way it's kept.
const setLook = async (look) => {
  await wait(900) // anything the page was still saving
  await page.eval(`localStorage.setItem('wms.tv.look.${me}', ${JSON.stringify(JSON.stringify(look))}); window.dispatchEvent(new Event('wms-tv-look'))`)
  await call('PUT', '/me/ui', { tvLook: look })
}
const openGuide = async (ten) => { await page.go(server + `/tv?tv=${ten ? 1 : 0}`); await page.until(`document.querySelector('.ctv-grid .ctv-row')`, 30000, 'the guide') }
const root = (name) => page.eval(`document.querySelector('.ctv-page, .ctv-player').dataset.${name}`)
const measure = `(() => {
  const page = document.querySelector('.ctv-page'), pr = page.getBoundingClientRect()
  const walker = document.createTreeWalker(page, NodeFilter.SHOW_TEXT)
  let top = Infinity, first = '', left = Infinity, right = -Infinity
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!n.textContent.trim() || n.parentElement.closest('.sr-only, [aria-hidden=true]')) continue
    const r = document.createRange(); r.selectNodeContents(n)
    const b = r.getBoundingClientRect()
    if (!b.width || b.bottom <= pr.top || b.top >= pr.bottom) continue
    if (b.top < top) { top = b.top; first = n.textContent.trim().slice(0, 24) }
    if (n.parentElement.closest('.ctv-grid, .ctv-bar, .ctv-hero-actions')) continue
    // What's cut off with an ellipsis stops where it's cut.
    let edge = b.right
    for (let e = n.parentElement; e && e !== page; e = e.parentElement) if (getComputedStyle(e).overflowX !== 'visible') edge = Math.min(edge, e.getBoundingClientRect().right)
    left = Math.min(left, b.left); right = Math.max(right, edge)
  }
  const prev = document.querySelector('.ctv-preview')?.getBoundingClientRect()
  return { top: Math.round(top - pr.top), first, left: Math.round(left - pr.left), right: Math.round(pr.right - right), sideways: document.documentElement.scrollWidth - innerWidth, inner: page.scrollWidth - page.clientWidth, preview: prev ? Math.round(prev.width) : 0, previewTop: prev ? Math.round(prev.top - pr.top) : 0, pageH: Math.round(pr.height) }
})()`

// ---------------------------------------- every theme at every size
await setLook({})
for (const [name, s] of only === 'audit' ? [] : Object.entries(SIZES)) {
  await size(s)
  await openGuide(s.ten)
  for (const theme of THEMES) {
    await setLook({ theme })
    await wait(1300)
    const m = await page.eval(measure)
    const say = `${name} ${theme}`
    expect(`${say}: room above the first text (${m.top}px, "${m.first}")`, m.top >= s.top, m)
    expect(`${say}: room above the preview`, m.previewTop >= s.top, m)
    expect(`${say}: room to the left and right`, m.left >= s.side && m.right >= s.side, m)
    expect(`${say}: nothing scrolls sideways`, m.sideways <= 0 && m.inner <= 0, m)
    if (theme === 'compact') expect(`${say}: the preview fits its theme`, m.preview >= 180, m)
    else expect(`${say}: the preview starts big (${m.preview}px)`, m.preview > 320 || (name === 'small' && m.preview >= 250), m)
    await shot(`${name}-${theme}`)
  }
}

if (only !== 'audit') {
// ---------------------------------------- moving an earlier look over
await size(SIZES.desktop)
await page.eval(`localStorage.setItem('wms.tv.look.${me}', JSON.stringify({ layout: 'classic', colours: 'custom', customBg: '#10203a', customAccent: '#ffb300', previewSize: 'l', previewPos: 'bl', density: 'large' }))`)
await call('PUT', '/me/ui', { tvLook: { layout: 'classic', colours: 'custom', customBg: '#10203a', customAccent: '#ffb300', previewSize: 'l', previewPos: 'bl', density: 'large' } })
await openGuide(false)
await wait(1200)
const old = await page.eval(`(() => { const p = document.querySelector('.ctv-page'); return { theme: p.dataset.tvtheme, shape: p.dataset.shape, bg: p.style.getPropertyValue('--ctv-bg'), hi: p.style.getPropertyValue('--ctv-hi'), w: Math.round(document.querySelector('.ctv-preview').getBoundingClientRect().width), left: document.querySelector('.ctv-hero').classList.contains('side-left') } })()`)
expect('an earlier look keeps its layout, colours, size and side', old.theme === 'custom' && old.shape === 'box' && old.bg === '#10203a' && old.hi === '#ffb300' && old.w === 460 && old.left, old)

// ---------------------------------------- which wins
await setLook({})
await call('PUT', '/tv/settings', { tvTheme: 'teletext' })
await page.eval(`localStorage.removeItem('wms.tv.owner')`)
await openGuide(false); await wait(1200)
expect('with no choice, the owner\'s default for new viewers', (await root('tvtheme')) === 'teletext', await root('tvtheme'))
await page.eval(`document.documentElement.style.setProperty('--tv-theme', 'listings'); window.dispatchEvent(new Event('wms-tv-look'))`)
await wait(500)
expect('an app theme\'s suggestion beats that', (await root('tvtheme')) === 'listings', await root('tvtheme'))
await setLook({ theme: 'compact' }); await wait(500)
expect('a viewer\'s own choice beats both', (await root('tvtheme')) === 'compact', await root('tvtheme'))
await page.eval(`document.documentElement.style.removeProperty('--tv-theme')`)
await call('PUT', '/tv/settings', { tvTheme: 'classic' })
await setLook({})
await page.eval(`localStorage.removeItem('wms.tv.owner')`)

// ---------------------------------------- the Look panel, with the keyboard alone
await openGuide(false); await wait(800)
await page.eval(`[...document.querySelectorAll('.ctv-bar button')].find((b) => b.textContent === 'Look').focus()`)
await press('Enter')
await page.until(`document.querySelector('.ctv-look')`, 5000, 'the panel'); await wait(500)
await shot('look-panel')
const cards = await page.eval(`(() => { const s = document.querySelector('.ctv-look').getBoundingClientRect(); return [...document.querySelectorAll('.ctv-look .ctv-themecard')].every((c) => c.getBoundingClientRect().right <= s.right + 1) })()`)
expect('the cards stay inside the panel', cards)
await page.eval(`document.querySelector('.ctv-themecard.on').focus()`)
const was = await root('tvtheme')
await press('ArrowDown'); await wait(300)
expect('arrows move among the themes, and it shows straight away', (await root('tvtheme')) !== was)
expect('and the keyboard stays on the one chosen', await page.eval(`document.activeElement.getAttribute('aria-checked') === 'true'`))
await page.eval(`[...document.querySelectorAll('.ctv-themecard')].find((c) => c.textContent.startsWith('Custom')).click()`); await wait(300)
await page.eval(`[...document.querySelectorAll('.ctv-shapes .ctv-themecard')].find((c) => c.textContent === 'Newspaper').click()`); await wait(300)
expect('Custom takes any layout', (await root('shape')) === 'listings')
await page.eval(`document.querySelector('.ctv-swatch[data-colour="#17223b"]').click()`); await wait(300)
const custom = await page.eval(`(() => { const s = document.querySelector('.ctv-page').style; return { bg: s.getPropertyValue('--ctv-bg'), text: s.getPropertyValue('--ctv-text') } })()`)
expect('a colour overrides, and the text follows it', custom.bg === '#17223b' && custom.text === '#ffffff', custom)
await shot('custom')
await page.eval(`[...document.querySelectorAll('.ctv-editor-actions button')].find((b) => b.textContent.startsWith('Back to')).click()`); await wait(300)
expect('and there is a way back', await page.eval(`document.querySelector('.ctv-page').style.getPropertyValue('--ctv-bg') === 'var(--bg)'`))
await press('Escape'); await wait(300)
expect('Escape closes the panel', await page.eval(`!document.querySelector('.ctv-look')`))
expect('and the keyboard goes back to the Look button', await page.eval(`document.activeElement?.textContent === 'Look'`))

// ---------------------------------------- the preview's size
await setLook({ theme: 'modern' }); await wait(700)
const width = () => page.eval(`Math.round(document.querySelector('.ctv-preview').getBoundingClientRect().width)`)
const start = await width()
await page.eval(`document.querySelector('.ctv-preview-handle').focus()`)
await press('ArrowLeft'); await wait(250)
const chosen = await width()
expect('arrows on the corner resize it', chosen === start - 24, [start, chosen])
await size(SIZES.tv); await wait(700)
expect('a size someone chose isn\'t overridden by a bigger window', (await width()) === chosen, [chosen, await width()])
await setLook({ theme: 'modern' }); await wait(700)
expect('automatic grows with the window', (await width()) > start + 100, [start, await width()])
await size(SIZES.desktop); await wait(400)

// ---------------------------------------- the arrows, in every theme
for (const theme of THEMES) {
  await setLook({ theme }); await wait(500)
  await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
  const a = await page.eval(`document.activeElement.getAttribute('aria-label')`)
  await press('ArrowDown'); await wait(120); await press('ArrowRight'); await wait(120)
  const b = await page.eval(`document.activeElement.getAttribute('aria-label')`)
  expect(`${theme}: arrows move through the grid`, a !== b && /^Channel /.test(b || ''), [a, b])
  await press('ArrowUp'); await wait(100); await press('ArrowLeft'); await wait(100)
}

// ---------------------------------------- every other TV page
for (const theme of ['teletext', 'listings', 'modern']) {
  await setLook({ theme })
  for (const [name, path, ready] of [['recordings', '/tv/recordings', '.ctv-list-head h1'], ['help', '/tv/encrypted', '.ctv-list-head h1']]) {
    await page.go(server + path); await page.until(`document.querySelector('${ready}')`, 10000, name); await wait(500)
    expect(`${name} follow ${theme}`, (await root('tvtheme')) === theme)
    const gap = await page.eval(`Math.round(document.querySelector('${ready}').getBoundingClientRect().top - document.querySelector('.ctv-page').getBoundingClientRect().top)`)
    expect(`${name} have room at the top in ${theme}`, gap >= 22, gap)
    await shot(`${name}-${theme}`)
  }
  const channels = await call('GET', '/guide?list=1&limit=1')
  if (channels.channels?.length) {
    await page.go(server + '/tv/watch/' + channels.channels[0].key); await page.until(`document.querySelector('.ctv-player')`, 10000, 'the player'); await wait(1500)
    expect(`the player follows ${theme}`, (await root('tvtheme')) === theme)
    await shot(`player-${theme}`)
  }
}

}

// ---------------------------------------- names, focus, sizes and words, on every screen
// Everything to press has a name and is big enough; focus can be seen on it; every word can be read.
const AUDIT = `(minSize) => {
  const out = []
  const parse = (c) => {
    let m = /rgba?\\(([\\d.]+)[ ,]+([\\d.]+)[ ,]+([\\d.]+)(?:[ ,/]+([\\d.]+))?/.exec(c)
    if (m) return [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : +m[4]]
    m = /color\\(srgb ([\\d.]+) ([\\d.]+) ([\\d.]+)(?: \\/ ([\\d.]+))?/.exec(c)
    if (m) return [m[1] * 255, m[2] * 255, m[3] * 255, m[4] === undefined ? 1 : +m[4]]
    return null
  }
  const over = (a, b) => [0, 1, 2].map((i) => a[i] * a[3] + b[i] * (1 - a[3])).concat(1)
  const lum = (c) => { const f = (v) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]) }
  const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
  // What's behind an element, as one or more colours where a gradient gives several. Not known over a picture.
  const behind = (el) => {
    let stack = [], picture = false
    for (let e = el; e; e = e.parentElement) {
      const c = getComputedStyle(e)
      if (e.matches('.ctv-preview-screen, .stage, video, .ctv-pic') || e.querySelector(':scope > .ctv-hero-art')) picture = true
      const img = c.backgroundImage
      if (img && img !== 'none') {
        if (img.includes('url(')) picture = true
        const stops = img.match(/(rgba?|color)\\([^)]*\\)/g) || []
        const opaque = stops.map(parse).filter(Boolean)
        if (opaque.length) { stack.push(opaque); if (opaque.every((o) => o[3] >= 1)) return picture ? null : compose(stack) }
      }
      const bg = parse(c.backgroundColor)
      if (bg && bg[3] > 0) { stack.push([bg]); if (bg[3] >= 1) return picture ? null : compose(stack) }
    }
    return picture ? null : compose(stack.concat([[[255, 255, 255, 1]]]))
  }
  const compose = (stack) => {
    let results = stack[stack.length - 1].map((c) => over(c, [255, 255, 255, 1]))
    for (let i = stack.length - 2; i >= 0; i--) results = stack[i].flatMap((c) => results.map((r) => over(c, r)))
    return results
  }
  const shown = (e) => { const r = e.getBoundingClientRect(); const c = getComputedStyle(e); return r.width > 1 && r.height > 1 && c.visibility !== 'hidden' && c.display !== 'none' && !e.closest('[aria-hidden=true], .sr-only') }
  const nameOf = (e) => {
    const by = e.getAttribute('aria-labelledby')
    return (e.getAttribute('aria-label') || (by && document.getElementById(by)?.textContent) || e.closest('label')?.textContent || (e.labels && e.labels[0]?.textContent) || e.innerText || e.title || e.getAttribute('alt') || e.placeholder || '').trim()
  }
  const say = (e) => (e.tagName.toLowerCase() + (e.className && e.className.baseVal === undefined ? '.' + String(e.className).trim().split(/\\s+/).slice(0, 2).join('.') : '') + ' "' + nameOf(e).slice(0, 26) + '"')
  const root = document.querySelector('[role=dialog]') || document.querySelector('.ctv-player') || document.querySelector('main')
  const controls = [...root.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, [tabindex="0"], [role=slider]')].filter((e) => shown(e) && !e.disabled)
  document.activeElement?.blur()
  // What's on its way to a colour isn't there yet.
  const still = document.createElement('style'); still.textContent = '* { transition: none !important; animation: none !important; }'; document.head.append(still)
  for (const e of controls) {
    if (!nameOf(e)) out.push('no name: ' + say(e))
    const r = (e.type === 'radio' || e.type === 'checkbox') && e.closest('label') ? e.closest('label').getBoundingClientRect() : e.getBoundingClientRect()
    if (minSize && r.height < minSize - 0.5 && !e.closest('.ctv-preview-bar, .legal-note, p, small, .set-prose, .ctv-editor-actions') && !e.matches('.ctv-preview-handle, .ctv-swatch, input[type=radio], input[type=checkbox]')) out.push('small (' + Math.round(r.height) + '): ' + say(e))
    // Focus has to change something that shows.
    const style = (x) => { const c = getComputedStyle(x); return [c.outlineStyle + c.outlineWidth + c.outlineColor, c.boxShadow, c.backgroundColor, c.borderColor, c.color, c.backgroundImage].join('|') }
    if (e.matches('.ctv-cell')) continue
    // A dialog can hold on to focus: see it without by giving it to something else first.
    if (document.activeElement === e) controls.find((x) => x !== e)?.focus({ preventScroll: true })
    const before = style(e)
    e.focus({ preventScroll: true })
    if (document.activeElement === e && style(e) === before) out.push('focus does not show: ' + say(e))
    e.blur()
  }
  // Words
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  still.remove()
  const seen = new Set()
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!n.textContent.trim()) continue
    const el = n.parentElement
    if (seen.has(el) || !shown(el) || el.closest('option, select, .ctv-preview-screen, script, style')) continue
    seen.add(el)
    const c = getComputedStyle(el)
    const fg = parse(c.color)
    if (!fg || Number(c.opacity) === 0) continue
    const bgs = behind(el)
    if (!bgs) continue
    const size = parseFloat(c.fontSize), bold = parseInt(c.fontWeight) >= 700
    const need = size >= 24 || (size >= 18.66 && bold) ? 3 : 4.5
    const worst = Math.min(...bgs.map((b) => ratio(over(fg, b), b)))
    if (worst < need - 0.05 && !el.closest('button:disabled, a[aria-disabled=true]')) out.push('hard to read (' + worst.toFixed(2) + '): ' + c.color + ' "' + n.textContent.trim().slice(0, 26) + '" in ' + el.tagName.toLowerCase() + '.' + String(el.className).split(' ')[0])
  }
  const extra = document.documentElement.scrollWidth - innerWidth
  if (extra > 0) out.push('scrolls sideways by ' + extra)
  return [...new Set(out)]
}`
const audit = (min) => page.eval(`(${AUDIT})(${min || 0})`)
const checkScreen = async (what, min) => {
  // Focus only shows once the keyboard has been used, as for anyone using it.
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Shift', code: 'ShiftLeft', windowsVirtualKeyCode: 16 })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Shift', code: 'ShiftLeft', windowsVirtualKeyCode: 16 })
  const problems = await audit(min)
  expect(`${what}: names, focus${min ? ', sizes' : ''} and words`, problems.length === 0, problems.slice(0, 14))
}

const SCREENS = {
  desktop: { ...SIZES.desktop, min: 0 },
  phone: { ...SIZES.phone, min: 44 },
  tv: { ...SIZES.tv, min: 44 },
}
const keyOn = async (key) => {
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key, code: key, windowsVirtualKeyCode: key.toUpperCase().charCodeAt(0), text: key })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key, windowsVirtualKeyCode: key.toUpperCase().charCodeAt(0) })
}
const closeDialog = async () => { await press('Escape'); await wait(350) }
for (const [name, s] of only === 'sizes' ? [] : Object.entries(SCREENS)) {
  await size(s)
  // A phone's pointer is a finger, which is what makes its controls big enough for one.
  await page.send('Emulation.setTouchEmulationEnabled', { enabled: !!s.mobile, maxTouchPoints: s.mobile ? 5 : 1 })
  await openGuide(s.ten)
  for (const theme of THEMES) {
    await setLook(theme === 'custom' ? { theme, custom: { shape: 'teletext', font: 'serif' }, themeColours: { custom: { bg: '#fedcba', cell: '#123456', hi: '#ff00ff', accent: '#00ffff' } } } : { theme }); await wait(900)
    await checkScreen(`${name} ${theme}: the guide`, s.min)
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    await keyOn('i'); await page.until(`document.querySelector('[role=dialog]')`, 4000, 'details'); await wait(250)
    await checkScreen(`${name} ${theme}: about a programme`, s.min)
    await closeDialog()
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    await keyOn('s'); await page.until(`document.querySelector('[role=dialog]')`, 4000, 'search'); await wait(250)
    await checkScreen(`${name} ${theme}: search`, s.min)
    await closeDialog()
  }
  // The pages around it, in one theme of each kind and the most extreme colours.
  const channels = await call('GET', '/guide?list=1&limit=1')
  for (const theme of ['classic', 'listings', 'teletext', 'outline', 'modern']) {
    await setLook({ theme })
    for (const [n, path, ready] of [['recordings', '/tv/recordings', '.ctv-list-head h1'], ['help', '/tv/encrypted', '.ctv-list-head h1']]) {
      await page.go(server + path + (s.ten ? '?tv=1' : '?tv=0')); await page.until(`document.querySelector('${ready}')`, 10000, n); await wait(450)
      await checkScreen(`${name} ${theme}: ${n}`, s.min)
    }
    if (channels.channels?.length) {
      await page.go(server + '/tv/watch/' + channels.channels[0].key + (s.ten ? '?tv=1' : '?tv=0')); await page.until(`document.querySelector('.ctv-banner')`, 12000, 'the banner'); await wait(500)
      await checkScreen(`${name} ${theme}: the player and banner`, 0)
      await shot(`audit-player-${name}-${theme}`)
      await keyOn('g'); await page.until(`document.querySelector('.ctv-mini')`, 5000, 'the mini guide'); await wait(900)
      await checkScreen(`${name} ${theme}: the mini guide`, 0)
      await shot(`audit-mini-${name}-${theme}`)
    }
    await openGuide(s.ten)
  }
  // The panel and settings keep to the app's own colours, so once each.
  await setLook({ theme: 'modern' }); await wait(500)
  await page.eval(`[...document.querySelectorAll('.ctv-bar button')].find((b) => b.textContent === 'Look').click()`)
  await page.until(`document.querySelector('.ctv-look')`, 5000, 'the panel'); await wait(350)
  await checkScreen(`${name}: the Look panel`, s.min)
  await shot(`audit-panel-${name}`)
  await closeDialog()
  for (const [n, path, ready] of [['settings, Appearance', '/settings/appearance', '#wittytv .ctv-themes'], ['settings, Channels', '/settings/channels', '.set-card'], ['a channel\'s editor', '/settings/channels/edit/1', '.tve-logo']]) {
    await page.go(server + path + (s.ten ? '?tv=1' : '?tv=0')); await page.until(`document.querySelector('${ready}')`, 12000, n); await wait(700)
    await checkScreen(`${name}: ${n}`, 0)
    await shot(`audit-${n.replace(/\W+/g, '-')}-${name}`)
  }
}

// ---------------------------------------- a controller, in every theme
if (only !== 'sizes') {
  await size(SIZES.tv)
  await page.send('Emulation.setTouchEmulationEnabled', { enabled: false })
  await page.send('Page.addScriptToEvaluateOnNewDocument', { source: `
    window.__pad = null
    navigator.getGamepads = () => [window.__pad, null, null, null]
    window.__press = async (i, ms = 70, axes) => {
      const buttons = Array.from({ length: 17 }, (_, n) => ({ pressed: n === i, value: n === i ? 1 : 0, touched: n === i }))
      window.__pad = { id: window.__padId || 'Xbox 360 Controller (STANDARD GAMEPAD)', index: 0, connected: true, mapping: 'standard', buttons, axes: axes || [0, 0, 0, 0], timestamp: performance.now() }
      await new Promise((d) => setTimeout(d, ms))
      window.__pad = { id: window.__pad.id, index: 0, connected: true, mapping: 'standard', buttons: buttons.map(() => ({ pressed: false, value: 0, touched: false })), axes: [0, 0, 0, 0], timestamp: performance.now() }
      await new Promise((d) => setTimeout(d, 90))
    }
  ` })
  await openGuide(true)
  await page.eval(`(() => { window.__pad = { id: 'Xbox 360 Controller (STANDARD GAMEPAD)', index: 0, connected: true, mapping: 'standard', buttons: Array.from({ length: 17 }, () => ({ pressed: false, value: 0, touched: false })), axes: [0, 0, 0, 0], timestamp: 1 }; window.dispatchEvent(new Event('gamepadconnected')) })()`)
  await wait(200)
  const pad = (i, axes) => page.eval(`window.__press(${i}, 70, ${JSON.stringify(axes || null)})`)
  const focusName = () => page.eval(`(document.activeElement.getAttribute('aria-label') || document.activeElement.textContent || '').trim()`)
  const UP = 12, DOWN = 13, LEFT = 14, RIGHT = 15, A = 0, B = 1
  for (const theme of THEMES) {
    await setLook({ theme }); await wait(600)
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    const a = await focusName()
    await pad(DOWN); await wait(100)
    const b = await focusName()
    expect(`${theme}: the D-pad moves down the guide`, a !== b && /^Channel /.test(b), [a, b])
    await pad(RIGHT); await wait(100)
    const c = await focusName()
    expect(`${theme}: and along it`, b !== c && /^Channel /.test(c), [b, c])
    await pad(0, undefined); await wait(50)
    // Pressed on a programme to come: its details, or the player. Either way, B comes back.
    const went = await page.eval(`location.pathname + (document.querySelector('[role=dialog]') ? ' dialog' : '')`)
    expect(`${theme}: A acts on the highlighted programme`, went !== '/tv', went)
    await pad(B); await wait(700)
    await page.until(`location.pathname === '/tv'`, 5000, 'back at the guide').catch(() => {})
    expect(`${theme}: B comes back to the guide`, await page.eval(`location.pathname === '/tv' && !document.querySelector('[role=dialog]')`), await page.eval(`location.pathname`))
    await page.until(`document.querySelector('.ctv-cell[data-focused="1"]')`, 10000, 'the highlight')
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    // Up out of the grid to the buttons above it, with nothing but the pad.
    let reached = false
    for (let i = 0; i < 10 && !reached; i++) { await pad(UP); await wait(60); reached = await page.eval(`!document.activeElement.closest('.ctv-grid') && !!document.activeElement.closest('.ctv-bar, .ctv-hero')`) }
    expect(`${theme}: up reaches the controls above the grid`, reached, await focusName())
    // The left stick does the same as the D-pad.
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    const d = await focusName()
    await page.eval(`window.__press(-1, 70, [0, 1, 0, 0])`); await wait(100)
    expect(`${theme}: the stick moves too`, d !== await focusName(), d)
    await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
    await pad(UP); await wait(80)
  }
  // Every layout a pad comes in: Nintendo swaps its two face buttons, an unknown one has its D-pad on a hat.
  await setLook({ theme: 'modern' }); await wait(500)
  await page.eval(`window.__padId = 'Nintendo Switch Pro Controller (STANDARD GAMEPAD)'; document.querySelector('.ctv-cell[data-focused="1"]').focus()`)
  await pad(B); await wait(600)
  expect('a Nintendo pad confirms with its A, on the right', await page.eval(`location.pathname !== '/tv' || !!document.querySelector('[role=dialog]')`))
  await pad(A); await wait(700)
  await page.until(`location.pathname === '/tv' && document.querySelector('.ctv-cell[data-focused="1"]')`, 8000, 'the guide').catch(() => {})
  await page.eval(`window.__padId = 'Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 09cc)'`)
  await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]')?.focus()`)
  const e0 = await focusName(); await pad(DOWN); await wait(100)
  expect('a DualShock or DualSense pad moves the same', e0 !== await focusName())
  // The Look panel, by pad alone.
  await page.eval(`[...document.querySelectorAll('.ctv-bar button')].find((b) => b.textContent === 'Look').focus()`)
  await pad(A); await page.until(`document.querySelector('.ctv-look')`, 4000, 'the panel by A'); await wait(400)
  await page.eval(`document.querySelector('.ctv-themecard.on').focus()`)
  const before = await root('tvtheme')
  await pad(DOWN); await wait(250)
  expect('a pad moves among the themes', (await root('tvtheme')) !== before)
  await pad(B); await wait(400)
  expect('B closes the panel', await page.eval(`!document.querySelector('.ctv-look')`))
  await page.eval(`window.__pad = null`)
  await page.go(server + '/tv?tv=0')
}

// ---------------------------------------- touch
if (only !== 'sizes' && !attached) {
  await size(SIZES.phone)
  await page.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 })
  const tap = async (sel) => {
    const r = await page.eval(`(() => { const e = ${sel}; e.scrollIntoView({ block: 'center', inline: 'center' }); const b = e.getBoundingClientRect(); return [b.x + b.width / 2, b.y + b.height / 2] })()`)
    await page.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: r[0], y: r[1] }] })
    await page.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await wait(350)
  }
  for (const theme of THEMES) {
    await setLook({ theme }); await page.go(server + '/tv?tv=0'); await page.until(`document.querySelector('.ctv-cell[data-focused="1"]')`, 15000, 'the guide'); await wait(600)
    const was = await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').getAttribute('aria-label')`)
    await tap(`document.querySelector('.ctv-row:not(.current) .ctv-cell:not(.past)')`)
    const now = await page.eval(`document.querySelector('.ctv-cell[data-focused="1"]').getAttribute('aria-label')`)
    expect(`${theme}: a tap moves the highlight`, was !== now, [was, now])
    await tap(`[...document.querySelectorAll('.ctv-bar button')].find((b) => b.textContent === 'Look')`)
    expect(`${theme}: a tap opens Look`, await page.eval(`!!document.querySelector('.ctv-look')`))
    await tap(`[...document.querySelectorAll('.ctv-look .ctv-themecard')].find((c) => c.getAttribute('aria-checked') === 'false')`)
    const changed = await page.eval(`document.querySelector('.ctv-page').dataset.tvtheme`)
    expect(`${theme}: a tap chooses a theme`, changed !== theme, changed)
    await tap(`[...document.querySelectorAll('.ctv-look button')].find((b) => b.textContent === 'Done')`)
    expect(`${theme}: Done closes it`, await page.eval(`!document.querySelector('.ctv-look')`))
  }
  // Dragging the preview's corner with a finger.
  await setLook({ theme: 'compact' }); await page.go(server + '/tv?tv=0'); await page.until(`document.querySelector('.ctv-preview-handle')`, 15000, 'the preview'); await wait(600)
  const w0 = await page.eval(`Math.round(document.querySelector('.ctv-preview').getBoundingClientRect().width)`)
  const h = await page.eval(`(() => { const b = document.querySelector('.ctv-preview-handle').getBoundingClientRect(); return [b.x + b.width / 2, b.y + b.height / 2] })()`)
  await page.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: h[0], y: h[1] }] })
  await page.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: h[0] - 30, y: h[1] }] })
  await page.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: h[0] - 60, y: h[1] }] })
  await page.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
  await wait(400)
  const w1 = await page.eval(`Math.round(document.querySelector('.ctv-preview').getBoundingClientRect().width)`)
  expect('dragging the preview with a finger resizes it', w1 !== w0, [w0, w1])
  await page.send('Emulation.setTouchEmulationEnabled', { enabled: false })
}

await setLook({})
const failed = results.filter((r) => r.startsWith('FAIL')).length
console.log(`${results.length - failed} of ${results.length} passed`)
await browser.close()
