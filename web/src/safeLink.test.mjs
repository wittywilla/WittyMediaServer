import test from 'node:test'
import assert from 'node:assert/strict'
import { safeLink } from './safeLink.ts'

test('web addresses stay links', () => {
  assert.equal(safeLink('https://example.org/a?b=1#c'), 'https://example.org/a?b=1#c')
  assert.equal(safeLink('http://example.org/'), 'http://example.org/')
  assert.equal(safeLink('HTTPS://Example.org/'), 'https://example.org/')
})

test('nothing else does', () => {
  for (const u of [
    'javascript:alert(1)', 'JaVaScRiPt:alert(1)', ' javascript:alert(1)', 'java\tscript:alert(1)', 'java\nscript:alert(1)', '\x01javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>', 'vbscript:x', 'file:///etc/passwd', 'blob:http://example.org/x',
    'ms-msdt:/id x', 'search-ms:query=x', 'mailto:a@example.org', 'tel:1', 'about:blank',
    '/settings', '//example.org/a', 'example.org', '', null, undefined,
  ]) assert.equal(safeLink(u), '', String(u))
})
