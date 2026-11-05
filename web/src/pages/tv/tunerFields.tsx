import type { Driver, DriverField } from './tvApi'

// What a tuner driver asks for, as a form field, and how well each driver has
// been tried. Shared by the setup and a tuner's own settings.

export const STATUS: Record<Driver['status'], { label: string; desc: string }> = {
  tested: { label: 'Tested', desc: 'Tested end to end.' },
  mock: { label: 'Tried on stand-ins', desc: 'Built from the published protocol and tried against stand-in servers, not yet on real hardware.' },
  untested: { label: 'Not yet tried', desc: 'Built from the published documentation. It has not been tried on real hardware yet, so please report what you find.' },
  planned: { label: 'Not built yet', desc: '' },
}

export function Field({ f, value, onChange, autoFocus }: { f: DriverField; value: string; onChange: (v: string) => void; autoFocus?: boolean }) {
  const id = 'tf-' + f.key
  const help = f.help ? <small id={id + '-h'} className="tvs-help">{f.help}</small> : null
  const aria = { id, 'aria-describedby': f.help ? id + '-h' : undefined, required: f.required, autoFocus }
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
