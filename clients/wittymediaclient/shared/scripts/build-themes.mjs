#!/usr/bin/env node
// Turns the server's theme folders into plain data for clients that can't use
// CSS. Run with --check in CI to fail when themes.json is out of date.
import { readFileSync, readdirSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const themesDir = join(here, '..', '..', '..', '..', 'themes')
const outFile = join(here, '..', 'themes.json')

// The CSS variables a native client needs, with the name it uses for them.
const WANTED = {
  '--bg': 'background', '--bg-2': 'panel', '--bg-3': 'card', '--bg-4': 'border',
  '--fg': 'text', '--fg-2': 'textSecondary', '--fg-3': 'textHint',
  '--accent': 'accent', '--accent-fg': 'onAccent', '--accent-text': 'accentText',
  '--danger': 'danger', '--ok': 'ok',
}
const HEX = /^#[0-9a-fA-F]{6}$/

function colours(css, inherited) {
  const out = { ...inherited }
  for (const [, name, value] of css.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    const key = WANTED[name]
    if (key && HEX.test(value.trim())) out[key] = value.trim().toLowerCase()
  }
  return out
}

const read = (id) => readFileSync(join(themesDir, id, 'theme.css'), 'utf8')
const base = colours(read('default'), {})
const themes = readdirSync(themesDir, { withFileTypes: true })
  .filter((d) => d.isDirectory() && existsSync(join(themesDir, d.name, 'theme.json')))
  .map((d) => {
    const meta = JSON.parse(readFileSync(join(themesDir, d.name, 'theme.json'), 'utf8'))
    return { id: meta.id, name: meta.name, scheme: meta.scheme, colours: colours(read(d.name), base) }
  })
  .sort((a, b) => a.id.localeCompare(b.id))

for (const t of themes) {
  for (const key of Object.values(WANTED)) {
    if (!t.colours[key]) throw new Error(`theme ${t.id}: no value for ${key}`)
  }
}

const text = JSON.stringify({ defaultTheme: 'default', themes }, null, 2) + '\n'
if (process.argv.includes('--check')) {
  if (!existsSync(outFile) || readFileSync(outFile, 'utf8').replace(/\r\n/g, '\n') !== text) {
    console.error('shared/themes.json is out of date: run node shared/scripts/build-themes.mjs')
    process.exit(1)
  }
} else {
  writeFileSync(outFile, text)
  console.log(`wrote ${themes.length} themes`)
}
