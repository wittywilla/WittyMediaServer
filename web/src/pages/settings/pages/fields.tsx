// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The form controls of the page editor. There is one control per TYPE of property the
// catalogue knows (text, a whole number, a choice, an ordered list of choices, an icon, a
// library, ...), and every property of every component, source and condition is drawn with
// the one for its type: nothing here knows any component by name. A type this editor has
// never heard of gets a plain text box.

import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { IconClose, IconDown, IconPlus, IconUp } from '../../../icons'
import { sduiIcon } from '../../../sdui/render/icons'
import { Button, Chip, Chips, Field as Labelled, IconButton, Input, Menu, Select, Switch, Textarea } from '../../../ui'
import {
  conditionFields, conditionText, fieldFor, fromText, navScreens, pageKindSpec, toText, valueText, words,
  type Catalogue, type Condition, type Field, type Fix, type Issue, type LibraryRef, type Option, type PageEntry,
} from './editorModel.ts'

// An issue as a form shows it: what is wrong, and a button when the fix is a thing a button can do.
export type IssueView = { issue: Issue; fix: Fix | null; apply?: () => void }

// What a control may need to know beyond its own property.
export type FieldCtx = { cat: Catalogue; libraries: LibraryRef[]; pages: PageEntry[]; sections?: Option[] }

const SEVERITY: Record<string, string> = { error: 'Problem', warning: 'Worth a look' }

export function IssueNotes({ issues }: { issues: IssueView[] }) {
  if (!issues.length) return null
  return (
    <ul className="pe-issues">
      {issues.map((v, i) => (
        <li key={i} className={'pe-issue ' + (v.issue.severity === 'warning' ? 'warn' : 'error')}>
          <span><strong>{SEVERITY[v.issue.severity] || 'Problem'}:</strong> {v.issue.message}.</span>
          {v.fix && v.apply
            ? <Button size="sm" onClick={v.apply}>{v.fix.label}</Button>
            : v.issue.fix && <span className="pe-issue-fix">{v.issue.fix}</span>}
        </li>
      ))}
    </ul>
  )
}

// What a text box shows is kept as typed, and handed on as the value it stands for
// ("12" as the number 12). It follows the value when that changes from outside: an undo,
// a fix applied, another section chosen.
function useTyped(field: Field, value: unknown, onChange: (v: unknown) => void): [string, (t: string) => void] {
  const [text, setText] = useState(() => toText(value))
  const sent = useRef(JSON.stringify(value ?? null))
  useEffect(() => {
    const now = JSON.stringify(value ?? null)
    if (now !== sent.current) { sent.current = now; setText(toText(value)) }
  }, [value])
  return [text, (t) => { setText(t); const v = fromText(field, t); sent.current = JSON.stringify(v ?? null); onChange(v) }]
}

function variableLabel(v: string): string {
  if (v === '$each') return 'Each one the section is repeated for ($each)'
  return `This page’s ${words(v.slice(1)).toLowerCase()} (${v})`
}

function Facts({ field, extra }: { field: Field; extra?: ReactNode }) {
  const facts = [field.limits, field.defaultText, field.only].filter(Boolean)
  if (!field.help && !facts.length && !extra) return null
  return <>{field.help}{field.help && facts.length > 0 && ' '}{facts.length > 0 && <span className="pe-facts">{facts.join(' · ')}</span>}{extra}</>
}

// The places in the app a "where it leads" box can suggest: the pages, the libraries and the app's own screens.
function useRoutes(ctx: FieldCtx): Option[] {
  return useMemo(() => {
    const out: Option[] = []
    for (const p of ctx.pages) {
      const route = pageKindSpec(ctx.cat, p.kind)?.route || ''
      if (route.includes('{id}')) out.push({ value: route.replace('{id}', p.id), label: p.title })
      else if (route && !route.includes('{') && !p.match) out.push({ value: route, label: p.title })
    }
    for (const l of ctx.libraries) out.push({ value: `/library/${l.id}`, label: l.name })
    for (const s of navScreens(ctx.cat)) out.push({ value: s.route, label: s.label })
    const seen = new Set<string>()
    return out.filter((o) => !seen.has(o.value) && !!seen.add(o.value))
  }, [ctx.cat, ctx.pages, ctx.libraries])
}

