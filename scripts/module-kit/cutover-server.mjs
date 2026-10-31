// The changes to WittyMediaServer itself when its modules leave for their own
// repositories: scripts/cutover-modules.sh runs this after every module is released.
//
//   node scripts/module-kit/cutover-server.mjs <repository root> <released.json>
//
// released.json maps each repository's name to its tag: {"WittyPlay": "v0.1.1", ...}.
// Each edit says what it expects to find and stops if it isn't there, so a file that
// changed since this was written is noticed here rather than left half-converted.
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const root = process.argv[2]
const released = JSON.parse(readFileSync(process.argv[3], 'utf8'))
const OWNER = 'wittywilla'
const READERS = ['WittyInk', 'WittyEbook', 'WittyPDF', 'WittyComics']
const WEB_HALF = ['WittyPlay', 'WittyIntegratedFileBrowser'] // Go modules the web app also imports

function edit(path, change) {
  const file = join(root, path)
  const raw = readFileSync(file, 'utf8')
  const crlf = raw.includes('\r\n')
  const out = change(raw.replace(/\r\n/g, '\n'))
  writeFileSync(file, crlf ? out.replace(/\n/g, '\r\n') : out)
  console.log(`   edited ${path}`)
}

// Replaces what must be there.
function must(text, find, to, what) {
  const hit = typeof find === 'string' ? text.includes(find) : text.search(find) >= 0
  if (!hit) throw new Error(`expected ${what || find} and it isn't there: this file changed since the cutover was written`)
  return text.replace(find, to)
}

