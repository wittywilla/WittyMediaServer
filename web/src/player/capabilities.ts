// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mediaSourceWorks } from './mse'

// What this browser can play, found out by asking it, and sent with every request to
// play something. The server decides from this, so a browser that decodes HEVC gets
// the file as it is, and one that can't gets something simpler, with nothing converted
// that didn't have to be. Asked once per page load: the answers don't change.

export type Capabilities = {
  containers: string[]
  videoCodecs: string[]
  audioCodecs: string[]
  hdr: string[]
  tenBit: boolean
  hi10pH264: boolean
  interlaced: boolean
  maxAudioChannels: number
  sidecarSubtitles: boolean
  remux: boolean
  hlsVideo: string[]
  hlsAudio: string[]
  fmp4Video: string[]
}

// One codec string for each codec, in the most demanding form that's still common.
const VIDEO: [string, string][] = [
  ['h264', 'avc1.640028'],
  ['hevc', 'hvc1.1.6.L120.B0'],
  ['vp9', 'vp09.00.40.08'],
  ['vp8', 'vp8'],
  ['av1', 'av01.0.08M.08'],
]
const AUDIO: [string, string][] = [
  ['aac', 'mp4a.40.2'],
  ['mp3', 'mp4a.6B'],
  ['ac3', 'ac-3'],
  ['eac3', 'ec-3'],
  ['alac', 'alac'],
  ['flac', 'flac'],
  ['opus', 'opus'],
]

function asks() {
  const v = document.createElement('video')
  const can = (type: string) => { try { return v.canPlayType(type) !== '' } catch { return false } }
  const mse = (type: string) => { try { return typeof MediaSource !== 'undefined' && MediaSource.isTypeSupported(type) } catch { return false } }
  return { can, mse }
}

let cached: Capabilities | null = null

export function browserCapabilities(): Capabilities {
  if (cached) return cached
  const { can, mse } = asks()
  const mp4 = (c: string) => can(`video/mp4; codecs="${c}"`)
  const webm = (c: string) => can(`video/webm; codecs="${c}"`)
  const videoCodecs = VIDEO.filter(([id, c]) => id === 'vp8' ? webm(c) : mp4(c) || (id === 'vp9' && webm('vp09.00.40.08')) || (id === 'av1' && webm(c))).map(([id]) => id)
  const audioCodecs = AUDIO.filter(([id, c]) => id === 'opus' ? can('audio/ogg; codecs="opus"') || mp4(c) : mp4(c) || can(`audio/mp4; codecs="${c}"`)).map(([id]) => id)
  audioCodecs.push('pcm_s16le', 'pcm_s24le', 'pcm_u8', 'pcm_f32le')
  if (can('audio/ogg; codecs="vorbis"') || can('audio/webm; codecs="vorbis"')) audioCodecs.push('vorbis')
  if (!audioCodecs.includes('mp3') && can('audio/mpeg')) audioCodecs.push('mp3')
  if (!audioCodecs.includes('flac') && can('audio/flac')) audioCodecs.push('flac')

  const containers = ['mp4', 'm4v', 'mov']
  if (can('video/webm')) containers.push('webm')

  // HDR needs a screen that shows it as well as a decoder.
  const screenHDR = typeof matchMedia === 'function' && matchMedia('(dynamic-range: high)').matches
  const hdr: string[] = []
  if (screenHDR && (mp4('hvc1.2.4.L120.B0') || webm('vp09.02.40.10') || mp4('av01.0.08M.10'))) hdr.push('hdr10', 'hlg')

  // Streams go through Media Source where there is one; Safari on a phone plays
  // HLS itself, which takes H.264 and HEVC.
  const nativeHLS = can('application/vnd.apple.mpegurl')
  const hlsOK = mediaSourceWorks() || nativeHLS
  const fmp4Video: string[] = []
  const hlsVideo: string[] = []
  const inStream = (c: string) => mediaSourceWorks() ? mse(`video/mp4; codecs="${c}"`) : nativeHLS && mp4(c)
  if (inStream('avc1.640028')) hlsVideo.push('h264')
  if (inStream('hvc1.1.6.L120.B0')) { hlsVideo.push('hevc'); fmp4Video.push('hevc') }
  if (mediaSourceWorks() && inStream('av01.0.08M.08')) { hlsVideo.push('av1'); fmp4Video.push('av1') }
  if (mediaSourceWorks() && inStream('vp09.00.40.08')) { hlsVideo.push('vp9'); fmp4Video.push('vp9') }
  const hlsAudio = ['aac']
  if (mediaSourceWorks() && mse('audio/mp4; codecs="ac-3"')) hlsAudio.push('ac3')
  if (mediaSourceWorks() && mse('audio/mp4; codecs="ec-3"')) hlsAudio.push('eac3')

  cached = {
    containers, videoCodecs, audioCodecs, hdr,
    tenBit: mp4('hvc1.2.4.L120.B0') || webm('vp09.02.40.10') || mp4('av01.0.08M.10'),
    // 10-bit H.264: only software decoders, and not all of them.
    hi10pH264: mp4('avc1.6E0028'),
    // No browser deinterlaces reliably, so interlaced video is converted.
    interlaced: false,
    maxAudioChannels: 0,
    sidecarSubtitles: true,
    remux: hlsOK && hlsVideo.length > 0,
    hlsVideo, hlsAudio, fmp4Video,
  }
  return cached
}
