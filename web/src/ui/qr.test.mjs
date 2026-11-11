// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { qrMatrix, qrPath, QR_MAX_BYTES } from './qr.ts'

const finder = ['1111111', '1000001', '1011101', '1011101', '1011101', '1000001', '1111111']
const at = (m, x0, y0) => finder.every((row, y) => [...row].every((c, x) => m[y0 + y][x0 + x] === (c === '1')))

// The format bits beside the top-left finder, read back the way a scanner does.
function readFormat(m) {
  let bits = 0
  for (let i = 0; i <= 5; i++) bits |= (m[i][8] ? 1 : 0) << i
  bits |= (m[7][8] ? 1 : 0) << 6; bits |= (m[8][8] ? 1 : 0) << 7; bits |= (m[8][7] ? 1 : 0) << 8
  for (let i = 9; i < 15; i++) bits |= (m[8][14 - i] ? 1 : 0) << i
  return bits ^ 0x5412
}

test('the size follows the length: version 1 is 21 modules, version 10 is 57', () => {
  assert.equal(qrMatrix('A').length, 21)
  assert.equal(qrMatrix('x'.repeat(14)).length, 21)
  assert.equal(qrMatrix('x'.repeat(15)).length, 25)
  assert.equal(qrMatrix('x'.repeat(QR_MAX_BYTES)).length, 57)
  assert.throws(() => qrMatrix('x'.repeat(QR_MAX_BYTES + 1)))
})

test('finder patterns, timing and the dark module are where a scanner looks', () => {
  for (const text of ['https://192.168.1.20:9797/pair?code=BCDF-GHJK', 'x'.repeat(150), 'ünï©ödé']) {
    const m = qrMatrix(text), n = m.length
    assert.ok(at(m, 0, 0) && at(m, n - 7, 0) && at(m, 0, n - 7), 'three finders')
    for (let i = 8; i < n - 8; i++) { assert.equal(m[6][i], i % 2 === 0); assert.equal(m[i][6], i % 2 === 0) }
    assert.equal(m[n - 8][8], true)
  }
})

test('the format bits are a valid code word for level M, and both copies agree', () => {
  const m = qrMatrix('wms://pair?v=1&s=abc&u=&c=0123&k=xyz'), n = m.length
  const f = readFormat(m)
  // The 15 bits divide by the generator 0x537 with no remainder.
  let rem = f
  for (let i = 14; i >= 10; i--) if ((rem >>> i) & 1) rem ^= 0x537 << (i - 10)
  assert.equal(rem, 0)
  assert.equal(f >>> 13, 0, 'level M')
  let second = 0
  for (let i = 0; i < 8; i++) second |= (m[8][n - 1 - i] ? 1 : 0) << i
  for (let i = 8; i < 15; i++) second |= (m[n - 15 + i][8] ? 1 : 0) << i
  assert.equal(second ^ 0x5412, f)
})

test('the same text makes the same code, and different text a different one', () => {
  const a = JSON.stringify(qrMatrix('one')), b = JSON.stringify(qrMatrix('one')), c = JSON.stringify(qrMatrix('two'))
  assert.equal(a, b)
  assert.notEqual(a, c)
})

test('the path has a quiet zone of four modules', () => {
  const p = qrPath('A')
  assert.equal(p.size, 29)
  assert.ok(p.d.startsWith('M4 4h1v1h-1z'))
})
