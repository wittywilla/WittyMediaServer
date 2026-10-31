// The effect format: one text file, a JSON description in a comment at the top and GLSL
// after it. It's valid GLSL as it stands, so any editor colours it, and both hosts read
// the same files: see the README.
//
// Built-in effects are ours, and can use #ifdef WV_QUAD for geometry. A file someone adds
// themselves is untrusted, so validateUserShader() below holds it to a small safe subset.
// Graphics drivers are the real sandbox for shaders, since the language can't reach memory,
// files or the network, but a shader can still be slow on purpose. So it's refused unless
// its worst case can be counted up front.

export const FORMAT_VERSION = 1
export const MAX_USER_BYTES = 16 * 1024
const MAX_ITERATIONS = 64 // of one loop
const MAX_COST = 40000 // in operations, per pixel, worst case

export type EffectInfo = {
  id: string
  name: string
  description: string
  author?: string
  license?: string
  tags: string[]
  cost: 1 | 2 | 3 // how hard it works the graphics card
  motion: 'gentle' | 'lively' // calm mode leaves out the lively ones
  feedback: boolean // it reads its own last picture
  quads: number // geometry: how many quads it draws
}

export type EffectFile = EffectInfo & { source: string; builtin: boolean }

export type Parsed = { ok: true; effect: EffectFile } | { ok: false; error: string }

const ID = /^[a-z0-9][a-z0-9-]{0,39}$/
const OPEN = '/*wittyvisualizer'

// No control characters, and none of the invisible ones that can make text read differently.
const hidden = (c: number) => c < 32 || (c >= 127 && c <= 159) || c === 0x2028 || c === 0x2029 || (c >= 0x202a && c <= 0x202e) || (c >= 0x2066 && c <= 0x2069)
const printable = (s: string, max: number) => s.length <= max && ![...s].some((ch) => hidden(ch.codePointAt(0) as number))

export function parseEffect(text: string, builtin: boolean): Parsed {
  if (!builtin && text.length > MAX_USER_BYTES) return { ok: false, error: `This file is too large. An effect can be up to ${MAX_USER_BYTES / 1024} KB.` }
  const trimmed = text.replace(/^\uFEFF/, '').trimStart()
  if (!trimmed.startsWith(OPEN)) return { ok: false, error: 'This isn\'t an effect file: it should start with a /*wittyvisualizer comment holding its description.' }
  const end = trimmed.indexOf('*/')
  if (end < 0) return { ok: false, error: 'The description at the top of the file is never closed with */.' }
  let meta: Record<string, unknown>
  try {
    const v: unknown = JSON.parse(trimmed.slice(OPEN.length, end))
    if (typeof v !== 'object' || v === null || Array.isArray(v)) throw new Error('not an object')
    meta = v as Record<string, unknown>
  } catch (e) {
    return { ok: false, error: 'The description at the top of the file isn\'t valid JSON: ' + (e as Error).message }
  }
  if (meta.format !== FORMAT_VERSION) return { ok: false, error: `This effect is for format ${String(meta.format)}, and this version reads format ${FORMAT_VERSION}.` }
  if (typeof meta.id !== 'string' || !ID.test(meta.id)) return { ok: false, error: 'Its "id" should be up to 40 lower-case letters, numbers and dashes.' }
  if (typeof meta.name !== 'string' || !meta.name.trim() || !printable(meta.name, 40)) return { ok: false, error: 'Its "name" should be plain text of up to 40 characters.' }
  const description = meta.description ?? ''
  if (typeof description !== 'string' || !printable(description, 280)) return { ok: false, error: 'Its "description" should be plain text of up to 280 characters.' }
  for (const k of ['author', 'license'] as const) {
    if (meta[k] !== undefined && (typeof meta[k] !== 'string' || !printable(meta[k] as string, 80))) return { ok: false, error: `Its "${k}" should be plain text of up to 80 characters.` }
  }
  const tags = meta.tags ?? []
  if (!Array.isArray(tags) || tags.length > 8 || !tags.every((t) => typeof t === 'string' && printable(t, 24))) return { ok: false, error: 'Its "tags" should be a short list of words.' }
  const cost = meta.cost ?? 2
  if (cost !== 1 && cost !== 2 && cost !== 3) return { ok: false, error: 'Its "cost" should be 1, 2 or 3.' }
  const motion = meta.motion ?? 'lively'
  if (motion !== 'gentle' && motion !== 'lively') return { ok: false, error: 'Its "motion" should be "gentle" or "lively".' }
  if (meta.feedback !== undefined && typeof meta.feedback !== 'boolean') return { ok: false, error: 'Its "feedback" should be true or false.' }
  const source = trimmed.slice(end + 2)
  let quads = 0
  if (meta.geometry !== undefined) {
    if (!builtin) return { ok: false, error: 'Geometry is for built-in effects: an effect you add draws one full-screen picture.' }
    const q = (meta.geometry as { quads?: unknown }).quads
    if (typeof q !== 'number' || !Number.isInteger(q) || q < 1 || q > 8192) return { ok: false, error: 'Its "geometry" should say how many quads, up to 8192.' }
    quads = q
  }
  if (!builtin) {
    const why = validateUserShader(source)
    if (why) return { ok: false, error: why }
  }
  return {
    ok: true,
    effect: {
      id: meta.id, name: meta.name.trim(), description, tags: tags as string[], cost, motion,
      feedback: meta.feedback === true, quads, source, builtin,
      ...(meta.author ? { author: meta.author as string } : {}),
      ...(meta.license ? { license: meta.license as string } : {}),
    },
  }
}

