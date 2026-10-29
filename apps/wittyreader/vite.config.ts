import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// Inside the WittyMediaServer repository the plugins come from their source
// next door, so a change to a plugin shows up here at once. In WittyReader's
// own repository they come from node_modules (package.json).
const local = fileURLToPath(new URL('../../web/src/modules/', import.meta.url))
const alias = existsSync(local) ? [{ find: /^@wittywilla\/(witty[a-z]+)(.*)$/, replacement: local + '$1$2' }] : []

// A strict Content-Security-Policy for the built app: only its own scripts,
// no plugins or forms, and the one outside address it may call is the
// dictionary, which is off until the reader turns it on.
const CSP = [
  "default-src 'self'", "script-src 'self'", "style-src 'self' 'unsafe-inline'", "img-src 'self' blob: data:",
  "font-src 'self' blob: data:", "worker-src 'self' blob:", "connect-src 'self' blob: data: https://api.dictionaryapi.dev",
  "frame-src 'self'", "object-src 'none'", "base-uri 'none'", "form-action 'none'",
].join('; ')
const csp: Plugin = { name: 'csp', apply: 'build', transformIndexHtml: (html) => html.replace('<head>', `<head>\n    <meta http-equiv="Content-Security-Policy" content="${CSP}">`) }

export default defineConfig({
  base: './',
  plugins: [react(), csp],
  resolve: { alias, dedupe: ['react', 'react-dom', 'pdfjs-dist'] },
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: { port: 5180 },
})
