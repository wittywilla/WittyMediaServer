// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useMemo, useState } from 'react'
import { Input, Select, Switch } from '../../ui'
import { Row } from '../settings/ui'
import { asMap, groupFields, LARGE_SCHEMA, numberFrom, placeholderOf, withMapEntry, type Field, type Values } from './emulatorsLogic'
import './emulators.css'

// A form drawn from what the server says its settings are: a switch, a number, some words, one
// of a list, or a small table of those. WittyPlay's backend and each emulator describe their own
// settings, so a setting added there shows up here by itself.
//
// values holds what's set at this level and nothing else. A field that isn't set shows what
// applies instead (under), as a faint default; with canClear, a field that is set can be put
// back to that.

type Props = {
  schema: Field[]
  values: Values
  onChange: (key: string, value: unknown) => void // undefined: not set here any more
  under: (f: Field) => unknown
  locked?: (f: Field) => string | false // why a field can't be changed here, as its badge
  canClear?: boolean
  disabled?: boolean
}

const isSet = (v: unknown) => v !== undefined && v !== null

export function SchemaForm(props: Props) {
  if (!props.schema.length) return <p className="muted">There’s nothing to set here.</p>
  return props.schema.length > LARGE_SCHEMA ? <LargeForm {...props} /> : <Fields {...props} />
}

// A whole emulator's settings: a search, a switch for the ones most people never need, and a
// heading per section of its own file, each opened when wanted. Only open sections draw rows,
// so a schema of hundreds stays quick on a television's browser.
function LargeForm(props: Props) {
  const [query, setQuery] = useState('')
  const [advanced, setAdvanced] = useState(false)
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const id = useId()
  const groups = useMemo(() => groupFields(props.schema, query, advanced), [props.schema, query, advanced])
  const shown = groups.reduce((n, g) => n + g.fields.length, 0)
  const changed = (fs: Field[]) => fs.filter((f) => isSet(props.values[f.key])).length
  return (
    <div className="emu-large">
      <div className="emu-large-tools">
        <Input type="search" aria-label="Find a setting" placeholder="Find a setting" value={query} onChange={(e) => setQuery(e.target.value)} spellCheck={false} autoComplete="off" />
        <Switch checked={advanced} onChange={setAdvanced} label="Show advanced settings" />
      </div>
      <p className="muted emu-large-count" aria-live="polite">{query ? `${shown} ${shown === 1 ? 'setting matches' : 'settings match'}` : `${shown} settings in ${groups.length} sections`}</p>
      {groups.map((g) => {
        const isOpen = !!query || !!open[g.group]
        const n = changed(g.fields)
        return (
          <section key={g.group} className="emu-group">
            <h3>
              <button type="button" className="emu-group-head" aria-expanded={isOpen} aria-controls={`${id}-${g.group}`} onClick={() => setOpen((o) => ({ ...o, [g.group]: !isOpen }))}>
                <span>{g.group}</span>
                <small className="muted">{g.fields.length}{n ? `, ${n} set here` : ''}</small>
              </button>
            </h3>
            {isOpen && <div id={`${id}-${g.group}`}><Fields {...props} schema={g.fields} /></div>}
          </section>
        )
      })}
      {!groups.length && <p className="muted">No setting matches that.</p>}
    </div>
  )
}

function Fields({ schema, values, onChange, under, locked, canClear, disabled }: Props) {
  return (
    <div className="emu-form">
      {schema.map((f) => {
        const why = locked?.(f) || ''
        const off = disabled || !!why
        const mine = values[f.key], below = under(f)
        const set = (v: unknown) => onChange(f.key, v)
        const reset = canClear && isSet(mine) && !off
          ? <button type="button" className="btn sm ghost" aria-label={`Use the default for ${f.label}`} onClick={() => set(undefined)}>Use default</button>
          : null
        return (
          <Row key={f.key} label={f.label} desc={f.help} badge={why || undefined} stack={f.kind === 'map'}>
            <>
              {f.kind === 'map'
                ? <MapField f={f} value={isSet(mine) ? mine : below} onChange={set} disabled={off} />
                : <One kind={f.kind} f={f} label={f.label} value={mine} below={below} onChange={set} disabled={off} optional={!!canClear} />}
              {reset}
            </>
          </Row>
        )
      })}
    </div>
  )
}

// One control. optional: nothing chosen is allowed, and means whatever applies under it.
function One({ kind, f, label, value, below, onChange, disabled, optional }: { kind: Field['kind'] | NonNullable<Field['mapOf']>; f: Field; label: string; value: unknown; below: unknown; onChange: (v: unknown) => void; disabled: boolean; optional: boolean }) {
  if (kind === 'toggle') return <Switch checked={Boolean(isSet(value) ? value : below)} onChange={onChange} label={label} disabled={disabled} />
  if (kind === 'number') return <NumberInput f={f} label={label} value={value} below={below} onChange={onChange} disabled={disabled} />
  if (kind === 'choice') {
    const choices = f.choices || []
    const now = isSet(value) ? String(value) : optional ? '' : String(below ?? '')
    const hint = placeholderOf({ kind: 'choice', choices }, below)
    return (
      <Select aria-label={label} value={now} disabled={disabled} onChange={(e) => onChange(e.target.value === '' && optional ? undefined : e.target.value)}>
        {optional && <option value="">{hint ? `Default (${hint})` : 'Default'}</option>}
        {/* What's kept is always in the list, even if the server no longer offers it. */}
        {now && !choices.some((c) => c.value === now) && <option value={now}>{now}</option>}
        {choices.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
      </Select>
    )
  }
  return <Input aria-label={label} value={isSet(value) ? String(value) : ''} placeholder={placeholderOf({ kind: 'text' }, below)} disabled={disabled} spellCheck={false} autoComplete="off"
    onChange={(e) => onChange(e.target.value === '' && optional ? undefined : e.target.value)} />
}

// What's typed is kept as typed ("1." on the way to "1.5"), and the number goes up as it changes.
function NumberInput({ f, label, value, below, onChange, disabled }: { f: Field; label: string; value: unknown; below: unknown; onChange: (v: unknown) => void; disabled: boolean }) {
  const [text, setText] = useState(isSet(value) ? String(value) : '')
  useEffect(() => { setText((t) => (numberFrom(t) === (isSet(value) ? Number(value) : undefined) ? t : isSet(value) ? String(value) : '')) }, [value])
  return <Input aria-label={label} className="emu-number" type="number" inputMode="decimal" min={f.min} max={f.max} step={f.step} value={text} placeholder={placeholderOf({ kind: 'number' }, below)} disabled={disabled}
    onChange={(e) => { setText(e.target.value); onChange(numberFrom(e.target.value)) }} />
}

// A small table: one row for each thing the server names (an emulator, a kind of graphics
// card), each with the same control.
function MapField({ f, value, onChange, disabled }: { f: Field; value: unknown; onChange: (v: unknown) => void; disabled: boolean }) {
  const map = asMap(value), keys = f.mapKeys || []
  if (!keys.length) return <p className="muted emu-map-none">Nothing to set here yet.</p>
  const kind = f.mapOf || 'text'
  return (
    <ul className="emu-map">
      {keys.map((k) => (
        <li key={k.value}>
          <span>{k.label}</span>
          <One kind={kind} f={f} label={`${f.label}: ${k.label}`} value={map[k.value]} below={kind === 'toggle' ? false : undefined} disabled={disabled} optional={kind !== 'toggle'}
            onChange={(v) => onChange(withMapEntry(map, k.value, v))} />
        </li>
      ))}
    </ul>
  )
}
