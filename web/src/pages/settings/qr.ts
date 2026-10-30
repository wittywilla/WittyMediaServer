// A small QR code maker: byte mode, error correction level M, versions 1 to 10
// (up to 213 bytes), enough for a server address or an authenticator app's
// setup link. Returns rows of dark (true) and light modules, or null if the
// text does not fit.

const ECC_PER_BLOCK = [0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26]
const BLOCKS = [0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5]

const rawModules = (v: number) => {
  let r = (16 * v + 128) * v + 64
  if (v >= 2) {
    const n = Math.floor(v / 7) + 2
    r -= (25 * n - 10) * n - 55
    if (v >= 7) r -= 36
  }
  return r
}
const dataBytes = (v: number) => (rawModules(v) >> 3) - ECC_PER_BLOCK[v] * BLOCKS[v]

function gfMul(a: number, b: number) {
  let z = 0
  for (let i = 7; i >= 0; i--) {
    z = (z << 1) ^ ((z >>> 7) * 0x11d)
    z ^= ((b >>> i) & 1) * a
  }
  return z
}

function rsDivisor(degree: number) {
  const out = new Array<number>(degree).fill(0)
  out[degree - 1] = 1
  let root = 1
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < degree; j++) {
      out[j] = gfMul(out[j], root)
      if (j + 1 < degree) out[j] ^= out[j + 1]
    }
    root = gfMul(root, 2)
  }
  return out
}

function rsRemainder(data: number[], divisor: number[]) {
  const out = divisor.map(() => 0)
  for (const b of data) {
    const factor = b ^ (out.shift() as number)
    out.push(0)
    divisor.forEach((c, i) => { out[i] ^= gfMul(c, factor) })
  }
  return out
}

// The message, its error correction, interleaved block by block.
function codewords(text: Uint8Array, v: number) {
  const bits: number[] = []
  const put = (val: number, n: number) => { for (let i = n - 1; i >= 0; i--) bits.push((val >>> i) & 1) }
  put(4, 4)
  put(text.length, v <= 9 ? 8 : 16)
  text.forEach((b) => put(b, 8))
  const cap = dataBytes(v) * 8
  put(0, Math.min(4, cap - bits.length))
  put(0, (8 - (bits.length % 8)) % 8)
  for (let pad = 0xec; bits.length < cap; pad ^= 0xec ^ 0x11) put(pad, 8)
  const data: number[] = []
  for (let i = 0; i < bits.length; i += 8) data.push(bits.slice(i, i + 8).reduce((a, b) => (a << 1) | b, 0))

  const raw = rawModules(v) >> 3
  const n = BLOCKS[v], ecc = ECC_PER_BLOCK[v]
  const short = n - (raw % n), shortLen = Math.floor(raw / n)
  const div = rsDivisor(ecc)
  const blocks: number[][] = []
  for (let i = 0, k = 0; i < n; i++) {
    const part = data.slice(k, k + shortLen - ecc + (i < short ? 0 : 1))
    k += part.length
    const rem = rsRemainder(part, div)
    if (i < short) part.push(0)
    blocks.push(part.concat(rem))
  }
  const out: number[] = []
  for (let i = 0; i < blocks[0].length; i++) {
    blocks.forEach((b, j) => { if (i !== shortLen - ecc || j >= short) out.push(b[i]) })
  }
  return out
}

