// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Connect a device (Settings, Remote Access; From away), in a headless browser against a real test server:
// what it says first, the code and the QR code, More options, the administrator's password, the code
// really signing a device in (the device's half done here as the apps do it, with the server proving
// first), the link the QR code holds being spent with it, and turning connections from away on. Pictures
// of each step go to the out folder. No window opens.
//
// The server needs an administrator account and nothing else: a fresh data folder, set up through
// POST /api/setup from its own machine.
//
//   WMS_SERVER=http://connect.localhost:9831 WMS_USER=owner WMS_PASSWORD=... node scripts/test-connect.mjs out-folder
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { createHmac, randomBytes, scryptSync } from 'node:crypto'
const { launch } = await import('./cdp.mjs')

const server = process.env.WMS_SERVER || 'http://connect.localhost:9831'
const user = process.env.WMS_USER || 'owner'
const password = process.env.WMS_PASSWORD
const [out = '.'] = process.argv.slice(2)
if (!password) { console.error('Set WMS_PASSWORD.'); process.exit(2) }
mkdirSync(out, { recursive: true })
const results = []
const expect = (what, ok, detail = '') => { results.push(`${ok ? 'ok  ' : 'FAIL'} ${what}${ok ? '' : ' ' + JSON.stringify(detail)}`); if (!ok) process.exitCode = 1 }

const browser = await launch({ software: true, width: 1280, height: 900 })
const page = await browser.open(server + '/login')
const shot = async (name) => writeFileSync(join(out, `connect-${name}.png`), await page.shot())
const text = () => page.eval('document.body.innerText')
const click = (words) => page.eval(`(() => { const b = Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === ${JSON.stringify(words)}); if (!b) return false; b.click(); return true })()`)

// Signed in as the administrator, the way the page itself keeps it.
const login = await page.eval(`fetch('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: ${JSON.stringify(user)}, password: ${JSON.stringify(password)} }) }).then((r) => r.json())`)
expect('signed in', !!login.token, login)
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
const api = (method, path, body) => page.eval(`(async () => { const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + localStorage.getItem('wms.token') }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return { status: r.status, body: await r.json().catch(() => null) } })()`)

// 1. Peer to peer off: the screen says only home, and offers to change that.
await api('PUT', '/p2p/settings', { enabled: false })
await page.go(server + '/settings/remote')
await page.until(`document.querySelector('.cd h2') && document.querySelector('.cd-who li:nth-child(2)')`, 20000, 'Connect a device')
let words = await text()
expect('it says devices at home can connect', words.includes('Devices on your home network can connect.'))
expect('it says devices away can’t, yet', words.includes('Devices away from home can’t connect yet.'))
expect('it offers to let them', words.includes('Let devices connect from away…'))
expect('the technical details are folded away', await page.eval(`!document.querySelector('details.rem-tech').open`))
expect('no jargon on the main screen', !/peer|DHT|relay|NAT|hole punch|multiaddr|12D3KooW/i.test(await page.eval(`document.querySelector('.cd').innerText`)), await page.eval(`document.querySelector('.cd').innerText`))
await shot('1-home-only')

// 2. A code, for home: the administrator's password first.
await click('Show a code')
await page.until(`document.querySelector('[role=alertdialog], [role=dialog]') && document.querySelector('input[type=password]')`, 10000, 'the password dialog')
await shot('2-password')
await page.eval(`(() => { const i = document.querySelector('input[type=password]'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(i, ${JSON.stringify(password)}); i.dispatchEvent(new Event('input', { bubbles: true })) })()`)
await click('Show the code')
await page.until(`document.querySelector('.cd-code')`, 15000, 'the code')
const code = await page.eval(`document.querySelector('.cd-code').textContent`)
expect('a code of eight', /^[0-9A-Z]{4}-[0-9A-Z]{4}$/.test(code), code)
expect('no QR code without connections from away', !(await page.eval(`!!document.querySelector('.cd-qr svg')`)))
expect('it says the code is for home', (await text()).includes('This code works on your home network.'))
expect('a countdown', /Works once, for 1[45]:\d\d\./.test(await text()))
await shot('3-code-home')

