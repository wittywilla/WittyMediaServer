// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState } from 'react'
import { UseAComputer, advancedHere } from '../ui/advanced'
import type { QualityPreset, QualityView } from '../api'
import './quality.css'
import { EMPTY, HEIGHTS, MAX_MBPS, MIN_MBPS, capOf, entryOf, formatChoice, parseChoice, perHour, presetChoice, rate, sizeName, type Choice } from './quality'

// The player's quality menu: Auto, Original, then the sizes the picture has (only those at or
// below its own, and within what the profile is allowed), each with its rough data use; and
// behind "Advanced…", an exact size and rate, the codec, the sound, and stream information.
// Choosing asks for the video again at the same place (the player does that).
//
// Keyboard and TV remotes: ↑ and ↓ move through the choices, Enter or OK picks, Esc or Back
// closes (or goes back from Advanced). Screen readers hear a radio group with the current one checked.

export type QualityMenuProps = {
  view: QualityView
  // What is playing now: 'direct', 'remux', 'transcode', or 'live'.
  decision?: string
  // The presets to offer, where the view's own aren't right (a live channel).
  presets?: QualityPreset[]
  showInfo: boolean
  onShowInfo: (on: boolean) => void
  onChoose: (choice: string) => void
  onClose: () => void
  // Where the choice is remembered, for the note at the bottom.
  where: 'home' | 'away'
  // Live channels: the picture's own size isn't known, and Original means "as the channel sends it".
  live?: boolean
}

export default function QualityMenu({ view, decision, presets: given, showInfo, onShowInfo, onChoose, onClose, where, live }: QualityMenuProps) {
  const [page, setPage] = useState<'easy' | 'advanced'>('easy')
  const [walled, setWalled] = useState(false) // Advanced asked for on a phone or TV (ui/advanced.tsx)
  const root = useRef<HTMLDivElement | null>(null)
  const presets = given || view.presets
  const asked = view.asked
  const current = entryOf(asked, presets)
  const titleId = useId()
  // Focus the chosen entry on opening, and the first control on each page.
  useEffect(() => {
    const r = root.current
    if (!r) return
    // One selector at a time: a list matches whichever comes first in the page.
    const el = page === 'easy'
      ? r.querySelector<HTMLElement>('.qm-list [aria-checked="true"]') || r.querySelector<HTMLElement>('.qm-list [role="radio"]')
      : r.querySelector<HTMLElement>('.qm-adv select, .qm-adv input, .qm-adv button')
    el?.focus({ preventScroll: true })
  }, [page])
  const onKey = (e: React.KeyboardEvent) => {
    // The player's own keys (seek, volume) stay out of the menu.
    if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.key === 'Backspace' && !(e.target as HTMLElement).matches('input, select')) {
      e.preventDefault(); e.stopPropagation()
      if (page === 'advanced') setPage('easy'); else onClose()
      return
    }
    if (page !== 'easy' || (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'Home' && e.key !== 'End')) {
      if (e.key.startsWith('Arrow') || e.key === ' ' || e.key === 'Enter') e.stopPropagation()
      return
    }
    e.preventDefault(); e.stopPropagation()
    const items = Array.from(root.current?.querySelectorAll<HTMLElement>('.qm-list > button') || [])
    const i = items.indexOf(document.activeElement as HTMLElement)
    const next = e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : Math.max(0, Math.min(items.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)))
    items[next]?.focus()
  }
  const cap = capOf(view.limits)
  const limited = cap.height > 0 || cap.kbps > 0
  const srcKbps = view.sourceKbps || 0
  const original = live ? 'As the channel sends it'
    : decision === 'direct' && current === 'original' ? 'Plays directly'
    : srcKbps ? `Full quality · about ${rate(srcKbps)}` : 'Full quality'
  const entry = (id: string, label: string, note: string, choice: string, extra?: string) => (
    <button key={id} type="button" role="radio" aria-checked={current === id} className={'qm-item' + (current === id ? ' active' : '')}
      tabIndex={current === id || (current === 'custom' && id === 'auto') ? 0 : -1} onClick={() => onChoose(choice)}
      aria-describedby={`${titleId}-${id}`}>
      <span className="qm-label">{label}{extra && <span className="qm-badge">{extra}</span>}</span>
      <span className="qm-note" id={`${titleId}-${id}`}>{note}</span>
    </button>
  )
  return (
    <div className="qmenu" ref={root} role="dialog" aria-labelledby={titleId} onKeyDown={onKey} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <div className="qm-head">
        {page === 'advanced' && <button type="button" className="ibtn qm-back" onClick={() => setPage('easy')} aria-label="Back to quality">‹</button>}
        <h4 id={titleId}>{page === 'easy' ? 'Quality' : 'Advanced quality'}</h4>
        <button type="button" className="ibtn qm-close" onClick={onClose} aria-label="Close quality">✕</button>
      </div>
      {page === 'easy' ? (
        <>
          <div className="qm-list" role="radiogroup" aria-labelledby={titleId}>
            {entry('auto', 'Auto', 'Adjusts to your connection and settings', '', 'Recommended')}
            {entry('original', 'Original', original, 'original')}
            {presets.map((p) => entry(p.name, p.name, `about ${rate(p.kbps)} · ${perHour(p.kbps)}`, presetChoice(p)))}
            {current === 'custom' && entry('custom', 'Custom', customNote(asked), asked)}
          </div>
          {limited && <p className="qm-small">Up to {[cap.height ? sizeName(cap.height) : '', cap.kbps ? rate(cap.kbps) : ''].filter(Boolean).join(' and ')} {view.away ? 'away from home' : 'on this profile'}, as the server allows.</p>}
          <button type="button" className="qm-more" onClick={() => (advancedHere() ? setPage('advanced') : setWalled(true))}>Advanced…</button>
          {walled && <UseAComputer what="Advanced quality settings" onClose={() => setWalled(false)} />}
          <p className="qm-small">Remembered on this device {where === 'away' ? 'away from home' : 'at home'}.</p>
        </>
      ) : (
        <Advanced view={view} live={live} onApply={(c) => { onChoose(c); setPage('easy') }} showInfo={showInfo} onShowInfo={onShowInfo} />
      )}
    </div>
  )
}

