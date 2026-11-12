// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useId, useMemo, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Dock, type DockAction } from '../ui/Dock'
import { Field, Input, Select, Switch, Textarea } from '../ui/Field'
import { QrCode } from '../components/QrCode'
import { sectionId, withIds } from './model'
import type { SpecCard, SpecChip, SpecField, SpecPageData, SpecRow, SpecSection } from './types'
import './spec.css'

// One page of Design 2 drawn from data (docs/design-2, "Every feature, drawn"): eyebrow, title, lead, then rows,
// chips, notes and cards, a Dock for the page's own buttons, and a foot line. Colours, shape and type come from
// the look and theme in use (tokens only); every item that goes somewhere or is handled is a link or a button in
// the page's Tab order, reached by the arrows, a controller and a remote through the app's spatial navigation,
// with the Dock as the page's own zone. Items with neither are plain text and are not focusable.
//
// `handled` lists the ids onAction answers; an item with `to` needs no handler.
export interface SpecPageProps {
  page: SpecPageData
  onAction?: (id: string) => void
  // A field changed: its id and the new value (a File for a file picker). The page keeps the value and builds the spec again.
  onField?: (id: string, value: string | number | boolean | File) => void
  handled?: ReadonlySet<string>
  // Extra content under a section's items (a page's own control that has no place in the spec), by section index.
  after?: Record<number, ReactNode>
  className?: string
  // In a sheet or dialog that already has its own title: no header, only the sections, Dock and foot.
  embedded?: boolean
  // Content before the first section (a search field).
  before?: ReactNode
  // Part of a page that has its own heading: no eyebrow, title or lead of its own, and sections are h3.
  bare?: boolean
}

export default function SpecPage({ page: raw, onAction, onField, handled, after, className, embedded, before, bare }: SpecPageProps) {
  const page = useMemo(() => withIds(raw), [raw])
  const live = (it: { id?: string; to?: string; disabled?: boolean }) => !it.disabled && (!!it.to || (!!it.id && !!handled?.has(it.id) && !!onAction))
  const dock: DockAction[] = page.actions.map((a) => ({
    id: a.id || a.title, label: a.title, primary: a.kind === 'primary', to: a.to, disabled: a.disabled || (!a.to && !(a.id && handled?.has(a.id))),
    onClick: a.to ? undefined : () => a.id && onAction?.(a.id),
  }))
  return (
    <article className={'spec-page' + (embedded ? ' embedded' : '') + (bare ? ' bare' : '') + (className ? ' ' + className : '')} data-spec={page.key}>
      {!embedded && !bare && (
        <header className="spec-head">
          <p className="spec-eyebrow">{page.eyebrow}</p>
          <h1>{page.title}</h1>
          {page.lead && <p className="spec-lead">{page.lead}</p>}
        </header>
      )}
      {before}
      {page.sections.map((s, i) => (
        <Section key={i} s={s} bare={!!bare} hid={s.anchor || sectionId(page.key, i)} live={live} onAction={onAction} onField={onField}>{after?.[i]}</Section>
      ))}
      {page.foot && <p className="spec-foot">{page.foot}</p>}
      {dock.length > 0 && <Dock label={page.title} actions={dock} legend={false} className="spec-dock" />}
    </article>
  )
}

type Live = (it: { id?: string; to?: string; disabled?: boolean }) => boolean

type OnField = (id: string, value: string | number | boolean | File) => void