export function qrMatrix(input: string): boolean[][] | null {
  const text = new TextEncoder().encode(input)
  let v = 1
  while (v <= 10 && 4 + (v <= 9 ? 8 : 16) + 8 * text.length > dataBytes(v) * 8) v++
  if (v > 10) return null
  const size = v * 4 + 17
  const dark = Array.from({ length: size }, () => new Array<boolean>(size).fill(false))
  const fixed = Array.from({ length: size }, () => new Array<boolean>(size).fill(false))
  const set = (x: number, y: number, on: boolean) => { dark[y][x] = on; fixed[y][x] = true }

  for (let i = 0; i < size; i++) { set(6, i, i % 2 === 0); set(i, 6, i % 2 === 0) }
  for (const [cx, cy] of [[3, 3], [size - 4, 3], [3, size - 4]]) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const x = cx + dx, y = cy + dy, d = Math.max(Math.abs(dx), Math.abs(dy))
        if (x >= 0 && x < size && y >= 0 && y < size) set(x, y, d !== 2 && d !== 4)
      }
    }
  }
  if (v >= 2) {
    const num = Math.floor(v / 7) + 2
    const step = Math.ceil((v * 4 + 4) / (num * 2 - 2)) * 2
    const pos = [6]
    for (let p = size - 7; pos.length < num; p -= step) pos.splice(1, 0, p)
    pos.forEach((ax, i) => pos.forEach((ay, j) => {
      if ((i === 0 && j === 0) || (i === 0 && j === num - 1) || (i === num - 1 && j === 0)) return
      for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) set(ax + dx, ay + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1)
    }))
  }
  const format = (mask: number) => {
    const d = mask // level M is 00
    let rem = d
    for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537)
    const bits = ((d << 10) | rem) ^ 0x5412
    const bit = (i: number) => ((bits >>> i) & 1) !== 0
    for (let i = 0; i <= 5; i++) set(8, i, bit(i))
    set(8, 7, bit(6)); set(8, 8, bit(7)); set(7, 8, bit(8))
    for (let i = 9; i < 15; i++) set(14 - i, 8, bit(i))
    for (let i = 0; i < 8; i++) set(size - 1 - i, 8, bit(i))
    for (let i = 8; i < 15; i++) set(8, size - 15 + i, bit(i))
    set(8, size - 8, true)
  }
  format(0)
  if (v >= 7) {
    let rem = v
    for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25)
    const bits = (v << 12) | rem
    for (let i = 0; i < 18; i++) {
      const on = ((bits >>> i) & 1) !== 0
      const a = size - 11 + (i % 3), b = Math.floor(i / 3)
      set(a, b, on); set(b, a, on)
    }
  }
  const words = codewords(text, v)
  let i = 0
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5
    for (let vert = 0; vert < size; vert++) {
      for (let j = 0; j < 2; j++) {
        const x = right - j
        const y = ((right + 1) & 2) === 0 ? size - 1 - vert : vert
        if (!fixed[y][x] && i < words.length * 8) {
          dark[y][x] = ((words[i >>> 3] >>> (7 - (i & 7))) & 1) !== 0
          i++
        }
      }
    }
  }
  const flip = (mask: number, x: number, y: number) => {
    switch (mask) {
      case 0: return (x + y) % 2 === 0
      case 1: return y % 2 === 0
      case 2: return x % 3 === 0
      case 3: return (x + y) % 3 === 0
      case 4: return (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0
      case 5: return (x * y) % 2 + (x * y) % 3 === 0
      case 6: return ((x * y) % 2 + (x * y) % 3) % 2 === 0
      default: return ((x + y) % 2 + (x * y) % 3) % 2 === 0
    }
  }
  const applyMask = (mask: number) => {
    for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) if (!fixed[y][x] && flip(mask, x, y)) dark[y][x] = !dark[y][x]
  }
  // The pattern phones read most easily, by the standard's scoring.
  const penalty = () => {
    let score = 0, black = 0
    const line = (get: (k: number) => boolean) => {
      let s = 0, run = 0
      for (let k = 0; k < size; k++) {
        const on = get(k)
        s = (s << 1 | (on ? 1 : 0)) & 0x7ff
        if (k > 0 && on === get(k - 1)) { run++; if (run === 5) score += 3; else if (run > 5) score++ } else run = 1
        if (k >= 10 && (s === 0b10111010000 || s === 0b00001011101)) score += 40
      }
    }
    for (let y = 0; y < size; y++) {
      line((k) => dark[y][k])
      for (let x = 0; x < size; x++) {
        if (dark[y][x]) black++
        if (x + 1 < size && y + 1 < size && dark[y][x] === dark[y][x + 1] && dark[y][x] === dark[y + 1][x] && dark[y][x] === dark[y + 1][x + 1]) score += 3
      }
    }
    for (let x = 0; x < size; x++) line((k) => dark[k][x])
    return score + Math.floor(Math.abs(black * 20 - size * size * 10) / (size * size)) * 10
  }
  let best = 0, bestScore = Infinity
  for (let m = 0; m < 8; m++) {
    applyMask(m); format(m)
    const s = penalty()
    if (s < bestScore) { best = m; bestScore = s }
    applyMask(m)
  }
  applyMask(best); format(best)
  return dark
}
