import type { DriverField } from './tvApi'

// One setting of a WittyTuner driver, drawn from what the driver says about it. The tuner's own
// page and the guided setup both use it, so a field reads the same in both.
export default function TunerField({ f, value, onChange }: { f: DriverField; value: string; onChange: (v: string) => void }) {
  const id = 'tf-' + f.key
  const help = f.help ? <small id={id + '-h'} className="tvs-help">{f.help}</small> : null
  const aria = { id, 'aria-describedby': f.help ? id + '-h' : undefined, required: f.required }
  if (f.kind === 'toggle') {
    return <label className="check-row"><input type="checkbox" id={id} checked={(value || f.default) === '1'} onChange={(e) => onChange(e.target.checked ? '1' : '0')} /> {f.label}{help}</label>
  }
  return (
    <label htmlFor={id}>{f.label}{f.required ? ' (needed)' : ''}
      {f.kind === 'select' ? (
        <select className="select" {...aria} value={value || f.default || ''} onChange={(e) => onChange(e.target.value)}>
          {(f.options || []).map((o) => <option key={o} value={o}>{o || 'None'}</option>)}
        </select>
      ) : f.kind === 'textarea' ? (
        <textarea className="input mono" rows={4} {...aria} value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} />
      ) : (
        <input className="input" type={f.kind === 'password' ? 'password' : f.kind === 'number' ? 'number' : 'text'} autoComplete={f.kind === 'password' ? 'new-password' : 'off'} {...aria} value={value} placeholder={f.default} onChange={(e) => onChange(e.target.value)} />
      )}
      {help}
    </label>
  )
}
