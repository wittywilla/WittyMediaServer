import type { Scope, ViewPrefs } from './viewPrefs'

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

// The gear menu: viewing options that apply to this disc or to everything.
export default function ViewSettings({ prefs, scope, onChange, onScope, onClose, isDisc }: {
  prefs: ViewPrefs; scope: Scope; onChange: Change; onScope: (s: Scope) => void; onClose: () => void; isDisc: boolean
}) {
  const p = { prefs, onChange }
  return (
    <div className="view-settings" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} role="dialog" aria-label="Viewing settings"
      onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }}>
      <div className="vs-head">
        <h4>Viewing settings</h4>
        <button type="button" className="ibtn" onClick={onClose} aria-label="Close viewing settings">✕</button>
      </div>
      <Row label="Apply to">
        <div className="seg" role="radiogroup" aria-label="Apply to">
          <button type="button" role="radio" aria-checked={scope === 'item'} className={scope === 'item' ? 'active' : ''} onClick={() => onScope('item')}>{isDisc ? 'This disc' : 'This video'}</button>
          <button type="button" role="radio" aria-checked={scope === 'global'} className={scope === 'global' ? 'active' : ''} onClick={() => onScope('global')}>Everything</button>
        </div>
      </Row>
      <Row label="Seek bar"><Toggle k="seekBar" label="Seek bar" {...p} /></Row>
      <Row label="Chapter marks"><Toggle k="chapterMarks" label="Chapter marks" {...p} /></Row>
      <Row label={isDisc ? 'Disc remote' : 'Remote'}><Toggle k="remote" label="Remote" {...p} /></Row>
      <Row label="Remote style"><Seg k="remoteStyle" label="Remote style" opts={[['floating', 'Floating'], ['bar', 'In control bar']]} {...p} /></Row>
      <Row label="Hide controls after"><Seg k="hideAfter" label="Hide controls after" opts={[[2, '2 s'], [3.5, '3.5 s'], [6, '6 s'], [0, 'Never']]} {...p} /></Row>
      <Row label="Picture"><Seg k="picture" label="Picture" opts={[['fit', 'Fit'], ['fill', 'Fill screen']]} {...p} /></Row>
      {isDisc && <Row label="Forced captions"><Toggle k="forcedSubs" label="Forced captions: signs and foreign-language lines the disc shows even with subtitles off" {...p} /></Row>}
      {isDisc && <Row label="Disc restrictions"><Toggle k="discLocks" label="Disc restrictions: obey a disc's bans on seeking and skipping, as a disc player does" {...p} /></Row>}
      {isDisc && <Row label="Subtitle size"><Seg k="subtitleSize" label="Subtitle size" opts={[[0.8, 'S'], [1, 'M'], [1.25, 'L'], [1.5, 'XL']]} {...p} /></Row>}
      <Row label="3D video"><Seg k="stereo" label="3D video" opts={[['off', 'Off'], ['sbs', 'Side by side'], ['ou', 'Over-under']]} {...p} /></Row>
      {prefs.stereo !== 'off' && <Row label="Show 3D as"><Seg k="stereoOut" label="Show 3D as" opts={[['2d', '2D (one eye)'], ['anaglyph', 'Red/cyan glasses']]} {...p} /></Row>}
      <p className="vs-note">{scope === 'item' ? `Saved for ${isDisc ? 'this disc' : 'this video'} on this device.` : 'Saved for all videos on this device.'}</p>
    </div>
  )
}
