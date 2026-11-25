// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "A pointer only points", in a real browser on every games tab (ui/pointerChoose.ts):
//   1. the mouse crosses every card on screen: nothing may be chosen by that;
//   2. a card that isn't the chosen one is clicked: it's chosen, and nothing opens;
//   3. it's clicked again: it opens;
//   4. from the tab again, the focus is put on a card (as a remote's or a pad's arrows do) and Enter is pressed: it opens.
// Recent lost this once (#558 undid #547). Runs against a running server with WittyPlay and at least two games played:
//
//   WMS_SERVER=http://127.0.0.1:9811 WMS_NAME=... WMS_PASS=... node scripts/test-pointer-choose.mjs
const { launch } = await import('./cdp.mjs')
const server = process.env.WMS_SERVER || 'http://127.0.0.1:9811'
const wait = (ms) => new Promise((d) => setTimeout(d, ms))

// Each tab: where it is, and its cards.
const TABS = [
  { name: 'Recent', path: '/page/wittyplay-online', cards: '.sdui-item:has(a)' },
  { name: 'Consoles', path: '/wittyplay/consoles', cards: '.wplay-console' },
  { name: 'All games', path: '/wittyplay/library', cards: '.wpl-game' },
]
// Which card is chosen, by its place among the tab's cards (two games can have the same title): the one marked current,
// pressed or grown. -1 when none is marked yet.
const chosenIn = (sel) => `[...document.querySelectorAll(${JSON.stringify(sel)})].findIndex((c) =>
  c.getAttribute('aria-current') === 'true' || c.getAttribute('aria-pressed') === 'true' || c.classList.contains('grown'))`

let failed = 0
const ok = (m) => console.log('ok:   ' + m)
const bad = (m) => { console.log('FAIL: ' + m); failed++ }

const login = await (await fetch(server + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_NAME, password: process.env.WMS_PASS }) })).json()
if (!login.token) { console.log('FAIL: sign-in'); process.exit(1) }
const browser = await launch({ width: 1600, height: 900, software: true })
const page = await browser.open()
await page.go(server + '/')
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)

const mouse = (type, x, y, extra = {}) => page.send('Input.dispatchMouseEvent', { type, x, y, button: 'none', ...extra })
const click = async (x, y) => {
  await mouse('mouseMoved', x, y)
  await mouse('mousePressed', x, y, { button: 'left', clickCount: 1 })
  await mouse('mouseReleased', x, y, { button: 'left', clickCount: 1 })
}
// The centres of the cards on screen, by their place among the tab's cards.
const centres = (sel) => page.eval(`[...document.querySelectorAll(${JSON.stringify(sel)})].map((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.y + r.height / 2 < innerHeight ? [Math.round(r.x + r.width / 2), Math.round(r.y + r.height / 2)] : null })`)
const where = () => page.eval('location.pathname + location.search')
const open = async (tab) => {
  await page.go(server + tab.path)
  await page.until(`document.querySelectorAll(${JSON.stringify(tab.cards)}).length >= 2`, 20000)
  await wait(600)
}

for (const tab of TABS) {
  try { await open(tab) } catch { bad(`${tab.name}: fewer than two cards to try`); continue }
  const chosen = () => page.eval(chosenIn(tab.cards))
  let at = await centres(tab.cards)
  const before = await chosen()

  // 1. The mouse crosses every card on screen, and comes to rest on the last.
  const seen = at.filter(Boolean)
  for (const [x, y] of seen) { await mouse('mouseMoved', x, y); await wait(40) }
  await wait(300)
  const after = await chosen()
  if (after === before) ok(`${tab.name}: the mouse crossed ${seen.length} cards and chose nothing (card ${before} stays chosen)`)
  else bad(`${tab.name}: moving the mouse chose card ${after} (was ${before})`)

  // 2. A click on a card that isn't the chosen one chooses it, and opens nothing.
  const target = at.findIndex((c, i) => c && i !== before)
  const start = await where()
  await click(...at[target])
  await wait(400)
  const now = await chosen()
  if ((await where()) !== start) { bad(`${tab.name}: a first click opened ${await where()}`); continue }
  if (now === target) ok(`${tab.name}: a click chose card ${target} and opened nothing`)
  else bad(`${tab.name}: a click on card ${target} left card ${now} chosen`)

  // 3. A second click on the same card opens it. The page may have moved a little (the chosen card can grow).
  at = await centres(tab.cards)
  await click(...at[target])
  await wait(900)
  if ((await where()) !== start) ok(`${tab.name}: a second click opened ${await where()}`)
  else bad(`${tab.name}: a second click on the chosen card opened nothing`)

  // 4. A remote or a pad: the arrows put the focus on a card (here, straight on the second), and Enter opens it.
  await open(tab)
  await page.eval(`(() => { const c = document.querySelectorAll(${JSON.stringify(tab.cards)})[1]; (c.matches('a, button') ? c : c.querySelector('a, button')).focus() })()`)
  await wait(300)
  const focusedChosen = await chosen()
  await page.key('Enter')
  await wait(900)
  if ((await where()) !== tab.path) ok(`${tab.name}: the focus chose card ${focusedChosen}, and Enter opened ${await where()}`)
  else bad(`${tab.name}: Enter on the focused card opened nothing`)
}
browser.close()
console.log(failed ? `${failed} failed` : 'all passed')
process.exit(failed ? 1 : 0)
