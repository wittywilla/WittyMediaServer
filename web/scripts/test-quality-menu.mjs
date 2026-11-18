// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The player's quality menu, in a headless browser on a running test server with a video. No
// window opens. It plays the video, opens the menu from the quality chip and fails:
// - a menu that isn't a labelled dialog with a radio group, or with no option or more than one
//   marked as in use, or with a size above the video's own;
// - options and notes whose text doesn't reach WCAG AA contrast on the menu's surface;
// - ↓ and Enter that don't move to and pick the next option, or a choice that isn't asked of
//   the server, or that loses the place in the video;
// - an Advanced page whose controls have no names, or "Show stream info" that shows nothing;
// - Esc that doesn't close it.
// It saves pictures of both pages at desktop and phone size.
//
//   WMS_SERVER=http://transcode.localhost:19511 WMS_TOKEN=... WMS_ITEM=1 node scripts/test-quality-menu.mjs out-folder
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import('./cdp.mjs')

const server = process.env.WMS_SERVER || 'http://transcode.localhost:19511'
const token = process.env.WMS_TOKEN || ''
const item = process.env.WMS_ITEM || '1'
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const faults = []
const check = (ok, what) => { console.log((ok ? 'ok   ' : 'FAIL ') + what); if (!ok) faults.push(what) }

const browser = await launch({ software: true, width: 1440, height: 900 })
const page = await browser.open(server + '/')
await page.send('Emulation.setFocusEmulationEnabled', { enabled: true })
const size = (w, h, mobile = false) => page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile })
const shot = async (name) => { writeFileSync(join(out, name), await page.shot()); console.log('     picture ' + name) }

await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.quality'); true`)
// Every /playback request's body, to see what the menu asked for.
const recordAsks = `(() => { if (window.__asks) return true; window.__asks = []; const f = window.fetch; window.fetch = (u, o) => { if (String(u).includes('/playback') && o?.body) window.__asks.push(JSON.parse(o.body)); return f(u, o) }; return true })()`

async function openPlayer() {
  await page.go(server + '/play/' + item)
  await page.eval(recordAsks)
  await page.until(`document.querySelector('.chip-q[aria-haspopup="dialog"]')`, 60000, 'the quality chip')
  await page.until(`(() => { const v = document.querySelector('.player video'); return v && v.readyState >= 2 })()`, 90000, 'the picture')
}
const openMenu = async () => {
  await page.eval(`document.querySelector('.qmenu') || document.querySelector('.chip-q').click(), true`)
  await page.until(`document.querySelector('.qmenu')`, 5000, 'the menu')
  await wait(400)
}

await openPlayer()
await openMenu()
const m = await page.eval(`(() => {
  const d = document.querySelector('.qmenu'), label = document.getElementById(d.getAttribute('aria-labelledby'))?.textContent
  const radios = [...d.querySelectorAll('.qm-list [role=radio]')]
  return { role: d.getAttribute('role'), label, group: !!d.querySelector('[role=radiogroup][aria-labelledby]'), names: radios.map((r) => r.querySelector('.qm-label').textContent),
    checked: radios.filter((r) => r.getAttribute('aria-checked') === 'true').length, focused: document.activeElement?.getAttribute('aria-checked'),
    described: radios.every((r) => document.getElementById(r.getAttribute('aria-describedby'))?.textContent) }
})()`)
check(m.role === 'dialog' && m.label === 'Quality' && m.group, 'a dialog named Quality, with a radio group')
check(m.checked === 1, `one option marked as in use (${m.checked})`)
check(m.focused === 'true', 'focus starts on the option in use')
check(m.described, 'every option has its note read with it')
check(m.names[0].startsWith('Auto') && m.names[1] === 'Original', `Auto and Original first (${m.names.join(', ')})`)
const src = await page.eval(`(() => { const v = document.querySelector('.player video'); return Math.max(v.videoHeight, Math.round(v.videoWidth * 9 / 16)) })()`)
check(!m.names.includes('4K') || src >= 2000, `no size above the video's own ${src} lines`)

// Contrast of each kind of text on the menu's own surface.
const contrast = await page.eval(`(() => {
  const rgb = (s) => s.match(/[\\d.]+/g).slice(0, 3).map(Number)
  const lum = ([r, g, b]) => { const f = (c) => { c /= 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
  const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05) }
  const bgOf = (el) => { for (let e = el; e; e = e.parentElement) { const c = getComputedStyle(e).backgroundColor; if (!/rgba\\(.*, 0\\)$/.test(c) && c !== 'transparent') return rgb(c) } return [0, 0, 0] }
  return ['.qm-label', '.qm-note', '.qm-head h4', '.qm-small', '.qm-more', '.qm-item.active .qm-label'].map((s) => { const el = document.querySelector('.qmenu ' + s); return el ? [s, Math.round(ratio(rgb(getComputedStyle(el).color), bgOf(el)) * 10) / 10] : [s, 0] })
})()`)
for (const [s, r] of contrast) check(r >= 4.5, `${s} contrast ${r}:1`)
await shot('quality-menu-desktop.png')

