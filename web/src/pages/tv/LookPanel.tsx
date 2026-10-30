import { resetLook, setLook, type Look } from './look'

// "Look": everything about how the guide appears on this device. Changes show
// straight away behind the panel.

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

export default function LookPanel({ look, onClose }: { look: Look; onClose: () => void }) {
  return (
    <div className="modal-backdrop ctv-look-backdrop" onClick={onClose}>
      <div className="modal ctv-look" role="dialog" aria-modal="true" aria-labelledby="ctv-look-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ctv-look-h">Look of the guide</h2>
        <p className="muted">For this device. Changes show straight away.</p>
        <Choice label="Layout" value={look.layout} onChange={(v) => setLook({ layout: v })} options={[{ v: 'classic', label: 'Classic box' }, { v: 'streaming', label: 'Streaming' }]}
          hint={look.layout === 'classic' ? 'Chunky and bevelled, like a cable or satellite box of the early 2000s.' : 'Clean rows, with a line down every channel at the current time.'} />
        <Choice label="Colours" value={look.colours} onChange={(v) => setLook({ colours: v })} options={[{ v: 'cable', label: 'Cable blue' }, { v: 'satellite', label: 'Satellite black and amber' }, { v: 'theme', label: 'My theme' }]} />
        <Choice label="Preview" value={look.preview} onChange={(v) => setLook({ preview: v })} options={[{ v: 'highlight', label: 'The highlighted channel' }, { v: 'last', label: 'The last channel I watched' }, { v: 'off', label: 'Off' }]}
          hint="A preview runs a stream on the server, and on a tuner it uses up one tuner. If the server struggles, use the last channel, or turn it off." />
        {look.preview !== 'off' && (
          <>
            <Choice label="Preview position" value={look.previewPos} onChange={(v) => setLook({ previewPos: v })} options={[{ v: 'tr', label: 'Top right' }, { v: 'tl', label: 'Top left' }, { v: 'br', label: 'Bottom right' }, { v: 'bl', label: 'Bottom left' }, { v: 'custom', label: 'Where I drag it' }]}
              hint="You can also drag it by its handle, or focus the handle and use the arrow keys." />
            <Choice label="Preview size" value={look.previewSize} onChange={(v) => setLook({ previewSize: v })} options={[{ v: 's', label: 'Small' }, { v: 'm', label: 'Medium' }, { v: 'l', label: 'Large' }]} />
          </>
        )}
        <Choice label="Row size" value={look.density} onChange={(v) => setLook({ density: v })} options={[{ v: 'compact', label: 'Compact' }, { v: 'normal', label: 'Normal' }, { v: 'large', label: 'Large' }]} />
        <Choice label="Hours across the screen" value={look.hours} onChange={(v) => setLook({ hours: v })} options={[{ v: 0, label: 'What fits' }, { v: 1.5, label: '1½' }, { v: 2, label: '2' }, { v: 3, label: '3' }, { v: 4, label: '4' }]} />
        <fieldset className="ctv-choice">
          <legend>Show</legend>
          <div className="ctv-choice-checks">
            <label><input type="checkbox" checked={look.nowLine} onChange={(e) => setLook({ nowLine: e.target.checked })} /> A line at the current time, down all channels</label>
            <label><input type="checkbox" checked={look.progress} onChange={(e) => setLook({ progress: e.target.checked })} /> How far into each programme it is</label>
            <label><input type="checkbox" checked={look.subtitles} onChange={(e) => setLook({ subtitles: e.target.checked })} /> Episode names</label>
            <label><input type="checkbox" checked={look.logos} onChange={(e) => setLook({ logos: e.target.checked })} /> Channel logos</label>
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
