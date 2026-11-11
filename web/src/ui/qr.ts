// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// A QR code, made here in the page: the pairing code it carries never goes to
// anyone else's server to be drawn. Byte mode, error correction level M,
// versions 1 to 10 (up to 213 bytes), which is all a pairing link needs.
// ISO/IEC 18004; the layout follows the standard's own order of steps.

// For versions 1 to 10 at level M: error correction codewords per block, then
// how many blocks of how many data codewords, in two groups.
const BLOCKS: number[][] = [
  [10, 1, 16, 0, 0], [16, 1, 28, 0, 0], [26, 1, 44, 0, 0], [18, 2, 32, 0, 0], [24, 2, 43, 0, 0],
  [16, 4, 27, 0, 0], [18, 4, 31, 0, 0], [22, 2, 38, 2, 39], [22, 3, 36, 2, 37], [26, 4, 43, 1, 44],
]
// Where the alignment patterns' centres are, along each axis.
const ALIGN: number[][] = [[], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50]]

export const QR_MAX_BYTES = 213

const dataCodewords = (v: number) => { const b = BLOCKS[v - 1]; return b[1] * b[2] + b[3] * b[4] }
// Byte mode spends 4 bits on the mode and 8 (16 from version 10) on the length.
const capacity = (v: number) => dataCodewords(v) - (v < 10 ? 2 : 3)

// Multiplying in GF(2^8) over x^8+x^4+x^3+x^2+1, for the Reed-Solomon codes.
function gfMul(x: number, y: number): number {
  let z = 0
  for (let i = 7; i >= 0; i--) { z = (z << 1) ^ ((z >>> 7) * 0x11d); z ^= ((y >>> i) & 1) * x }
  return z
}
function rsDivisor(degree: number): number[] {
  const out = new Array<number>(degree).fill(0)
  out[degree - 1] = 1
  let root = 1
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < degree; j++) { out[j] = gfMul(out[j], root); if (j + 1 < degree) out[j] ^= out[j + 1] }
    root = gfMul(root, 2)
  }
  return out
}
function rsRemainder(data: number[], divisor: number[]): number[] {
  const out = new Array<number>(divisor.length).fill(0)
  for (const b of data) {
    const factor = b ^ (out.shift() as number)
    out.push(0)
    divisor.forEach((c, i) => { out[i] ^= gfMul(c, factor) })
  }
  return out
}

// The codewords in the order they are drawn: the data blocks interleaved, then
// their error correction interleaved.
function codewords(bytes: Uint8Array, v: number): number[] {
  const bits: number[] = []
  const put = (val: number, len: number) => { for (let i = len - 1; i >= 0; i--) bits.push((val >>> i) & 1) }
  put(4, 4)
  put(bytes.length, v < 10 ? 8 : 16)
  for (const b of bytes) put(b, 8)
  const total = dataCodewords(v) * 8
  put(0, Math.min(4, total - bits.length))
  while (bits.length % 8) bits.push(0)
  for (let pad = 0xec; bits.length < total; pad ^= 0xec ^ 0x11) put(pad, 8)
  const data: number[] = []
  for (let i = 0; i < bits.length; i += 8) data.push(bits.slice(i, i + 8).reduce((a, b) => (a << 1) | b, 0))

  const [ecLen, n1, len1, n2, len2] = BLOCKS[v - 1]
  const divisor = rsDivisor(ecLen)
  const blocks: number[][] = [], ecs: number[][] = []
  let at = 0
  for (let i = 0; i < n1 + n2; i++) {
    const len = i < n1 ? len1 : len2
    const block = data.slice(at, at + len)
    at += len
    blocks.push(block)
    ecs.push(rsRemainder(block, divisor))
  }
  const out: number[] = []
  for (let i = 0; i < Math.max(len1, len2); i++) for (const b of blocks) if (i < b.length) out.push(b[i])
  for (let i = 0; i < ecLen; i++) for (const e of ecs) out.push(e[i])
  return out
}

const MASKS: ((x: number, y: number) => boolean)[] = [
  (x, y) => (x + y) % 2 === 0, (_x, y) => y % 2 === 0, (x) => x % 3 === 0, (x, y) => (x + y) % 3 === 0,
  (x, y) => (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0, (x, y) => ((x * y) % 2) + ((x * y) % 3) === 0,
  (x, y) => (((x * y) % 2) + ((x * y) % 3)) % 2 === 0, (x, y) => (((x + y) % 2) + ((x * y) % 3)) % 2 === 0,
]

// How hard a pattern is for a scanner, by the standard's four rules; lower is better.
function penalty(m: boolean[][]): number {
  const n = m.length
  let score = 0, dark = 0
  const line = (get: (i: number) => boolean) => {
    let run = 1
    for (let i = 1; i <= n; i++) {
      if (i < n && get(i) === get(i - 1)) { run++; continue }
      if (run >= 5) score += run - 2
      run = 1
    }
    // A finder-like 1:1:3:1:1 with four light modules beside it.
    for (let i = 0; i + 10 < n; i++) {
      const s = (k: number) => get(i + k)
      const core = (o: number) => s(o) && !s(o + 1) && s(o + 2) && s(o + 3) && s(o + 4) && !s(o + 5) && s(o + 6)
      if ((core(0) && !s(7) && !s(8) && !s(9) && !s(10)) || (!s(0) && !s(1) && !s(2) && !s(3) && core(4))) score += 40
    }
  }
  for (let y = 0; y < n; y++) { line((x) => m[y][x]); line((x) => m[x][y]) }
  for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) {
    if (m[y][x]) dark++
    if (x + 1 < n && y + 1 < n && m[y][x] === m[y][x + 1] && m[y][x] === m[y + 1][x] && m[y][x] === m[y + 1][x + 1]) score += 3
  }
  return score + Math.floor(Math.abs(dark * 20 - n * n * 10) / (n * n)) * 10
}

