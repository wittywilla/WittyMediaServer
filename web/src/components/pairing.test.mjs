// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { approveLink, browserName, clock, onlyThisDevice, pinGroups, serverClockOffset, tidyCode, typedAddress } from './pairing.ts'

test('a browser is named as the server names it in the list of devices', () => {
  const names = [
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36', 'Chrome on macOS'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0', 'Edge on Windows'],
    ['Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0', 'Firefox on Linux'],
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1', 'Safari on iOS'],
    ['Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36', 'Chrome on Android'],
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15', 'Safari on macOS'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 OPR/115.0.0.0', 'Opera on Windows'],
    ['SomethingElse/1.0', 'Web browser'],
    ['', 'Web browser'],
  ]
  for (const [ua, want] of names) assert.equal(browserName(ua), want)
  // It fits what the server keeps of a device's name.
  for (const [ua] of names) assert.ok(browserName(ua).length <= 60)
})

test('the QR code holds a whole address a phone can open', () => {
  assert.equal(approveLink('https://media.example.org/pair?code=MQRK-XLGN', 'http://192.168.1.5:9797'), 'https://media.example.org/pair?code=MQRK-XLGN')
  assert.equal(approveLink('/pair?code=MQRK-XLGN', 'http://192.168.1.5:9797'), 'http://192.168.1.5:9797/pair?code=MQRK-XLGN')
  assert.equal(approveLink('/pair?code=MQRK-XLGN', 'http://192.168.1.5:9797/'), 'http://192.168.1.5:9797/pair?code=MQRK-XLGN')
  assert.equal(approveLink('/pair?code=MQRK-XLGN', 'file://'), '')
  assert.equal(approveLink('', 'http://192.168.1.5:9797'), '')
  assert.equal(approveLink('javascript:alert(1)', 'http://192.168.1.5:9797'), '')
})

test('the address to type has no code in it', () => {
  assert.equal(typedAddress('http://192.168.1.5:9797/pair?code=MQRK-XLGN'), '192.168.1.5:9797/pair')
  assert.equal(typedAddress('https://media.example.org/pair?code=MQRK-XLGN'), 'https://media.example.org/pair')
  assert.equal(typedAddress('nonsense'), '')
})

test('an address that means this very machine is no use to a phone', () => {
  for (const a of ['http://localhost:9797/pair', 'http://127.0.0.1:9797/pair', 'http://[::1]:9797/pair', 'http://app.localhost/pair']) assert.equal(onlyThisDevice(a), true, a)
  for (const a of ['http://192.168.1.5:9797/pair', 'https://media.example.org/pair', 'http://localhost.example.org/pair', 'nonsense']) assert.equal(onlyThisDevice(a), false, a)
})

test('time and the PIN are written the way people read them', () => {
  assert.equal(clock(299.2), '5:00')
  assert.equal(clock(61), '1:01')
  assert.equal(clock(0.4), '0:01')
  assert.equal(clock(0), '0:00')
  assert.equal(clock(-5), '0:00')
  assert.equal(pinGroups('309978'), '309 978')
  assert.equal(pinGroups('12345678'), '1234 5678')
  assert.equal(pinGroups('1234'), '1234')
  assert.equal(pinGroups(''), '')
})

test('a typed code is tidied only when it is plainly the device’s kind', () => {
  assert.equal(tidyCode('mqrk xlgn'), 'MQRK-XLGN')
  assert.equal(tidyCode(' mqrkxlgn '), 'MQRK-XLGN')
  assert.equal(tidyCode('MQRK-XLGN'), 'MQRK-XLGN')
  assert.equal(tidyCode('mqrk-xl'), 'MQRK-XL')
  assert.equal(tidyCode('123 456'), '123 456')
})

test('countdowns go by the server’s clock, whatever this device’s says', () => {
  // The server says "expires at 1300, which is 300 seconds from now": its clock reads 1000.
  assert.equal(serverClockOffset(1300, 300, 1000), 0)
  assert.equal(serverClockOffset(1300, 300, 400), 600)   // this device is ten minutes slow
  assert.equal(serverClockOffset(1300, 300, 1090), -90)  // and here a minute and a half fast
  assert.equal(serverClockOffset(1300, undefined, 400), 0)
  assert.equal(serverClockOffset(1300, 0, 400), 0)
  assert.equal(serverClockOffset(NaN, 300, 400), 0)
})
