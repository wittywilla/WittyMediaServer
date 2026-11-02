// node --experimental-strip-types --test web/src/help/markdown.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { inline, parse, plain, slug, target } from './markdown.ts'

test('headings get the address GitHub gives them, and repeats are numbered', () => {
  const b = parse('# Getting started\n\n## If it won\'t start\n\n## If it won\'t start\n\n### `doctor --fix`: what it does')
  assert.deepEqual(b.map((x) => x.id), ['getting-started', 'if-it-wont-start', 'if-it-wont-start-1', 'doctor---fix-what-it-does'])
  assert.equal(slug('Settings → Repair'), 'settings--repair')
})

test('bold, italic, code and links, nested', () => {
  assert.deepEqual(inline('**Settings → Repair**'), [{ t: 'strong', c: [{ t: 'text', v: 'Settings → Repair' }] }])
  assert.deepEqual(inline('a `b*c*` d'), [{ t: 'text', v: 'a ' }, { t: 'code', v: 'b*c*' }, { t: 'text', v: ' d' }])
  const [link] = inline('[**Repair** it](REPAIR.md#a-b)')
  assert.equal(link.t, 'link')
  assert.equal(link.href, 'REPAIR.md#a-b')
  assert.equal(plain(link.c), 'Repair it')
  assert.equal(plain(inline('snake_case_name and 2 * 3 * 4')), 'snake_case_name and 2 * 3 * 4')
  assert.equal(plain(inline('\\*not bold\\*')), '*not bold*')
  assert.equal(plain(inline('[not a link] (really)')), '[not a link] (really)')
})

test('lists nest, and a code block inside a step stays in it', () => {
  const b = parse('1. First\n   ```bash\n   wittymediaserver doctor\n   ```\n2. Second\n   - inner\n   - more\n\n3. Third\n\nAfter')
  assert.equal(b.length, 2)
  assert.equal(b[0].t, 'list')
  assert.equal(b[0].ordered, true)
  assert.equal(b[0].items.length, 3)
  assert.equal(b[0].items[0][1].t, 'code')
  assert.equal(b[0].items[0][1].v, 'wittymediaserver doctor')
  assert.equal(b[0].items[1][1].items.length, 2)
  assert.equal(b[1].t, 'p')
})

test('tables, with pipes kept where they are escaped or in code', () => {
  const [t] = parse('| A | B |\n|---|:-:|\n| 1 \\| 2 | `x|y` |\n| only one |')
  assert.equal(t.t, 'table')
  assert.equal(t.align[1], 'center')
  assert.equal(plain(t.rows[0][0]), '1 | 2')
  assert.equal(plain(t.rows[0][1]), 'x|y')
  assert.equal(t.rows[1].length, 2)
})

test('raw HTML never becomes anything', () => {
  const b = parse('# T\n\n---\n\n<sub>WittyMediaServer documentation</sub>\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>')
  assert.deepEqual(b.map((x) => x.t), ['heading', 'hr'])
})

test('a picture is a line of its own, with the italic line under it as its caption', () => {
  const [f, p] = parse('![The home page](images/home.webp)\n*Sintel, [CC BY](https://example.com/).*\n\nNext.')
  assert.equal(f.t, 'figure')
  assert.equal(f.name, 'home.webp')
  assert.equal(f.alt, 'The home page')
  assert.equal(plain(f.caption), 'Sintel, CC BY.')
  assert.equal(p.t, 'p')
  assert.deepEqual(parse('![No caption](./images/a-b_1.png)\n\nNext.').map((x) => x.t), ['figure', 'p'])
  const [step] = parse('1. Press it.\n\n   ![The button](images/b.webp)\n2. Done.')
  assert.equal(step.items[0][1].t, 'figure')
  assert.equal(parse('Words\n![Pic](images/p.webp)')[1].t, 'figure')
})

test('only a name in docs/images can be a picture, and nothing else is fetched', () => {
  for (const bad of ['https://evil.example/x.png', '//evil.example/x.png', 'images/../../secret.png', '/api/x.png', 'javascript:alert', 'images/x.svg', 'data:image/png;base64,AA']) {
    const b = parse(`![Words](${bad})`)
    assert.deepEqual(b.map((x) => x.t), ['p'], bad)
    assert.equal(plain(b[0].c), 'Words')
  }
  assert.deepEqual(parse('![](https://evil.example/x.png)'), [])
})

test('only guides and plain web addresses are links', () => {
  assert.deepEqual(target('REPAIR.md#x'), { kind: 'guide', file: 'REPAIR', hash: 'x' })
  assert.deepEqual(target('../SECURITY.md'), { kind: 'guide', file: 'SECURITY', hash: '' })
  assert.deepEqual(target('#here'), { kind: 'here', hash: 'here' })
  assert.deepEqual(target('https://example.com/'), { kind: 'web', href: 'https://example.com/' })
  for (const bad of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,x', '//evil.example', 'vbscript:x', 'file:///etc/passwd', '/settings', 'a b.md']) {
    assert.equal(target(bad), null, bad)
  }
})

test('nothing makes it slow or falls over', () => {
  const t = Date.now()
  parse('*'.repeat(20000))
  parse('['.repeat(5000))
  parse('`'.repeat(5000))
  parse('- a\n'.repeat(5000))
  parse('> '.repeat(2000) + 'x')
  parse(' '.repeat(50) + '- '.repeat(2000))
  assert.ok(Date.now() - t < 5000, `took ${Date.now() - t} ms`)
})
