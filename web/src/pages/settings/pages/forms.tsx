// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The forms of the page editor: one section's (its properties, where its items come from,
// when it is shown, whether it repeats), the page's own, its variants, and the page as
// JSON for those who would rather type. Every list of fields comes from the catalogue.

import { useEffect, useState, type ReactNode } from 'react'
import { IconClose, IconDown, IconPlus, IconUp } from '../../../icons'
import { sduiIcon } from '../../../sdui/render/icons'
import { Button, Chip, Chips, Field as Labelled, IconButton, Input, Notice, Select, Switch, Textarea } from '../../../ui'
import type { Checked } from './api'
import { ConditionEditor, IconPicker, IssueNotes, PropField, type FieldCtx, type IssueView } from './fields'
import {
  componentFields, componentSpec, conditionText, exampleSection, fieldFor, formFactorLabel, libraryKindLabel, pageKindSpec, pretty, sectionTitle,
  setValue, slug, sourceFields, sourceGroups, sourceRule, sourceSpec, uniqueId, validId, variablesFor, words,
  type Catalogue, type Page, type Section, type Variant,
} from './editorModel.ts'

// The issues of one thing (a section, the page), each with the field it is about.
export type Located = { field: string; view: IssueView }
const at = (all: Located[], field: string) => all.filter((l) => l.field === field || l.field.startsWith(field + '/')).map((l) => l.view)
const others = (all: Located[], shown: string[]) => all.filter((l) => !shown.some((f) => l.field === f || l.field.startsWith(f + '/'))).map((l) => l.view)

function Group({ title, desc, children, id }: { title: string; desc?: ReactNode; children: ReactNode; id?: string }) {
  return (
    <section className="pe-group" aria-label={title} id={id}>
      <h3>{title}</h3>
      {desc && <p className="pe-group-desc">{desc}</p>}
      {children}
    </section>
  )
}

// An ID is typed freely and taken on when it is a good one, so the tree isn't renamed letter by letter.
export function IdBox({ value, taken, onCommit, what, hint }: { value: string; taken: string[]; onCommit: (id: string) => void; what: string; hint?: string }) {
  const [text, setText] = useState(value)
  useEffect(() => { setText(value) }, [value])
  const problem = text === value ? '' : !validId(text) ? 'Lower-case letters, digits, - and _, starting with a letter.' : taken.includes(text) ? `Another ${what} already has this ID.` : ''
  const commit = () => { if (text !== value && !problem) onCommit(text); else if (problem) setText(value) }
  return (
    <Labelled label="ID" hint={hint || `What the ${what} is known by: profiles’ own arrangements and other sections refer to it. Lower-case letters, digits, - and _.`} error={problem || undefined}>
      <Input value={text} spellCheck={false} autoCapitalize="none" onChange={(e) => setText(e.target.value)} onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); commit() } }} />
    </Labelled>
  )
}

// ---------------------------------------------------------------- a source

// Which detail of an item each line of a card shows, when not the usual one.
function MapEditor({ cat, value, onChange, issues }: { cat: Catalogue; value: unknown; onChange: (v: unknown) => void; issues: IssueView[] }) {
  const map = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
  const [line, setLine] = useState('')
  const [detail, setDetail] = useState('')
  const pick = (v: string, set: (s: string) => void, choices: string[] | undefined, label: string) => (choices?.length
    ? <Select inputSize="sm" aria-label={label} value={v} onChange={(e) => set(e.target.value)}><option value="">{label}…</option>{choices.map((c) => <option key={c} value={c}>{words(c)}</option>)}</Select>
    : <Input inputSize="sm" aria-label={label} placeholder={label} value={v} spellCheck={false} onChange={(e) => set(e.target.value.trim())} />)
  const add = () => { if (line && detail) { onChange({ ...map, [line]: detail }); setLine(''); setDetail('') } }
  return (
    <div className="pe-map">
      <div className="ui-field-label">Other details on the cards</div>
      <div className="hint">A card’s line (its title, its subtitle…) can show another detail of each item, such as its studio. The check below names the lines and details there are if one isn’t right.</div>
      {Object.entries(map).map(([k, v]) => (
        <div key={k} className="pe-map-row">
          <span className="pe-map-key">{words(k)}</span><span aria-hidden="true">shows</span>
          {pick(String(v ?? ''), (s) => onChange({ ...map, [k]: s }), cat.mapFields, `What ${k} shows`)}
          <IconButton size="sm" label={`Remove what ${k} shows`} onClick={() => { const next = { ...map }; delete next[k]; onChange(Object.keys(next).length ? next : undefined) }}><IconClose /></IconButton>
        </div>
      ))}
      <div className="pe-map-row">
        {pick(line, setLine, cat.mapProps?.filter((p) => !(p in map)), 'The card’s line')}
        <span aria-hidden="true">shows</span>
        {pick(detail, setDetail, cat.mapFields, 'The item’s detail')}
        <Button size="sm" variant="ghost" disabled={!line || !detail} onClick={add}>Add</Button>
      </div>
      <IssueNotes issues={issues} />
    </div>
  )
}

