import { useEffect, useState } from 'react'
import { api, getPrefs } from '../api'
import Tip from '../components/Tip'
import { Row, Switch } from './settings/ui'

type HW = { name: string; label: string; vendor: string; compiled: boolean; tested: boolean; working: boolean; error?: string }
type Mode = 'easy' | 'advanced' | 'expert'

const GUIDE = 'https://github.com/wittywilla/WittyMediaServer/blob/master/docs/TRANSCODING.md'

const PROFILES: { id: string; name: string; text: string; hw: string }[] = [
  { id: 'speed', name: 'Speed', text: 'Starts and seeks fastest, with a slightly softer picture.', hw: 'Raspberry Pi, Intel N100/J-series, NAS boxes, laptops on battery, several viewers at once.' },
  { id: 'balanced', name: 'Balanced', text: 'A good balance of picture and speed. The default.', hw: 'Any desktop or laptop from the last ~8 years; any working hardware encoder.' },
  { id: 'quality', name: 'Quality', text: 'A noticeably sharper picture, using about three times the processor work of Balanced.', hw: '6+ core CPUs (Ryzen 5 / Core i5 12th gen or newer) or NVENC Turing+ / Quick Sync 11th gen+.' },
]

const PRESETS = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow']

export default function TranscodeSettings() {
  const [s, setS] = useState<Record<string, string> | null>(null)
  const [hw, setHw] = useState<HW[]>([])
  const [hwDone, setHwDone] = useState(false)
  const [saved, setSaved] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then(setS) }, [])
  useEffect(() => {
    let t: any
    const load = () => api.get<any>('/system').then((r) => { setHw(r.hardware || []); setHwDone(!!r.hwTested); if (!r.hwTested) t = setTimeout(load, 2000) }).catch(() => {})
    load()
    return () => clearTimeout(t)
  }, [])
  if (!s) return null
  const set = (k: string, v: string) => setS({ ...s, [k]: v })
  const mode = (s.qualityMode || 'easy') as Mode
  const enabled = new Set((s.hwEncoders || '').split(',').filter(Boolean))
  const toggleHW = (name: string, on: boolean) => {
    const next = hw.map((h) => h.name).filter((n) => (n === name ? on : enabled.has(n)))
    set('hwEncoders', next.join(','))
  }
  const save = async () => { setS(await api.put('/settings', { ...s, hwAccel: '' })); getPrefs(true); setSaved(true); setTimeout(() => setSaved(false), 2000) }
  const status = (h: HW) => !h.compiled ? 'Not included in this FFmpeg build'
    : !h.tested ? (hwDone ? 'Not supported on this operating system' : 'Testing…')
    : h.working ? 'Working on this machine' : 'Not found on this machine'
  const crf = Number(s.crf || 21)
  return (
    <div className="panel transcode">
      <p className="muted" style={{ marginTop: 0 }}>Not sure what to pick? The defaults suit most machines; <a href={GUIDE} target="_blank" rel="noreferrer">the transcoding guide</a> explains each choice and recommends hardware.</p>
      <div className="form">
        <h2 className="section-title">Playback</h2>
        <Row label="Allow direct play" desc="Files the browser can play as they are (MP4/M4V/MOV/WebM with H.264, VP9 or AV1 and AAC/MP3/Opus audio) are sent untouched: full quality, no CPU use, instant seeking. Turn this off to force every file through the transcoder, e.g. to test a profile or to cap bandwidth with a bitrate limit.">
          <Switch label="Allow direct play" checked={s.directPlay !== '0'} onChange={(v) => set('directPlay', v ? '1' : '0')} />
        </Row>
        <div className="row2">
          <label>Maximum resolution <Tip>Videos taller than this are scaled down while transcoding. 720p roughly halves the work compared with 1080p; use it on weak machines or slow upload links. Direct play is not affected.</Tip>
            <select className="select" value={s.maxHeight || '0'} onChange={(e) => set('maxHeight', e.target.value)}>
              <option value="0">Source</option><option value="480">480p</option><option value="720">720p</option><option value="1080">1080p</option><option value="1440">1440p</option>
            </select>
          </label>
          <label>Stream audio codec <Tip>AAC works everywhere except some Chromium builds without proprietary codecs (Electron, some Linux distributions); choose MP3 there.</Tip>
            <select className="select" value={s.audioCodec || 'aac'} onChange={(e) => set('audioCodec', e.target.value)}>
              <option value="aac">AAC</option><option value="mp3">MP3</option>
            </select>
          </label>
        </div>

        <h2 className="section-title">Hardware encoders <Tip wide>Tick the encoders you want to use. WittyMediaServer test-encodes with each one at start-up; encoders your GPU/CPU or driver does not have are greyed out. When several are ticked, the first working one in this list is used. Nothing ticked = software (libx264), which gives the best quality per bit but uses the most CPU.</Tip></h2>
        <div className="hw-list">
          {hw.map((h) => {
            const usable = h.compiled && h.working
            return (
              <label key={h.name} className={'hw-item' + (usable ? '' : ' disabled')} title={h.error || status(h)}>
                <input type="checkbox" disabled={!usable} checked={usable && enabled.has(h.name)} onChange={(e) => toggleHW(h.name, e.target.checked)} />
                <div>
                  <div><b>{h.label}</b> <code>{h.name}</code></div>
                  <div className={'hw-status' + (usable ? ' ok' : '')}>{status(h)}</div>
                </div>
              </label>
            )
          })}
          {hw.length === 0 && <div className="muted">Loading…</div>}
        </div>

        <h2 className="section-title">Transcode quality</h2>
        <div className="seg big">
          {(['easy', 'advanced', 'expert'] as Mode[]).map((m) => <button key={m} className={mode === m ? 'active' : ''} onClick={() => set('qualityMode', m)}>{m[0].toUpperCase() + m.slice(1)}</button>)}
          <Tip wide><b>Easy</b>: pick one of three tested profiles.<br /><b>Advanced</b>: choose the encoder speed, quality level, bitrate cap and audio bitrate yourself.<br /><b>Expert</b>: Advanced plus raw FFmpeg options. Settings apply to new streams; restart playback to hear/see a change.</Tip>
        </div>

        {mode === 'easy' && (
          <div className="profiles">
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
            <div className="row2">
              <label>Encoder speed <Tip>The x264 preset. Faster presets use less CPU but need more bits for the same quality. Hardware encoders map this onto their own presets (NVENC p1–p7, Quick Sync veryfast–slow, AMF speed/balanced/quality).</Tip>
                <select className="select" value={s.x264Preset || 'veryfast'} onChange={(e) => set('x264Preset', e.target.value)}>
                  {PRESETS.map((p) => <option key={p} value={p}>{p}{p === 'veryfast' ? ' (default)' : ''}</option>)}
                </select>
              </label>
              <label>Quality level: {crf} {crf <= 18 ? '(visually lossless)' : crf <= 21 ? '(high)' : crf <= 24 ? '(good)' : '(small files)'}
                <Tip>Constant quality (x264 CRF). Lower is better and bigger: 18 ≈ visually lossless, 21 default, 23–26 for mobile or slow links. Hardware encoders get the equivalent setting (NVENC -cq, Quick Sync -global_quality, VAAPI/AMF -qp, VideoToolbox -q:v).</Tip>
                <input className="range" style={{ '--pct': ((Number(crf) - 16) / 14) * 100 + '%' } as React.CSSProperties} type="range" min={16} max={30} value={crf} onChange={(e) => set('crf', e.target.value)} />
              </label>
            </div>
            <div className="row2">
              <label>Bitrate cap <Tip>Upper limit on the video bitrate (VBV). Use it when streaming over the internet: 4 Mb/s suits 720p on a typical upload, 8 Mb/s 1080p. No cap = quality decides the size.</Tip>
                <select className="select" value={s.maxBitrate || ''} onChange={(e) => set('maxBitrate', e.target.value)}>
                  <option value="">No cap</option><option value="2M">2 Mb/s</option><option value="4M">4 Mb/s</option><option value="8M">8 Mb/s</option><option value="12M">12 Mb/s</option><option value="20M">20 Mb/s</option>
                </select>
              </label>
              <label>Stereo audio bitrate <Tip>AAC stereo bitrate. 160 kb/s is transparent for most people; 256 kb/s for music-heavy content. Surround audio always uses 384 kb/s.</Tip>
                <select className="select" value={s.audioBitrate || ''} onChange={(e) => set('audioBitrate', e.target.value)}>
                  <option value="">192 kb/s (default)</option><option value="128k">128 kb/s</option><option value="160k">160 kb/s</option><option value="256k">256 kb/s</option><option value="320k">320 kb/s</option>
                </select>
              </label>
            </div>
          </>
        )}

        {mode === 'expert' && (
          <>
            <label>Encoder options <span className="muted">(replace everything after <code>-c:v &lt;encoder&gt;</code>)</span>
              <Tip wide>Leave empty to keep the options generated from the controls above. Example for libx264: <code>-preset slow -crf 20 -tune film -x264-params aq-mode=3</code>. Example for NVENC: <code>-preset p6 -tune hq -rc vbr -cq 22 -b:v 0 -spatial-aq 1</code>. Keyframes are still forced on segment boundaries, so seeking keeps working. Quotes group words.</Tip>
              <input className="input mono" value={s.ffmpegVideoArgs || ''} onChange={(e) => set('ffmpegVideoArgs', e.target.value)} placeholder="-preset slow -crf 20 -tune film" />
            </label>
            <label>Extra output options <span className="muted">(added before the HLS muxer options)</span>
              <Tip wide>For anything else FFmpeg supports, e.g. <code>-threads 4</code> to leave cores free, or <code>-max_muxing_queue_size 4096</code> for files with sparse streams. Wrong options make transcodes fail: check the server log (<code>-log debug</code> prints every FFmpeg command line).</Tip>
              <input className="input mono" value={s.ffmpegOutputArgs || ''} onChange={(e) => set('ffmpegOutputArgs', e.target.value)} placeholder="-threads 4" />
            </label>
          </>
        )}

        <div><button className="btn primary" onClick={save}>Save</button> {saved && <span className="ok" style={{ marginLeft: 10 }}>Saved — applies to newly started streams</span>}</div>
      </div>
    </div>
  )
}
