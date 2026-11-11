// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { IconClose } from '../icons'
import { EQ_FREQS, EQ_PRESETS, isAppleMobile } from './sound'
import type { PlayerView } from './types'

// How it sounds on this device: kept on the device, as speakers differ.

const CROSSFADES = [0, 2, 4, 6, 8, 12]

export function SoundPanel({ p, onClose }: { p: PlayerView; onClose: () => void }) {
  const s = p.sound
  const fmtHz = (f: number) => (f >= 1000 ? `${f / 1000}k` : String(f))
  return (
    <div className="np-side">
      <div className="np-side-head">
        <h3>Sound</h3>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Close sound settings"><IconClose /></button>
      </div>
      <div className="np-sound">
        {isAppleMobile() && <p className="np-note">On an iPhone or iPad, the equaliser and volume leveling stop when the screen locks. That's how those devices treat them, and playing without them keeps going.</p>}
        {p.soundNote && <p className="np-note" role="status">{p.soundNote}</p>}
        <section>
          <h4>Between songs</h4>
          <label className="np-check"><input type="checkbox" checked={s.gapless} onChange={(e) => p.setSound({ gapless: e.target.checked })} /><span>No gaps: start the next song right as this one ends</span></label>
          <label className="np-field"><span>Crossfade</span>
            <select className="select sm" value={s.crossfade} onChange={(e) => p.setSound({ crossfade: Number(e.target.value) })}>
              {CROSSFADES.map((c) => <option key={c} value={c}>{c ? `${c} seconds` : 'Off'}</option>)}
            </select>
          </label>
          {s.crossfade > 0 && <label className="np-check"><input type="checkbox" checked={s.crossfadeAlbums} onChange={(e) => p.setSound({ crossfadeAlbums: e.target.checked })} /><span>Also within an album. Off keeps songs that run together as they were made</span></label>}
          <p className="np-hint">Works for files sent as they are. Formats converted while playing, and CD images, follow their own rules.</p>
        </section>
        <section>
          <h4>Volume leveling</h4>
          <label className="np-field"><span>ReplayGain</span>
            <select className="select sm" value={s.replayGain} onChange={(e) => p.setSound({ replayGain: e.target.value as typeof s.replayGain })}>
              <option value="off">Off</option><option value="track">Each song the same</option><option value="album">Keep each album's balance</option>
            </select>
          </label>
          <p className="np-hint">Uses the loudness tags in your files, which tagging tools such as MusicBrainz Picard or foobar2000 can add. Songs without them play as they are.</p>
        </section>
        <section>
          <h4>Equaliser</h4>
          <label className="np-check"><input type="checkbox" checked={s.eqOn} onChange={(e) => p.setSound({ eqOn: e.target.checked })} /><span>Use the equaliser</span></label>
          <label className="np-field"><span>Preset</span>
            <select className="select sm" disabled={!s.eqOn} value={EQ_PRESETS.some((e) => e.id === s.eqPreset) ? s.eqPreset : 'custom'}
              onChange={(e) => { const pre = EQ_PRESETS.find((x) => x.id === e.target.value); if (pre) p.setSound({ eqPreset: pre.id, eqGains: pre.gains }) }}>
              {EQ_PRESETS.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
              {!EQ_PRESETS.some((e) => e.id === s.eqPreset) && <option value="custom">Your own</option>}
            </select>
          </label>
          <div className={'np-eq' + (s.eqOn ? '' : ' off')}>
            {EQ_FREQS.map((f, i) => (
              <label key={f}>
                <span className="np-eq-hz">{fmtHz(f)}</span>
                <input className="range" type="range" disabled={!s.eqOn} min={-12} max={12} step={1} value={s.eqGains[i]} aria-label={`${fmtHz(f)} hertz`} aria-valuetext={`${s.eqGains[i]} decibels`}
                  style={{ '--pct': ((s.eqGains[i] + 12) / 24) * 100 + '%' } as React.CSSProperties}
                  onChange={(e) => { const g = [...s.eqGains]; g[i] = Number(e.target.value); p.setSound({ eqGains: g, eqPreset: 'custom' }) }} />
                <span className="np-eq-db">{s.eqGains[i] > 0 ? '+' : ''}{s.eqGains[i]}</span>
              </label>
            ))}
            <label>
              <span className="np-eq-hz">Level</span>
              <input className="range" type="range" disabled={!s.eqOn} min={-12} max={12} step={1} value={s.preamp} aria-label="Overall level" aria-valuetext={`${s.preamp} decibels`}
                style={{ '--pct': ((s.preamp + 12) / 24) * 100 + '%' } as React.CSSProperties} onChange={(e) => p.setSound({ preamp: Number(e.target.value) })} />
              <span className="np-eq-db">{s.preamp > 0 ? '+' : ''}{s.preamp}</span>
            </label>
          </div>
          <p className="np-hint">If boosting makes loud parts crackle, lower the overall level a little.</p>
        </section>
      </div>
    </div>
  )
}