// 3. The code signs a device in, as the apps do it: the server proves it made the code first.
const clean = code.replace('-', '')
const k = scryptSync(Buffer.from(clean), Buffer.from('wittymedia-web-join-v1'), 64, { N: 1 << 15, r: 8, p: 1, maxmem: 64 * 1024 * 1024 })
const find = k.subarray(0, 32).toString('hex'), proofKey = k.subarray(32)
const proof = (role, dn, sn, id) => { const m = createHmac('sha256', proofKey); for (const p of [Buffer.from('wittymedia web join 1'), Buffer.from(role), dn, sn, Buffer.from(id)]) { m.update(Buffer.from([p.length >> 8, p.length & 255])); m.update(p) } return m.digest() }
const dn = randomBytes(16)
// From this script, as another device would: Node has no .localhost names, so by the address itself.
const direct = server.replace(/\/\/[^/:]+/, '//127.0.0.1')
const post = (path, body) => fetch(direct + '/api' + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }).then(async (r) => ({ status: r.status, body: await r.json().catch(() => null) }))
const hello = await post('/connect/hello', { v: 1, find, nonce: dn.toString('base64') })
const sn = Buffer.from(hello.body?.nonce || '', 'base64')
expect('the server proves it made the code', hello.status === 200 && proof('server', dn, sn, hello.body.publicKey).toString('base64') === hello.body.proof, hello)
const answer = await post('/connect/answer', { v: 1, find, nonce: dn.toString('base64'), serverNonce: hello.body.nonce, proof: proof('device', dn, sn, hello.body.publicKey).toString('base64'), device: 'Test laptop', platform: 'Linux' })
expect('and the device is signed in', answer.status === 200 && !!answer.body.token && answer.body.user?.name === user, answer)
expect('once', (await post('/connect/hello', { v: 1, find, nonce: dn.toString('base64') })).status === 404)
await click('Done')

// 4. Connections from away: what is contacted is said first, and agreed to.
await page.until(`Array.from(document.querySelectorAll('button')).some((b) => b.textContent.trim() === 'Let devices connect from away…')`, 10000, 'the offer to turn it on')
await click('Let devices connect from away…')
await page.until(`document.querySelector('.cd-parties li')`, 10000, 'who is contacted')
expect('who is contacted is listed before', (await page.eval(`document.querySelectorAll('.cd-parties li').length`)) >= 3)
expect('turning it on waits for the box to be ticked', await page.eval(`Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === 'Turn it on').disabled`))
await shot('4-turn-on')
await page.eval(`document.querySelector('.ui-dialog input[type=checkbox], [role=dialog] input[type=checkbox]').click()`)
await click('Turn it on')
await page.until(`document.querySelector('.cd-who li:nth-child(2)') && !document.querySelector('.cd-who li:nth-child(2)').textContent.includes('can’t connect yet')`, 20000, 'the status to change')
await shot('5-away-on')

// 5. A code with its QR code: the link under More options, which is the QR code's.
await click('Show a code')
await page.until(`document.querySelector('input[type=password]') || document.querySelector('.cd-code')`, 10000, 'the password or the code')
if (await page.eval(`!!document.querySelector('input[type=password]')`)) {
  await page.eval(`(() => { const i = document.querySelector('input[type=password]'); Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(i, ${JSON.stringify(password)}); i.dispatchEvent(new Event('input', { bubbles: true })) })()`)
  await click('Show the code')
}
await page.until(`document.querySelector('.cd-code') && document.querySelector('.cd-qr svg')`, 15000, 'the code and its QR code')
const code2 = await page.eval(`document.querySelector('.cd-code').textContent`)
expect('a new code', code2 !== code && /^[0-9A-Z]{4}-[0-9A-Z]{4}$/.test(code2), code2)
expect('a QR code beside it', await page.eval(`document.querySelector('.cd-qr svg').getAttribute('aria-label') === 'QR code to connect a device'`))
expect('the steps say which way where', /On your home network: choose Enter a code[\s\S]*Away from home: scan the QR code/.test(await text()))
await shot('6-code-and-qr')
await page.eval(`document.querySelector('details.cd-more').open = true`)
const link = await page.eval(`(document.querySelector('.cd-link') || {}).textContent || ''`)
expect('More options has the link', /^wittymedia:\/\/p2p\/12D3KooW[1-9A-HJ-NP-Za-km-z]+\?/.test(link), link)
await shot('7-more-options')
// The page that's been made smaller: the code stays readable and nothing runs off the side.
await page.send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true })
await new Promise((r) => setTimeout(r, 400))
expect('nothing runs off the side on a phone', await page.eval(`document.documentElement.scrollWidth <= innerWidth + 1`))
await shot('8-phone')
await page.send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false })

// 6. The From away page is the same screen.
await page.go(server + '/away')
// The page's title says Connect a device, so the card under it doesn't say it again (#678).
await page.until(`document.querySelector('.cd .cd-who')`, 15000, 'Connect a device on the From away page')
expect('From away is the same screen', (await text()).includes('Show a code'))
expect('From away says Connect a device once', (await page.eval(`document.querySelector('main').innerText`)).split('Connect a device').length - 1 === 1, await page.eval(`document.querySelector('main').innerText`))
await shot('9-away-page')
await api('DELETE', '/connect/offer')

console.log(results.join('\n'))
await browser.close()
