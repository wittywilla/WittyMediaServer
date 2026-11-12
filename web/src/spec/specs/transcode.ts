// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, choice, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'

// Settings, Live transcoding (canvas page "transcode"): Playback, Hardware encoders (the list of encoders is the
// page's own, under that section), Picture quality in Easy, Advanced or Expert. Field ids are the server's setting
// names (directPlay, maxHeight, audioCodec, x264Preset, crf, maxBitrate, audioBitrate, ffmpegVideoArgs,
// ffmpegOutputArgs); mode chips are "mode:<easy|advanced|expert>", profile rows "profile:<id>".
export interface TranscodeValues {
  directPlay: boolean; maxHeight: string; audioCodec: string
  mode: 'easy' | 'advanced' | 'expert'; profile: string
  preset: string; crf: number; maxBitrate: string; audioBitrate: string; videoArgs: string; outputArgs: string
}
export const PROFILES: { id: string; name: string; text: string; hw: string }[] = [
  { id: 'speed', name: 'Speed', text: 'Starts and seeks fastest, with a slightly softer picture.', hw: 'Raspberry Pi, Intel N100 and J-series, NAS boxes, laptops on battery, several viewers at once.' },
  { id: 'balanced', name: 'Balanced', text: 'A good balance of picture and speed. The default.', hw: 'Any desktop or laptop from the last 8 years or so, or any working hardware encoder.' },
  { id: 'quality', name: 'Quality', text: 'A noticeably sharper picture, using about three times the processor work of Balanced.', hw: 'Processors with 6 or more cores, such as a Ryzen 5 or a 12th generation Core i5, or an NVIDIA or Intel hardware encoder.' },
]
export const PRESETS = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow']
export const HARDWARE = 'Hardware encoders'
const opts = (l: [string, string][]) => l.map(([value, label]) => ({ value, label }))
export const crfWords = (c: number) => (c <= 18 ? 'looks the same as the source' : c <= 21 ? 'high' : c <= 24 ? 'good' : 'small files')

export function transcodeSpec(v: TranscodeValues): SpecPageData {
  const sections: SpecSection[] = [
    anchored(secF('Playback', [
      { id: 'directPlay', kind: 'switch', label: 'Allow direct play', value: v.directPlay, hint: 'Files a device can play as they are get sent untouched: full quality, no processor work, instant seeking. Off sends every file through conversion, which is mainly useful for testing.' },
      { id: 'maxHeight', kind: 'select', label: 'Maximum resolution', value: v.maxHeight, options: opts([['0', 'Source'], ['480', '480p'], ['720', '720p'], ['1080', '1080p'], ['1440', '1440p'], ['2160', '2160p (4K)']]), hint: 'Taller videos are converted down to this while playing, even ones that could otherwise play as they are. 720p is about half the work of 1080p, which helps on weak machines and slow connections. Source sends 4K Blu-ray discs as 1080p, since converting 4K is beyond most machines: choose 2160p to send them as 4K.' },
      { id: 'audioCodec', kind: 'select', label: 'Sound format while converting', value: v.audioCodec, options: opts([['aac', 'AAC'], ['mp3', 'MP3']]), hint: 'AAC works nearly everywhere. Choose MP3 only for a browser that can’t play AAC, such as some Linux builds of Chromium.' },
    ]), 'playback'),
    anchored(secN(HARDWARE, 'Your graphics chip can convert video for a fraction of the processor work. Tick the ones to use: the first that works is used. With none ticked it’s all software, which gives the best quality for the size and uses the most processor. Each is tested when the server starts, and those your machine doesn’t have are greyed out.'), 'hardware-encoders'),
    anchored(secP('Picture quality', [
      { id: 'mode:easy', title: 'Easy', on: v.mode === 'easy' }, { id: 'mode:advanced', title: 'Advanced', on: v.mode === 'advanced' }, { id: 'mode:expert', title: 'Expert', on: v.mode === 'expert' },
    ]), 'picture-quality'),
    secN('', 'These apply to streams started after saving.'),
  ]
  if (v.mode === 'easy') {
    sections.push(choice(secR('Quality profile', PROFILES.map((p) => ({ id: 'profile:' + p.id, title: p.name, sub: `${p.text} Best for: ${p.hw}`, on: v.profile === p.id })))))
  } else {
    const f: SpecField[] = [
      { id: 'x264Preset', kind: 'select', label: 'Encoder speed', value: v.preset, options: PRESETS.map((p) => ({ value: p, label: p + (p === 'veryfast' ? ' (default)' : '') })), hint: 'Faster settings use less processor and need more data for the same picture. Hardware encoders use their own nearest setting.' },
      { id: 'crf', kind: 'slider', label: `Quality level: ${v.crf} (${crfWords(v.crf)})`, value: v.crf, min: 16, max: 30, hint: 'Lower looks better and sends more data. 21 is the default, and 23 to 26 suit phones and slow connections.' },
      { id: 'maxBitrate', kind: 'select', label: 'Bitrate limit', value: v.maxBitrate, options: opts([['', 'No limit'], ['2M', '2 Mb/s'], ['4M', '4 Mb/s'], ['8M', '8 Mb/s'], ['12M', '12 Mb/s'], ['20M', '20 Mb/s']]), hint: 'The most data the picture may use. Over the internet, 4 Mb/s suits 720p and 8 Mb/s suits 1080p on a typical upload. With no limit, quality decides.' },
      { id: 'audioBitrate', kind: 'select', label: 'Stereo sound bitrate', value: v.audioBitrate, options: opts([['', '192 kb/s (default)'], ['128k', '128 kb/s'], ['160k', '160 kb/s'], ['256k', '256 kb/s'], ['320k', '320 kb/s']]), hint: '160 kb/s sounds the same as the source to most people, and 256 kb/s suits music. Surround sound always uses 384 kb/s.' },
    ]
    sections.push(secF('Quality', f))
    if (v.mode === 'expert') {
      sections.push(secF('Encoder options', [
        { id: 'ffmpegVideoArgs', kind: 'text', label: 'Encoder options', value: v.videoArgs, placeholder: '-preset slow -crf 20 -tune film', autoComplete: 'off', hint: 'These replace the encoder’s options. Leave empty to keep the options from the controls above. For the software encoder: -preset slow -crf 20 -tune film. For NVENC: -preset p6 -tune hq -rc vbr -cq 22 -b:v 0. Only tuning options are accepted: anything that could read or write a file is refused. Keyframes stay on segment boundaries, so seeking keeps working.' },
        { id: 'ffmpegOutputArgs', kind: 'text', label: 'Extra options', value: v.outputArgs, placeholder: '-threads 4', autoComplete: 'off', hint: 'These are added to the output, such as -threads 4 to leave cores free, or -max_muxing_queue_size 4096 for files with sparse streams. Wrong options make conversions fail, which the log explains.' },
      ]))
    }
  }
  return mk('transcode', 'Settings · Transcoding', 'Live transcoding', 'What happens while someone watches a video their device can’t play as it is. Your files aren’t changed, and the work stops when they stop watching.', sections)
}
