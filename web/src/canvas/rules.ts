// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A canvas is a short silent video that loops behind a song or a title (docs/MUSIC.md).
// It is decoration, so it gives way to everything: to someone who asked for less motion,
// to a connection that is counting its data, to a tab nobody is looking at. These are the
// rules, with no page in them.

// What GET /media/{id}/canvas answers.
// The words: constants are marked with tk() and translated where they're shown, with tx(); a sentence with something
// filled in goes through say(), English here so the tests need nothing else, and t() when the app passes it.
// (Local, so this file loads without the app around it.)
const tk = <S extends string>(english: S): S => english
type Say = (english: string, vars?: Record<string, string | number | null | undefined>) => string
const english: Say = (s, v) => s.replace(/\{(\w+)\}/g, (m, k: string) => (v && v[k] != null ? String(v[k]) : m))

export type CanvasInfo = {
  available: boolean; url?: string; mime?: string; width?: number; height?: number; durationMs?: number; loop: boolean
  source?: 'upload' | 'generated' | 'sidecar' | 'metadata-folder' | string
  reason?: string; task?: string; canGenerate: boolean; error?: string
  limited?: boolean // the server says this viewer is away from home, with the data of a stream limited
}

// What the device says about itself.
export type Device = {
  reducedMotion: boolean // the system's "reduce motion"
  saveData: boolean // the browser's data saver (navigator.connection.saveData)
  robust: boolean // this device was set to robust streaming: its connection is poor (Settings, Playback)
  off: boolean // this person turned canvases off
}

// Why a canvas isn't fetched at all, '' when it may be. The still artwork stays instead.
export function canvasBlocked(d: Device, info?: Pick<CanvasInfo, 'limited'> | null): '' | 'off' | 'motion' | 'data' | 'away' {
  if (d.off) return 'off'
  if (d.reducedMotion) return 'motion'
  if (d.saveData || d.robust) return 'data'
  if (info?.limited) return 'away'
  return ''
}

// Whether the listing is worth asking for. An administrator's page needs it whatever the
// device says, to offer making one; anyone else's only if a canvas could be shown.
export const wantsListing = (d: Device, admin: boolean) => admin || canvasBlocked(d) === ''

// Whether the video should be loaded and playing right now. Its address is only given to
// the page while this is true, so a canvas that is off the screen, behind a hidden tab or
// behind paused music is never downloaded.
export function canvasPlays(o: { blocked: string; available: boolean; onScreen: boolean; tabHidden: boolean; paused?: boolean }): boolean {
  return o.blocked === '' && o.available && o.onScreen && !o.tabHidden && !o.paused
}

// An album's canvas, from its songs'. A song can have its own, and the album's covers the
// rest, so the one most of the songs share is the album's; with none shared, the first.
// A handful of songs is enough to tell, so no more than `sample` are ever asked about.
export const ALBUM_SAMPLE = 4
export function albumCanvas(infos: (CanvasInfo | null)[]): CanvasInfo | null {
  const have = infos.filter((c): c is CanvasInfo => !!c && c.available && !!c.url)
  if (!have.length) return null
  // The address without its item: "/api/media/6/canvas/file?v=abc" names the file by "abc".
  const version = (c: CanvasInfo) => c.url!.split('?v=')[1] || c.url!
  const count = new Map<string, number>()
  for (const c of have) count.set(version(c), (count.get(version(c)) || 0) + 1)
  let best = have[0]
  for (const c of have) if ((count.get(version(c)) || 0) > (count.get(version(best)) || 0)) best = c
  return best
}

// Why a file found beside the media isn't used, in words for an administrator.
export function refusedWords(reason: string | undefined): string {
  switch (reason) {
    case 'too-big': return tk('There is a canvas file beside this, but it is over 64 MB, so it isn’t used: it would be sent to every phone that opened this.')
    case 'too-long': return tk('There is a canvas file beside this, but it is longer than a minute, so it isn’t used.')
    default: return ''
  }
}

export const SOURCE_WORDS: Record<string, string> = {
  upload: tk('Uploaded here, and kept in the server’s own folder.'),
  generated: tk('Cut from the video by the server.'),
  sidecar: tk('A file beside the media.'),
  'metadata-folder': tk('A file in the metadata folder.'),
}

// The seconds an administrator can ask for (POST /media/{id}/canvas/generate takes 2 to 15).
export const GEN_MIN_SEC = 2
export const GEN_MAX_SEC = 15
export function generateBody(start: string, length: string, runtimeSec: number, t: Say = english): { body: { startSec?: number; durationSec?: number }; error: string } {
  const body: { startSec?: number; durationSec?: number } = {}
  if (start.trim() !== '') {
    const s = parseClock(start)
    if (s === null) return { body, error: t('Give the start as seconds, or as minutes:seconds.') }
    if (runtimeSec > 0 && s >= runtimeSec) return { body, error: t('That start is after the end of the video.') }
    body.startSec = s
  }
  if (length.trim() !== '') {
    const d = Number(length)
    if (!Number.isFinite(d) || d < GEN_MIN_SEC || d > GEN_MAX_SEC) return { body, error: t('A canvas is {min} to {max} seconds long.', { min: GEN_MIN_SEC, max: GEN_MAX_SEC }) }
    body.durationSec = d
  }
  return { body, error: '' }
}

// "90", "1:30" or "1:02:03" as seconds; null for anything else.
export function parseClock(v: string): number | null {
  const parts = v.trim().split(':')
  if (parts.length < 1 || parts.length > 3 || parts.some((p) => !/^\d+(\.\d+)?$/.test(p))) return null
  if (parts.slice(1).some((p) => Number(p) >= 60)) return null
  return parts.reduce((n, p) => n * 60 + Number(p), 0)
}