export function SourceEditor({ section, pageKind, onChange, issues, ctx }: { section: Section; pageKind: string; onChange: (source: Record<string, unknown> | undefined) => void; issues: Located[]; ctx: FieldCtx }) {
  const cat = ctx.cat
  const rule = sourceRule(cat, section.component)
  const src = section.source
  const type = typeof src?.type === 'string' ? src.type : ''
  const label = componentSpec(cat, section.component)?.label || section.component
  if (rule === 'none') {
    if (!src || !Object.keys(src).length) return null
    return (
      <Group title="Where its items come from">
        <IssueNotes issues={issues.map((l) => l.view)} />
        <Button size="sm" onClick={() => onChange(undefined)}>Remove the source: {label} doesn’t take one</Button>
      </Group>
    )
  }
  const groups = sourceGroups(cat, section.component, pageKind)
  const spec = sourceSpec(cat, type)
  const fields = sourceFields(cat, type, variablesFor(cat, pageKind, !!section.repeat?.over))
  const known = groups.some((g) => g.items.some((e) => e.type === type))
  const extra = Object.keys(src || {}).filter((k) => k !== 'type' && k !== 'map' && !fields.some((f) => f.name === k))
  const setType = (t: string) => {
    if (!t) { onChange(undefined); return }
    // What the new source also takes is kept: the library, the limit.
    const next: Record<string, unknown> = { type: t }
    for (const p of sourceSpec(cat, t)?.params || []) if (src && src[p.name] !== undefined) next[p.name] = src[p.name]
    if (src?.map !== undefined) next.map = src.map
    onChange(next)
  }
  const shown = ['source/type', 'source/map', ...fields.map((f) => 'source/' + f.name), ...extra.map((k) => 'source/' + k)]
  const rest = issues.filter((l) => l.field === 'source' || (l.field.startsWith('source/') && !shown.some((f) => l.field === f || l.field.startsWith(f + '/')))).map((l) => l.view)
  return (
    <Group title="Where its items come from" desc={rule === 'required' ? `${label} is filled from a source: choose one.` : `${label} can be filled from a source, or written by hand.`}>
      <IssueNotes issues={rest} />
      <div className="pe-field">
        <Labelled label="Source" required={rule === 'required'} hint={spec ? <>{spec.description}{spec.yields?.length ? <span className="pe-facts"> Gives: {spec.yields.map((y) => words(y).toLowerCase()).join(', ')}.</span> : null}</> : undefined} className={at(issues, 'source/type').length ? 'invalid' : undefined}>
          <Select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">{rule === 'required' ? 'Choose a source…' : 'No source'}</option>
            {groups.map((g) => (
              <optgroup key={g.category} label={g.label}>
                {g.items.map((e) => <option key={e.type} value={e.type} disabled={!e.fits && e.type !== type}>{e.label}{e.fits ? '' : ` (${e.why.toLowerCase()})`}</option>)}
              </optgroup>
            ))}
            {type && !known && <option value={type}>{type} (not a source for this)</option>}
          </Select>
        </Labelled>
        <IssueNotes issues={at(issues, 'source/type')} />
      </div>
      {fields.map((f) => <PropField key={type + f.name} field={f} value={src?.[f.name]} ctx={ctx} issues={at(issues, 'source/' + f.name)} onChange={(v) => onChange(setValue(src, f.name, v))} />)}
      {extra.map((k) => (
        <div key={k} className="pe-cond-rule">
          <PropField field={fieldFor(cat, { name: k, type: typeof src?.[k] === 'object' ? 'object' : 'string', label: `${k} (not something this source takes)` })} value={src?.[k]} ctx={ctx} issues={at(issues, 'source/' + k)} onChange={(v) => onChange(setValue(src, k, v))} />
          <IconButton size="sm" label={`Remove ${k}`} onClick={() => onChange(setValue(src, k, undefined))}><IconClose /></IconButton>
        </div>
      ))}
      {type && <MapEditor cat={cat} value={src?.map} issues={at(issues, 'source/map')} onChange={(v) => onChange(setValue(src, 'map', v))} />}
    </Group>
  )
}

