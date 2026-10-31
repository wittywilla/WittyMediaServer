// Hands a game over to the Moonlight app, if someone has installed it. Moonlight is a
// separate program under GPL-3.0: this only starts it, and nothing of it is copied, linked
// or bundled. A server's page asks for this and a page could be hostile, so what it asks is
// checked strictly here, and the page never gets to name a program or a path.
const { spawn } = require('child_process')
const fs = require('fs')
const net = require('net')
const os = require('os')
const path = require('path')

const WINDOWS_FOLDER = 'Moonlight Game Streaming'
const MAC_PROGRAM = '/Applications/Moonlight.app/Contents/MacOS/Moonlight'
const FLATPAK_APP = 'com.moonlight_stream.Moonlight'
const MAX_APP = 200
const MAX_NAME = 253
// A hostile page shouldn't be able to fill the screen with programs.
const MIN_GAP_MS = 2000

const NOT_INSTALLED = "Moonlight isn't installed on this computer. Install it, then try again."
const REFUSED = "That isn't something Moonlight can be asked to do."
const FAILED = "Moonlight couldn't be started."
const TOO_SOON = 'Moonlight was only just started. Give it a moment.'

const NAME = /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*$/
const CONTROL = /\p{Cc}/u

const validPort = (port) => /^[0-9]{1,5}$/.test(port) && Number(port) >= 1 && Number(port) <= 65535

// A plain name, an IPv4 address or [an IPv6 address], with a port if it needs one.
function validHost(host) {
  if (typeof host !== 'string') return false
  const bracket = /^\[([0-9A-Fa-f:.]+)\](?::(.*))?$/.exec(host)
  if (bracket) return net.isIPv6(bracket[1]) && (bracket[2] === undefined || validPort(bracket[2]))
  const [name, port, ...rest] = host.split(':')
  if (rest.length > 0 || name.length > MAX_NAME || !NAME.test(name)) return false
  return port === undefined || validPort(port)
}

// Game names are free text set on the PC. Nothing here may read as an option.
function validApp(app) {
  return typeof app === 'string' && app.trim() !== '' && app.length <= MAX_APP &&
    !app.trimStart().startsWith('-') && !CONTROL.test(app)
}

function validArgs(args) {
  if (!Array.isArray(args)) return false
  if (args[0] === 'stream') return args.length === 3 && validHost(args[1]) && validApp(args[2])
  if (args[0] === 'pair') return args.length === 4 && validHost(args[1]) && args[2] === '--pin' && typeof args[3] === 'string' && /^[0-9]{4}$/.test(args[3])
  return false
}

function canRun(file) {
  try {
    if (!fs.statSync(file).isFile()) return false
    fs.accessSync(file, fs.constants.X_OK)
    return true
  } catch { return false }
}

const isFolder = (dir) => { try { return fs.statSync(dir).isDirectory() } catch { return false } }

// Reads the folders in PATH rather than asking a shell. An empty or relative entry
// means "wherever we happen to be", which is no place to find a program to run.
function onPath(name, env, runnable) {
  for (const dir of String(env.PATH || '').split(':')) {
    if (!path.posix.isAbsolute(dir)) continue
    const file = path.posix.join(dir, name)
    if (runnable(file)) return file
  }
  return ''
}

// Where this computer has Moonlight: { command, prefix }, or null. Never from a page.
function findMoonlight({ platform = process.platform, env = process.env, home = os.homedir(), runnable = canRun, folder = isFolder } = {}) {
  if (platform === 'win32') {
    const roots = ['C:\\Program Files', env['ProgramFiles(x86)']].filter(Boolean)
    for (const root of roots) {
      const file = path.win32.join(root, WINDOWS_FOLDER, 'Moonlight.exe')
      if (runnable(file)) return { command: file, prefix: [] }
    }
    return null
  }
  if (platform === 'darwin') return runnable(MAC_PROGRAM) ? { command: MAC_PROGRAM, prefix: [] } : null
  const program = onPath('moonlight', env, runnable)
  if (program) return { command: program, prefix: [] }
  // Flatpak keeps it in one of two places, and having one there is what says it's installed.
  const flatpak = onPath('flatpak', env, runnable)
  const installed = [path.posix.join('/var/lib/flatpak/app', FLATPAK_APP), path.posix.join(home, '.local/share/flatpak/app', FLATPAK_APP)].some(folder)
  return flatpak && installed ? { command: flatpak, prefix: ['run', FLATPAK_APP] } : null
}

const status = (find = findMoonlight) => ({ installed: find() !== null })

// One launcher per app, so the gap between starts holds whoever asks.
function launcher({ find = findMoonlight, run = spawn, now = Date.now } = {}) {
  let last = -Infinity
  return (args) => new Promise((resolve) => {
    if (!validArgs(args)) return resolve({ ok: false, error: REFUSED })
    const found = find()
    if (!found) return resolve({ ok: false, error: NOT_INSTALLED })
    if (now() - last < MIN_GAP_MS) return resolve({ ok: false, error: TOO_SOON })
    last = now()
    try {
      // No shell, so no character in a game's name means anything but itself. It carries on after this app closes.
      const child = run(found.command, [...found.prefix, ...args], { detached: true, stdio: 'ignore', shell: false })
      child.once('error', () => resolve({ ok: false, error: FAILED }))
      child.once('spawn', () => { child.unref(); resolve({ ok: true }) })
    } catch { resolve({ ok: false, error: FAILED }) }
  })
}

module.exports = { validArgs, validHost, validApp, findMoonlight, status, launcher, MIN_GAP_MS, NOT_INSTALLED, REFUSED, FAILED, TOO_SOON }
