// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// web/tests/moduleCards: a module's card Provider and its screens, rendered by React as the app
// draws them, with the module's bundle, the server and the module host replaced by
// tests/moduleCards/stubs.tsx. Nothing else is needed.
import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'
import react from '@vitejs/plugin-react'

const here = dirname(fileURLToPath(import.meta.url))
const entry = join(here, '../tests/moduleCards/moduleCards.test.tsx')
const stubs = join(here, '../tests/moduleCards/stubs.tsx')
const registry = join(here, '../src/moduleRegistry.tsx')
const stubbed = new Set(['./api', './moduleBundles', './moduleHost', './components/ModuleMissing'])

const dir = mkdtempSync(join(tmpdir(), 'test-module-cards-'))
await build({
  configFile: false, logLevel: 'error', publicDir: false,
  plugins: [react(), { name: 'stubs', enforce: 'pre', resolveId: (src, importer) => (importer === registry && stubbed.has(src) ? stubs : null) }],
  ssr: { noExternal: true },
  build: { ssr: entry, outDir: dir, rollupOptions: { output: { entryFileNames: 'moduleCards.test.mjs' } } },
})
const r = spawnSync(process.execPath, ['--test', '--test-timeout=30000', join(dir, 'moduleCards.test.mjs')], { stdio: 'inherit' })
rmSync(dir, { recursive: true, force: true })
process.exit(r.status === 0 ? 0 : 1)
