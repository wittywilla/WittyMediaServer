#!/usr/bin/env node
// Builds a checkout against the module repositories beside it, with nothing published. Run it
// from any repository or worktree; it writes only files that git ignores.
//
//   node scripts/dev-modules.mjs                       this checkout, against <repos>/<Repo>
//   node scripts/dev-modules.mjs WittyPlay=<folder>    and use that folder for WittyPlay (a worktree)
//   node scripts/dev-modules.mjs WittyPlay=            back to <repos>/WittyPlay
//   node scripts/dev-modules.mjs --repos /mnt/d/projects   where the repositories are (a clone elsewhere)
//   node scripts/dev-modules.mjs --root                the repositories' folder itself: its go.work,
//                                                      its node_modules, and a copy of this script
//   node scripts/dev-modules.mjs --off                 remove what it made in this checkout
//
// What it writes in a checkout:
//   go.work, go.work.sum    "use" this checkout and every module repository. Go refuses one module
//                           listed twice, so one go.work can't hold a repository and its worktrees:
//                           each checkout needs its own, and this makes it.
//   node_modules links      @wittywilla/<id> -> its repository, in web/ for the server
//   .dev-modules            what was chosen above, kept for the next run
//
// A checkout that tracks go.work is from before the modules left the server: left alone.
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const SERVER = 'WittyMediaServer'
const here = fileURLToPath(import.meta.url)

const args = process.argv.slice(2)
const quiet = take('--quiet')
const webOnly = take('--web-only')
const off = take('--off')
const rootMode = take('--root')
let reposArg = ''
const at = args.indexOf('--repos')
if (at >= 0) { reposArg = args[at + 1] || ''; args.splice(at, 2) }