function Section({ s, hid, live, onAction, onField, bare, children }: { s: SpecSection; bare: boolean; hid: string; live: Live; onAction?: (id: string) => void; onField?: OnField; children?: ReactNode }) {
  const H = bare ? 'h3' : 'h2'
  const h = s.h ? <H id={hid} className="spec-h">{s.h}</H> : null
  const label = s.h ? { 'aria-labelledby': hid } : {}
  if (s.kind === 'note') {
    return <section className="spec-sec" {...label}>{h}<p className={'spec-note' + (s.tone ? ' tone' : '')}>{s.text}</p>{children}</section>
  }
  return (
    <section className="spec-sec" {...label}>
      {h}
      {s.kind === 'rows' && <ul className="spec-rows" role={s.choice ? 'radiogroup' : undefined} {...(s.choice ? label : {})} onKeyDown={s.choice ? arrowChoose : undefined}>{s.items.map((r) => <li key={r.id}><Row r={r} live={live(r)} onAction={onAction} choice={!!s.choice} /></li>)}</ul>}
      {s.kind === 'fields' && <div className="spec-fields">{s.items.map((f) => <FieldItem key={f.id} f={f} onField={onField} />)}</div>}
      {s.kind === 'chips' && <div className="spec-chips" role="group" {...label}>{s.items.map((c) => <Chip key={c.id} c={c} live={live(c)} onAction={onAction} />)}</div>}
      {s.kind === 'cards' && <ul className="spec-cards">{s.items.map((c) => <li key={c.id}><Card c={c} live={live(c)} onAction={onAction} /></li>)}</ul>}
      {children}
    </section>
  )
}

// In a choice, Up and Down (and Left and Right) move to the next row and choose it, as radio buttons do.
function arrowChoose(e: React.KeyboardEvent<HTMLUListElement>) {
  const d = e.key === 'ArrowDown' || e.key === 'ArrowRight' ? 1 : e.key === 'ArrowUp' || e.key === 'ArrowLeft' ? -1 : 0
  if (!d) return
  const rows = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('[role=radio]:not([disabled])'))
  const i = rows.indexOf(document.activeElement as HTMLElement)
  const next = rows[i + d]
  if (i < 0 || !next) return
  e.preventDefault(); e.stopPropagation(); next.focus(); next.click()
}

function Row({ r, live, onAction, choice }: { r: SpecRow; live: boolean; onAction?: (id: string) => void; choice?: boolean }) {
  const body = (
    <>
      <span className="spec-row-main"><span className="spec-row-title">{r.title}</span>{r.sub && <small className="spec-row-sub">{r.sub}</small>}</span>
      {r.value && <span className="spec-row-value">{r.value}</span>}
    </>
  )
  const cls = 'spec-row' + (r.on ? ' on' : '') + (live ? ' live' : '')
  const cur = r.on && !choice ? { 'aria-current': 'true' as const } : {}
  if (live && r.to) return <Link to={r.to} className={cls} {...cur}>{body}</Link>
  if (live && choice) return <button type="button" role="radio" aria-checked={!!r.on} tabIndex={r.on ? 0 : -1} className={cls} onClick={() => r.id && onAction?.(r.id)}>{body}</button>
  if (live) return <button type="button" className={cls} onClick={() => r.id && onAction?.(r.id)} {...cur}>{body}</button>
  return <div className={cls} {...cur}>{body}</div>
}

function Chip({ c, live, onAction }: { c: SpecChip; live: boolean; onAction?: (id: string) => void }) {
  const cls = 'spec-chip' + (c.on ? ' on' : '')
  if (live && c.to) return <Link to={c.to} className={cls} aria-label={c.label} aria-current={c.on ? 'true' : undefined}>{c.title}</Link>
  if (live) return <button type="button" className={cls} aria-label={c.label} aria-pressed={!!c.on} onClick={() => c.id && onAction?.(c.id)}>{c.title}</button>
  return <span className={cls} aria-current={c.on ? 'true' : undefined}>{c.title}</span>
}

function Card({ c, live, onAction }: { c: SpecCard; live: boolean; onAction?: (id: string) => void }) {
  const tint: CSSProperties | undefined = c.bg ? ({ '--spec-card-a': c.bg, '--spec-card-b': c.bg2 || c.bg } as CSSProperties) : undefined
  const body = (
    <>
      <span className="spec-card-art" aria-hidden="true" style={c.art ? { backgroundImage: `url("${c.art}")` } : undefined} />
      <span className="spec-card-title">{c.title}</span>
      {c.sub && <small className="spec-card-sub">{c.sub}</small>}
    </>
  )
  const cls = 'spec-card' + (c.on ? ' on' : '') + (c.art ? ' has-art' : '')
  const cur = c.on ? { 'aria-current': 'true' as const } : {}
  if (live && c.to) return <Link to={c.to} className={cls} style={tint} {...cur}>{body}</Link>
  if (live) return <button type="button" className={cls} style={tint} onClick={() => c.id && onAction?.(c.id)} {...cur}>{body}</button>
  return <div className={cls} style={tint} {...cur}>{body}</div>
}