// What a shader someone adds may not contain. Directives, because they can expand to any
// size and turn extensions on. The names, because the host declares them. Textures only
// through the helpers, as the helpers know the layout. Arrays and structs have no use that
// is worth the rules they'd need here.
const FORBIDDEN_WORDS = new Set([
  'while', 'do', 'switch', 'case', 'goto', 'struct', 'uniform', 'in', 'out', 'inout', 'layout', 'precision', 'invariant', 'centroid',
  'smooth', 'flat', 'attribute', 'varying', 'buffer', 'shared', 'coherent', 'volatile', 'restrict', 'readonly', 'writeonly',
  'sampler2D', 'sampler3D', 'samplerCube', 'texture', 'texture2D', 'textureLod', 'textureOffset', 'textureGrad', 'texelFetch', 'textureSize',
  'iAudio', 'iPrev', 'iA', 'iB', 'iGuard', 'Frame', 'main', 'fragColor', 'vUv', 'asm', 'class', 'template',
])

type Token = { t: string; line: number }

function tokenize(src: string): Token[] | string {
  const out: Token[] = []
  let line = 1
  let i = 0
  while (i < src.length) {
    const c = src[i]
    if (c === '\n') { line++; i++; continue }
    if (c === ' ' || c === '\t' || c === '\r') { i++; continue }
    if (c === '/' && src[i + 1] === '/') { while (i < src.length && src[i] !== '\n') i++; continue }
    if (c === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2)
      if (end < 0) return `Line ${line}: a comment is never closed.`
      for (let k = i; k < end; k++) if (src[k] === '\n') line++
      i = end + 2
      continue
    }
    if (c.charCodeAt(0) > 126 || c.charCodeAt(0) < 32) return `Line ${line}: only plain ASCII text is allowed.`
    if (c === '#') return `Line ${line}: # directives aren't allowed in an effect you add.`
    if (c === '\\') return `Line ${line}: a backslash isn't allowed.`
    const word = /^[A-Za-z_][A-Za-z0-9_]*/.exec(src.slice(i, i + 80))
    if (word) { out.push({ t: word[0], line }); i += word[0].length; continue }
    const num = /^(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?[uUfF]?/.exec(src.slice(i, i + 40))
    if (num) { out.push({ t: num[0], line }); i += num[0].length; continue }
    const two = src.slice(i, i + 2)
    if (['++', '--', '+=', '-=', '*=', '/=', '==', '!=', '<=', '>=', '&&', '||'].includes(two)) { out.push({ t: two, line }); i += 2; continue }
    out.push({ t: c, line }); i++
  }
  return out
}

const isWord = (t: string) => /^[A-Za-z_]/.test(t)
const asNumber = (t: string | undefined) => (t !== undefined && /^[\d.]/.test(t) ? parseFloat(t) : NaN)

// Reads for (int i = 0; i < 8; i++) and its close relatives, and says how many times it runs.
function loopRuns(tok: Token[], at: number): { runs: number; bodyAt: number } | string {
  const where = `Line ${tok[at].line}:`
  let i = at + 1
  const needs = 'a loop must look like for (int i = 0; i < 8; i++), with plain numbers'
  if (tok[i]?.t !== '(') return `${where} ${needs}.`
  i++
  if (tok[i]?.t !== 'int' && tok[i]?.t !== 'float') return `${where} ${needs}.`
  const name = tok[i + 1]?.t
  if (!name || !isWord(name) || tok[i + 2]?.t !== '=') return `${where} ${needs}.`
  let sign = 1
  i += 3
  if (tok[i]?.t === '-') { sign = -1; i++ }
  const from = sign * asNumber(tok[i]?.t)
  if (Number.isNaN(from) || tok[i + 1]?.t !== ';' || tok[i + 2]?.t !== name) return `${where} ${needs}.`
  const cmp = tok[i + 3]?.t
  i += 4
  sign = 1
  if (tok[i]?.t === '-') { sign = -1; i++ }
  const to = sign * asNumber(tok[i]?.t)
  if (Number.isNaN(to) || tok[i + 1]?.t !== ';' || tok[i + 2]?.t !== name) return `${where} ${needs}.`
  const op = tok[i + 3]?.t
  i += 4
  let step = 0
  if (op === '++') step = 1
  else if (op === '--') step = -1
  else if (op === '+=' || op === '-=') { step = asNumber(tok[i]?.t) * (op === '+=' ? 1 : -1); i++ }
  if (!step || Number.isNaN(step) || tok[i]?.t !== ')') return `${where} ${needs}.`
  i++
  const span = cmp === '<' || cmp === '<=' ? to - from : cmp === '>' || cmp === '>=' ? from - to : NaN
  if (Number.isNaN(span) || (step > 0) !== (cmp === '<' || cmp === '<=') || span < 0) return `${where} ${needs}.`
  const runs = Math.ceil(span / Math.abs(step)) + (cmp === '<=' || cmp === '>=' ? 1 : 0)
  if (runs > MAX_ITERATIONS) return `${where} a loop can run at most ${MAX_ITERATIONS} times, and this one runs ${runs}.`
  return { runs, bodyAt: i }
}

