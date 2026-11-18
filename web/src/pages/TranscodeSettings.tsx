// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { LoadError } from '../ui'
import SpecPage from '../spec/SpecPage'
import { HARDWARE, PROFILES, transcodeSpec, type TranscodeValues } from '../spec/specs/transcode'
import { PageHead, SaveBar, useServerSettings } from './settings/ui'

// Live transcoding: what happens to a video while someone watches it, for a device that
// can't play the file as it is. Files aren't changed. Converting them ahead of time is
// Library transcoding, a page of its own.

type HW = { name: string; label: string; vendor: string; compiled: boolean; tested: boolean; working: boolean; error?: string; reason?: string; device?: string }
// A graphics device the server found, and why it can't use one (Linux and FreeBSD).
type GPU = { path: string; vendor?: string; driver?: string; group: number; mode?: string; usable: boolean; problem?: string; fix?: string }
type Mode = 'easy' | 'advanced' | 'expert'
// Other work the graphics chip can take from the processor, each tested with a tiny real job.
type GPUPath = { id: string; label: string; kind: string; encoder?: string; tested: boolean; working: boolean; off: boolean; device?: string; reason?: string; error?: string }
type InUse = { encoder: string; pipeline?: string; toneMap?: string }

export default function TranscodeSettings() {
  const st = useServerSettings()
  const { s, set, on, dirty, busy, discard } = st
  const [hw, setHw] = useState<HW[]>([])
  const [hwDone, setHwDone] = useState(false)
  const [gpus, setGpus] = useState<GPU[]>([])
  const [paths, setPaths] = useState<GPUPath[]>([])
  const [inUse, setInUse] = useState<InUse | null>(null)
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    const load = () => api.get<{ hardware?: HW[]; hwTested?: boolean; gpus?: GPU[]; gpuPaths?: GPUPath[]; gpuInUse?: InUse }>('/system').then((r) => {
      setHw(r.hardware || [])
      setGpus(r.gpus || [])
      setPaths(r.gpuPaths || [])
      setInUse(r.gpuInUse || null)
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
  // Never saved ("auto"): every encoder that works here, until the owner chooses.
  const enabled = new Set((s.hwEncoders ?? 'auto') === 'auto' ? hw.filter((h) => h.compiled && h.working).map((h) => h.name) : (s.hwEncoders || '').split(',').filter(Boolean))
  const toggleHW = (name: string, want: boolean) => set('hwEncoders', hw.map((h) => h.name).filter((n) => (n === name ? want : enabled.has(n))).join(','))
  const pathsOff = new Set((s.gpuPathsOff || '').split(',').filter(Boolean))
  const togglePath = (id: string, use: boolean) => set('gpuPathsOff', paths.map((p) => p.id).filter((p) => (p === id ? !use : pathsOff.has(p))).join(','))
  const pathStatus = (p: GPUPath) => p.working ? 'Working on this machine' + (p.device ? ' (' + p.device + ')' : '') : p.reason || (hwDone ? 'Not found on this machine' : 'Testing…')
  const named = (id?: string) => hw.find((h) => h.name === id)?.label || paths.find((p) => p.id === id)?.label || id
  // An older setting that the list of encoders replaced: saving says it's gone.
  const save = () => { set('hwAccel', ''); st.save() }
  const status = (h: HW) => !h.compiled ? 'Not included in this FFmpeg build'
    : h.working ? 'Working on this machine' + (h.device ? ' (' + h.device + ')' : '')
      : h.reason ? h.reason
        : !h.tested ? (hwDone ? 'Not supported on this operating system' : 'Testing…')
          : 'Not found on this machine'
  const crf = Number(s.crf || 21)
  const view: TranscodeValues = {
    directPlay: on('directPlay'), maxHeight: s.maxHeight || '0', audioCodec: s.audioCodec || 'aac', mode, profile: s.qualityProfile || 'balanced',
    preset: s.x264Preset || 'veryfast', crf, maxBitrate: s.maxBitrate || '', audioBitrate: s.audioBitrate || '', videoArgs: s.ffmpegVideoArgs || '', outputArgs: s.ffmpegOutputArgs || '',
  }
  const page = transcodeSpec(view)
  const hwAt = page.sections.findIndex((x) => x.h === HARDWARE)
  const onField = (id: string, v: string | number | boolean | File) => set(id, id === 'directPlay' ? (v ? '1' : '0') : String(v))
  const onAction = (id: string) => { if (id.startsWith('mode:')) set('qualityMode', id.slice(5)); else if (id.startsWith('profile:')) set('qualityProfile', id.slice(8)) }
  const handled = new Set([...['easy', 'advanced', 'expert'].map((m) => 'mode:' + m), ...PROFILES.map((p) => 'profile:' + p.id)])
  return (
    <div className="transcode">
      {head}
      <SpecPage embedded page={page} handled={handled} onAction={onAction} onField={onField} after={{
        [hwAt]: (
          <>
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
        {paths.length > 0 && (
          <div className="hw-devices">
            <h4>More work on the graphics chip</h4>
            <div className="hw-list">
              {paths.map((p) => (
                <label key={p.id} className={'hw-item' + (p.working ? '' : ' disabled')} title={p.error || pathStatus(p)}>
                  <input type="checkbox" disabled={!p.working} checked={p.working && !pathsOff.has(p.id)} onChange={(e) => togglePath(p.id, e.target.checked)} />
                  <div>
                    <div className="hw-name"><b>{p.label}</b> <code>{p.id}</code></div>
                    <div className={'hw-status' + (p.working ? ' ok' : '')}>{pathStatus(p)}</div>
                  </div>
                </label>
              ))}
            </div>
            {inUse && (
              <p className="muted">
                New streams use {inUse.encoder === 'libx264' || inUse.encoder === 'libopenh264' ? 'the processor (' + inUse.encoder + ')' : named(inUse.encoder)}
                {inUse.pipeline ? ', with the picture decoded and resized on the chip' : ''}
                {inUse.toneMap ? ', and map HDR on the chip' : ''}. A file the chip can’t decode, or a job that fails on it, goes on with the processor instead, without stopping.
              </p>
            )}
          </div>
        )}
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
          </>
        ),
      }} />
      <SaveBar dirty={dirty} busy={busy} save={save} discard={discard} />
    </div>
  )
}
