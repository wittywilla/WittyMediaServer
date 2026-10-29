// Copies the built web client into www/ for Capacitor. Run `npm run build` in ../../web first.
const fs = require('fs'), path = require('path')
const src = path.join(__dirname, '..', '..', 'web', 'dist'), dst = path.join(__dirname, 'www')
if (!fs.existsSync(path.join(src, 'assets'))) { console.error('build the web client first: cd web && npm run build'); process.exit(1) }
fs.rmSync(dst, { recursive: true, force: true })
fs.cpSync(src, dst, { recursive: true })
console.log('copied web/dist -> clients/mobile/www')
