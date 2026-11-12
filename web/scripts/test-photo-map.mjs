// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Tries the photo map in a headless browser, on a running test server: phone, desktop and TV
// sizes in a dark and a light theme, then the mouse, a finger, two fingers, the keyboard and a
// remote's arrows, and an online map that doesn't answer. No window opens. It expects a Photos
// library with pictures that have places, such as the demo photos, and an admin account: it turns
// the online map on for a moment, at an address that can't answer, and puts the settings back.
//
//   WMS_SERVER=http://127.0.0.1:9869 WMS_NAME=... WMS_PASS=... node scripts/test-photo-map.mjs out-folder
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

const server = process.env.WMS_SERVER || 'http://127.0.0.1:9869'
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
let failed = 0
const expect = (what, ok, detail) => {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${what}${ok || detail === undefined ? '' : ' ' + JSON.stringify(detail)}`)
  if (!ok) failed++
}
const call = async (method, path, body, token) => {
  const r = await fetch(server + '/api' + path, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) }, body: body ? JSON.stringify(body) : undefined })
  return { status: r.status, data: await r.json().catch(() => ({})) }
}
const { data: { token } } = await call('POST', '/login', { name: process.env.WMS_NAME, password: process.env.WMS_PASS })
if (!token) throw new Error('could not sign in: set WMS_NAME and WMS_PASS')
const lib = (await call('GET', '/libraries', null, token)).data.find((l) => l.type === 'photos')
if (!lib) throw new Error('no Photos library')
const placed = (await call('GET', `/photos/map?library=${lib.id}&zoom=0&fit=1`, null, token)).data.bounds?.count || 0
if (!placed) throw new Error('no pictures with a place in ' + lib.name)

const browser = await launch({ software: true, width: 1440, height: 900, args: ['--disable-renderer-backgrounding', '--disable-background-timer-throttling', '--disable-backgrounding-occluded-windows'] })
const page = await browser.open(server + '/')
await page.send('Emulation.setFocusEmulationEnabled', { enabled: true }) // focus styles need real focus
// Signed in as the page does it, so pictures come with the session's cookie.
await page.eval(`fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: ${JSON.stringify(JSON.stringify({ name: process.env.WMS_NAME, password: process.env.WMS_PASS }))} }).then((r) => r.json()).then((j) => localStorage.setItem('wms.token', j.token))`)
const SIZES = { phone: { w: 390, h: 844, mobile: true }, desktop: { w: 1440, h: 900 }, tv: { w: 1920, h: 1080 } }
const size = async ({ w, h, mobile }) => {
  await page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: !!mobile })
  await page.send('Emulation.setTouchEmulationEnabled', { enabled: !!mobile, maxTouchPoints: mobile ? 5 : 1 })
}
const open = async () => {
  await page.go(`${server}/library/${lib.id}?tab=map`)
  await page.until(`document.querySelectorAll('.gal-pin').length && document.querySelector('.gal-land use')`, 30000, 'the map')
  await wait(700) // the groups for the first view
  await page.until(`[...document.querySelectorAll('.gal-pin:not([aria-hidden]) img')].every((i) => i.complete)`, 30000, 'the pictures of the groups').catch(() => {})
}
const shot = async (name) => writeFileSync(join(out, `map-${name}.png`), await page.shot())
// What's on the map now: the pins in view, how close any two come, the land, and the list.
const STATE = `(() => {
  const m = document.querySelector('.gal-map'), r = m.getBoundingClientRect()
  const pins = [...m.querySelectorAll('.gal-pin')].map((b) => { const q = b.getBoundingClientRect(); return { x: q.left + q.width / 2 - r.left, y: q.top + q.height / 2 - r.top, id: b.querySelector('img').src, label: b.getAttribute('aria-label') } })
    .filter((p) => p.x > 0 && p.x < r.width && p.y > 0 && p.y < r.height)
  let near = Infinity
  for (let i = 0; i < pins.length; i++) for (let j = i + 1; j < pins.length; j++) near = Math.min(near, Math.hypot(pins[i].x - pins[j].x, pins[i].y - pins[j].y))
  const land = document.querySelector('.gal-land defs path'), pt = land.ownerSVGElement.createSVGPoint()
  const isLand = (lat, lon) => { pt.x = (lon + 180) / 360 * 1000; pt.y = (0.5 - Math.log(Math.tan(Math.PI / 4 + lat * Math.PI / 360)) / (2 * Math.PI)) * 1000; return land.isPointInFill(pt) }
  const use = getComputedStyle(document.querySelector('.gal-land use'))
  const q = (u) => new URL(u, location.href).searchParams
  const asks = performance.getEntriesByType('resource').map((e) => e.name).filter((n) => n.includes('/api/photos/map') && n.includes('bbox'))
  return {
    pins: pins.length, ids: pins.map((p) => p.id), at: Object.fromEntries(pins.map((p) => [p.id, [p.x, p.y]])), near, labels: pins.slice(0, 2).map((p) => p.label),
    land: ['48.85,2.35', '30,31.2', '-25,135', '38.5,-98', '-80,40', '66,-172'].every((s) => isLand(...s.split(',').map(Number))),
    sea: ['30,-40', '0,-150', '-20,75'].every((s) => !isLand(...s.split(',').map(Number))),
    landFill: use.fill, water: getComputedStyle(m).backgroundColor,
    places: document.querySelectorAll('.gal-map-list li').length, unique: new Set(pins.map((p) => p.id)).size,
    zoom: asks.length ? Number(q(asks[asks.length - 1]).get('zoom')) : -1, moving: m.classList.contains('moving'),
    sideways: document.documentElement.scrollWidth > innerWidth + 1, transform: document.querySelector('.gal-map-layer').style.transform,
  }
})()`
const state = () => page.eval(STATE)
const box = () => page.eval(`(() => { const r = document.querySelector('.gal-map').getBoundingClientRect(); return { x: r.left, y: r.top, w: r.width, h: r.height } })()`)
const KEYS = { ArrowUp: 38, ArrowDown: 40, ArrowLeft: 37, ArrowRight: 39, Enter: 13, Escape: 27, Tab: 9, '+': 187 }
const press = async (key) => {
  const windowsVirtualKeyCode = KEYS[key]
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key, code: key === '+' ? 'Equal' : key, windowsVirtualKeyCode, text: key === 'Enter' ? '\r' : key === '+' ? '+' : undefined })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key === '+' ? 'Equal' : key, windowsVirtualKeyCode })
}
const settle = () => wait(900)

// ---------------------------------------- every size, dark and light
for (const [name, s] of Object.entries(SIZES)) {
  await size(s)
  for (const theme of ['default', 'light']) {
    await page.eval(`localStorage.setItem('wms.theme', '${theme}')`)
    await open()
    const st = await state()
    const say = `${name} ${theme === 'default' ? 'dark' : 'light'}`
    expect(`${say}: groups drawn`, st.pins > 0, st)
    expect(`${say}: no two groups cover each other`, st.near >= 50, st.near)
    expect(`${say}: every continent is land, and the sea isn't`, st.land && st.sea)
    expect(`${say}: land and water look different`, st.landFill !== st.water, [st.landFill, st.water])
    expect(`${say}: the list has each place in view`, st.places === st.unique, [st.places, st.unique])
    expect(`${say}: nothing scrolls sideways`, !st.sideways)
    expect(`${say}: the groups' pictures load`, await page.eval(`[...document.querySelectorAll('.gal-pin:not([aria-hidden]) img')].every((i) => i.naturalWidth > 0)`))
    expect(`${say}: groups are named for a screen reader`, /^\d[\d,]* pictures?, \d+\.\d+° [NS], \d+\.?\d*° [EW]/.test(st.labels[0] || ''), st.labels)
    await shot(`${name}-${theme === 'default' ? 'dark' : 'light'}`)
  }
}
await page.eval(`localStorage.setItem('wms.theme', 'default')`)