// qrMatrix is the code for text: rows of modules, true for dark, without the
// quiet zone. It throws when the text is longer than QR_MAX_BYTES bytes.
export function qrMatrix(text: string): boolean[][] {
  const bytes = new TextEncoder().encode(text)
  let v = 1
  while (v <= 10 && capacity(v) < bytes.length) v++
  if (v > 10) throw new Error('too long for a QR code here')
  const n = 17 + 4 * v
  const mod: boolean[][] = Array.from({ length: n }, () => new Array<boolean>(n).fill(false))
  const fn: boolean[][] = Array.from({ length: n }, () => new Array<boolean>(n).fill(false))
  const set = (x: number, y: number, dark: boolean) => { if (x >= 0 && y >= 0 && x < n && y < n) { mod[y][x] = dark; fn[y][x] = true } }

  for (let i = 0; i < n; i++) { set(6, i, i % 2 === 0); set(i, 6, i % 2 === 0) }
  for (const [cx, cy] of [[3, 3], [n - 4, 3], [3, n - 4]]) {
    for (let dy = -4; dy <= 4; dy++) for (let dx = -4; dx <= 4; dx++) {
      const d = Math.max(Math.abs(dx), Math.abs(dy))
      set(cx + dx, cy + dy, d !== 2 && d !== 4)
    }
  }
  const al = ALIGN[v - 1]
  for (const cx of al) for (const cy of al) {
    if ((cx === 6 && cy === 6) || (cx === 6 && cy === n - 7) || (cx === n - 7 && cy === 6)) continue
    for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) set(cx + dx, cy + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1)
  }
  const format = (mask: number) => {
    let rem = mask // level M is 00, so the five data bits are the mask alone
    for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537)
    const bits = ((mask << 10) | rem) ^ 0x5412
    const bit = (i: number) => ((bits >>> i) & 1) === 1
    for (let i = 0; i <= 5; i++) set(8, i, bit(i))
    set(8, 7, bit(6)); set(8, 8, bit(7)); set(7, 8, bit(8))
    for (let i = 9; i < 15; i++) set(14 - i, 8, bit(i))
    for (let i = 0; i < 8; i++) set(n - 1 - i, 8, bit(i))
    for (let i = 8; i < 15; i++) set(8, n - 15 + i, bit(i))
    set(8, n - 8, true)
  }
  format(0) // reserves the places; the real bits go in once the mask is chosen
  if (v >= 7) {
    let rem = v
    for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25)
    const bits = (v << 12) | rem
    for (let i = 0; i < 18; i++) {
      const dark = ((bits >>> i) & 1) === 1, a = n - 11 + (i % 3), b = Math.floor(i / 3)
      set(a, b, dark); set(b, a, dark)
    }
  }

  const data = codewords(bytes, v)
  let i = 0
  for (let right = n - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5
    for (let vert = 0; vert < n; vert++) for (let j = 0; j < 2; j++) {
      const x = right - j, upward = ((right + 1) & 2) === 0, y = upward ? n - 1 - vert : vert
      if (!fn[y][x] && i < data.length * 8) { mod[y][x] = ((data[i >>> 3] >>> (7 - (i & 7))) & 1) === 1; i++ }
    }
  }

  const flip = (mask: number) => { for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) if (!fn[y][x] && MASKS[mask](x, y)) mod[y][x] = !mod[y][x] }
  let best = 0, least = Infinity
  for (let mask = 0; mask < 8; mask++) {
    flip(mask); format(mask)
    const p = penalty(mod)
    if (p < least) { least = p; best = mask }
    flip(mask)
  }
  flip(best); format(best)
  return mod
}

// qrPath is the code as one SVG path in a box of (modules + 2 * quiet) units,
// and that box's size. The quiet zone is the blank margin scanners need.
export function qrPath(text: string, quiet = 4): { d: string; size: number } {
  const m = qrMatrix(text)
  let d = ''
  m.forEach((row, y) => row.forEach((dark, x) => { if (dark) d += `M${x + quiet} ${y + quiet}h1v1h-1z` }))
  return { d, size: m.length + 2 * quiet }
}
