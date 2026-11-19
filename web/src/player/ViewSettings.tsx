// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { Scope, ViewPrefs } from './viewPrefs'
import type { DeviceStreaming } from './streaming'
import { num, t } from '../i18n'

type Change = (p: Partial<ViewPrefs>) => void

// Row, Toggle and Seg live at module level so re-renders keep the same DOM
// (focus and controller navigation stay put while the video plays).
function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="vs-row"><span>{label}</span><div className="vs-ctl">{children}</div></div>
}

function Toggle({ k, label, prefs, onChange }: { k: 'seekBar' | 'chapterMarks' | 'remote' | 'forcedSubs' | 'discLocks'; label: string; prefs: ViewPrefs; onChange: Change }) {
  return (
    <button type="button" role="switch" aria-checked={prefs[k]} aria-label={label} className={'switch' + (prefs[k] ? ' on' : '')}
      onClick={() => onChange({ [k]: !prefs[k] })}><i /></button>
  )
}

function Seg<T extends string | number>({ k, label, opts, prefs, onChange }: { k: keyof ViewPrefs; label: string; opts: [T, string][]; prefs: ViewPrefs; onChange: Change }) {
  return (
    <div className="seg" role="radiogroup" aria-label={label}>
      {opts.map(([v, l]) => (
        <button type="button" key={String(v)} role="radio" aria-checked={prefs[k] === v} className={prefs[k] === v ? 'active' : ''}
          onClick={() => onChange({ [k]: v } as Partial<ViewPrefs>)}>{l}</button>
      ))}
    </div>
  )
}

// The gear menu: viewing options that apply to this disc or to everything, and 3D,
// which only ever applies to this disc (show3D: the disc player, which draws it).
export default function ViewSettings({ prefs, scope, onChange, onScope, onClose, isDisc, show3D, streaming, onStreaming }: {
  prefs: ViewPrefs; scope: Scope; onChange: Change; onScope: (s: Scope) => void; onClose: () => void; isDisc: boolean; show3D?: boolean
  // This device's streaming choice, for videos (not discs). Changing it asks for the video again at the same place.
  streaming?: DeviceStreaming; onStreaming?: (v: DeviceStreaming) => void
}) {
  const p = { prefs, onChange }
  return (
    <div className="view-settings" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} role="dialog" aria-label={t('Viewing settings')}
      onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }}>
      <div className="vs-head">
        <h4>{t('Picture and controls')}</h4>
        <button type="button" className="ibtn" onClick={onClose} aria-label={t('Close viewing settings')}>✕</button>
      </div>
      <Row label={t('Apply to')}>
        <div className="seg" role="radiogroup" aria-label={t('Apply to')}>
          <button type="button" role="radio" aria-checked={scope === 'item'} className={scope === 'item' ? 'active' : ''} onClick={() => onScope('item')}>{isDisc ? t('This disc') : t('This video')}</button>
          <button type="button" role="radio" aria-checked={scope === 'global'} className={scope === 'global' ? 'active' : ''} onClick={() => onScope('global')}>{t('Everything')}</button>
        </div>
      </Row>
      <h5 className="vs-sub">{t('Controls')}</h5>
      <Row label={t('Control bar')}><Seg k="controls" label={t('Control bar')} opts={[['auto', t('Automatic')], ['detailed', t('Detailed')], ['simple', t('Simple')]]} {...p} /></Row>
      <Row label={t('Seek bar')}><Toggle k="seekBar" label={t('Seek bar')} {...p} /></Row>
      <Row label={t('Chapter marks')}><Toggle k="chapterMarks" label={t('Chapter marks')} {...p} /></Row>
      <Row label={isDisc ? t('Disc remote') : t('Remote')}><Toggle k="remote" label={t('Remote')} {...p} /></Row>
      <Row label={t('Remote style')}><Seg k="remoteStyle" label={t('Remote style')} opts={[['floating', t('Floating')], ['bar', t('Above the timeline')], ['barBelow', t('Below the timeline')]]} {...p} /></Row>
      <Row label={t('Hide controls after')}><Seg k="hideAfter" label={t('Hide controls after')} opts={[[2, t('{n} s', { n: num(2) })], [3.5, t('{n} s', { n: num(3.5) })], [6, t('{n} s', { n: num(6) })], [0, t('Never')]]} {...p} /></Row>
      <h5 className="vs-sub">{t('Picture')}</h5>
      <Row label={t('Picture')}><Seg k="picture" label={t('Picture')} opts={[['fit', t('Fit')], ['fill', t('Fill screen')]]} {...p} /></Row>
      {isDisc && <Row label={t('Forced captions')}><Toggle k="forcedSubs" label={t('Forced captions: signs and foreign-language lines the disc shows even with subtitles off')} {...p} /></Row>}
      {isDisc && <Row label={t('Disc restrictions')}><Toggle k="discLocks" label={t('Disc restrictions: obey a disc’s bans on seeking and skipping, as a disc player does')} {...p} /></Row>}
      {isDisc && <Row label={t('Subtitle size')}><Seg k="subtitleSize" label={t('Subtitle size')} opts={[[0.8, 'S'], [1, 'M'], [1.25, 'L'], [1.5, 'XL']]} {...p} /></Row>}
      {show3D && <>
        <h5 className="vs-sub">{isDisc ? t('3D (this disc only)') : t('3D (this video only)')}</h5>
        <p className="vs-note">{isDisc ? t('3D settings never apply to other discs. Use them only for a 3D picture: on a 2D one they show half of it, stretched.') : t('3D settings never apply to other videos. Use them only for a 3D picture: on a 2D one they show half of it, stretched.')}</p>
        <Row label={t('3D video')}><Seg k="stereo" label={t('3D video')} opts={[['off', t('Off')], ['sbs', t('Side by side')], ['ou', t('Over-under')]]} {...p} /></Row>
        {prefs.stereo !== 'off' && <Row label={t('Show 3D as')}><Seg k="stereoOut" label={t('Show 3D as')} opts={[['2d', t('2D (one eye)')], ['anaglyph', t('Red/cyan glasses')]]} {...p} /></Row>}
      </>}
      {streaming && onStreaming && (
        <><h5 className="vs-sub">{t('Streaming')}</h5><Row label={t('Streaming on this device')}>
          <div className="seg" role="radiogroup" aria-label={t('Streaming on this device')}>
            {([['auto', t('Server’s choice')], ['standard', t('Standard')], ['robust', t('Robust')]] as [DeviceStreaming, string][]).map(([v, l]) => (
              <button type="button" key={v} role="radio" aria-checked={streaming === v} className={streaming === v ? 'active' : ''} onClick={() => onStreaming(v)}>{l}</button>
            ))}
          </div>
        </Row></>
      )}
      <p className="vs-note">{scope === 'item' ? (isDisc ? t('Seek bar, remote and picture: saved for this disc on this device.') : t('Seek bar, remote and picture: saved for this video on this device.')) : t('Seek bar, remote and picture: saved for all videos on this device.')}{show3D ? ' ' + (isDisc ? t('3D: saved for this disc.') : t('3D: saved for this video.')) : ''}</p>
    </div>
  )
}
