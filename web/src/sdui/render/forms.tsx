// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Forms and controls (docs/SDUI.md, "Forms and controls"): a List row with a switch, a choice, a field or a slider, and a Form that
// saves them together. Outside a Form a control runs its row's command when it changes (a switch as the server has already worked
// it out; the rest with the new value); inside one it runs nothing, and Save runs the Form's command with every named value.
import { createContext, useContext, useState, type ReactNode } from 'react'
import { Button } from '../../ui/Button'
import { Input, Select, Switch } from '../../ui/Field'
import type { Action } from '../types'
import { formValues, saveAction, withValue, type Control } from '../formsModel'
import type { Node } from '../types'
import { usePage } from './context'

// What a Form is holding: values by control name, and whether it is in one.
type FormState = { values: Record<string, unknown>; set: (name: string, v: unknown) => void } | null
const FormCtx = createContext<FormState>(null)

export function ControlView({ control, label, action }: { control: Control; label: string; action: Action | undefined }) {
  const page = usePage()
  const form = useContext(FormCtx)
  const name = control.name || ''
  const [local, setLocal] = useState<unknown>(control.value)
  const value = form && name in form.values ? form.values[name] : local
  const change = (v: unknown, run: boolean) => {
    setLocal(v)
    if (form) { if (name) form.set(name, v); return }
    if (run && !page.inert) page.act(withValue(action, control, v))
  }
  const off = !!control.disabled || !!page.inert
  const id = 'sdui-c-' + name
  const why = control.disabled && control.why ? <span className="muted sdui-control-why" id={id + 'w'}>{control.why}</span> : null
  const described = why ? id + 'w' : undefined
  switch (control.type) {
    case 'switch': return <span className="sdui-control">{why}<Switch label={label} checked={value === true} disabled={off} onChange={(v) => change(v, true)} /></span>
    case 'select': return <span className="sdui-control">{why}<Select inputSize="sm" aria-label={label} aria-describedby={described} disabled={off} value={String(value ?? '')} options={control.options || []} onChange={(e) => change(e.target.value, true)} /></span>
    case 'field': return <span className="sdui-control">{why}<Input inputSize="sm" aria-label={label} aria-describedby={described} disabled={off} type={control.input === 'number' ? 'number' : control.input === 'url' ? 'url' : control.input === 'search' ? 'search' : control.input === 'password' ? 'password' : 'text'}
      placeholder={control.placeholder} maxLength={control.maxLength} value={String(value ?? '')} onChange={(e) => change(e.target.value, false)}
      onBlur={() => { if (!form && !page.inert) page.act(withValue(action, control, value)) }} onKeyDown={(e) => { if (e.key === 'Enter' && !form) page.act(withValue(action, control, value)) }} /></span>
    case 'slider': return <span className="sdui-control">{why}<input type="range" className="sdui-slider" aria-label={label} aria-describedby={described} disabled={off} min={control.min ?? 0} max={control.max ?? 100} step={control.step ?? 1} value={Number(value ?? control.min ?? 0)}
      onChange={(e) => change(Number(e.target.value), false)} onPointerUp={() => { if (!form && !page.inert) page.act(withValue(action, control, value)) }} onKeyUp={() => { if (!form && !page.inert) page.act(withValue(action, control, value)) }} />{control.unit && <span className="muted">{Number(value ?? 0)} {control.unit}</span>}</span>
  }
}

// The Form: its children (Lists, Text, Notices), then Save and Cancel. Save is off until something has changed.
export function FormView({ title, submitLabel, cancelLabel, action, nodes, children }: { title?: string; submitLabel?: string; cancelLabel?: string; action: Action | undefined; nodes?: Node[]; children: ReactNode }) {
  const page = usePage()
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [busy, setBusy] = useState(false)
  const changed = Object.keys(values).length > 0
  return (
    <FormCtx.Provider value={{ values, set: (n, v) => setValues((x) => ({ ...x, [n]: v })) }}>
      <form className="sdui-form" onSubmit={(e) => { e.preventDefault(); if (!changed || page.inert) return; setBusy(true); page.act(saveAction(action, formValues(nodes, values))); window.setTimeout(() => { setBusy(false); setValues({}) }, 600) }}>
        {title && <h2 className="sdui-form-title">{title}</h2>}
        {children}
        <div className="sdui-form-actions">
          {cancelLabel && <Button variant="ghost" onClick={() => !page.inert && page.act({ type: 'back' })}>{cancelLabel}</Button>}
          <Button type="submit" variant="primary" busy={busy} disabled={!changed || page.inert}>{submitLabel || 'Save'}</Button>
        </div>
      </form>
    </FormCtx.Provider>
  )
}