// ---------------------------------------------------------------- a section

export function SectionForm({ section, page, taken, issues, ctx, onChange, onRename }: {
  section: Section; page: Page; taken: string[]; issues: Located[]; ctx: FieldCtx
  onChange: (next: Section, tag?: string) => void
  onRename: (id: string) => void
}) {
  const cat = ctx.cat
  const spec = componentSpec(cat, section.component)
  const fields = componentFields(cat, section.component)
  const extraProps = Object.keys(section.props || {}).filter((k) => !fields.some((f) => f.name === k))
  const setProp = (name: string, v: unknown) => {
    const props = setValue(section.props, name, v)
    const next = { ...section }
    if (Object.keys(props).length) next.props = props; else delete next.props
    onChange(next, `${section.id}/props/${name}`)
  }
  const setSource = (source: Record<string, unknown> | undefined) => {
    const next = { ...section }
    if (source) next.source = source; else delete next.source
    onChange(next, `${section.id}/source`)
  }
  const repeat = section.repeat
  const setRepeat = (r: Section['repeat']) => {
    const next = { ...section }
    if (r?.over) next.repeat = r; else delete next.repeat
    onChange(next, `${section.id}/repeat`)
  }
  const kinds = cat.libraryKinds || []
  const shown = ['id', 'when', 'scope', 'repeat/over', 'repeat/kinds', 'repeat/limit', 'source', ...fields.map((f) => 'props/' + f.name), ...extraProps.map((k) => 'props/' + k)]
  const example = exampleSection(cat, section.component, taken)
  const onlyOn = spec?.formFactors?.length ? spec.formFactors.map((f) => formFactorLabel(cat, f).toLowerCase()).join(', ') : ''

  return (
    <div className="pe-form">
      <header className="pe-form-head">
        <span className="pe-form-icon" aria-hidden="true">{sduiIcon(spec?.icon)}</span>
        <div><h2 tabIndex={-1}>{sectionTitle(cat, section)}</h2><p>{spec ? <><strong>{spec.label}.</strong> {spec.description}{onlyOn && ` Only shown on: ${onlyOn}.`}</> : `“${section.component}” isn’t a component this server knows.`}</p></div>
      </header>
      <IssueNotes issues={others(issues, shown)} />

      {(fields.length > 0 || extraProps.length > 0) && (
        <Group title="How it looks">
          {fields.map((f) => <PropField key={section.component + f.name} field={f} value={section.props?.[f.name]} ctx={ctx} issues={at(issues, 'props/' + f.name)} onChange={(v) => setProp(f.name, v)} />)}
          {extraProps.map((k) => (
            <div key={k} className="pe-cond-rule">
              <PropField field={fieldFor(cat, { name: k, type: typeof section.props?.[k] === 'object' ? 'object' : 'string', label: `${k} (not a property of ${spec?.label || 'this'})` })} value={section.props?.[k]} ctx={ctx} issues={at(issues, 'props/' + k)} onChange={(v) => setProp(k, v)} />
              <IconButton size="sm" label={`Remove ${k}`} onClick={() => setProp(k, undefined)}><IconClose /></IconButton>
            </div>
          ))}
        </Group>
      )}

      <SourceEditor section={section} pageKind={page.kind} ctx={ctx} issues={issues.filter((l) => l.field === 'source' || l.field.startsWith('source/'))} onChange={setSource} />

      <Group title="When it is shown" desc="Leave this alone to show the section to everyone, everywhere.">
        <ConditionEditor value={section.when} ctx={ctx} name={sectionTitle(cat, section)} issues={at(issues, 'when')} onChange={(c) => onChange({ ...section, when: c }, `${section.id}/when`)} />
      </Group>

      {(cat.repeats || []).length > 0 && (
        <Group title="Repeat" desc="One copy of the section for each library, saved view or collection. Use $each in the source for the one each copy is about, and {each} in the title for its name.">
          <div className="pe-field">
            <Labelled label="Make a copy for" className={at(issues, 'repeat/over').length ? 'invalid' : undefined}>
              <Select value={repeat?.over || ''} onChange={(e) => setRepeat(e.target.value ? { ...(repeat || {}), over: e.target.value } : undefined)}>
                <option value="">Nothing: show it once</option>
                {(cat.repeats || []).map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
                {repeat?.over && !(cat.repeats || []).some((o) => o.value === repeat.over) && <option value={repeat.over}>{repeat.over} (not a choice)</option>}
              </Select>
            </Labelled>
            <IssueNotes issues={at(issues, 'repeat/over')} />
          </div>
          {repeat?.over && (
            <>
              {(repeat.over === 'libraries' || !!repeat.kinds?.length) && kinds.length > 0 && (
                <div className="pe-field">
                  <div className="ui-field-label" id={`pe-kinds-${section.id}`}>Only libraries of these kinds</div>
                  <Chips label="Only libraries of these kinds" className="pe-anyof">
                    {kinds.map((k) => {
                      const on = !!repeat.kinds?.includes(k)
                      return <Chip key={k} selected={on} onClick={() => { const next = on ? (repeat.kinds || []).filter((x) => x !== k) : [...(repeat.kinds || []), k]; setRepeat({ ...repeat, kinds: next.length ? next : undefined }) }}>{libraryKindLabel(cat, k)}</Chip>
                    })}
                    {(repeat.kinds || []).filter((k) => !kinds.includes(k)).map((k) => <Chip key={k} onRemove={() => setRepeat({ ...repeat, kinds: (repeat.kinds || []).filter((x) => x !== k) })} removeLabel={`Remove ${k}`}>{k} (not a kind)</Chip>)}
                  </Chips>
                  <div className="hint">None chosen: every kind.</div>
                  <IssueNotes issues={at(issues, 'repeat/kinds')} />
                </div>
              )}
              <PropField field={fieldFor(cat, { name: 'limit', type: 'int', label: 'Most copies', help: 'Leave empty to let the device decide.', min: 0 })} value={repeat.limit || undefined} ctx={ctx} issues={at(issues, 'repeat/limit')}
                onChange={(v) => setRepeat({ ...repeat, limit: typeof v === 'number' && v > 0 ? v : undefined })} />
            </>
          )}
        </Group>
      )}

      <Group title="More">
        <IdBox value={section.id} taken={taken.filter((t) => t !== section.id)} what="section" onCommit={onRename} />
        <IssueNotes issues={at(issues, 'id')} />
        <PropField field={fieldFor(cat, { name: 'scope', type: 'scope', label: 'Colours of', help: 'The section takes the colours set for this module or name in Appearance. Leave empty for the page’s own.' })} value={section.scope} ctx={ctx} issues={at(issues, 'scope')}
          onChange={(v) => { const next = { ...section }; if (typeof v === 'string' && v) next.scope = v; else delete next.scope; onChange(next, `${section.id}/scope`) }} />
        <PropField field={fieldFor(cat, { name: 'locked', type: 'bool', label: 'Keep it where it is', help: 'Profiles can’t hide or move this section when they arrange the page for themselves.' })} value={!!section.locked} ctx={ctx}
          onChange={(v) => { const next = { ...section }; if (v === true) next.locked = true; else delete next.locked; onChange(next) }} />
      </Group>

      {spec && (spec.renders || spec.states || spec.a11y || example) && (
        <details className="pe-about">
          <summary>How {spec.label} is drawn, and an example</summary>
          {spec.renders && <dl>{Object.entries(spec.renders).map(([ff, text]) => <div key={ff}><dt>{formFactorLabel(cat, ff)}</dt><dd>{text}</dd></div>)}</dl>}
          {spec.states && <dl>{Object.entries(spec.states).map(([k, text]) => <div key={k}><dt>{words(k)}</dt><dd>{text}</dd></div>)}</dl>}
          {spec.a11y && <p><strong>Read aloud as:</strong> {spec.a11y}</p>}
          {example && (
            <>
              <pre className="pe-code" tabIndex={0} aria-label={`An example of ${spec.label}`}>{pretty(spec.example)}</pre>
              <Button size="sm" variant="ghost" onClick={() => onChange({ ...example, id: section.id })}>Make this section the example</Button>
            </>
          )}
        </details>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- the page

export function PageForm({ page, source, issues, ctx, onChange }: { page: Page; source: string; issues: Located[]; ctx: FieldCtx; onChange: (next: Page, tag?: string) => void }) {
  const cat = ctx.cat
  const kind = pageKindSpec(cat, page.kind)
  const matchFields = kind?.match || []
  const set = (name: string, v: unknown) => onChange(setValue(page as unknown as Record<string, unknown>, name, v) as unknown as Page, 'page/' + name)
  const setMatch = (name: string, v: unknown) => {
    const match = setValue(page.match as Record<string, unknown> | undefined, name, v)
    const next = { ...page }
    if (Object.keys(match).length) next.match = match; else delete next.match
    onChange(next)
  }
  const kinds = cat.personalKinds || []
  const allowed = page.personal ? page.personal.allow || [] : kinds.map((k) => k.value)
  const setAllowed = (list: string[]) => {
    const next = { ...page }
    // Everything allowed is how a page starts, and isn't written down.
    if (kinds.every((k) => list.includes(k.value))) delete next.personal
    else next.personal = { allow: kinds.map((k) => k.value).filter((k) => list.includes(k)) }
    onChange(next)
  }
  const placeholders = (cat.placeholders || []).map((p) => p.value).join(' ')
  const shown = ['title', 'icon', 'description', 'kind', 'match', 'scope', 'personal', 'ttlSeconds', 'sections', 'variants']
  return (
    <div className="pe-form">
      <IssueNotes issues={others(issues, shown)} />
      <Group title="About the page">
        <PropField field={fieldFor(cat, { name: 'title', type: 'string', label: 'Title', required: true, maxLength: 120, help: `Shown at the top of the page and in the menu. These fill themselves in: ${placeholders}` })} value={page.title} ctx={ctx} issues={at(issues, 'title')} onChange={(v) => onChange({ ...page, title: typeof v === 'string' ? v : '' }, 'page/title')} />
        <div className="pe-field">
          <Labelled label="Icon" hint="For a menu entry made from the page."><IconPicker cat={cat} value={page.icon} onChange={(v) => set('icon', v)} /></Labelled>
          <IssueNotes issues={at(issues, 'icon')} />
        </div>
        <PropField field={fieldFor(cat, { name: 'description', type: 'text', label: 'Note to yourself', maxLength: 500, help: 'For whoever edits the page. Nobody else sees it.' })} value={page.description} ctx={{ ...ctx, cat: { ...cat, placeholders: [] } }} issues={at(issues, 'description')} onChange={(v) => set('description', v)} />
      </Group>

      <Group title="What it is for" desc={kind ? `${kind.label}. ${kind.description} It opens at ${kind.route.replace('{id}', page.id)}.` : undefined}>
        <div className="pe-field">
          <Labelled label="Kind of page" hint={source === 'custom' ? 'The kind decides when the app opens the page and what its sources may refer to.' : 'A page that takes the place of a built-in one keeps that page’s kind.'} className={at(issues, 'kind').length ? 'invalid' : undefined}>
            <Select value={page.kind} disabled={source !== 'custom'} onChange={(e) => onChange({ ...page, kind: e.target.value })}>
              {cat.pageKinds.map((k) => <option key={k.kind} value={k.kind}>{k.label}</option>)}
              {!kind && <option value={page.kind}>{page.kind || 'Not chosen'} (not a kind)</option>}
            </Select>
          </Labelled>
          <IssueNotes issues={at(issues, 'kind')} />
        </div>
        {(matchFields.includes('library') || !!page.match?.library) && (
          <PropField field={fieldFor(cat, { name: 'library', type: 'library', label: 'Only for the library', help: 'Leave on “Every library” for all of them. The most particular page wins.' })} value={page.match?.library} ctx={ctx} issues={at(issues, 'match/library')} onChange={(v) => setMatch('library', v)} />
        )}
        {(matchFields.includes('libraryKind') || !!page.match?.libraryKind) && (
          <PropField field={fieldFor(cat, { name: 'libraryKind', type: 'enum', label: 'Only for libraries of the kind', options: (cat.libraryKinds || []).map((k) => ({ value: k, label: libraryKindLabel(cat, k) })) })} value={page.match?.libraryKind} ctx={ctx} issues={at(issues, 'match/libraryKind')} onChange={(v) => setMatch('libraryKind', v)} />
        )}
        {(matchFields.includes('mediaKind') || !!page.match?.mediaKind) && (
          <PropField field={fieldFor(cat, { name: 'mediaKind', type: 'enum', label: 'Only for titles of the kind', options: (cat.mediaKinds || []).map((k) => ({ value: k, label: words(k) })) })} value={page.match?.mediaKind} ctx={ctx} issues={at(issues, 'match/mediaKind')} onChange={(v) => setMatch('mediaKind', v)} />
        )}
      </Group>

      {kinds.length > 0 && (
        <Group title="What profiles may change for themselves" desc="Each profile can arrange a page its own way, as far as you allow here. A section you lock stays put whatever they choose.">
          {kinds.map((k) => {
            const on = allowed.includes(k.value)
            return (
              <div key={k.value} className="pe-bool-row">
                <span className="ui-field-label" id={`pe-allow-${k.value}`}>{k.label}</span>
                <Switch checked={on} labelledBy={`pe-allow-${k.value}`} onChange={(v) => setAllowed(v ? [...allowed, k.value] : allowed.filter((x) => x !== k.value))} />
              </div>
            )
          })}
          <IssueNotes issues={at(issues, 'personal')} />
        </Group>
      )}

      <Group title="More">
        <PropField field={fieldFor(cat, { name: 'scope', type: 'scope', label: 'Colours of', help: 'The whole page takes the colours set for this module or name in Appearance.' })} value={page.scope} ctx={ctx} issues={at(issues, 'scope')} onChange={(v) => set('scope', v)} />
        <PropField field={fieldFor(cat, { name: 'ttlSeconds', type: 'int', label: 'Ask again after', help: 'How many seconds an app may show the page before asking for it again. Leave empty for the usual.', min: 0, max: 86400 })} value={page.ttlSeconds || undefined} ctx={ctx} issues={at(issues, 'ttlSeconds')} onChange={(v) => set('ttlSeconds', typeof v === 'number' && v > 0 ? v : undefined)} />
      </Group>
    </div>
  )
}

// ---------------------------------------------------------------- variants

export function VariantsForm({ page, ctx, issuesOf, onChange, onEdit, current, max }: {
  page: Page; ctx: FieldCtx; current: number; max: number
  issuesOf: (variant: number) => Located[]
  onChange: (next: Page, tag?: string) => void
  onEdit: (variant: number) => void
}) {
  const cat = ctx.cat
  const variants = page.variants || []
  const setAll = (list: Variant[], tag?: string) => { const next = { ...page }; if (list.length) next.variants = list; else delete next.variants; onChange(next, tag) }
  const setOne = (i: number, v: Variant, tag?: string) => setAll(variants.map((x, n) => (n === i ? v : x)), tag)
  const add = (copy: boolean) => {
    const id = uniqueId('variant', variants.map((v) => v.id))
    // A copy keeps the sections' IDs, so a profile's own arrangement of the page carries over to it.
    setAll([...variants, { id, when: {}, sections: copy ? JSON.parse(JSON.stringify(page.sections || [])) : [] }])
    onEdit(variants.length)
  }
  const move = (i: number, to: number) => { const next = [...variants]; const [v] = next.splice(i, 1); next.splice(to, 0, v); setAll(next); if (current === i) onEdit(to) }
  return (
    <Group title="Variants" id="pe-variants" desc="Another set of sections for the same page, used instead of the page’s own when its condition holds: a simpler Home for Kids profiles, a different one on a television. The first variant whose condition holds is the one shown.">
      {variants.map((v, i) => {
        const issues = issuesOf(i)
        const name = v.label || words(v.id)
        return (
          <div key={i} className={'pe-variant' + (current === i ? ' on' : '')}>
            <div className="pe-variant-head">
              <strong>{i + 1}. {name}</strong>
              <span className="pe-variant-tools">
                <IconButton size="sm" label={`Try ${name} earlier`} disabled={i === 0} onClick={() => move(i, i - 1)}><IconUp /></IconButton>
                <IconButton size="sm" label={`Try ${name} later`} disabled={i === variants.length - 1} onClick={() => move(i, i + 1)}><IconDown /></IconButton>
                <Button size="sm" variant="ghost" onClick={() => onEdit(i)}>{current === i ? 'Editing its sections' : 'Edit its sections'}</Button>
                <Button size="sm" variant="danger" onClick={() => { setAll(variants.filter((_, n) => n !== i)); onEdit(-1) }}>Remove</Button>
              </span>
            </div>
            <p className="hint">{v.sections?.length || 0} sections. Used when: {conditionText(cat, v.when).replace(/^Always$/, 'no condition yet')}</p>
            <IssueNotes issues={others(issues, ['id', 'when', 'label'])} />
            <Labelled label="Name" hint="For you, in this editor.">
              <Input value={v.label || ''} placeholder={words(v.id)} onChange={(e) => {
                const label = e.target.value
                // Until someone gives the variant an ID of their own, it follows the name.
                const rest = variants.filter((_, n) => n !== i).map((x) => x.id)
                const follows = /^variant(-[0-9]+)?$/.test(v.id) || (!!v.label && v.id === uniqueId(slug(v.label, 'variant'), rest))
                setOne(i, { ...v, label: label || undefined, id: follows && label.trim() ? uniqueId(slug(label, 'variant'), rest) : v.id }, `variant/${i}/label`)
              }} />
            </Labelled>
            <IdBox value={v.id} taken={variants.filter((_, n) => n !== i).map((x) => x.id)} what="variant" onCommit={(id) => setOne(i, { ...v, id })} />
            <IssueNotes issues={at(issues, 'id')} />
            <div className="ui-field-label">Use this variant when</div>
            <ConditionEditor value={v.when} ctx={ctx} name={`the variant ${name}`} issues={at(issues, 'when')} onChange={(c) => setOne(i, { ...v, when: c }, `variant/${i}/when`)} />
          </div>
        )
      })}
      {variants.length < max
        ? <div className="pe-row-actions">
            <Button size="sm" icon={<IconPlus size={14} />} onClick={() => add(true)}>Add a variant, starting from the page’s sections</Button>
            <Button size="sm" variant="ghost" onClick={() => add(false)}>Add an empty variant</Button>
          </div>
        : <p className="hint">A page has at most {max} variants.</p>}
    </Group>
  )
}

// ---------------------------------------------------------------- JSON

// The same thing as text, for those who would rather type it. What is typed is checked
// before it goes anywhere: text that isn't JSON, or isn't a page, is said to be so and
// never reaches the form.
export function JsonView<T>({ value, parse, check, onApply, what }: {
  value: T; what: string
  parse: (text: string) => { value: T } | { error: string }
  check: (value: T) => Promise<Checked>
  onApply: (value: T) => void
}) {
  const [text, setText] = useState(() => pretty(value))
  const [error, setError] = useState('')
  const [result, setResult] = useState<Checked | null>(null)
  const [busy, setBusy] = useState(false)
  const now = pretty(value)
  const edited = text !== now
  const read = (): T | null => {
    const got = parse(text)
    if ('error' in got) { setError(got.error); setResult(null); return null }
    setError('')
    return got.value
  }
  const run = async () => {
    const v = read()
    if (v === null) return
    setBusy(true)
    try { setResult(await check(v)) } catch (e) { setError(e instanceof Error ? e.message : String(e)) } finally { setBusy(false) }
  }
  const apply = () => {
    const v = read()
    if (v === null) return
    onApply(v)
    setText(pretty(v))
    setResult(null)
  }
  return (
    <div className="pe-json">
      <Labelled label={`${what} as JSON`} hint="For those who know the format (the guide describes it). Nothing here changes the form until you apply it, and text that isn’t right is never applied." error={error || undefined}>
        <Textarea className="pe-mono" rows={22} spellCheck={false} autoCapitalize="none" autoCorrect="off" value={text} onChange={(e) => { setText(e.target.value); setResult(null); setError('') }} />
      </Labelled>
      <div className="pe-row-actions">
        <Button size="sm" variant="primary" disabled={!edited} onClick={apply}>Apply to the form</Button>
        <Button size="sm" busy={busy} onClick={run}>Check it</Button>
        <Button size="sm" variant="ghost" disabled={!edited} onClick={() => { setText(now); setError(''); setResult(null) }}>Start again from the form</Button>
      </div>
      {result && (result.issues.length === 0
        ? <Notice tone="ok">Nothing wrong with it.</Notice>
        : <IssueNotes issues={result.issues.map((issue) => ({ issue: { ...issue, message: `${issue.path || 'The whole of it'}: ${issue.message}` }, fix: null }))} />)}
    </div>
  )
}
