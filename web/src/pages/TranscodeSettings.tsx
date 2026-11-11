// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import Tip from '../components/Tip'
import { LoadError, Segmented, Select } from '../ui'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './settings/ui'

// Live transcoding: what happens to a video while someone watches it, for a device that
// can't play the file as it is. Files aren't changed. Converting them ahead of time is
// Library transcoding, a page of its own.

type HW = { name: string; label: string; vendor: string; compiled: boolean; tested: boolean; working: boolean; error?: string; reason?: string; device?: string }
// A graphics device the server found, and why it can't use one (Linux and FreeBSD).
type GPU = { path: string; vendor?: string; driver?: string; group: number; mode?: string; usable: boolean; problem?: string; fix?: string }
type Mode = 'easy' | 'advanced' | 'expert'

const PROFILES: { id: string; name: string; text: string; hw: string }[] = [
  { id: 'speed', name: 'Speed', text: 'Starts and seeks fastest, with a slightly softer picture.', hw: 'Raspberry Pi, Intel N100 and J-series, NAS boxes, laptops on battery, several viewers at once.' },
  { id: 'balanced', name: 'Balanced', text: 'A good balance of picture and speed. The default.', hw: 'Any desktop or laptop from the last 8 years or so, or any working hardware encoder.' },
  { id: 'quality', name: 'Quality', text: 'A noticeably sharper picture, using about three times the processor work of Balanced.', hw: 'Processors with 6 or more cores, such as a Ryzen 5 or a 12th generation Core i5, or an NVIDIA or Intel hardware encoder.' },
]

const PRESETS = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow']