// ↓ then Enter picks the next option, asks the server for it, and keeps the place.
// The place in the film, as the controls show it (a rewrapped stream's own clock starts where it started).
const filmTime = `(() => { const t = document.querySelector('.player .time')?.textContent.split('/')[0].trim().split(':').map(Number) || [0]; return t.reduce((a, n) => a * 60 + n, 0) })()`
const before = await page.eval(filmTime)
const asked = await page.eval('window.__asks.length')
await page.eval(`(() => { const r = [...document.querySelectorAll('.qmenu .qm-list [role=radio]')]; r[r.length - 1].focus(); return true })()`)
await page.key('ArrowUp')
const target = await page.eval(`document.activeElement?.querySelector('.qm-label')?.textContent`)
check(!!target && target !== 'Auto' && target !== 'Original', `↑ moves to another option (${target})`)
// Enter as a keyboard sends it, with its text, so the button activates as it would for a person.
await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, text: String.fromCharCode(13) })
await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 })
await page.until(`window.__asks.length > ${asked}`, 10000, 'a new request')
const body = await page.eval('window.__asks[window.__asks.length - 1]')
check(/^h\d+-b\d+$/.test(body.quality || ''), `the server is asked for ${target} (quality: ${body.quality})`)
check(Math.abs((body.startSec || 0) - before) < 15, `at the same place (${Math.round(before)} s → ${Math.round(body.startSec)} s)`)
check(!(await page.eval(`!!document.querySelector('.qmenu')`)), 'the menu closes after choosing')
await page.until(`(() => { const v = document.querySelector('.player video'); return v && v.readyState >= 2 })()`, 90000, 'the picture again')
const h = await page.eval(`document.querySelector('.player video').videoHeight`)
check(h > 0 && h <= Number(body.quality.match(/h(\d+)/)[1]), `the picture is now ${h} lines`)
await openMenu()
check(await page.eval(`document.querySelector('.qmenu [aria-checked=true] .qm-label')?.textContent === ${JSON.stringify(target)}`), `${target} is marked as in use`)
check(await page.eval(`document.activeElement?.getAttribute('aria-checked') === 'true'`), 'reopened, focus is on the option in use')

// Advanced.
await page.eval(`document.querySelector('.qmenu .qm-more').click(), true`)
await page.until(`document.querySelector('.qm-adv')`, 5000, 'the advanced page')
const unnamed = await page.eval(`[...document.querySelectorAll('.qm-adv select, .qm-adv input, .qm-adv button')].filter((e) => !(e.labels?.length || e.getAttribute('aria-label') || e.getAttribute('aria-labelledby') || e.textContent.trim())).length`)
check(unnamed === 0, `every advanced control has a name (${unnamed} without)`)
const colours = await page.eval(`[...document.querySelectorAll('.qm-adv .seg button.active')].map((b) => { const c = getComputedStyle(b); return c.color !== c.backgroundColor })`)
check(colours.length > 0 && colours.every(Boolean), 'the chosen segment label differs from its fill')
const sw = await page.eval(`(() => { const s = document.querySelector('.qm-adv [role=switch]'); if (s.getAttribute('aria-checked') !== 'true') s.click(); return true })()`)
await page.until(`[...document.querySelectorAll('.stream-info div')].some((d) => /^(GPU|CPU): /.test(d.textContent))`, 8000, 'stream information')
const lines = await page.eval(`[...document.querySelectorAll('.stream-info div')].map((d) => d.textContent)`)
check(sw && lines.some((l) => /×/.test(l)) && lines.some((l) => /Converting|Direct/.test(l)) && lines.some((l) => /^(GPU|CPU): /.test(l)), `stream information, with GPU or CPU: ${lines.join(' | ')}`)
await wait(300)
await shot('quality-advanced-desktop.png')
await page.key('Escape')
check(await page.eval(`!!document.querySelector('.qm-list')`), 'Esc goes back from Advanced')
await page.key('Escape')
check(!(await page.eval(`!!document.querySelector('.qmenu')`)), 'Esc closes the menu')

// A phone.
await size(390, 844, true)
await wait(800)
await openMenu()
const fits = await page.eval(`(() => { const r = document.querySelector('.qmenu').getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth && r.bottom <= innerHeight })()`)
check(fits, 'on a phone the menu fits the screen')
await shot('quality-menu-phone.png')
await page.eval(`document.querySelector('.qmenu .qm-more').click(), true`)
await page.until(`document.querySelector('.qm-adv')`, 5000, 'the advanced page')
await wait(300)
await shot('quality-advanced-phone.png')

browser.close()
console.log(faults.length ? `\n${faults.length} fault(s)` : '\nno faults')
process.exit(faults.length ? 1 : 0)