// ---------------------------------------- the mouse
await size(SIZES.desktop)
await open()
let b = await box(), st = await state()
const mouse = (type, x, y, extra = {}) => page.send('Input.dispatchMouseEvent', { type, x, y, button: 'left', buttons: type === 'mouseReleased' ? 0 : 1, clickCount: 1, ...extra })
{
  const id = st.ids[0], [x0, y0] = st.at[id]
  // Start beside the pin, not on it, and drag right.
  const sx = b.x + b.w / 2 + 31, sy = b.y + b.h / 2 + 17
  await mouse('mousePressed', sx, sy)
  for (let i = 1; i <= 10; i++) await mouse('mouseMoved', sx + i * 20, sy)
  const during = (await state()).transform
  await mouse('mouseReleased', sx + 200, sy)
  await settle()
  st = await state()
  expect('a drag moves the layer, not every pin, while it runs', /translate/.test(during), during)
  const moved = st.at[id] ? st.at[id][0] - x0 : null
  expect('a drag moves the map with the mouse', moved !== null && Math.abs(moved - 200) < 4 && Math.abs(st.at[id][1] - y0) < 4, moved)
}
{
  const before = (await state()).zoom
  await page.send('Input.dispatchMouseEvent', { type: 'mouseWheel', x: b.x + b.w / 3, y: b.y + b.h / 3, deltaX: 0, deltaY: -400 })
  await settle()
  st = await state()
  expect('the wheel zooms in', st.zoom > before, [before, st.zoom])
  expect('the page didn\'t scroll with it', await page.eval('scrollY') === 0)
}

