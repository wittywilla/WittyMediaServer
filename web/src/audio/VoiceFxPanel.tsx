// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef } from 'react'
import { IconClose } from '../icons'
import { FX_GROUPS, FX_OFF, FX_PRESETS, fxValue, isOff, presetFx, whichPreset, type Fx } from './voiceFx'
import { t, tx } from '../i18n'

// Voice settings: every control for the singer's part and the music under it, beside the words.
// A panel at the side on a computer or a TV, a sheet over the lower half on a phone. Changes are
// heard straight away and kept for the account on this device.
export function VoiceFxPanel({ fx, onFx, onClose, ready, quick }: { fx: Fx; onFx: (fx: Fx) => void; onClose: () => void; ready: boolean; quick: boolean }) {
  const box = useRef<HTMLElement>(null)
  useEffect(() => { box.current?.querySelector<HTMLElement>('select, input, button')?.focus({ preventScroll: true }) }, [])
  const preset = whichPreset(fx)
  return (
    <aside className="kar-fx" ref={box} role="dialog" aria-label={t('Voice settings')}>
      <header className="kar-fx-head">
        <h2>{t('Voice settings')}</h2>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Close voice settings')} title={t('Close')}><IconClose /></button>
      </header>
      <p className="kar-fx-note">
        {!ready ? t('These are heard once the karaoke version plays.')
          : quick ? t('The quick version has no voice of its own, so the voice here is what\'s left of the song without the music: close, not exact.')
            : t('The voice and the music are separate parts, so each is shaped on its own.')}
      </p>
      <label className="kar-fx-preset">
        <span>{t('Preset')}</span>
        <select value={preset} onChange={(e) => onFx(presetFx(e.target.value))}>
          {preset === '' && <option value="">{t('Your own')}</option>}
          {FX_PRESETS.map((p) => <option key={p.id} value={p.id}>{tx(p.name)}</option>)}
        </select>
        <button type="button" className="kar-chip" onClick={() => onFx({ ...FX_OFF })} disabled={isOff(fx)}>{t('Reset')}</button>
      </label>
      {FX_GROUPS.map((g) => (
        <fieldset key={g.title} className="kar-fx-group">
          <legend>{tx(g.title)}</legend>
          {g.controls.map((c) => (
            <label key={c.id} className="kar-fx-row" title={tx(c.help)}>
              <span className="kar-fx-label">{tx(c.label)}</span>
              <input type="range" min={c.min} max={c.max} step={c.step} value={fx[c.id]}
                aria-valuetext={fxValue(c, fx[c.id], t)} aria-description={tx(c.help)}
                onChange={(e) => onFx({ ...fx, [c.id]: Number(e.target.value) })} />
              <output>{fxValue(c, fx[c.id], t)}</output>
            </label>
          ))}
        </fieldset>
      ))}
    </aside>
  )
}
