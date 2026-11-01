import { contrast, PREVIEW_SIZES, previewSizeName, readableOn, resetLook, setLook, themeSuggestion, type Colours, type Layout, type Look } from './look'

// "Look": how the guide appears. The first thing here is the big choice, the old
// cable box or the modern guide, each drawn as a small picture of itself. Changes
// show straight away behind the panel, and are kept with the account.

function Choice<T extends string | number>({ label, value, options, onChange, hint }: { label: string; value: T; options: { v: T; label: string }[]; onChange: (v: T) => void; hint?: string }) {
  return (
    <fieldset className="ctv-choice">
      <legend>{label}</legend>
      <div className="ctv-choice-opts">
        {options.map((o) => (
          <label key={String(o.v)} className={value === o.v ? 'on' : ''}>
            <input type="radio" name={label} checked={value === o.v} onChange={() => onChange(o.v)} /> {o.label}
          </label>
        ))}
      </div>
      {hint && <small>{hint}</small>}
    </fieldset>
  )
}

// A small drawing of each look, so the choice can be made at a glance.
function LookCard({ name, desc, on, onPick, kind }: { name: string; desc: string; on: boolean; onPick: () => void; kind: 'cable' | 'modern' }) {
  return (
    <button type="button" role="radio" aria-checked={on} className={'ctv-lookcard ' + kind + (on ? ' on' : '')} onClick={onPick}>
      <span className="ctv-lookcard-pic" aria-hidden="true">
        <i className="top" /><i className="pv" />
        <i className="row a" /><i className="row b" /><i className="row c" />
      </span>
      <b>{name}</b>
      <small>{desc}</small>
    </button>
  )
}

const SWATCHES: { v: Colours; label: string }[] = [
  { v: 'cable', label: 'Cable blue' }, { v: 'satellite', label: 'Satellite black and amber' }, { v: 'midnight', label: 'Midnight' }, { v: 'theme', label: 'My theme' }, { v: 'custom', label: 'My own colours' },
]

export default function LookPanel({ look, onClose }: { look: Look; onClose: () => void }) {
  const pick = (layout: Layout, colours: Colours) => setLook({ layout, colours })
  const theme = themeSuggestion()
  const hasTheme = !!(theme.layout || theme.colours)
  const accentContrast = contrast(look.customAccent, look.customBg)
  return (
    <div className="modal-backdrop ctv-look-backdrop" onClick={onClose}>
      <div className="modal ctv-look" role="dialog" aria-modal="true" aria-labelledby="ctv-look-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ctv-look-h">Look of the guide</h2>
        <p className="muted">Changes show straight away, and follow you to your other devices.</p>

        <div className="ctv-choice">
          <div className="ctv-lookcards" role="radiogroup" aria-label="Style">
            <LookCard kind="cable" name="Classic cable box" desc="Chunky, bevelled and square, like a box of the early 2000s." on={look.layout === 'classic'} onPick={() => pick('classic', look.colours === 'midnight' ? 'cable' : look.colours)} />
            <LookCard kind="modern" name="Modern" desc="Clean and dark, with the programme's picture up top." on={look.layout === 'streaming'} onPick={() => pick('streaming', look.colours === 'cable' || look.colours === 'satellite' ? 'midnight' : look.colours)} />
          </div>
        </div>

        <Choice label="Colours" value={look.colours} onChange={(v) => setLook({ colours: v })} options={SWATCHES} />
        {look.colours === 'custom' && (
          <div className="ctv-custom">
            <label>Background <input type="color" value={look.customBg} onChange={(e) => setLook({ customBg: e.target.value })} /></label>
            <label>Highlight <input type="color" value={look.customAccent} onChange={(e) => setLook({ customAccent: e.target.value })} /></label>
            <span className="ctv-custom-sample" style={{ background: look.customBg, color: readableOn(look.customBg) }}>Text on this background</span>
            {accentContrast < 4.5 && <small>These two are close together, so the guide will adjust the highlight until it can be read.</small>}
          </div>
        )}
        {hasTheme && <p className="muted ctv-theme-note">Your theme suggests {[theme.layout === 'classic' ? 'the classic box' : theme.layout === 'streaming' ? 'the modern look' : '', theme.colours ? `${theme.colours} colours` : ''].filter(Boolean).join(' with ')}. What you choose here wins. <button type="button" className="btn sm ghost" onClick={resetLook}>Use its suggestion</button></p>}

        <Choice label="Preview" value={look.preview} onChange={(v) => setLook({ preview: v })} options={[{ v: 'highlight', label: 'The highlighted channel' }, { v: 'last', label: 'The last channel I watched' }, { v: 'off', label: 'Hidden' }]}
          hint="A preview runs a stream on the server, and on a tuner it uses up one tuner. If the server struggles, use the last channel, or hide it." />
        {look.preview !== 'off' && (
          <>
            <Choice label="Preview side" value={look.previewSide} onChange={(v) => setLook({ previewSide: v })} options={[{ v: 'right', label: 'Right' }, { v: 'left', label: 'Left' }]} />
            <Choice label="Preview size" value={PREVIEW_SIZES.some((s) => s.w === look.previewW) ? look.previewW : 0} onChange={(v) => setLook({ previewW: v })} options={PREVIEW_SIZES.map((s) => ({ v: s.w, label: s.name }))}
              hint={`Now: ${previewSizeName(look.previewW)}. You can also drag the corner of the preview, or focus that corner and use the arrow keys. It's held to what fits this screen.`} />
          </>
        )}
        <Choice label="Row size" value={look.density} onChange={(v) => setLook({ density: v })} options={[{ v: 'compact', label: 'Compact' }, { v: 'normal', label: 'Normal' }, { v: 'large', label: 'Large' }]} />
        <Choice label="Hours across the screen" value={look.hours} onChange={(v) => setLook({ hours: v })} options={[{ v: 0, label: 'What fits' }, { v: 1.5, label: '1½' }, { v: 2, label: '2' }, { v: 3, label: '3' }, { v: 4, label: '4' }]} />
        <fieldset className="ctv-choice">
          <legend>Show</legend>
          <div className="ctv-choice-checks">
            <label><input type="checkbox" checked={look.nowLine} onChange={(e) => setLook({ nowLine: e.target.checked })} /> A line at the current time, down all channels</label>
            <label><input type="checkbox" checked={look.progress} onChange={(e) => setLook({ progress: e.target.checked })} /> How far into each programme it is</label>
            <label><input type="checkbox" checked={look.subtitles} onChange={(e) => setLook({ subtitles: e.target.checked })} /> Episode names and details in the rows</label>
            <label><input type="checkbox" checked={look.logos} onChange={(e) => setLook({ logos: e.target.checked })} /> Channel logos</label>
            <label><input type="checkbox" checked={look.art} onChange={(e) => setLook({ art: e.target.checked })} /> The programme's picture at the top (modern look)</label>
          </div>
        </fieldset>
        <div className="actions">
          <button className="btn ghost" onClick={resetLook}>Back to how it started</button>
          <button className="btn primary" onClick={onClose}>Done</button>
        </div>
      </div>
    </div>
  )
}

