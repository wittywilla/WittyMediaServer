// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// TV levels (#469): the two kinds of client in TV mode, at 1920x1080 with keys.
//   a TV device (?client=tv, as Android TV and Plasma Bigscreen send): the card with the address and a QR code, Back focused
//   a computer in TV mode (a browser, the desktop app with --tv): "Leave TV mode to change this.", Leave TV mode focused,
//     which opens the same page in the normal layout with a way back.
//   WMS_SERVER=http://tv.localhost:19527 WMS_TOKEN_FILE=<a file holding a sign-in token> node scripts/test-tv-levels.mjs <out folder>
// It needs a library or two on the server, and changes nothing there.
import { readFileSync, writeFileSync } from 'node:fs'
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const token = readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim()
const [out] = process.argv.slice(2)
const results = []
const check = (name, ok, detail) => { results.push({ name, ok: !!ok, detail }); console.log(ok ? 'PASS' : 'FAIL', name, detail ?? '') }
const focus = (page) => page.eval(`(() => { const a = document.activeElement; return a && a !== document.body ? a.textContent.trim().slice(0, 40) : 'nothing' })()`)
const key = async (page, k, code = 0) => {
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code: k, windowsVirtualKeyCode: code, ...(k === 'Enter' ? { text: String.fromCharCode(13) } : {}) })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code: k })
  await wait(500)
}

async function run(kind) {
  const s = await open(server, out, { path: '/legal' })
  const { page, shot, go } = s
  try {
    await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
    await go(`/settings/libraries?tv=1${kind === 'tv' ? '&client=tv' : ''}`, 4000)
    await page.until(`!!document.querySelector('.tvl-card')`, 30000, 'the card').catch(() => {})
    await wait(1500)
    await shot(`${kind}-settings-libraries`)
    const qr = await page.eval(`!!document.querySelector('.tvl-qr')`)
    const note = await page.eval(`document.querySelector('.tvl-note')?.textContent || ''`)
    const lines = await page.eval(`[...document.querySelectorAll('.tvl-now li')].map((l) => l.textContent)`)
    if (kind === 'tv') {
      check('tv: QR code and address', qr && (await page.eval(`!!document.querySelector('.tvl-addr')`)))
      check('tv: names the WittyMediaClient app', note.includes('WittyMediaClient'), note)
      check('tv: Back has focus', (await focus(page)) === 'Back', await focus(page))
      check('tv: what is set now', lines.length > 0, lines)
      await key(page, 'Escape', 27)
      check('tv: Back leaves the screen', (await page.eval('location.pathname')) !== '/settings/libraries', await page.eval('location.pathname'))
    } else {
      check('computer: no QR code', !qr)
      check('computer: says to leave TV mode', note.includes('Leave TV mode to change this.'), note)
      check('computer: Leave TV mode has focus', (await focus(page)) === 'Leave TV mode', await focus(page))
      check('computer: what is set now', lines.length > 0, lines)
      await key(page, 'Enter', 13)
      await wait(2500)
      const tvOff = await page.eval(`!document.documentElement.classList.contains('tv-mode')`)
      check('computer: Leave TV mode turns TV mode off', tvOff)
      check('computer: the same page', (await page.eval('location.pathname')) === '/settings/libraries', await page.eval('location.pathname'))
      check('computer: the editor itself shows', !(await page.eval(`!!document.querySelector('.tvl-card')`)) && /Add library/.test(await page.eval('document.body.innerText')))
      check('computer: the way back is offered', await page.eval(`!!document.querySelector('.tv-return-bar')`))
      await shot(`${kind}-after-leaving`)
      await page.eval(`[...document.querySelectorAll('.tv-return-bar button')].find((b) => /Back to TV mode/.test(b.textContent))?.click()`)
      await wait(2500)
      check('computer: Back to TV mode', await page.eval(`document.documentElement.classList.contains('tv-mode') && !!document.querySelector('.tvl-card')`))
    }
  } catch (e) {
    check(kind + ': ran', false, String(e && e.message || e))
  } finally {
    await s.browser.close()
  }
}

await run('tv')
await run('computer')
writeFileSync(out + '/clients-report.json', JSON.stringify(results, null, 1))
process.exit(results.every((r) => r.ok) ? 0 : 1)