// Returns why it can't be used, or null if it's fine.
export function validateUserShader(source: string): string | null {
  const tok = tokenize(source)
  if (typeof tok === 'string') return tok
  if (tok.length > 6000) return 'This shader is too long.'
  // Functions, in order. A function can call only the ones before it, so there's no recursion.
  const costs = new Map<string, number>()
  const known = new Set(['effect'])
  let foundEffect = false
  let i = 0
  while (i < tok.length) {
    const t = tok[i]
    if (FORBIDDEN_WORDS.has(t.t) || t.t.startsWith('gl_') || t.t.startsWith('wv_') || t.t.startsWith('WV_')) return `Line ${t.line}: "${t.t}" isn't allowed in an effect you add.`
    if (t.t === '[' || t.t === ']') return `Line ${t.line}: arrays aren't allowed in an effect you add.`
    // At the top level only constants and functions are possible.
    if (t.t === 'const') { while (i < tok.length && tok[i].t !== ';') { if (FORBIDDEN_WORDS.has(tok[i].t)) return `Line ${tok[i].line}: "${tok[i].t}" isn't allowed.`; i++ } i++; continue }
    if (!isWord(t.t) || !isWord(tok[i + 1]?.t ?? '') || tok[i + 2]?.t !== '(') return `Line ${t.line}: only constants and functions can be at the top level of an effect.`
    const name = tok[i + 1].t
    if (FORBIDDEN_WORDS.has(name) || name.startsWith('gl_') || name.startsWith('wv_') || name.startsWith('WV_')) return `Line ${t.line}: "${name}" isn't allowed in an effect you add.`
    if (costs.has(name)) return `Line ${t.line}: "${name}" is defined twice.`
    // Parameters
    let j = i + 3, depth = 1
    while (j < tok.length && depth > 0) {
      const w = tok[j].t
      if (FORBIDDEN_WORDS.has(w)) return `Line ${tok[j].line}: "${w}" isn't allowed in an effect you add.`
      if (w === '[' || w === ']') return `Line ${tok[j].line}: arrays aren't allowed in an effect you add.`
      if (w === '(') depth++
      if (w === ')') depth--
      j++
    }
    if (tok[j]?.t === ';') return `Line ${tok[j].line}: define a function where it's declared, before anything that calls it.`
    if (tok[j]?.t !== '{') return `Line ${t.line}: "${name}" has no body.`
    // The body: count its operations, multiplied by the loops around them.
    let cost = 0
    const stack: { mult: number; open: boolean; depth: number }[] = []
    let mult = 1, braces = 0, pendingLoop: number | null = null
    let k = j
    for (; k < tok.length; k++) {
      const w = tok[k].t
      if (FORBIDDEN_WORDS.has(w) || w.startsWith('gl_') || w.startsWith('wv_') || w.startsWith('WV_')) return `Line ${tok[k].line}: "${w}" isn't allowed in an effect you add.`
      if (w === '[' || w === ']') return `Line ${tok[k].line}: arrays aren't allowed in an effect you add.`
      if (w === '{') {
        braces++
        if (pendingLoop !== null) { stack.push({ mult, open: true, depth: braces }); mult *= pendingLoop; pendingLoop = null }
      } else if (w === '}') {
        if (stack.length && stack[stack.length - 1].depth === braces) mult = stack.pop()!.mult
        braces--
        if (braces === 0) break
      } else if (w === 'for') {
        const loop = loopRuns(tok, k)
        if (typeof loop === 'string') return loop
        if (tok[loop.bodyAt]?.t !== '{') return `Line ${tok[k].line}: a loop's body must be in { }.`
        pendingLoop = loop.runs
        k = loop.bodyAt - 1
      } else {
        cost += mult
        if (isWord(w) && tok[k + 1]?.t === '(' && costs.has(w)) cost += mult * costs.get(w)!
        if (isWord(w) && tok[k + 1]?.t === '(' && w === name) return `Line ${tok[k].line}: "${name}" calls itself, which GLSL doesn't allow.`
      }
      if (cost > MAX_COST) return `This shader could take too long to draw (over ${MAX_COST} steps for one pixel). Use fewer or shorter loops.`
    }
    if (braces !== 0) return `Line ${t.line}: "${name}" is missing a closing }.`
    costs.set(name, cost)
    known.add(name)
    if (name === 'effect') foundEffect = true
    i = k + 1
  }
  if (!foundEffect) return 'An effect needs a function: vec4 effect(vec2 uv) { ... }'
  return null
}