// ---------------------------------------- a finger, and two
await size(SIZES.phone)
await open()
b = await box()
{
  const before = await state()
  const touch = (type, points) => page.send('Input.dispatchTouchEvent', { type, touchPoints: points.map(([x, y], id) => ({ x, y, id })) })
  const cx = b.x + b.w / 2, cy = b.y + b.h / 2
  await touch('touchStart', [[cx, cy]])
  for (let i = 1; i <= 8; i++) await touch('touchMove', [[cx + i * 10, cy]]) // sideways: zoomed right out on a phone, the world is held in the middle
  await touch('touchEnd', [])
  await settle()
  const after = await state()
  const shared = before.ids.find((i) => after.at[i])
  expect('a finger drags the map', shared && Math.abs(after.at[shared][0] - before.at[shared][0] - 80) < 6, shared && [before.at[shared], after.at[shared]])
  await touch('touchStart', [[cx - 40, cy], [cx + 40, cy]])
  for (let i = 1; i <= 8; i++) await touch('touchMove', [[cx - 40 - i * 15, cy], [cx + 40 + i * 15, cy]])
  await touch('touchEnd', [])
  await settle()
  const pinched = await state()
  expect('two fingers zoom in', pinched.zoom > after.zoom, [after.zoom, pinched.zoom])
  await shot('phone-pinched')
}

// ---------------------------------------- keyboard and remote
await size(SIZES.tv)
await open()
b = await box()
{
  await page.eval(`document.querySelector('.gal-map').focus()`)
  await press('Enter')
  st = await state()
  expect('Enter on the map lets the arrows move it', st.moving)
  const id = st.ids[0], x0 = st.at[id][0]
  await press('ArrowRight')
  await settle()
  st = await state()
  expect('an arrow moves the map a quarter of its width', st.at[id] && Math.abs(x0 - st.at[id][0] - b.w / 4) < 4, st.at[id] && [x0, st.at[id][0], b.w / 4])
  const z = st.zoom
  await press('+')
  await settle()
  expect('+ zooms in', (await state()).zoom > z)
  await press('Escape')
  expect('Escape lets the arrows go again', !(await state()).moving)
  await shot('tv-keyboard')
  await press('Tab')
  const first = await page.eval(`document.activeElement.className + '|' + document.activeElement.getAttribute('aria-label')`)
  expect('Tab goes to the first group', first.startsWith('gal-pin|'), first)
  await press('ArrowRight')
  const next = await page.eval(`document.activeElement.className + '|' + document.activeElement.getAttribute('aria-label')`)
  expect('a remote\'s arrow goes on to another group or the list', next !== first && /gal-pin|^\|/.test(next), next)
  await shot('tv-remote-focus')
  await press('Enter')
  await settle()
  const opened = await page.eval(`({ area: document.querySelectorAll('.gal-map-area .gal-tile').length, focus: document.activeElement.tagName })`)
  st = await state()
  expect('Enter on a group lists its pictures or zooms in on it', opened.area > 0 || st.zoom > z, opened)
}
{
  await open()
  await page.eval(`document.querySelector('.gal-map-list li button').click()`)
  await page.until(`document.querySelectorAll('.gal-map-area .gal-tile').length`, 15000, 'the list\'s pictures')
  const head = await page.eval(`document.activeElement.textContent`)
  expect('choosing a place in the list shows its pictures, and says how many', /^\d[\d,]* pictures?/.test(head), head)
  await shot('tv-place-pictures')
}

// ---------------------------------------- an online map that doesn't answer
{
  const was = (await call('GET', '/settings', null, token)).data
  await call('PUT', '/settings', { mapOnline: '1', mapTiles: 'https://tiles.invalid/{z}/{x}/{y}.png', mapAttribution: 'Test credit' }, token)
  try {
    await size(SIZES.desktop)
    await open()
    const note = await page.eval(`document.querySelector('.gal-map-note')?.textContent || ''`)
    expect('an online map says so under the map', /tiles\.invalid/.test(note) || /isn.t answering/.test(note), note)
    await page.until(`/isn.t answering/.test(document.querySelector('.gal-map-note')?.textContent || '')`, 20000, 'the outline coming back')
    st = await state()
    expect('the outline comes back when it doesn\'t answer', st.land && st.pins > 0)
    expect('this device can choose the outline', await page.eval(`!!document.querySelector('.gal-map-ctl [aria-pressed]')`))
    await shot('desktop-online-unanswered')
  } finally {
    await call('PUT', '/settings', { mapOnline: was.mapOnline || '0', mapTiles: was.mapTiles || '', mapAttribution: was.mapAttribution || '' }, token)
  }
}

const errors = page.logs.filter((l) => l.startsWith('exception'))
expect('no errors in the page', !errors.length, errors)
await browser.close()
console.log(failed ? `${failed} failed` : 'all passed')
process.exit(failed ? 1 : 0)
