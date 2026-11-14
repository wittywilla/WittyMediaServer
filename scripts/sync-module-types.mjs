#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Brings web/src/moduletypes up to date: what the page's TypeScript knows of each module's part in
// the browser, written out as declarations. Types only, so nothing of a module's code reaches the
// app, and a clean clone can build with no module's repository beside it. Run it after a module's
// host interface changes:
//
//   node scripts/sync-module-types.mjs [folder with the module repositories]
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const server = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const web = join(server, 'web')
const repos = resolve(process.argv[2] || join(server, '..'))
const out = join(web, 'src/moduletypes')
const tsc = join(web, 'node_modules/typescript/bin/tsc')

const modules = {
  wittyink: 'WittyInk', wittyebook: 'WittyEbook', wittypdf: 'WittyPDF', wittycomics: 'WittyComics', wittyvisualizer: 'WittyVisualizer',
  wittyplay: 'WittyPlay', wittyintegratedfilebrowser: 'WittyIntegratedFileBrowser',
}

const entryOf = (dir) => {
  for (const f of ['web/index.ts', 'web/index.tsx', 'index.ts', 'index.tsx']) if (existsSync(join(dir, f))) return join(dir, f)
  throw new Error(`${dir} has no entry`)
}

const walk = (d) => readdirSync(d).flatMap((n) => {
  const p = join(d, n)
  return statSync(p).isDirectory() ? walk(p) : [p]
})

rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })
for (const [id, repo] of Object.entries(modules)) {
  const dir = join(repos, repo)
  const tmp = join(web, 'node_modules/.moduletypes', id)
  rmSync(tmp, { recursive: true, force: true })
  const config = join(tmp, 'tsconfig.json')
  mkdirSync(tmp, { recursive: true })
  writeFileSync(config, JSON.stringify({
    compilerOptions: {
      declaration: true, emitDeclarationOnly: true, noEmitOnError: false, skipLibCheck: true, target: 'ES2022', module: 'ESNext', moduleResolution: 'bundler',
      jsx: 'react-jsx', strict: false, types: ['vite/client'], typeRoots: [join(web, 'node_modules/@types')], outDir: join(tmp, 'out'), rootDir: repos,
      paths: {
        react: [join(web, 'node_modules/@types/react')], 'react/*': [join(web, 'node_modules/@types/react/*')], 'react-dom': [join(web, 'node_modules/@types/react-dom')],
        'vite/client': [join(web, 'node_modules/vite/client')], 'pdfjs-dist': [join(web, 'node_modules/pdfjs-dist')], 'pdfjs-dist/*': [join(web, 'node_modules/pdfjs-dist/*')],
        ...Object.fromEntries(Object.entries(modules).map(([m, r]) => [`@wittywilla/${m}`, [entryOf(join(repos, r))]])),
      },
    },
    files: [entryOf(dir)],
  }))
  // A file that doesn't compile here still writes what it can: these are only declarations.
  try { execFileSync(process.execPath, [tsc, '-p', config], { stdio: 'pipe' }) } catch { /* see above */ }
  const made = join(tmp, 'out', repo)
  if (!existsSync(made)) throw new Error(`no declarations for ${id}`)
  renameSync(made, join(out, id))
  // Every module these declarations name is one of these folders.
  for (const f of walk(join(out, id))) {
    let text = readFileSync(f, 'utf8').replace(/\r\n/g, '\n')
    text = text.replace(/(from |import\()(['"])\.\.\/(?:\.\.\/)*(Witty[A-Za-z]+)\/([^'"]*)\2/g, (all) => all)
    writeFileSync(f, text)
  }
  const entry = relative(dir, entryOf(dir)).replace(/\\/g, '/').replace(/\.tsx?$/, '')
  writeFileSync(join(out, id, 'package.json'), JSON.stringify({ name: `@wittywilla/${id}`, types: `./${entry}.d.ts` }) + '\n')
  writeFileSync(join(out, `${id}.d.ts`), `export * from './${id}/${entry}'\nexport { default } from './${id}/${entry}'\n`)
  console.log('types of', id)
}
rmSync(join(web, 'node_modules/.moduletypes'), { recursive: true, force: true })
