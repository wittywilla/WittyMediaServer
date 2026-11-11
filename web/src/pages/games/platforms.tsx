// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which server platforms a core runs on, and how it draws there: the server's coreSupport, in
// words. Every device plays every game, since the server runs it and streams it, so what
// matters is the server's platform. See docs/GAMING-API.md, "Which platforms a core runs on".

export type Draws = 'gpu' | 'cpu' | 'cpu-fallback' | 'none'
export type PlatformSupport = { os: string; arch?: string; build: boolean; draws: Draws; api?: 'opengl' | 'vulkan' | 'metal'; note?: string }
export type CoreSupport = { here: PlatformSupport; platforms: PlatformSupport[] }
export type CoreAlternative = { id: string; name: string; console: string }

const osNames: Record<string, string> = { windows: 'Windows', linux: 'Linux', macos: 'macOS', freebsd: 'FreeBSD' }
const archNames: Record<string, Record<string, string>> = {
  linux: { arm64: 'ARM64', arm32: '32-bit ARM' },
  macos: { x64: 'Intel', arm64: 'Apple silicon' },
}

export const platformName = (p: { os: string; arch?: string }) => {
  const os = osNames[p.os] || p.os
  const arch = p.arch ? archNames[p.os]?.[p.arch] : undefined
  return arch ? `${os} (${arch})` : os
}

export const drawsWords: Record<Draws, string> = {
  gpu: 'on a graphics card',
  cpu: 'by the processor',
  'cpu-fallback': 'by the processor, slower',
  none: 'can’t play',
}

// A few words for a list of cores: how this one does on this server.
export const hereShort = (s?: CoreSupport) => {
  if (!s) return ''
  if (!s.here.build) return 'not for this server'
  if (s.here.draws === 'none') return 'can’t draw here'
  if (s.here.draws === 'cpu-fallback') return 'drawn by the processor here'
  return ''
}

// A core's platforms: where it runs, how it draws on this server, and that every device plays it.
export function CorePlatforms({ support, compact }: { support?: CoreSupport; compact?: boolean }) {
  if (!support) return null
  const runs = support.platforms.filter((p) => p.build && p.draws !== 'none')
  const not = support.platforms.filter((p) => !p.build || p.draws === 'none')
  const here = support.here
  return (
    <span className="gm-platforms">
      <small>
        <b>On this server: </b>
        {!here.build ? (here.note || 'not published for it.') : here.draws === 'none' ? (here.note || 'can’t draw.') : `drawn ${drawsWords[here.draws]}.${here.note && here.draws !== 'cpu' ? ` ${here.note}` : ''}`}
      </small>
      {!compact && (
        <small className="muted">
          Runs on {runs.map((p) => `${platformName(p)}${p.draws === 'cpu-fallback' ? ', drawn by the processor' : p.draws === 'gpu' ? ', on a graphics card' : ''}`).join(runs.some((p) => p.draws !== 'cpu') ? '; ' : ', ') || 'no server platform yet'}.
          {not.length > 0 && ` Not on ${not.map(platformName).join(', ')}.`} Every device plays it: the server streams the game.
        </small>
      )}
    </span>
  )
}

const orList = (names: string[]) => names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} or ${names[names.length - 1]}`

// Where a console's games would play, for a game this server can't run.
export const runsOnWords = (runsOn?: PlatformSupport[]) => {
  if (!runsOn?.length) return ''
  const gpu = runsOn.filter((p) => p.draws === 'gpu').map(platformName)
  const cpu = runsOn.filter((p) => p.draws === 'cpu' || p.draws === 'cpu-fallback').map(platformName)
  const parts = []
  if (gpu.length) parts.push(`${orList(gpu)} with a graphics card`)
  if (cpu.length) parts.push(`${orList(cpu)}, drawn by the processor`)
  return `These games play on servers running ${parts.join('; and on ')}.`
}
