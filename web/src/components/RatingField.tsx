import { useState } from 'react'

const MOVIE = ['G', 'PG', 'PG-13', 'R', 'NC-17']
const TV = ['TV-Y', 'TV-Y7', 'TV-G', 'TV-PG', 'TV-14', 'TV-MA']

// RatingField picks a content rating. The rating decides what Kids and Guest
// profiles can see; any certificate the server understands may be typed
// under "Other" (12A, 15, FSK 16, …).
export default function RatingField({ value, onChange, tv }: { value: string; onChange: (v: string) => void; tv?: boolean }) {
  const known = [...(tv ? TV : MOVIE), ...(tv ? MOVIE : TV)]
  const [other, setOther] = useState(value !== '' && !known.includes(value))
  return (
    <label>Content rating <span className="muted">(decides what Kids and Guest profiles can see)</span>
      <div className="field-row">
        <select className="select" value={other ? '__other' : value} onChange={(e) => {
          if (e.target.value === '__other') { setOther(true); return }
          setOther(false)
          onChange(e.target.value)
        }}>
          <option value="">Not rated</option>
          <optgroup label={tv ? 'TV' : 'Movies'}>{(tv ? TV : MOVIE).map((r) => <option key={r} value={r}>{r}</option>)}</optgroup>
          <optgroup label={tv ? 'Movies' : 'TV'}>{(tv ? MOVIE : TV).map((r) => <option key={r} value={r}>{r}</option>)}</optgroup>
          <option value="__other">Other…</option>
        </select>
        {other && <input className="input" aria-label="Other rating" placeholder="e.g. 12A, 15, FSK 16" value={value} onChange={(e) => onChange(e.target.value)} />}
      </div>
    </label>
  )
}