export function IconPicker({ cat, value, onChange, id, describedBy, none = 'No icon' }: { cat: Catalogue; value: unknown; onChange: (v: unknown) => void; id?: string; describedBy?: string; none?: string }) {
  const [open, setOpen] = useState(false)
  const panel = useId()
  const cur = typeof value === 'string' ? value : ''
  return (
    <div className="pe-icon">
      <div className="pe-icon-now">
        <button type="button" id={id} className="btn" aria-expanded={open} aria-controls={open ? panel : undefined} aria-describedby={describedBy} onClick={() => setOpen((o) => !o)}>
          {sduiIcon(cur)}<span>{cur ? words(cur) : none}</span>
        </button>
        {cur && <Button size="sm" variant="ghost" onClick={() => onChange(undefined)}>Clear</Button>}
      </div>
      {open && (
        <div className="pe-icon-grid" id={panel} role="group" aria-label="Icons" data-nav-group>
          {(cat.icons || []).map((name) => (
            <button type="button" key={name} className={'pe-icon-cell' + (name === cur ? ' on' : '')} aria-pressed={name === cur} onClick={() => { onChange(name); setOpen(false) }}>
              {sduiIcon(name)}<span>{words(name)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// An ordered list of choices, none twice: what is chosen, in order, with a way to move
// each, take it out, and add another.
function OrderedChoice({ field, value, onChange, id }: { field: Field; value: unknown; onChange: (v: unknown) => void; id?: string }) {
  const chosen = Array.isArray(value) ? value.map(String) : []
  const left = field.options.filter((o) => !chosen.includes(o.value))
  const label = (v: string) => field.options.find((o) => o.value === v)?.label || `${v} (not a choice)`
  const move = (i: number, to: number) => { const next = [...chosen]; const [x] = next.splice(i, 1); next.splice(to, 0, x); onChange(next) }
  return (
    <div className="pe-ordered">
      {chosen.length > 0 && (
        <ol className="pe-ordered-list">
          {chosen.map((v, i) => (
            <li key={v + i}>
              <span>{label(v)}</span>
              <IconButton size="sm" label={`Move ${label(v)} earlier`} disabled={i === 0} onClick={() => move(i, i - 1)}><IconUp /></IconButton>
              <IconButton size="sm" label={`Move ${label(v)} later`} disabled={i === chosen.length - 1} onClick={() => move(i, i + 1)}><IconDown /></IconButton>
              <IconButton size="sm" label={`Remove ${label(v)}`} onClick={() => { const next = chosen.filter((_, n) => n !== i); onChange(next.length ? next : undefined) }}><IconClose /></IconButton>
            </li>
          ))}
        </ol>
      )}
      {left.length > 0 && (
        <Select id={id} inputSize="sm" aria-label={`Add to ${field.label}`} value="" onChange={(e) => { if (e.target.value) onChange([...chosen, e.target.value]) }}>
          <option value="">{chosen.length ? 'Add another…' : field.hasDefault ? 'The usual ones. Choose your own…' : 'Choose…'}</option>
          {left.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </Select>
      )}
    </div>
  )
}

// Any of a few choices where the order means nothing (which devices, which days).
function AnyOf({ field, value, onChange }: { field: Field; value: unknown; onChange: (v: unknown) => void }) {
  const chosen = Array.isArray(value) ? value.map(String) : []
  const extra = chosen.filter((v) => !field.options.some((o) => o.value === v))
  const toggle = (v: string) => onChange(chosen.includes(v) ? chosen.filter((x) => x !== v) : [...chosen, v])
  return (
    <Chips label={field.label} className="pe-anyof">
      {field.options.map((o) => <Chip key={o.value} selected={chosen.includes(o.value)} onClick={() => toggle(o.value)}>{o.label}</Chip>)}
      {extra.map((v) => <Chip key={v} onRemove={() => toggle(v)} removeLabel={`Remove ${v}`}>{v} (not a choice)</Chip>)}
    </Chips>
  )
}

// PropField: one property, with its label, its help from the catalogue, what it is when
// left alone, where it applies, and whatever the validator says about it.
export function PropField({ field, value, onChange, issues = [], ctx, unordered, bare }: {
  field: Field; value: unknown; onChange: (v: unknown) => void; issues?: IssueView[]; ctx: FieldCtx
  unordered?: boolean // a list of choices whose order means nothing
  bare?: boolean      // the label is drawn by whoever holds the field
}) {
  const [text, setText] = useTyped(field, value, onChange)
  const listId = useId()
  const labelId = useId()
  const routes = useRoutes(ctx)
  const notes = issues.length ? <IssueNotes issues={issues} /> : undefined
  const label = <>{field.label}{!field.known && <span className="pe-unknown"> ({field.type})</span>}</>

  if (field.kind === 'bool') {
    const on = typeof value === 'boolean' ? value : field.defaultValue === true
    return (
      <div className={'pe-bool' + (issues.length ? ' invalid' : '')}>
        <div className="pe-bool-row">
          <div><div className="ui-field-label" id={labelId}>{field.label}</div><div className="hint"><Facts field={field} /></div></div>
          <Switch checked={on} labelledBy={labelId} onChange={(v) => onChange(v)} />
        </div>
        {notes}
      </div>
    )
  }

  const insert = field.placeholders && (ctx.cat.placeholders || []).length > 0 && (
    <Menu label="Placeholders" align="start"
      trigger={(p) => <button type="button" className="btn sm ghost pe-insert" {...p}><IconPlus size={14} /> Insert a name that fills itself in</button>}
      items={(ctx.cat.placeholders || []).map((o) => ({ key: o.value, label: o.label, hint: o.value, onSelect: () => setText(text + o.value) }))} />
  )
  const variables = field.variables.length > 0 && <datalist id={listId}>{field.variables.map((v) => <option key={v} value={v} label={variableLabel(v)} />)}</datalist>

  let control: ReactNode
  let hintExtra: ReactNode = null
  switch (field.kind) {
    case 'multiline': case 'markdown':
      control = <Textarea rows={field.kind === 'markdown' ? 6 : 3} value={text} maxLength={field.maxLength || undefined} placeholder={field.placeholder} onChange={(e) => setText(e.target.value)} />
      if (field.kind === 'markdown') hintExtra = <span className="pe-facts"> Markdown: **bold**, *italic*, lists and [links](/library/3).</span>
      break
    case 'int': case 'number':
      control = <Input type="number" inputMode={field.kind === 'int' ? 'numeric' : 'decimal'} step={field.kind === 'int' ? 1 : 'any'} min={field.min} max={field.max} value={text} placeholder={field.placeholder} onChange={(e) => setText(e.target.value)} />
      break
    case 'enum': {
      const cur = toText(value)
      const known = field.options.some((o) => o.value === cur)
      control = (
        <Select value={cur} onChange={(e) => onChange(e.target.value || undefined)}>
          <option value="">{field.hasDefault ? `${valueText(field, field.defaultValue)} (the usual)` : field.required ? 'Choose…' : 'Not set'}</option>
          {field.options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          {cur && !known && <option value={cur}>{cur} (not a choice)</option>}
        </Select>
      )
      break
    }
    case 'enumList':
      control = unordered ? <AnyOf field={field} value={value} onChange={onChange} /> : <OrderedChoice field={field} value={value} onChange={onChange} />
      break
    case 'icon':
      control = <IconPicker cat={ctx.cat} value={value} onChange={onChange} none={field.hasDefault ? `${words(String(field.defaultValue))} (the usual)` : 'No icon'} />
      break
    case 'section': {
      const cur = toText(value)
      const options = ctx.sections || []
      control = (
        <Select value={cur} onChange={(e) => onChange(e.target.value || undefined)}>
          <option value="">{field.required ? 'Choose a section…' : 'Not set'}</option>
          {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          {cur && !options.some((o) => o.value === cur) && <option value={cur}>{cur} (not on this page)</option>}
        </Select>
      )
      break
    }
    case 'library': {
      const cur = toText(value)
      const known = field.variables.includes(cur) || ctx.libraries.some((l) => String(l.id) === cur)
      control = (
        <Select value={cur} onChange={(e) => onChange(fromText(field, e.target.value))}>
          <option value="">{field.required ? 'Choose a library…' : 'Every library'}</option>
          {field.variables.map((v) => <option key={v} value={v}>{variableLabel(v)}</option>)}
          {ctx.libraries.map((l) => <option key={l.id} value={String(l.id)}>{l.name}</option>)}
          {cur && !known && <option value={cur}>{cur.startsWith('$') ? `${cur} (means nothing here)` : `Library #${cur} (not one of yours)`}</option>}
        </Select>
      )
      break
    }
    case 'route':
      control = <Input value={text} placeholder={field.placeholder || '/library/3'} list={listId} onChange={(e) => setText(e.target.value)} />
      hintExtra = <><span className="pe-facts"> A place in the app, beginning with /.</span><datalist id={listId}>{routes.map((r) => <option key={r.value} value={r.value} label={r.label} />)}</datalist></>
      break
    case 'url':
      control = <Input type="url" inputMode="url" value={text} placeholder={field.placeholder || 'https://'} onChange={(e) => setText(e.target.value)} />
      break
    case 'image':
      control = <Input value={text} placeholder={field.placeholder || '/api/…'} onChange={(e) => setText(e.target.value)} />
      hintExtra = <span className="pe-facts"> The address of a picture on this server, beginning /api/.</span>
      break
    case 'json':
      control = <Textarea className="pe-mono" rows={3} spellCheck={false} value={text} placeholder={field.placeholder} onChange={(e) => setText(e.target.value)} />
      break
    case 'scope':
      control = <Input value={text} placeholder={field.placeholder} spellCheck={false} onChange={(e) => setText(e.target.value)} />
      hintExtra = <span className="pe-facts"> A module’s ID, or a name of your own in lower case.</span>
      break
    default:
      // Text, a reference, and any type that came after this editor was written.
      control = <Input value={text} maxLength={field.maxLength || undefined} placeholder={field.placeholder} list={field.variables.length ? listId : undefined} spellCheck={field.kind === 'text' && field.type !== 'string' ? undefined : false} onChange={(e) => setText(e.target.value)} />
      if (field.variables.length) hintExtra = <span className="pe-facts"> Or one of: {field.variables.join(', ')}.</span>
  }

  if (bare) return <div className={'pe-bare' + (issues.length ? ' invalid' : '')}>{control}{variables}{notes}</div>
  return (
    <div className="pe-field">
      <Labelled label={label} required={field.required} hint={<Facts field={field} extra={hintExtra} />} className={issues.length ? 'invalid' : undefined}>{control}</Labelled>
      {variables}
      {insert}
      {notes}
    </div>
  )
}

// ---------------------------------------------------------------- conditions

const GROUP_WORDS: Record<'any' | 'all', { legend: string; add: string; one: string }> = {
  any: { legend: 'At least one of these holds', add: 'Add an alternative', one: 'Alternative' },
  all: { legend: 'Every one of these holds', add: 'Add another group', one: 'Group' },
}
const MAX_DEPTH = 8

// ConditionEditor: when something is shown. Rules are added one at a time from the
// catalogue's list; all of them must hold. "At least one of", "every one of" and "but
// not when" hold further conditions, nested as deep as the server allows.
export function ConditionEditor({ value, onChange, ctx, issues = [], depth = 0, name }: {
  value: Condition | null | undefined; onChange: (c: Condition) => void; ctx: FieldCtx; issues?: IssueView[]; depth?: number; name: string
}) {
  const c: Condition = value || {}
  // A rule has no "usual" value: it is there or it isn't.
  const fields = useMemo(() => conditionFields(ctx.cat).map((f): Field => ({ ...f, hasDefault: false, defaultValue: undefined, defaultText: '', placeholder: '' })), [ctx.cat])
  const active = fields.filter((f) => f.name in c)
  const unknown = Object.keys(c).filter((k) => k !== 'all' && k !== 'any' && k !== 'not' && !fields.some((f) => f.name === k))
  const inactive = fields.filter((f) => !(f.name in c))
  const set = (k: string, v: unknown) => onChange({ ...c, [k]: v })
  const drop = (k: string) => { const next = { ...c }; delete next[k]; onChange(next) }
  const blank = (f: Field): unknown => (f.kind === 'bool' ? true : f.kind === 'enumList' ? [] : '')
  const group = (k: 'any' | 'all') => (Array.isArray(c[k]) ? c[k] as Condition[] : null)
  const not = c.not && typeof c.not === 'object' ? c.not as Condition : null

  return (
    <div className={'pe-cond' + (depth ? ' nested' : '')}>
      {depth === 0 && <p className="pe-cond-says"><strong>Shown:</strong> {conditionText(ctx.cat, c)}</p>}
      <IssueNotes issues={issues} />
      {active.map((f) => (
        <div key={f.name} className="pe-cond-rule">
          <PropField field={f} value={c[f.name]} ctx={ctx} unordered onChange={(v) => set(f.name, v === undefined ? blank(f) : v)} />
          <IconButton size="sm" label={`Remove the rule ${f.label}`} onClick={() => drop(f.name)}><IconClose /></IconButton>
        </div>
      ))}
      {unknown.map((k) => (
        <div key={k} className="pe-cond-rule">
          <PropField field={fieldFor(ctx.cat, { name: k, type: 'unknown', label: words(k) })} value={c[k]} ctx={ctx} onChange={(v) => set(k, v === undefined ? '' : v)} />
          <IconButton size="sm" label={`Remove the rule ${words(k)}`} onClick={() => drop(k)}><IconClose /></IconButton>
        </div>
      ))}
      {(['any', 'all'] as const).map((k) => {
        const list = group(k)
        if (!list) return null
        const w = GROUP_WORDS[k]
        return (
          <fieldset key={k} className="pe-cond-group">
            <legend>{w.legend}</legend>
            {list.map((sub, i) => (
              <div key={i} className="pe-cond-sub">
                <div className="pe-cond-sub-head"><span>{w.one} {i + 1}</span><Button size="sm" variant="ghost" onClick={() => { const next = list.filter((_, n) => n !== i); if (next.length) set(k, next); else drop(k) }}>Remove</Button></div>
                <ConditionEditor value={sub} ctx={ctx} depth={depth + 1} name={`${name}, ${w.one.toLowerCase()} ${i + 1}`} onChange={(v) => set(k, list.map((x, n) => (n === i ? v : x)))} />
              </div>
            ))}
            <Button size="sm" variant="ghost" icon={<IconPlus size={14} />} onClick={() => set(k, [...list, {}])}>{w.add}</Button>
          </fieldset>
        )
      })}
      {not && (
        <fieldset className="pe-cond-group">
          <legend>But not when</legend>
          <div className="pe-cond-sub">
            <div className="pe-cond-sub-head"><span>{Object.keys(not).length ? 'All of this holds' : 'With no rule here, this is never shown'}</span><Button size="sm" variant="ghost" onClick={() => drop('not')}>Remove</Button></div>
            <ConditionEditor value={not} ctx={ctx} depth={depth + 1} name={`${name}, but not when`} onChange={(v) => set('not', v)} />
          </div>
        </fieldset>
      )}
      <div className="pe-cond-add">
        {inactive.length > 0 && (
          <Select inputSize="sm" aria-label={`Add a rule to ${name}`} value="" onChange={(e) => { const f = fields.find((x) => x.name === e.target.value); if (f) set(f.name, blank(f)) }}>
            <option value="">Add a rule…</option>
            {inactive.map((f) => <option key={f.name} value={f.name}>{f.label}</option>)}
          </Select>
        )}
        {depth < MAX_DEPTH && (
          <Menu label="Combine rules" align="start"
            trigger={(p) => <button type="button" className="btn sm ghost" {...p}>Combine…</button>}
            items={[
              { key: 'any', label: 'At least one of…', hint: 'Shown when any one of several sets of rules holds', disabled: !!group('any'), onSelect: () => set('any', [{}, {}]) },
              { key: 'all', label: 'Every one of…', hint: 'Several sets of rules that must all hold', disabled: !!group('all'), onSelect: () => set('all', [{}]) },
              { key: 'not', label: 'But not when…', hint: 'Hidden when these rules hold', disabled: !!not, onSelect: () => set('not', {}) },
            ]} />
        )}
      </div>
    </div>
  )
}
