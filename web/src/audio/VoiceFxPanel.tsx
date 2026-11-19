// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { memo, useEffect, useRef, useState } from 'react'
import { IconClose } from '../icons'
import { Segmented } from '../ui/Choice'
import { advancedHere } from '../ui/advanced'
import { TvPart } from '../tvmode/OnAComputer'
import { useTvMode } from '../tvmode/mode'
import { FX_GROUPS, FX_OFF, FX_PRESETS, VOICE_LEVELS, VOICE_PRESETS, VOICE_RANGES, fxValue, isOff, presetFx, whichPreset, type Fx } from './voiceFx'
import { t, tx } from '../i18n'

// Voice settings: the singer's part and the music under it, beside the words. A panel at the side on a computer or a
// TV, a sheet over the lower half on a phone. It opens on the simple presets, nine kinds of voice as buttons that a
// remote or a game pad reaches with its arrows; Advanced holds every control. Changes are heard straight away and kept
// for the account on this device.
//
// TV mode (docs/TV-MODE.md): the presets are `full`. Advanced is the part `karaoke.voice-advanced`, `desktopOnly`: a
// television shows the note saying where to change it, as a phone does (ui/advanced.tsx), and the presets stay.
//
// Drawn only when what it shows changes (memo): karaoke draws itself again many times a second as the words move.
export const VoiceFxPanel = memo(function VoiceFxPanel({ fx, onFx, onClose, ready, quick }: { fx: Fx; onFx: (fx: Fx) => void; onClose: () => void; ready: boolean; quick: boolean }) {
  const box = useRef<HTMLElement>(null)
  const [view, setView] = useState<'presets' | 'advanced'>('presets')
  const tv = useTvMode()
  const preset = whichPreset(fx)
  useEffect(() => {
    box.current?.querySelector<HTMLElement>('.kar-fx-presets [aria-pressed="true"], .kar-fx-presets button')?.focus({ preventScroll: true })
  }, [])
  const presetName = preset ? tx(FX_PRESETS.find((p) => p.id === preset)?.name) : t('Your own settings')
  const sliders = FX_GROUPS.map((g) => (
    <fieldset key={g.title} className="kar-fx-group">
      <legend>{tx(g.title)}</legend>
      {g.controls.map((c) => (
        <label key={c.id} className="kar-fx-row" title={tx(c.help)}>
          <span className="kar-fx-label">{tx(c.label)}</span>
          <input type="range" min={c.min} max={c.max} step={c.step} value={fx[c.id]}
            aria-valuetext={fxValue(c, fx[c.id], t)} aria-description={tx(c.help)}
            onChange={(e) => { const v = Number(e.target.value); if (Number.isFinite(v)) onFx({ ...fx, [c.id]: v }) }} />
          <output>{fxValue(c, fx[c.id], t)}</output>
        </label>
      ))}
    </fieldset>
  ))
  return (
    <aside className="kar-fx" ref={box} role="dialog" aria-label={t('Voice settings')}>
      <header className="kar-fx-head">
        <h2>{t('Voice settings')}</h2>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Close voice settings')} title={t('Close')}><IconClose /></button>
      </header>
      <Segmented className="kar-fx-views" label={t('Voice settings')} value={view} onChange={setView}
        options={[{ value: 'presets', label: t('Presets') }, { value: 'advanced', label: t('Advanced') }]} />
      <p className="kar-fx-note">
        {!ready ? t('These are heard once the karaoke version plays.')
          : quick ? t('The quick version has no voice of its own, so the voice here is what\'s left of the song without the music: close, not exact.')
            : t('The voice and the music are separate parts, so each is shaped on its own.')}
      </p>
      {view === 'presets' ? (
        <div className="kar-fx-presets">
          <p className="kar-fx-lead">{t('Choose the voice closest to the singer’s.')}</p>
          {VOICE_RANGES.map((r) => (
            <fieldset key={r.id} className="kar-fx-voice">
              <legend>{tx(r.name)}</legend>
              <div className="kar-fx-voice-row">
                {VOICE_LEVELS.map((l) => {
                  const p = VOICE_PRESETS.find((x) => x.range === r.id && x.level === l.id)!
                  const on = preset === p.id
                  return <button type="button" key={p.id} className={'kar-chip' + (on ? ' on' : '')} aria-pressed={on} aria-label={tx(p.name)} onClick={() => onFx(presetFx(p.id))}>{tx(l.name)}</button>
                })}
              </div>
            </fieldset>
          ))}
          <div className="kar-fx-foot">
            <button type="button" className={'kar-chip' + (preset === 'off' ? ' on' : '')} aria-pressed={preset === 'off'} onClick={() => onFx({ ...FX_OFF })}>{t('As recorded')}</button>
            {preset === '' && <span className="kar-fx-custom" role="status">{t('Your own settings, from Advanced. Choose a voice to go back to a preset.')}</span>}
          </div>
        </div>
      ) : (
        <div className="kar-fx-advanced">
          <div className="kar-fx-preset">
            <span className="kar-fx-from">{t('Now: {name}', { name: presetName })}</span>
            <button type="button" className="kar-chip" onClick={() => setView('presets')}>{t('Presets')}</button>
            <button type="button" className="kar-chip" onClick={() => onFx({ ...FX_OFF })} disabled={isOff(fx)}>{t('Reset')}</button>
          </div>
          {tv ? <TvPart id="karaoke.voice-advanced">{sliders}</TvPart>
            : advancedHere() ? sliders
              : <p className="kar-fx-note kar-fx-wall" role="note">{t('Voice settings need a bigger screen and a mouse or trackpad.')} {t('Open it in the WittyMediaClient app on a computer, or in a web browser, to make changes. What you choose there shows here too.')}</p>}
        </div>
      )}
    </aside>
  )
})
