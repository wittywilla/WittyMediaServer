// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a sign-in may go in an address: web/tests/medialinks.test.ts, the real api.ts in Node.
// Nothing else is needed.
import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'

const entry = fileURLToPath(new URL('../tests/medialinks.test.ts', import.meta.url))
const dir = mkdtempSync(join(tmpdir(), 'test-links-'))
await build({ configFile: false, logLevel: 'warn', publicDir: false, ssr: { noExternal: true }, build: { ssr: entry, outDir: dir, emptyOutDir: true, rollupOptions: { output: { entryFileNames: 'medialinks.test.mjs' } } } })
const r = spawnSync(process.execPath, ['--test', '--test-timeout=30000', '--test-force-exit', join(dir, 'medialinks.test.mjs')], { stdio: 'inherit' })
rmSync(dir, { recursive: true, force: true })
process.exit(r.status === 0 ? 0 : 1)
