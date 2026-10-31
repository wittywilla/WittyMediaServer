import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { test } from 'node:test'
import { MAX_USER_BYTES, parseEffect, validateUserShader } from '../format.ts'

const head = (extra = {}) => `/*wittyvisualizer ${JSON.stringify({ format: 1, id: 'mine', name: 'Mine', ...extra })} */\n`
const body = 'vec4 effect(vec2 uv) { return vec4(uv, spectrum(uv.x), 1.0); }'
const refused = (src, why) => {
  const r = validateUserShader(src)
  assert.ok(r, `should refuse: ${src}`)
  if (why) assert.match(r, why)
}

test('every built-in effect reads, and they are all different', () => {
  const dir = new URL('../effects/', import.meta.url)
  const files = readdirSync(dir).filter((f) => f.endsWith('.glsl'))
  assert.ok(files.length >= 12, 'at least a dozen')
  const ids = new Set(), names = new Set()
  for (const f of files) {
    const r = parseEffect(readFileSync(new URL(f, dir), 'utf8'), true)
    assert.ok(r.ok, `${f}: ${r.error}`)
    assert.equal(r.effect.id + '.glsl', f, 'named after its file')
    assert.ok(r.effect.description.length > 20, `${f} says what it is`)
    assert.ok(!ids.has(r.effect.id) && !names.has(r.effect.name))
    ids.add(r.effect.id); names.add(r.effect.name)
  }
})

test('a calm mode has effects to show', () => {
  const dir = new URL('../effects/', import.meta.url)
  const gentle = readdirSync(dir).filter((f) => f.endsWith('.glsl')).filter((f) => parseEffect(readFileSync(new URL(f, dir), 'utf8'), true).effect.motion === 'gentle')
  assert.ok(gentle.length >= 6, `only ${gentle.length} gentle effects`)
})

test('a plain effect file is accepted', () => {
  const r = parseEffect(head({ description: 'Mine', author: 'Me', license: 'MIT', tags: ['test'], cost: 1, motion: 'gentle', feedback: true }) + body, false)
  assert.ok(r.ok, r.error)
  assert.equal(r.effect.id, 'mine')
  assert.equal(r.effect.builtin, false)
  assert.equal(r.effect.feedback, true)
})

test('descriptions that are wrong are turned away with a reason', () => {
  const bad = (text, why) => { const r = parseEffect(text, false); assert.equal(r.ok, false); assert.match(r.error, why) }
  bad(body, /start with/)
  bad('/*wittyvisualizer {nope */' + body, /valid JSON/)
  bad('/*wittyvisualizer {"format":1', /never closed/)
  bad('/*wittyvisualizer [] */' + body, /valid JSON/)
  bad(head({ format: 2 }) + body, /format 2/)
  bad(head({ id: 'Has Spaces' }) + body, /"id"/)
  bad(head({ id: '../../etc' }) + body, /"id"/)
  bad(head({ id: 'x'.repeat(41) }) + body, /"id"/)
  bad(head({ name: '' }) + body, /"name"/)
  bad(head({ name: 'x'.repeat(41) }) + body, /"name"/)
  bad(head({ name: 'bad‮name' }) + body, /"name"/)
  bad(head({ name: 7 }) + body, /"name"/)
  bad(head({ description: 'x'.repeat(281) }) + body, /"description"/)
  bad(head({ author: {} }) + body, /"author"/)
  bad(head({ tags: 'one' }) + body, /"tags"/)
  bad(head({ cost: 9 }) + body, /"cost"/)
  bad(head({ motion: 'wild' }) + body, /"motion"/)
  bad(head({ feedback: 'yes' }) + body, /"feedback"/)
  bad(head({ geometry: { quads: 10 } }) + body, /Geometry/)
  bad(head() + ' '.repeat(MAX_USER_BYTES) + body, /too large/)
})

test('prototype pollution in a description does nothing', () => {
  const r = parseEffect('/*wittyvisualizer {"format":1,"id":"a","name":"A","__proto__":{"polluted":true}} */' + body, false)
  assert.ok(r.ok)
  assert.equal({}.polluted, undefined)
})

test('a shader someone adds can use the helpers, numbers and short loops', () => {
  const fine = [
    body,
    'const float K = 2.0;\nfloat f(float x) { return x * K; }\nvec4 effect(vec2 uv) { return vec4(f(uv.x), f(uv.y), 0.0, 1.0); }',
    'vec4 effect(vec2 uv) { float s = 0.0; for (int i = 0; i < 16; i++) { s += sin(float(i) * uv.x); } return vec4(s); }',
    'vec4 effect(vec2 uv) { float s = 0.0; for (float i = 8.0; i >= 0.0; i -= 1.0) { s += i; } return vec4(s); }',
    '// a comment with # and while in it\n/* and another: uniform */\nvec4 effect(vec2 uv) { return vec4(prev(uv).rgb * 0.9, 1.0); }',
    'vec4 effect(vec2 uv) { for (int i = 0; i < 4; i++) { for (int j = 0; j < 4; j++) { if (i == j) { continue; } } } return vec4(0.0); }',
  ]
  for (const src of fine) assert.equal(validateUserShader(src), null, src)
})