// Digits and letters of a code in groups, read out whole by a screen reader.
const groups = (v: string) => v.replace(/\s+/g, '').split('')

function FieldItem({ f, onField }: { f: SpecField; onField?: OnField }) {
  const id = f.id || ''
  const sid = useId()
  const set = (v: string | number | boolean | File) => onField?.(id, v)
  const dis = f.disabled || !onField
  const common = { disabled: dis, placeholder: f.placeholder, autoComplete: f.autoComplete, autoFocus: f.autoFocus, 'data-field': id }
  if (f.kind === 'code') {
    const v = String(f.value ?? '')
    return (
      <div className="spec-field spec-code" role="group" aria-label={f.label}>
        <span className="spec-field-label">{f.label}</span>
        <span className="spec-code-chars" role="text" aria-label={groups(v).join(' ')}>{groups(v).map((c, i) => <b key={i} aria-hidden="true">{c}</b>)}</span>
        {f.hint && <span className="spec-field-hint">{f.hint}</span>}
      </div>
    )
  }
  if (f.kind === 'qr') {
    return (
      <div className="spec-field spec-qr">
        <span className="spec-field-label">{f.label}</span>
        <QrCode text={f.text || ''} label={f.hint || f.label} />
        {f.hint && <span className="spec-field-hint">{f.hint}</span>}
      </div>
    )
  }
  if (f.kind === 'switch') {
    return (
      <div className="spec-field spec-switch">
        <span id={sid + 'l'} className="spec-field-label">{f.label}{f.hint && <small className="spec-field-hint">{f.hint}</small>}</span>
        <Switch checked={!!f.value} onChange={set} labelledBy={sid + 'l'} disabled={dis} />
      </div>
    )
  }
  const body =
    f.kind === 'select' ? <Select {...common} value={String(f.value ?? '')} options={f.options} onChange={(e) => set(e.target.value)} />
    : f.kind === 'slider' ? <SliderControl f={f} dis={dis} set={set} />
    : f.kind === 'number' ? <Input {...common} type="number" inputMode="decimal" value={String(f.value ?? '')} min={f.min} max={f.max} step={f.step} onChange={(e) => set(e.target.value === '' ? '' : Number(e.target.value))} />
    : f.kind === 'pin' ? <Input {...common} className="spec-pin" type="password" inputMode="numeric" pattern="[0-9]*" autoComplete={f.autoComplete || 'one-time-code'} maxLength={f.length} value={String(f.value ?? '')} onChange={(e) => set(e.target.value.replace(/\D/g, '').slice(0, f.length))} />
    : f.kind === 'textarea' ? <Textarea {...common} rows={4} value={String(f.value ?? '')} onChange={(e) => set(e.target.value)} />
    : f.kind === 'time' ? <Input {...common} type="time" value={String(f.value ?? '')} onChange={(e) => set(e.target.value)} />
    : f.kind === 'file' ? <input {...common} aria-label={f.label} className="spec-file" type="file" accept={f.accept} onChange={(e) => { const file = e.target.files?.[0]; if (file) set(file) }} />
    : <Input {...common} type={f.kind === 'password' ? 'password' : 'text'} value={String(f.value ?? '')} onChange={(e) => set(e.target.value)} />
  return <Field label={f.label} hint={f.hint} error={f.error} required={f.required} className="spec-field">{body}</Field>
}

function SliderControl({ f, dis, set }: { f: SpecField; dis: boolean; set: (v: number) => void }) {
  const v = Number(f.value ?? f.min ?? 0)
  return (
    <span className="spec-slider">
      <input type="range" aria-label={f.label} min={f.min} max={f.max} step={f.step ?? 1} value={v} disabled={dis} aria-valuetext={v + (f.unit || '')} onChange={(e) => set(Number(e.target.value))} />
      <output>{v}{f.unit}</output>
    </span>
  )
}