export default function TranscodeSettings() {
  const st = useServerSettings()
  const { s, set, on, dirty, busy, discard } = st
  const [hw, setHw] = useState<HW[]>([])
  const [hwDone, setHwDone] = useState(false)
  const [gpus, setGpus] = useState<GPU[]>([])
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    const load = () => api.get<{ hardware?: HW[]; hwTested?: boolean; gpus?: GPU[] }>('/system').then((r) => {
      setHw(r.hardware || [])
      setGpus(r.gpus || [])
      setHwDone(!!r.hwTested)
      if (!r.hwTested) t = setTimeout(load, 2000) // only until the first test has finished
    }).catch(() => {})
    load()
    return () => clearTimeout(t)
  }, [])
  const head = <PageHead title="Live transcoding" desc={<>What happens while someone watches a video their device can’t play as it is. Your files aren’t changed, and the work stops when they stop watching. To convert files ahead of time, see <Link to="/settings/library-transcoding">Library transcoding</Link>.</>} />
  if (st.failed) return <>{head}<LoadError what="This page" onRetry={st.retry} /></>
  if (!s) return head
  const mode = (s.qualityMode || 'easy') as Mode
  const enabled = new Set((s.hwEncoders || '').split(',').filter(Boolean))
  const toggleHW = (name: string, want: boolean) => set('hwEncoders', hw.map((h) => h.name).filter((n) => (n === name ? want : enabled.has(n))).join(','))
  // An older setting that the list of encoders replaced: saving says it's gone.
  const save = () => { set('hwAccel', ''); st.save() }
  const status = (h: HW) => !h.compiled ? 'Not included in this FFmpeg build'
    : h.working ? 'Working on this machine' + (h.device ? ' (' + h.device + ')' : '')
      : h.reason ? h.reason
        : !h.tested ? (hwDone ? 'Not supported on this operating system' : 'Testing…')
          : 'Not found on this machine'
  const crf = Number(s.crf || 21)
  return (
    <div className="transcode">
      {head}
      <Section title="Playback" desc={<>Not sure what to pick? The defaults suit most machines, and <Link to="/help/transcoding">the transcoding guide</Link> explains each choice.</>}>
        <Row label="Allow direct play" desc="Files a device can play as they are get sent untouched: full quality, no processor work, instant seeking. Off sends every file through conversion, which is mainly useful for testing.">
          <Switch label="Allow direct play" checked={on('directPlay')} onChange={(v) => set('directPlay', v ? '1' : '0')} />
        </Row>
        <Row label="Maximum resolution" desc="Taller videos are converted down to this while playing, even ones that could otherwise play as they are. 720p is about half the work of 1080p, which helps on weak machines and slow connections. Source sends 4K Blu-ray discs as 1080p, since converting 4K is beyond most machines: choose 2160p to send them as 4K.">
          <Select value={s.maxHeight || '0'} onChange={(e) => set('maxHeight', e.target.value)} options={[{ value: '0', label: 'Source' }, { value: '480', label: '480p' }, { value: '720', label: '720p' }, { value: '1080', label: '1080p' }, { value: '1440', label: '1440p' }, { value: '2160', label: '2160p (4K)' }]} />
        </Row>
        <Row label="Sound format while converting" desc="AAC works nearly everywhere. Choose MP3 only for a browser that can’t play AAC, such as some Linux builds of Chromium.">
          <Select value={s.audioCodec || 'aac'} onChange={(e) => set('audioCodec', e.target.value)} options={[{ value: 'aac', label: 'AAC' }, { value: 'mp3', label: 'MP3' }]} />
        </Row>
      </Section>

      <Section title="Hardware encoders" desc="Your graphics chip can convert video for a fraction of the processor work. Tick the ones to use: the first that works is used. With none ticked it’s all software, which gives the best quality for the size and uses the most processor. Each is tested when the server starts, and those your machine doesn’t have are greyed out.">
        <div className="hw-list">
          {hw.map((h) => {
            const usable = h.compiled && h.working
            return (
              <label key={h.name} className={'hw-item' + (usable ? '' : ' disabled')} title={h.error || status(h)}>
                <input type="checkbox" disabled={!usable} checked={usable && enabled.has(h.name)} onChange={(e) => toggleHW(h.name, e.target.checked)} />
                <div>
                  <div className="hw-name"><b>{h.label}</b> <code>{h.name}</code></div>
                  <div className={'hw-status' + (usable ? ' ok' : '')}>{status(h)}</div>
                </div>
              </label>
            )
          })}
          {hw.length === 0 && <div className="muted">Loading…</div>}
        </div>
        {gpus.length > 0 && (
          <div className="hw-devices">
            <h4>Graphics devices found</h4>
            <ul>
              {gpus.map((g) => (
                <li key={g.path}>
                  <code>{g.path}</code> {[g.vendor && g.vendor !== 'other' ? g.vendor[0].toUpperCase() + g.vendor.slice(1) : '', g.driver, g.group >= 0 ? 'group ' + g.group : ''].filter(Boolean).join(', ')}
                  <div className={'hw-status' + (g.usable ? ' ok' : ' bad')}>{g.usable ? 'The server can use it' : g.problem}</div>
                  {g.fix && <div className="hw-fix">{g.fix}</div>}
                </li>
              ))}
            </ul>
          </div>
        )}
      </Section>

      <Section title="Picture quality" desc="These apply to streams started after saving." actions={
        <Segmented label="How much to choose yourself" value={mode} onChange={(m) => set('qualityMode', m)} options={[{ value: 'easy', label: 'Easy' }, { value: 'advanced', label: 'Advanced' }, { value: 'expert', label: 'Expert' }]} />
      }>
        {mode === 'easy' && (
          <div className="profiles" role="radiogroup" aria-label="Quality profile">
            {PROFILES.map((p) => (
              <label key={p.id} className={'profile' + ((s.qualityProfile || 'balanced') === p.id ? ' active' : '')}>
                <input type="radio" name="profile" checked={(s.qualityProfile || 'balanced') === p.id} onChange={() => set('qualityProfile', p.id)} />
                <div><b>{p.name}</b><div className="muted">{p.text}</div><div className="best">Best for: {p.hw}</div></div>
              </label>
            ))}
          </div>
        )}
        {mode !== 'easy' && (
          <>
            <Row label="Encoder speed" desc="Faster settings use less processor and need more data for the same picture. Hardware encoders use their own nearest setting.">
              <Select value={s.x264Preset || 'veryfast'} onChange={(e) => set('x264Preset', e.target.value)} options={PRESETS.map((p) => ({ value: p, label: p + (p === 'veryfast' ? ' (default)' : '') }))} />
            </Row>
            <Row label={<>Quality level: {crf} {crf <= 18 ? '(looks the same as the source)' : crf <= 21 ? '(high)' : crf <= 24 ? '(good)' : '(small files)'}</>} desc="Lower looks better and sends more data. 21 is the default, and 23 to 26 suit phones and slow connections." stack>
              <input className="range" aria-label="Quality level" style={{ '--pct': ((crf - 16) / 14) * 100 + '%' } as React.CSSProperties} type="range" min={16} max={30} value={crf} onChange={(e) => set('crf', e.target.value)} />
            </Row>
            <Row label="Bitrate limit" desc="The most data the picture may use. Over the internet, 4 Mb/s suits 720p and 8 Mb/s suits 1080p on a typical upload. With no limit, quality decides.">
              <Select value={s.maxBitrate || ''} onChange={(e) => set('maxBitrate', e.target.value)} options={[{ value: '', label: 'No limit' }, { value: '2M', label: '2 Mb/s' }, { value: '4M', label: '4 Mb/s' }, { value: '8M', label: '8 Mb/s' }, { value: '12M', label: '12 Mb/s' }, { value: '20M', label: '20 Mb/s' }]} />
            </Row>
            <Row label="Stereo sound bitrate" desc="160 kb/s sounds the same as the source to most people, and 256 kb/s suits music. Surround sound always uses 384 kb/s.">
              <Select value={s.audioBitrate || ''} onChange={(e) => set('audioBitrate', e.target.value)} options={[{ value: '', label: '192 kb/s (default)' }, { value: '128k', label: '128 kb/s' }, { value: '160k', label: '160 kb/s' }, { value: '256k', label: '256 kb/s' }, { value: '320k', label: '320 kb/s' }]} />
            </Row>
          </>
        )}
        {mode === 'expert' && (
          <>
            <Row stack label={<>Encoder options <Tip wide>Leave empty to keep the options from the controls above. For the software encoder: <code>-preset slow -crf 20 -tune film</code>. For NVENC: <code>-preset p6 -tune hq -rc vbr -cq 22 -b:v 0</code>. Only tuning options are accepted: anything that could read or write a file is refused. Keyframes stay on segment boundaries, so seeking keeps working.</Tip></>} desc="These replace the encoder’s options.">
              <input className="input mono" aria-label="Encoder options" value={s.ffmpegVideoArgs || ''} onChange={(e) => set('ffmpegVideoArgs', e.target.value)} placeholder="-preset slow -crf 20 -tune film" />
            </Row>
            <Row stack label={<>Extra options <Tip wide>Such as <code>-threads 4</code> to leave cores free, or <code>-max_muxing_queue_size 4096</code> for files with sparse streams. Wrong options make conversions fail, which the log explains.</Tip></>} desc="These are added to the output.">
              <input className="input mono" aria-label="Extra options" value={s.ffmpegOutputArgs || ''} onChange={(e) => set('ffmpegOutputArgs', e.target.value)} placeholder="-threads 4" />
            </Row>
          </>
        )}
      </Section>
      <SaveBar dirty={dirty} busy={busy} save={save} discard={discard} />
    </div>
  )
}
