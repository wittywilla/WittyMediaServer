// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// Compresses each script and stylesheet, and keeps only that: the server sends it as it is to
// browsers that take it, with no compressing work of its own, and unpacks it for any that don't
// (see spa() in internal/server). An install is 5 MB smaller than with both copies.
function precompress(): Plugin {
  let dir = ''
  return {
    name: 'precompress',
    apply: 'build',
    configResolved(config) {
      // Whatever output folder the build was given (build.outDir or --outDir).
      dir = path.join(path.resolve(config.root, config.build.outDir), 'assets')
    },
    closeBundle() {
      if (!dir || !existsSync(dir)) return
      for (const f of readdirSync(dir)) {
        if (!/\.(js|mjs|css)$/.test(f)) continue
        const file = path.join(dir, f)
        const raw = readFileSync(file)
        if (raw.length > 1024) {
          writeFileSync(file + '.gz', gzipSync(raw, { level: 9 }))
          rmSync(file)
        }
      }
    },
  }
}

// pdf.js fetches its image decoders and colour profile by name, from a folder it
// is told about, so they can't go through the bundler. The folder carries pdf.js's
// version because everything under assets/ is cached for good. The script engine
// for PDF scripts stays behind: PDF scripts never run here.
const pdfVersion = String(JSON.parse(readFileSync(new URL('./node_modules/pdfjs-dist/package.json', import.meta.url), 'utf8')).version)
function pdfAssets(): Plugin {
  let dir = ''
  return {
    name: 'pdf-assets',
    apply: 'build',
    configResolved(config) {
      // Whatever output folder the build was given (build.outDir or --outDir).
      dir = path.join(path.resolve(config.root, config.build.outDir), 'assets', `pdfjs-${pdfVersion}`)
    },
    closeBundle() {
      if (!dir) return
      for (const [from, match] of [['wasm', /^(openjpeg|jbig2|qcms).*\.(wasm|js)$/], ['iccs', /\.icc$/]] as const) {
        const src = fileURLToPath(new URL(`./node_modules/pdfjs-dist/${from}/`, import.meta.url))
        const dst = path.join(dir, from)
        mkdirSync(dst, { recursive: true })
        for (const f of readdirSync(src)) if (match.test(f)) copyFileSync(src + f, path.join(dst, f))
      }
    },
  }
}

// dist/.gitkeep is tracked so `//go:embed all:dist` in web/embed.go compiles on a clean clone,
// before any web build. emptyOutDir wipes it, so put it back once the build is written.
function keepGitkeep(): Plugin {
  let dir = ''
  return {
    name: 'keep-gitkeep',
    apply: 'build',
    configResolved(config) {
      // Whatever output folder the build was given (build.outDir or --outDir).
      dir = path.resolve(config.root, config.build.outDir)
    },
    closeBundle() {
      if (dir && existsSync(dir)) writeFileSync(path.join(dir, '.gitkeep'), '')
    },
  }
}

export default defineConfig({
  plugins: [react(), precompress(), pdfAssets(), keepGitkeep()],
  define: { __WEB_MODULES__: JSON.stringify([]), __PDF_ASSETS__: JSON.stringify(`/assets/pdfjs-${pdfVersion}/`) },
  // The browsers this app has always reached, TVs included, whatever the bundler's default is now.
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false, target: ['es2020', 'edge88', 'firefox78', 'chrome87', 'safari14'] },
  server: {
    port: 5173,
    // The guides and legal wording are outside web/.
    fs: { allow: ['..'] },
    proxy: { '/api': { target: process.env.WMS_API || 'http://127.0.0.1:9797', changeOrigin: false } },
  },
})
