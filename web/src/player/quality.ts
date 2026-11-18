// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { PlaybackOutput, QualityLimits, QualityPreset, QualityView, StreamWork } from '../api'

// The viewer's choice of picture size and data rate (docs/CLIENT-API.md, Quality). The same
// short string the server reads: '' is Auto (the profile's saved default for home or away),
// 'original' is no limits of the viewer's own, 'h720-b4000' at most 720p and 4 Mbps.
//
// This device keeps a default of its own for home and for away from home, which wins over the
// profile's. Choosing in the player's menu changes this device's default for where it is now.
// Plain logic and storage, no React, so the tests run it as it is.

export type Choice = { maxHeight: number; maxKbps: number; videoCodec: string; audioKbps: number; audioChannels: number }
export const EMPTY: Choice = { maxHeight: 0, maxKbps: 0, videoCodec: '', audioKbps: 0, audioChannels: 0 }

// What the menus offer.
export const HEIGHTS = [2160, 1440, 1080, 900, 720, 576, 480, 360, 240]
export const MIN_MBPS = 0.5
export const MAX_MBPS = 80

export function parseChoice(s: string | null | undefined): Choice | null {
  const v = (s || '').trim().toLowerCase()
  if (!v || v === 'auto') return null
  const c = { ...EMPTY }
  if (v === 'original') return c
  for (const f of v.split(/[-,; ]+/)) {
    if (f.length < 2) continue
    if (f[0] === 'v') { if (f.slice(1) === 'h264') c.videoCodec = 'h264'; continue }
    const n = Number(f.slice(1))
    if (!Number.isInteger(n) || n <= 0) continue
    if (f[0] === 'h' && n >= 144 && n <= 4320) c.maxHeight = n
    else if (f[0] === 'b' && n >= 200 && n <= 400000) c.maxKbps = n
    else if (f[0] === 'a' && n >= 64 && n <= 640) c.audioKbps = n
    else if (f[0] === 'c' && (n === 2 || n === 6)) c.audioChannels = n
  }
  return c
}

export function formatChoice(c: Choice | null): string {
  if (!c) return ''
  const f: string[] = []
  if (c.maxHeight > 0) f.push('h' + c.maxHeight)
  if (c.maxKbps > 0) f.push('b' + Math.round(c.maxKbps))
  if (c.videoCodec) f.push('v' + c.videoCodec)
  if (c.audioKbps > 0) f.push('a' + c.audioKbps)
  if (c.audioChannels > 0) f.push('c' + c.audioChannels)
  return f.length ? f.join('-') : 'original'
}

export const presetChoice = (p: QualityPreset) => formatChoice({ ...EMPTY, maxHeight: p.height, maxKbps: p.kbps })

// Which easy-menu entry a choice is: 'auto', 'original', a preset's name, or 'custom'.
export function entryOf(choice: string, presets: QualityPreset[]): string {
  const c = parseChoice(choice)
  if (!c) return 'auto'
  if (formatChoice(c) === 'original') return 'original'
  const p = presets.find((x) => x.height === c.maxHeight && x.kbps === c.maxKbps && !c.videoCodec && !c.audioKbps && !c.audioChannels)
  return p ? p.name : 'custom'
}

// Rough data use, as people say it.
export function rate(kbps: number): string {
  if (!kbps) return ''
  if (kbps < 1000) return `${Math.round(kbps)} kbps`
  const m = kbps / 1000
  return `${m >= 10 || Number.isInteger(m) ? Math.round(m) : m.toFixed(1)} Mbps`
}
// Per hour of watching, for the notes: "about 3.6 GB an hour".
export function perHour(kbps: number): string {
  const gb = (kbps * 3600) / 8 / 1e6
  return gb >= 1 ? `${gb >= 10 ? Math.round(gb) : gb.toFixed(1)} GB an hour` : `${Math.round(gb * 1000)} MB an hour`
}

// A picture's size as people name it: 4K, 1080p.
export function sizeName(height: number, width = 0): string {
  let h = height
  if (width > 0 && height > 0 && width < height) h = width
  else if (width > 0 && height > 0 && width * 9 > height * 16) h = Math.round((width * 9) / 16)
  if (h >= 2000) return '4K'
  // The name it's known by when it's within a few lines of one (a wide 854×460 is 480p).
  const near = [1440, 1080, 720, 576, 480, 360, 240].find((n) => Math.abs(h - n) <= n * 0.05)
  return near ? `${near}p` : h > 0 ? `${h}p` : ''
}

// A short description of a choice, for the chip and announcements.
export function describe(choice: string, presets: QualityPreset[]): string {
  const e = entryOf(choice, presets)
  if (e === 'auto') return 'Auto'
  if (e === 'original') return 'Original'
  if (e !== 'custom') return e
  const c = parseChoice(choice)!
  return [c.maxHeight ? sizeName(c.maxHeight) : '', c.maxKbps ? rate(c.maxKbps) : ''].filter(Boolean).join(' · ') || 'Custom'
}

// The highest the menu may offer.
export const capOf = (l: QualityLimits | undefined) => ({ height: l?.maxHeight || 0, kbps: l?.maxKbps || 0 })

// --------------------------------------------------------------- this device's defaults

const KEY = 'wms.quality'
const AWAY = 'wms.quality.away'
type Saved = { home: string; away: string }

function load(): Saved {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || 'null') as Partial<Saved> | null
    return { home: formatChoice(parseChoice(v?.home)), away: formatChoice(parseChoice(v?.away)) }
  } catch { return { home: '', away: '' } }
}
export const deviceDefaults = load

