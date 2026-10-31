// Runs WittyPlay's browser tests, which feed simulated gamepads to the real code in
// plain Node: esbuild (already here for the build) puts each together, and Node's own
// test runner runs it. Nothing extra to install.
import { spawnSync } from 'node:child_process'
import { readdirSync, rmSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { build } from 'esbuild'

const dir = fileURLToPath(new URL('../../modules/wittyplay/web/tests/', import.meta.url))
let failed = false
for (const f of readdirSync(dir).filter((n) => n.endsWith('.test.ts'))) {
  const out = dir + f.replace(/\.ts$/, '.tmp.mjs') // beside the test, as it finds its data from there
  await build({ entryPoints: [dir + f], outfile: out, bundle: true, format: 'esm', platform: 'node', logLevel: 'warning', jsx: 'automatic' })
  const r = spawnSync(process.execPath, ['--test', '--test-timeout=30000', '--test-force-exit', out], { stdio: 'inherit' })
  rmSync(out, { force: true })
  if (r.status !== 0) failed = true
}
process.exit(failed ? 1 : 0)
