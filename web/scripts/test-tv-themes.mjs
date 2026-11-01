// Walks WittyTV's themes in a headless browser, in every theme at phone, desktop and TV sizes, on a
// running test server with channels. No window opens. It measures the room above the first text and
// to the sides of it, checks nothing scrolls sideways and that the preview starts bigger than it
// used to, drives the Look panel and the guide with the keyboard alone, and checks every TV page
// follows the theme. A picture of each is written to the folder given.
//
// Sign in details come from the environment. It changes that account's look and the server's theme
// for new viewers, so use a test server. Channels have to exist already.
//
//   WMS_SERVER=http://tv.localhost:9811 WMS_USER=tester WMS_PASSWORD=... node scripts/test-tv-themes.mjs out-folder
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
const expect = (what, ok, detail = '') => { results.push(`${ok ? 'ok  ' : 'FAIL'} ${what}${ok ? '' : ' ' + JSON.stringify(detail)}`); if (!ok) process.exitCode = 1 }

const browser = await launch({ software: true, width: 1440, height: 900 })
const page = await browser.open(server + '/')
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
    if (!n.parentElement.closest('.ctv-grid, .ctv-bar, .ctv-hero-actions')) { left = Math.min(left, b.left); right = Math.max(right, b.right) }
  }
  const prev = document.querySelector('.ctv-preview')?.getBoundingClientRect()
  return { top: Math.round(top - pr.top), first, left: Math.round(left - pr.left), right: Math.round(pr.right - right), sideways: document.documentElement.scrollWidth - innerWidth, inner: page.scrollWidth - page.clientWidth, preview: prev ? Math.round(prev.width) : 0, previewTop: prev ? Math.round(prev.top - pr.top) : 0, pageH: Math.round(pr.height) }
})()`

// ---------------------------------------- every theme at every size
await setLook({})
for (const [name, s] of Object.entries(SIZES)) {
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

await setLook({})
for (const r of results) console.log(r)
const failed = results.filter((r) => r.startsWith('FAIL')).length
console.log(`${results.length - failed} of ${results.length} passed`)
await browser.close()