function customNote(choice: string): string {
  const c = parseChoice(choice)
  if (!c) return ''
  return [c.maxHeight ? `up to ${sizeName(c.maxHeight)}` : '', c.maxKbps ? `about ${rate(c.maxKbps)}` : '', c.videoCodec ? 'H.264 only' : '',
    c.audioKbps ? `sound ${c.audioKbps} kbps` : '', c.audioChannels === 2 ? 'stereo' : c.audioChannels === 6 ? '5.1' : ''].filter(Boolean).join(' · ')
}

// The advanced page: every part of the choice, applied together.
function Advanced({ view, live, onApply, showInfo, onShowInfo }: { view: QualityView; live?: boolean; onApply: (c: string) => void; showInfo: boolean; onShowInfo: (on: boolean) => void }) {
  const start = parseChoice(view.asked) || EMPTY
  const [c, setC] = useState<Choice>(start)
  const cap = capOf(view.limits)
  const src = live ? 0 : view.sourceHeight && view.sourceWidth ? Math.max(view.sourceHeight, Math.round((view.sourceWidth * 9) / 16)) : view.sourceHeight || 0
  const heights = HEIGHTS.filter((h) => (!cap.height || h <= cap.height) && (!src || h <= src + src / 20))
  const maxMbps = cap.kbps ? Math.min(MAX_MBPS, cap.kbps / 1000) : MAX_MBPS
  const mbps = c.maxKbps / 1000
  const set = (p: Partial<Choice>) => setC((x) => ({ ...x, ...p }))
  const id = useId()
  const setMbps = (v: number) => set({ maxKbps: Number.isFinite(v) && v > 0 ? Math.round(Math.max(MIN_MBPS, Math.min(maxMbps, v)) * 1000) : 0 })
  return (
    <div className="qm-adv">
      <label className="qm-field" htmlFor={id + 'h'}>
        <span>Largest picture</span>
        <select id={id + 'h'} value={c.maxHeight} onChange={(e) => set({ maxHeight: Number(e.target.value) })}>
          <option value={0}>{cap.height ? `As allowed (${sizeName(cap.height)})` : 'The original size'}</option>
          {heights.map((h) => <option key={h} value={h}>{h}p{h === 2160 ? ' (4K)' : ''}</option>)}
        </select>
      </label>
      <div className="qm-field">
        <label htmlFor={id + 'b'}>Most data</label>
        <div className="qm-rate">
          <input type="range" min={0} max={maxMbps} step={0.5} value={mbps} aria-label="Most data, in megabits a second" aria-valuetext={mbps ? `${rate(c.maxKbps)}` : 'No limit'}
            onChange={(e) => setMbps(Number(e.target.value))} style={{ '--pct': (mbps / maxMbps) * 100 + '%' } as React.CSSProperties} className="range" />
          <span className="qm-num">
            <input id={id + 'b'} type="number" inputMode="decimal" min={0} max={maxMbps} step={0.5} value={mbps || ''} placeholder="No limit"
              onChange={(e) => setMbps(Number(e.target.value))} aria-describedby={id + 'bn'} />
            <span aria-hidden="true">Mbps</span>
          </span>
        </div>
        <span className="qm-small" id={id + 'bn'}>{mbps ? `About ${perHour(c.maxKbps)}.` : cap.kbps ? `No limit of your own: up to ${rate(cap.kbps)}, as allowed.` : 'No limit: 0 or empty.'}</span>
      </div>
      <div className="qm-field">
        <span id={id + 'v'}>Video format</span>
        <div className="seg" role="radiogroup" aria-labelledby={id + 'v'}>
          {([['', 'Any my device plays'], ['h264', 'H.264 only']] as const).map(([v, l]) => (
            <button type="button" key={v} role="radio" aria-checked={c.videoCodec === v} className={c.videoCodec === v ? 'active' : ''} onClick={() => set({ videoCodec: v })}>{l}</button>
          ))}
        </div>
        <span className="qm-small">H.264 plays on everything; other formats are converted to it. Conversions are always H.264.</span>
      </div>
      <label className="qm-field" htmlFor={id + 'a'}>
        <span>Sound quality (when converted)</span>
        <select id={id + 'a'} value={c.audioKbps} onChange={(e) => set({ audioKbps: Number(e.target.value) })}>
          <option value={0}>The server’s setting</option>
          {view.audioRates.map((r) => <option key={r} value={r}>{r} kbps</option>)}
        </select>
      </label>
      <div className="qm-field">
        <span id={id + 'c'}>Sound channels</span>
        <div className="seg" role="radiogroup" aria-labelledby={id + 'c'}>
          {([[0, 'As the source'], [2, 'Stereo'], [6, '5.1']] as const).map(([v, l]) => (
            <button type="button" key={v} role="radio" aria-checked={c.audioChannels === v} className={c.audioChannels === v ? 'active' : ''} onClick={() => set({ audioChannels: v })}>{l}</button>
          ))}
        </div>
      </div>
      <div className="qm-field qm-row">
        <span id={id + 'i'}>Show stream info</span>
        <button type="button" role="switch" aria-checked={showInfo} aria-labelledby={id + 'i'} className={'switch' + (showInfo ? ' on' : '')} onClick={() => onShowInfo(!showInfo)}><i /></button>
      </div>
      <div className="qm-actions">
        <button type="button" className="btn ghost sm" onClick={() => setC(EMPTY)}>Reset</button>
        <button type="button" className="btn primary sm" onClick={() => onApply(formatChoice(c))}>Apply</button>
      </div>
    </div>
  )
}

// StreamInfo: what is playing, in a corner of the picture, while "Show stream info" is on.
export function StreamInfo({ lines }: { lines: string[] }) {
  if (!lines.length) return null
  return (
    <div className="stream-info" role="group" aria-label="Stream information">
      {lines.map((l) => <div key={l}>{l}</div>)}
    </div>
  )
}