test('anything outside the small subset is refused', () => {
  refused('void other() {}', /needs a function/)
  refused('#define X 1\n' + body, /directives/)
  refused('#extension GL_foo : enable\n' + body, /directives/)
  refused('#version 300 es\n' + body, /directives/)
  refused(`${body}\\`, /backslash/)
  refused('uniform float x;\n' + body, /uniform/)
  refused('vec4 effect(vec2 uv) { while (true) {} return vec4(0.0); }', /while/)
  refused('vec4 effect(vec2 uv) { do { } while (true); return vec4(0.0); }', /"do"/)
  refused('vec4 effect(vec2 uv) { return texture(iAudio, uv); }', /texture/)
  refused('vec4 effect(vec2 uv) { return texelFetch(iPrev, ivec2(0), 0); }', /texelFetch/)
  refused('vec4 effect(vec2 uv) { return vec4(gl_FragCoord); }', /gl_FragCoord/)
  refused('float a[4];\n' + body, /only constants and functions|arrays/)
  refused('vec4 effect(vec2 uv) { float a[4]; return vec4(0.0); }', /arrays/)
  refused('struct S { float x; };\n' + body, /struct/)
  refused('void main() { }\n' + body, /main/)
  refused('float wv_row(float x, float y) { return 0.0; }\n' + body, /wv_row/)
  refused('layout(location = 0) out vec4 c;\n' + body, /layout/)
  refused('float f();\n' + body, /declared/)
  refused('vec4 effect(vec2 uv) { return vec4(0.0);', /closing/)
  refused('vec4 effect(vec2 uv) { return vec4(0.0); }\nvec4 effect(vec2 uv) { return vec4(1.0); }', /twice/)
  refused('float f(float x) { return f(x); }\n' + body, /calls itself/)
  refused('vec4 effect(vec2 uv) { /* never closed\n return vec4(0.0); }', /never closed/)
  refused('vec4 effect(vec2 uv) { return vec4(0.0); } é', /ASCII/)
})

test('loops must be countable, and short', () => {
  const loop = (head) => `vec4 effect(vec2 uv) { float s = 0.0; for (${head}) { s += 1.0; } return vec4(s); }`
  refused(loop('int i = 0; i < 65; i++'), /at most 64/)
  refused(loop('int i = 0; i < 1000000; i++'), /at most 64/)
  refused(loop('int i = 0; i < 10; i--'), /must look like/)
  refused(loop('int i = 0; i > 10; i++'), /must look like/)
  refused(loop('int i = 0; i < n; i++'), /must look like/)
  refused(loop('int i = 0; ; i++'), /must look like/)
  refused(loop(';;'), /must look like/)
  refused(loop('float i = 0.0; i < 1.0; i += 0.001'), /at most 64/)
  refused(loop('int i = 0; i < 4; i += 0'), /must look like/)
  refused(loop('int i = 0; j < 4; i++'), /must look like/)
  refused('vec4 effect(vec2 uv) { for (int i = 0; i < 4; i++) return vec4(0.0); }', /in \{ \}/)
})

test('slowness can\'t be hidden by nesting loops or calling functions', () => {
  const inner = 'float a(float x) { float s = x; for (int i = 0; i < 64; i++) { s += sin(s); } return s; }'
  const mid = 'float b(float x) { float s = x; for (int i = 0; i < 64; i++) { s += a(s); } return s; }'
  refused(`${inner}\n${mid}\nvec4 effect(vec2 uv) { float s = 0.0; for (int i = 0; i < 64; i++) { s += b(s); } return vec4(s); }`, /too long/)
  refused('vec4 effect(vec2 uv) { float s = 0.0; for (int i = 0; i < 64; i++) { for (int j = 0; j < 64; j++) { for (int k = 0; k < 64; k++) { s += 1.0; } } } return vec4(s); }', /too long/)
})

test('hostile input is only ever turned away, quickly', () => {
  const nasty = [
    '', ' ', '{', '}', '(', ')', ';', '/*', '*/', 'for', 'for (', 'vec4', 'vec4 effect', 'vec4 effect(', 'vec4 effect() {',
    '}'.repeat(5000), '{'.repeat(5000), '('.repeat(5000), 'a '.repeat(8000), '1'.repeat(16000), '/*'.repeat(8000),
    'vec4 effect(vec2 uv) {' + 'for (int i = 0; i < 2; i++) {'.repeat(500) + '}'.repeat(501),
  ]
  const began = performance.now()
  for (const src of nasty) {
    const r = validateUserShader(src)
    assert.equal(typeof r, 'string', JSON.stringify(src.slice(0, 40)))
    parseEffect(head() + src, false)
  }
  let seed = 11
  const rnd = (n) => (seed = (seed * 48271) % 2147483647) % n
  const words = ['vec4', 'effect', '(', ')', '{', '}', 'for', 'int', 'i', '=', '0', ';', '<', '8', '++', 'float', ',', 'return', '#', '[', '"', '\n']
  for (let n = 0; n < 2000; n++) {
    let src = ''
    for (let k = rnd(60); k > 0; k--) src += words[rnd(words.length)] + ' '
    validateUserShader(src) // must not throw
  }
  assert.ok(performance.now() - began < 3000)
})