function take(flag) {
  const i = args.indexOf(flag)
  if (i < 0) return false
  args.splice(i, 1)
  return true
}
const say = (s) => { if (!quiet) console.log(s) }
const die = (s) => { console.error(s); process.exit(1) }
const git = (dir, ...a) => execFileSync('git', a, { cwd: dir, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim()
const slash = (p) => p.split(sep).join('/')

function findRepos(from) {
  if (reposArg) return resolve(reposArg)
  if (process.env.WMS_MODULE_REPOS) return resolve(process.env.WMS_MODULE_REPOS)
  for (let d = from; ; d = dirname(d)) {
    if (existsSync(join(d, 'WittyModule', 'go.mod'))) return d
    if (dirname(d) === d) return ''
  }
}

// Every repository in the folder: its Go modules and its npm package, by looking.
function scan(repos, overrides) {
  const out = []
  for (const name of readdirSync(repos).sort()) {
    if (name === SERVER || name.startsWith(SERVER + '-')) continue
    const dir = overrides[name] || join(repos, name)
    if (!existsSync(join(repos, name, '.git')) && !overrides[name]) continue
    if (existsSync(dir)) out.push({ name, dir, ...describe(dir) })
  }
  return out
}

function describe(dir) {
  const goDirs = []
  const candidates = [dir, ...readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory() && !e.name.startsWith('.') && e.name !== 'node_modules' && e.name !== 'testdata')
    .map((e) => join(dir, e.name))]
  for (const c of candidates) if (existsSync(join(c, 'go.mod'))) goDirs.push(c)
  let npm = ''
  if (existsSync(join(dir, 'package.json'))) {
    const n = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8')).name || ''
    if (n.startsWith('@wittywilla/')) npm = n.slice('@wittywilla/'.length)
  }
  return { goDirs, npm }
}

const goModulePath = (d) => /^module\s+(\S+)/m.exec(readFileSync(join(d, 'go.mod'), 'utf8'))?.[1] || ''

function versionAtLeast(a, b) {
  const n = (v) => v.split('.').map(Number)
  const [x, y] = [n(a), n(b)]
  for (let i = 0; i < 3; i++) if ((x[i] || 0) !== (y[i] || 0)) return (x[i] || 0) > (y[i] || 0)
  return true
}

// Each version of a module that any go.mod here requires points at its folder, so Go doesn't go
// looking for versions that aren't published. It has to name the version: Go refuses a workspace
// module replaced at all versions.
function workFile(atDir, goDirs) {
  const local = new Map(goDirs.map((d) => [goModulePath(d), d]))
  const wanted = new Set()
  let go = '1.26.0'
  let toolchain = ''
  for (const d of goDirs) {
    const mod = readFileSync(join(d, 'go.mod'), 'utf8')
    const g = /^go\s+(\S+)/m.exec(mod)?.[1]
    if (g && versionAtLeast(g, go)) go = g
    const t = /^toolchain\s+go(\S+)/m.exec(mod)?.[1]
    if (t && (!toolchain || versionAtLeast(t, toolchain))) toolchain = t
    for (const m of mod.matchAll(/^\s*(?:require\s+)?(github\.com\/wittywilla\/\S+)\s+(v\S+)/gm)) {
      if (local.has(m[1])) wanted.add(m[1] + ' ' + m[2])
    }
  }
  const path = (d) => {
    if (resolve(d) === resolve(atDir)) return '.'
    const rel = relative(atDir, d)
    // A relative path works from Windows and from WSL. Across drives or file systems there is none.
    if (isAbsolute(rel) || rel.startsWith('..' + sep + '..' + sep + '..' + sep + '..' + sep + '..')) return slash(resolve(d))
    return rel.startsWith('..') ? slash(rel) : './' + slash(rel)
  }
  const lines = [
    '// Made by scripts/dev-modules.mjs, for building against local module repositories. Git ignores it.',
    '// Run that again rather than editing this, and never `go work use` here.',
    `go ${go}`, '',
  ]
  if (toolchain) lines.splice(3, 0, `toolchain go${toolchain}`)
  lines.push('use (', ...goDirs.map((d) => `\t${path(d)}`), ')', '')
  if (wanted.size) {
    lines.push('replace (')
    for (const w of [...wanted].sort()) {
      const [m, v] = w.split(' ')
      lines.push(`\t${m} ${v} => ${path(local.get(m))}`)
    }
    lines.push(')', '')
  }
  return lines.join('\n')
}

function isLink(p) {
  try { return lstatSync(p).isSymbolicLink() } catch { return false }
}

function makeLink(path, target) {
  if (isLink(path)) {
    let same = false
    try { same = realpathSync(path) === realpathSync(target) } catch { /* a dangling link */ }
    if (same) return
    unlinkSync(path)
  } else if (existsSync(path)) {
    rmSync(path, { recursive: true, force: true }) // npm's own copy of it, from a tag
  }
  mkdirSync(dirname(path), { recursive: true })
  symlinkSync(resolve(target), path, process.platform === 'win32' ? 'junction' : 'dir')
}

function linkPackages(nodeModulesParent, repos, only) {
  let n = 0
  for (const r of repos) {
    if (!r.npm || (only && !only.has(r.npm))) continue
    makeLink(join(nodeModulesParent, 'node_modules', '@wittywilla', r.npm), r.dir)
    n++
  }
  return n
}

function readChoices(top) {
  const f = join(top, '.dev-modules')
  const c = { repos: '', overrides: {} }
  if (!existsSync(f)) return c
  for (const line of readFileSync(f, 'utf8').split(/\r?\n/)) {
    const [k, ...v] = line.split('=')
    if (!k || k.startsWith('#')) continue
    if (k === 'repos') c.repos = v.join('=')
    else c.overrides[k] = v.join('=')
  }
  return c
}

function writeChoices(top, c) {
  const lines = []
  if (c.repos) lines.push(`repos=${c.repos}`)
  for (const [k, v] of Object.entries(c.overrides)) lines.push(`${k}=${v}`)
  const f = join(top, '.dev-modules')
  if (lines.length) writeFileSync(f, lines.join('\n') + '\n')
  else rmSync(f, { force: true })
}

function setUpRoot(repos) {
  if (resolve(here) !== resolve(repos, 'dev-modules.mjs')) copyFileSync(here, join(repos, 'dev-modules.mjs'))
  const list = scan(repos, {})
  const goDirs = list.flatMap((r) => r.goDirs)
  writeFileSync(join(repos, 'go.work'), workFile(repos, goDirs))
  const linked = linkPackages(repos, list)
  say(`${repos}: go.work with ${goDirs.length} modules, ${linked} npm links`)
}

function main() {
  const start = process.cwd()
  if (rootMode) {
    const repos = findRepos(start)
    if (!repos) die('no module repositories found: say where with --repos')
    setUpRoot(repos)
    return
  }
  let top
  try { top = git(start, 'rev-parse', '--show-toplevel') } catch { die('run this inside a repository or worktree') }
  top = resolve(top)

  let tracked = false
  try { git(top, 'ls-files', '--error-unmatch', 'go.work'); tracked = true } catch { /* the usual case */ }
  if (tracked) { say('this checkout tracks go.work, so it is from before the modules left the server: nothing to do'); return }

  const choices = readChoices(top)
  if (off) {
    for (const f of ['go.work', 'go.work.sum', '.dev-modules']) rmSync(join(top, f), { force: true })
    for (const p of [join(top, 'web', 'node_modules', '@wittywilla'), join(top, 'node_modules', '@wittywilla')]) {
      if (!existsSync(p)) continue
      for (const e of readdirSync(p)) if (isLink(join(p, e))) unlinkSync(join(p, e))
    }
    say('removed what dev-modules made here')
    return
  }
  for (const a of args) {
    const [k, ...v] = a.split('=')
    if (!/^[A-Za-z0-9._-]+$/.test(k) || !a.includes('=')) die(`not a choice: ${a} (want Repo=<folder>)`)
    const value = v.join('=')
    if (value) {
      if (!existsSync(value)) die(`${k}: there is no ${value}`)
      choices.overrides[k] = resolve(value)
    } else delete choices.overrides[k]
  }
  if (reposArg) choices.repos = resolve(reposArg)
  if (!reposArg && choices.repos) reposArg = choices.repos
  const repos = findRepos(top)
  if (!repos) {
    die('no module repositories found. In a clone outside them, say where: node scripts/dev-modules.mjs --repos /mnt/d/projects')
  }
  writeChoices(top, choices)

  const list = scan(repos, choices.overrides)
  const isServer = existsSync(join(top, 'cmd', 'wittymediaserver'))

  if (!webOnly) {
    const own = describe(top)
    const ownPaths = new Set(own.goDirs.map(goModulePath))
    const others = list.flatMap((r) => r.goDirs).filter((d) => !ownPaths.has(goModulePath(d)) && !resolve(d).startsWith(top + sep) && resolve(d) !== top)
    if (own.goDirs.length) {
      writeFileSync(join(top, 'go.work'), workFile(top, [...own.goDirs, ...others]))
      say(`go.work: this checkout and ${others.length} other modules, from ${repos}`)
    }
  }
  if (isServer) {
    const n = linkPackages(join(top, 'web'), list)
    say(`web/node_modules: ${n} links to module repositories`)
  } else if (existsSync(join(top, 'package.json'))) {
    const p = JSON.parse(readFileSync(join(top, 'package.json'), 'utf8'))
    const uses = new Set(Object.keys({ ...p.dependencies, ...p.devDependencies, ...p.peerDependencies })
      .filter((d) => d.startsWith('@wittywilla/')).map((d) => d.slice('@wittywilla/'.length)))
    const n = linkPackages(top, list.filter((r) => resolve(r.dir) !== top), uses)
    if (n) say(`node_modules: ${n} links to module repositories`)
  }
  if (Object.keys(choices.overrides).length) say(`using: ${JSON.stringify(choices.overrides)}`)
}

main()