// Takes one step out of a workflow, with the comments above it.
function removeStep(text, name) {
  const lines = text.split('\n')
  let at = lines.findIndex((l) => l.trim() === `- name: ${name}`)
  if (at < 0) throw new Error(`expected a workflow step named ${name}`)
  let end = at + 1
  while (end < lines.length && !/^ {6}(- |# )/.test(lines[end]) && (lines[end].startsWith('        ') || lines[end] === '')) end++
  while (at > 0 && /^ {6}# /.test(lines[at - 1])) at--
  lines.splice(at, end - at)
  return lines.join('\n')
}

const npmName = (repo) => `@${OWNER}/${repo.toLowerCase()}`
const goModule = (repo) => `github.com/${OWNER}/${repo.toLowerCase()}`

// The container build. Node's small image has no git, which github: dependencies can need.
edit('Dockerfile', (t) => {
  t = must(t, 'RUN npm ci --ignore-scripts', '# The modules are packages on GitHub, at the tags in package.json.\nRUN apk add --no-cache git\nRUN npm ci --ignore-scripts')
  t = must(t, /# WittyIntegratedFileBrowser's web half lives beside its Go half\.\n/, '')
  t = t.replace(/^COPY modules\/.*\n/gm, '')
  t = must(t, /# The server builds the disc engines from local source[^\n]*\n# so go\.work isn't needed[^\n]*\n/, '')
  t = must(t, /&& cd modules\/wittyplay && (GOARM="\$\{TARGETVARIANT#v\}" CGO_ENABLED=0 GOOS="\$TARGETOS" GOARCH="\$TARGETARCH" go build -trimpath -ldflags="-s -w" -o \/out\/wittyplay-host) \.\/cmd\/wittyplay-host/,
    `&& $1 github.com/${OWNER}/wittyplay/cmd/wittyplay-host`, 'the wittyplay-host build')
  if (t.includes('modules/')) throw new Error('Dockerfile still names modules/')
  return t
})

// CI. Each module's own repository tests and builds it now.
edit('.github/workflows/ci.yml', (t) => {
  t = removeStep(t, 'Modules (their own go.mod)')
  t = removeStep(t, 'Modules build on their own (GOWORK=off)')
  t = must(t, /( {6}# Every manifest is sound[^\n]*\n {6}# server version[^\n]*\n {6}- name: Module manifests and version ranges\n {8}run: \|\n)(?: {10}[^\n]*\n)+/,
    `$1          server="$(go run ./cmd/wms version | awk '{print $2}')"
          # The tool wants each manifest in a folder named for its module.
          tmp="$(mktemp -d)"
          for m in $(go list -m -f '{{.Path}}' all | grep '^github.com/${OWNER}/' | grep -v '/wittymodule$$'); do
            cp -r "$(go list -m -f '{{.Dir}}' "$m")" "$tmp/\${m##*/}"
          done
          for f in web/module-manifests/*.json; do
            mkdir "$tmp/$(basename "$f" .json)" && cp "$f" "$tmp/$(basename "$f" .json)/module.json"
          done
          chmod -R u+w "$tmp"
          go run github.com/${OWNER}/wittymodule/cmd/wittymodule check -server "$server" "$tmp"/*/
`, 'the manifests step')
  t = must(t, /( {12}flags=""; case \$t in freebsd[^\n]*\n) {12}\(cd modules\/wittyplay && (GOOS=[^\n]*?) -o \/dev\/null \.\/cmd\/wittyplay-host\) \|\| exit 1\n {12}\(cd modules\/wittyphotos[^\n]*\n/,
    `$1            $2 -o /dev/null github.com/${OWNER}/wittyplay/cmd/wittyplay-host || exit 1\n`, 'the cross-compile lines')
  t = must(t, /( {6}- run: cd web && npm ci[^\n]*\n)/, '$1      - name: The module manifests the server embeds match the modules in package.json\n        run: git diff --exit-code web/module-manifests\n', 'the web build')
  const left = t.split('\n').filter((l) => /(^|[^a-z/.])modules\/witty|\.\/modules\/witty|go\.work/.test(l))
  if (left.length) throw new Error('ci.yml still names module folders:\n' + left.join('\n'))
  return t
})

edit('.github/workflows/security.yml', (t) => must(t, '      - go.work\n', ''))

edit('.github/dependabot.yml', (t) => {
  t = must(t, 'directories: ["/", "/modules/*"]', 'directory: /')
  return must(t, '"/apps/wittyreader", ', '')
})

edit('scripts/build-release.sh', (t) => must(t,
  /\(cd modules\/wittyplay && (CGO_ENABLED=0 GOOS="\$os" GOARCH="\$arch" GOARM=7 go build -trimpath [^\n]*?) -o "\$OLDPWD\/\$out\/wittyplay-host\$ext" \.\/cmd\/wittyplay-host\)/,
  `$1 -o "$out/wittyplay-host$ext" github.com/${OWNER}/wittyplay/cmd/wittyplay-host`, 'the wittyplay-host build'))

edit('.gitignore', (t) => {
  if (t.includes('go.work')) throw new Error('.gitignore already names go.work')
  return t.replace(/\n*$/, '\n\n# For developing a server and modules together: scripts/dev-modules.sh\ngo.work\ngo.work.sum\n')
})

// Finding the copy of FFmpeg for working on this project: go.work is gone, so another
// file that only a source checkout has says this is one.
edit('internal/ffmpeg/ffmpeg.go', (t) => must(t, 'os.Stat("go.work")', 'os.Stat("gomod.go")'))

// The web app takes its modules from node_modules, and the server's copies of their
// manifests from a folder the web build keeps in step, committed so that building the
// server doesn't need Node.
edit('web/embed.go', (t) => {
  t = must(t, '//go:embed src/modules/*/module.json', '//go:embed module-manifests/*.json')
  return must(t, 'fs.Glob(moduleManifests, "src/modules/*/module.json")', 'fs.Glob(moduleManifests, "module-manifests/*.json")')
})

mkdirSync(join(root, 'web/module-manifests'), { recursive: true })
writeFileSync(join(root, 'web/scripts/sync-module-manifests.mjs'), `// Keeps web/module-manifests in step with the modules package.json asks for: the server
// embeds these (web/embed.go), and shouldn't need Node to build. CI fails if they differ
// from what is committed. Go modules with a web half have their manifest in Go.
import { copyFileSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs'
import { join, resolve } from 'node:path'

const web = resolve(import.meta.dirname, '..')
const installed = join(web, 'node_modules/@${OWNER}')
const out = join(web, 'module-manifests')
rmSync(out, { recursive: true, force: true })
mkdirSync(out)
for (const id of readdirSync(installed)) {
  const manifest = join(installed, id, 'module.json')
  if (existsSync(manifest) && !existsSync(join(installed, id, 'go.mod'))) copyFileSync(manifest, join(out, id + '.json'))
}
`)
for (const gone of ['web/scripts/check-modules.mjs', 'web/scripts/build-module-bundle.mjs']) rmSync(join(root, gone), { force: true })

edit('web/package.json', (t) => {
  const p = JSON.parse(t)
  p.scripts.build = p.scripts.build.replace('node scripts/check-modules.mjs', 'node scripts/sync-module-manifests.mjs')
  if (!p.scripts.build.includes('sync-module-manifests')) throw new Error('expected the build script to run check-modules.mjs')
  delete p.scripts['check:modules']
  // Reader modules, and the Go modules with a web half.
  for (const [repo, tag] of Object.entries(released)) {
    if (WEB_HALF.includes(repo) || READERS.includes(repo)) p.dependencies[npmName(repo)] = `github:${OWNER}/${repo}#${tag}`
  }
  p.dependencies = Object.fromEntries(Object.entries(p.dependencies).sort(([a], [b]) => a.localeCompare(b)))
  return JSON.stringify(p, null, 2) + '\n'
})

edit('web/tsconfig.json', (t) => {
  const c = JSON.parse(t)
  for (const k of Object.keys(c.compilerOptions.paths)) if (k.startsWith('@wittywilla/')) delete c.compilerOptions.paths[k]
  // Also finds them from a module cloned beside this checkout (scripts/dev-modules.sh).
  c.compilerOptions.paths = { '@wittywilla/*': ['./node_modules/@wittywilla/*'], ...c.compilerOptions.paths }
  c.include = c.include.filter((i) => !i.includes('modules/'))
  return JSON.stringify(c, null, 2) + '\n'
})

edit('web/vite.config.ts', (t) => {
  t = must(t, "import { readFileSync, readdirSync, writeFileSync } from 'node:fs'", "import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'")
  t = must(t, "new URL('./src/modules/', import.meta.url)", "new URL('./node_modules/@wittywilla/', import.meta.url)")
  t = must(t, 'const webModules = readdirSync(modulesDir).flatMap((d) => {\n  try {\n',
    "const webModules = readdirSync(modulesDir).flatMap((d) => {\n  // The Go modules' web halves aren't modules of the web app.\n  if (!existsSync(modulesDir + d + '/module.json') || existsSync(modulesDir + d + '/go.mod')) return []\n  try {\n")
  t = must(t, /  \/\/ Plugins \(src\/modules\/\*\) are imported[\s\S]*?  build: \{ outDir/, `  // The modules are packages in node_modules, as TypeScript source. dedupe also resolves
  // their imports from here for one cloned beside this checkout (scripts/dev-modules.sh).
  resolve: {
    dedupe: ['react', 'react-dom', 'pdfjs-dist', ${[...READERS_NPM(), ...WEB_HALF_NPM()].map((n) => `'${n}'`).join(', ')}],
  },
  optimizeDeps: { exclude: [${[...READERS_NPM(), ...WEB_HALF_NPM()].map((n) => `'${n}'`).join(', ')}] },
  build: { outDir`, 'the module aliases')
  t = must(t, "// the module's web half lives outside web/", '// a module cloned beside this checkout')
  return t
})

// Tests that read a module's folder read the released source instead.
edit('internal/bdsession/bdj_test.go', (t) => must(t, 'os.ReadFile("../../modules/wittybluray/bdj/testdata/00000.jar")', 'os.ReadFile(testJar())'))
writeFileSync(join(root, 'internal/bdsession/bdj_jar_test.go'), `package bdsession

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// testJar is the test application that comes with the WittyBluray this server requires.
func testJar() string {
	dir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "${goModule('WittyBluray')}").Output()
	if err != nil {
		return ""
	}
	return filepath.Join(strings.TrimSpace(string(dir)), "bdj", "testdata", "00000.jar")
}
`)

edit('internal/modules/modules_test.go', (t) => {
  t = must(t, '[]string{"run", "./modules/wittymodule/cmd/wittymodule"}', `[]string{"run", "${goModule('WittyModule')}/cmd/wittymodule"}`)
  t = must(t, `	tool("check", "modules/wittyphotos")
	tool("pack", "-out", out, "-targets", runtime.GOOS+"/"+runtime.GOARCH, "modules/wittyphotos")
	raw, err := os.ReadFile(filepath.Join(root, "modules", "wittyphotos", "module.json"))
`, `	// The released source, in a folder named for the module, as its own repository is.
	list := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "${goModule('WittyPhotos')}")
	list.Dir = root
	released, err := list.Output()
	if err != nil {
		t.Fatal(err)
	}
	photos := filepath.Join(t.TempDir(), "wittyphotos")
	if err := os.CopyFS(photos, os.DirFS(strings.TrimSpace(string(released)))); err != nil {
		t.Fatal(err)
	}
	tool("check", photos)
	tool("pack", "-out", out, "-targets", runtime.GOOS+"/"+runtime.GOARCH, photos)
	raw, err := os.ReadFile(filepath.Join(photos, "module.json"))
`)
  return t
})

// Raising the server's version checked the modules' folders. CI checks their ranges now.
edit('scripts/bump-server.sh', (t) => must(t, /# A server past a module's maxServer[^\n]*\nmanifests=[\s\S]*?\n\s*exit 1\n\s*\}\nfi\n/,
  "# A server past a module's maxServer would quietly run its built-in copy: CI fails on it.\n", "the check of the modules' folders"))

// A comment that would otherwise point at a folder that's gone.
edit('web/src/pages/Games.tsx', (t) => t.replace('(modules/wittyplay/web)', "(the web folder of WittyPlay's repository)"))

// An app in this repository that uses the modules, if one has arrived since this was
// written: its dependencies move to the released tags, and anything that reached into a
// module's folder is reported, since only a person can say what it should do now.
const apps = join(root, 'apps')
if (existsSync(apps)) {
  for (const app of readdirSync(apps)) {
    const pkg = join('apps', app, 'package.json')
    if (!existsSync(join(root, pkg))) continue
    edit(pkg, (t) => {
      const p = JSON.parse(t)
      for (const [repo, tag] of Object.entries(released)) {
        for (const group of ['dependencies', 'devDependencies']) {
          if (p[group]?.[npmName(repo)]) p[group][npmName(repo)] = `github:${OWNER}/${repo}#${tag}`
        }
      }
      return JSON.stringify(p, null, 2) + '\n'
    })
    for (const f of readdirSync(join(apps, app))) {
      if (!/^(vite\.config\.|tsconfig)/.test(f)) continue
      if (/\.\.\/(\.\.\/)?(modules|web\/src\/modules)/.test(readFileSync(join(apps, app, f), 'utf8'))) {
        console.log(`   NEEDS A LOOK: apps/${app}/${f} reaches into a module's folder, which is gone`)
      }
    }
  }
}

function READERS_NPM() { return READERS.map(npmName) }
function WEB_HALF_NPM() { return WEB_HALF.map(npmName) }