export function setDeviceDefault(where: 'home' | 'away', choice: string) {
  const s = load()
  s[where] = formatChoice(parseChoice(choice))
  try {
    if (!s.home && !s.away) localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, JSON.stringify(s))
  } catch { /* private window: it stays for this page only */ }
  emit()
}

// Where this device is, as the server last said (it alone can tell). Home until it says.
export function isAway(): boolean {
  try { return sessionStorage.getItem(AWAY) === '1' } catch { return false }
}
export function noteAway(away: boolean | undefined) {
  if (away == null || away === isAway()) return
  try { sessionStorage.setItem(AWAY, away ? '1' : '0') } catch { /* not kept */ }
  emit()
}
export const here = (): 'home' | 'away' => (isAway() ? 'away' : 'home')

// What a request that starts a stream asks for: this device's default for where it is, or
// nothing (Auto: the profile's default).
export const deviceChoice = () => load()[here()]

// Requests that start a stream, which carry the choice as a header: a disc, a channel, catch-up.
// /playback carries it in its body.
const STARTS = /^\/items\/\d+\/dvd$|^\/tv\/[^/]+\/(tune|catchup)(\?|$)/
export const startsStream = (method: string, path: string) => (method === 'POST' || method === 'GET') && STARTS.test(path)

const subs = new Set<() => void>()
function emit() { subs.forEach((f) => f()) }
export const onQualityChange = (f: () => void) => { subs.add(f); return () => { subs.delete(f) } }

// --------------------------------------------------------------- stream information

// The lines "Show stream info" shows, from the server's description and the player's own view.
export function streamLines(decision: string | undefined, out: PlaybackOutput | undefined, seen?: { width: number; height: number; kbps?: number }): string[] {
  const lines: string[] = []
  const w = seen?.width || out?.width || 0, h = seen?.height || out?.height || 0
  if (w && h) lines.push(`${w}×${h}${sizeName(h, w) ? ` (${sizeName(h, w)})` : ''}`)
  const kbps = seen?.kbps || out?.kbps || 0
  if (kbps) lines.push(`${rate(kbps)}${decision === 'transcode' && !seen?.kbps ? ' at most' : ''}`)
  const v = out?.videoCodec ? codecName(out.videoCodec) : ''
  const a = out?.audioCodec ? [codecName(out.audioCodec), out.audioChannels ? channelName(out.audioChannels) : '', out.audioKbps ? rate(out.audioKbps) : ''].filter(Boolean).join(' ') : ''
  if (v || a) lines.push([v, a].filter(Boolean).join(' · '))
  const mode = decision === 'direct' ? 'Direct play' : decision === 'remux' ? 'Direct stream (rewrapped)' : decision === 'transcode' ? 'Converting' : decision === 'live' ? 'Live' : ''
  if (mode) lines.push(mode)
  const work = workLine(out?.work)
  if (work && decision !== 'direct' && decision !== 'remux') lines.push(work)
  return lines
}

// Whether the graphics chip (GPU) or the processor (CPU) does a conversion, in words.
export function workLine(w: StreamWork | undefined): string {
  if (!w?.encoder || w.encoder === 'copy') return ''
  const name = encoderName(w.encoder)
  if (!w.gpuEncode) return `CPU: the processor converts it (${name})`
  const steps = [w.gpuDecode ? 'decodes, resizes' : '', w.gpuToneMap ? 'maps HDR' : '', 'encodes'].filter(Boolean)
  const and = steps.length > 1 ? steps.slice(0, -1).join(', ') + ' and ' + steps[steps.length - 1] : steps[0]
  return `GPU: the graphics chip ${and} (${name})${w.gpuDecode ? '' : '; the processor decodes'}`
}

const CODECS: Record<string, string> = { h264: 'H.264', hevc: 'HEVC', av1: 'AV1', vp9: 'VP9', vp8: 'VP8', mpeg2video: 'MPEG-2', aac: 'AAC', ac3: 'Dolby Digital', eac3: 'Dolby Digital Plus', mp3: 'MP3', opus: 'Opus', flac: 'FLAC', dts: 'DTS', truehd: 'Dolby TrueHD' }
export const codecName = (c: string) => CODECS[c] || c.toUpperCase()
export const channelName = (n: number) => (n === 1 ? 'mono' : n === 2 ? 'stereo' : n === 6 ? '5.1' : n === 8 ? '7.1' : `${n} ch`)
const ENCODERS: Record<string, string> = { libx264: 'x264', libopenh264: 'OpenH264', h264_nvenc: 'NVIDIA NVENC', h264_qsv: 'Intel Quick Sync', h264_amf: 'AMD AMF', h264_vaapi: 'VA-API', h264_videotoolbox: 'VideoToolbox' }
export const encoderName = (e: string) => ENCODERS[e] || e

// The presets a live channel offers, where the server sent none: all of them, within the limits.
export function livePresets(view: QualityView | undefined, all: QualityPreset[]): QualityPreset[] {
  const cap = capOf(view?.limits)
  return all.filter((p) => !cap.height || p.height <= cap.height).map((p) => (cap.kbps && p.kbps > cap.kbps ? { ...p, kbps: cap.kbps } : p))
}

// "Show stream info", kept on this device.
const INFO = 'wms.streamInfo'
export function showStreamInfo(): boolean {
  try { return localStorage.getItem(INFO) === '1' } catch { return false }
}
export function setShowStreamInfo(on: boolean) {
  try { if (on) localStorage.setItem(INFO, '1'); else localStorage.removeItem(INFO) } catch { /* not kept */ }
}
