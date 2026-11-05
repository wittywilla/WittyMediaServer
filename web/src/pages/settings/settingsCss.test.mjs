import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

// settings-page.css loads with the Settings page only. A class in it that a page elsewhere uses
// would look right there only after Settings had been opened, so every class it names has to be
// used by no file outside the Settings page. The settings files other pages open count as outside.
const src = fileURLToPath(new URL('../../', import.meta.url))
const shared = new Set(['pages/settings/ui.tsx', 'pages/settings/jump.ts', 'pages/settings/MetadataOptional.tsx', 'pages/settings/store/storeApi.ts', 'pages/settings/Legal.tsx', 'pages/settings/GamingCustom.tsx', 'pages/settings/Backup.tsx'])
const inside = (r) => r === 'pages/SettingsPage.tsx' || (r.startsWith('pages/settings/') && !shared.has(r) && !r.startsWith('pages/settings/logs/'))

function files(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((f) => (f.isDirectory() ? files(join(dir, f.name)) : /\.(tsx?|mjs)$/.test(f.name) && !f.name.endsWith('.test.mjs') ? [join(dir, f.name)] : []))
}

test('what only Settings loads is used only by Settings', () => {
  const css = readFileSync(join(src, 'pages/settings/settings-page.css'), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')
  const outside = files(src).filter((f) => !inside(relative(src, f).split(sep).join('/'))).map((f) => readFileSync(f, 'utf8')).join('\n')
  const used = (c) => new RegExp('(^|[^\\w-])' + c + '($|[^\\w-])').test(outside)
  // Each selector has to name at least one class nothing outside uses.
  const rules = [...css.matchAll(/([^{}]+)\{[^{}]*\}/g)].map((m) => m[1].trim()).filter((h) => !h.startsWith('@'))
  assert.ok(rules.length > 100)
  for (const head of rules) {
    for (const sel of head.split(',')) {
      const classes = [...sel.matchAll(/\.([a-zA-Z][\w-]*)/g)].map((m) => m[1])
      assert.ok(classes.some((c) => !used(c)), `"${sel.trim()}" in settings-page.css styles something used outside Settings: move the rule to settings.css`)
    }
  }
})
