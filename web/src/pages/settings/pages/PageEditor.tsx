// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Editing one page: its sections on the left (a tree, since some hold others), the form
// of whichever is chosen in the middle, and on the right the page as a phone, a tablet, a
// desktop or a television would be sent it. Changes are kept here, with undo, until they
// are saved to the draft; they are checked by the server as they are made, and each
// problem is shown beside the field it is about.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { announce } from '../../../a11y'
import { ApiError } from '../../../api'
import { BackLink } from '../../../components/common'
import { IconMore, IconPlus } from '../../../icons'
import { sduiIcon } from '../../../sdui/render/icons'
import { ask, Button, Dialog, EmptyState, LinkButton, LoadError, Menu, Select, Spinner, Tabs, toast, type MenuEntry } from '../../../ui'
import { PageHead } from '../ui'
import { getExport, getPage, getPresets, isConflict, issuesOf, message, removePage, savePage, validatePage, type PageAnswer } from './api'
import { DraftBar, useEditor } from './context'
import { IssueNotes, type FieldCtx, type IssueView } from './fields'
import { JsonView, PageForm, SectionForm, VariantsForm, type Located } from './forms'
import { download, useEditKeys, useUnsavedGuard } from './guard'
import { Preview } from './Preview'
import { SectionTree } from './SectionTree'
import {
  SOURCE_HELP, SOURCE_WORDS, applyFix, canHold, componentSpec, exportName, fresh, insertAt, issueWhere, limitNote, listIds, locateIssue, matchText, newSection,
  pageName, pageRoute, palette, parseFix, parsePageJson, renameId, rootSections, samePage, samePath, sectionAt, sectionChoices, sectionTitle, tidyPage, undoBack, undoForward,
  undoPush, undoStart, updateAt, withRootSections, words,
  type Issue, type IssueTarget, type Page, type Path, type Preset, type Section,
} from './editorModel.ts'

type Tab = 'section' | 'page' | 'issues' | 'json'

export default function PageEditor() {
  const { pageId = '' } = useParams()
  const [answer, setAnswer] = useState<PageAnswer | null>(null)
  const [error, setError] = useState<'none' | 'missing' | 'failed'>('none')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let live = true
    setAnswer(null); setError('none')
    getPage(pageId, true).then((a) => { if (live) setAnswer(a) }).catch((e) => { if (live) setError(e instanceof ApiError && e.status === 404 ? 'missing' : 'failed') })
    return () => { live = false }
  }, [pageId, attempt])
  if (error === 'missing') {
    return (
      <>
        <BackLink to="/settings/pages" label="Pages" />
        <PageHead title="No such page" />
        <EmptyState title={`There is no page called “${pageId}”`} text="It may have been deleted, or the draft it was in discarded." action={<LinkButton to="/settings/pages">See the pages there are</LinkButton>} />
      </>
    )
  }
  if (error === 'failed') return <LoadError what="This page" onRetry={() => setAttempt((n) => n + 1)} />
  if (!answer) return <Spinner />
  return <Editor key={pageId + ':' + attempt} answer={answer} reload={() => setAttempt((n) => n + 1)} />
}

function Editor({ answer, reload }: { answer: PageAnswer; reload: () => void }) {
  const { cat, libraries, status, refresh, onConflict, showInApp } = useEditor()
  const navigate = useNavigate()
  const [search, setSearch] = useSearchParams()
  const [undo, setUndo] = useState(() => undoStart<Page>({ ...answer.page, sections: answer.page.sections || [] }))
  const [baseline, setBaseline] = useState<Page>(undo.present)
  const [source, setSource] = useState(answer.source)
  const [variant, setVariant] = useState(-1)
  const [selected, setSelected] = useState<Path | null>(null)
  const [tab, setTab] = useState<Tab>('page')
  const [issues, setIssues] = useState<Issue[]>(answer.issues || [])
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(0)
  const [adding, setAdding] = useState<Path | null>(null)
  const [presets, setPresets] = useState<Preset[]>([])
  const [focusForm, setFocusForm] = useState(0)
  const previewWith = useRef<Record<string, string>>({})
  const form = useRef<HTMLDivElement | null>(null)
  const grid = useRef<HTMLDivElement | null>(null)

  const page = undo.present
  const dirty = !samePage(page, baseline)
  const tidy = useMemo(() => tidyPage(page), [page])
  const name = pageName(cat, page)
  const leave = useUnsavedGuard(dirty, `“${name}”`)
  const change = useCallback((next: Page, tag = '') => setUndo((u) => undoPush(u, next, tag, Date.now())), [])

  // A variant that an undo took away leaves the page's own sections showing.
  const at = variant >= 0 && variant < (page.variants?.length || 0) ? variant : -1
  const sections = rootSections(page, at)
  const section = selected ? sectionAt(sections, selected) : undefined
  useEffect(() => { if (selected && !section) setSelected(null) }, [selected, section])

  useEffect(() => { getPresets().then(setPresets).catch(() => setPresets([])) }, [])

  // Checked by the server a moment after each change: the same rules a save is held to.
  useEffect(() => {
    const stop = new AbortController()
    const t = window.setTimeout(() => { validatePage(tidy, stop.signal).then((r) => setIssues(r.issues)).catch(() => { /* the next change asks again */ }) }, 450)
    return () => { window.clearTimeout(t); stop.abort() }
  }, [tidy])

  const located = useMemo(() => issues.map((issue) => ({ issue, at: locateIssue(issue.path, page.id) })), [issues, page.id])
  const view = useCallback((l: { issue: Issue; at: IssueTarget }): IssueView => {
    const fix = l.at.where === 'other' ? null : parseFix(l.issue, tidy, l.at.pointer)
    return { issue: l.issue, fix, apply: fix ? () => { change(applyFix(page, l.at.pointer, fix)); if (fix.kind === 'moveToEnd') setSelected(null); announce(`${fix.label}: done`) } : undefined }
  }, [tidy, page, change])
  const sectionIssues = (v: number, path: Path): Located[] => located.filter((l) => l.at.where === 'section' && l.at.variant === v && samePath(l.at.path, path)).map((l) => ({ field: l.at.field, view: view(l) }))
  const pageIssues: Located[] = located.filter((l) => l.at.where === 'page').map((l) => ({ field: l.at.field, view: view(l) }))
  const variantIssues = (v: number): Located[] => located.filter((l) => l.at.where === 'variant' && l.at.variant === v).map((l) => ({ field: l.at.field, view: view(l) }))
  const errors = issues.filter((i) => i.severity !== 'warning').length
  const warnings = issues.length - errors
  const marks = (path: Path) => {
    let e = 0, w = 0
    for (const l of located) if (l.at.where === 'section' && l.at.variant === at && samePath(l.at.path, path)) { if (l.issue.severity === 'warning') w++; else e++ }
    return { errors: e, warnings: w }
  }

  const save = async (): Promise<boolean> => {
    if (saving) return false
    setSaving(true)
    try {
      const check = await validatePage(tidy)
      setIssues(check.issues)
      if (!check.ok) {
        setTab('issues')
        toast('Not saved yet. There are problems to put right first.', { tone: 'warn' })
        return false
      }
      await savePage(tidy, onConflict)
      setBaseline(page)
      setSaved((n) => n + 1)
      if (source === 'builtin') setSource('changed')
      await refresh()
      toast('Saved to the draft')
      announce('Saved to the draft')
      return true
    } catch (e) {
      const list = issuesOf(e)
      if (list) { setIssues(list); setTab('issues'); toast('Not saved. The server found problems, listed under Problems.', { tone: 'danger' }) }
      else if (isConflict(e)) toast('Not saved. The draft changed in the meantime; your changes are still here.', { tone: 'warn' })
      else toast(message(e), { tone: 'danger' })
      return false
    } finally { setSaving(false) }
  }

  const doUndo = () => { if (undo.past.length) { setUndo(undoBack); announce('Undone') } }
  const doRedo = () => { if (undo.future.length) { setUndo(undoForward); announce('Redone') } }
  useEditKeys({ undo: doUndo, redo: doRedo, save: () => void save() })

  // On a narrow screen the form is below the list: choosing a section brings it into view.
  useEffect(() => {
    if (!focusForm) return
    const stacked = grid.current ? getComputedStyle(grid.current).gridTemplateColumns.trim().split(/\s+/).length < 2 : false
    const head = form.current?.querySelector<HTMLElement>('h2[tabindex]') || form.current?.querySelector<HTMLElement>('[role=tab][aria-selected=true]')
    if (stacked) form.current?.scrollIntoView({ block: 'start', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
    head?.focus({ preventScroll: true })
  }, [focusForm])

  const select = (path: Path) => { setSelected(path); setTab('section'); setFocusForm((n) => n + 1) }
  const setSections = (next: Section[], pick: Path | null, tag = '') => { change(withRootSections(page, at, next), tag); if (pick) setSelected(pick); else setSelected(null) }

  const insert = (make: (taken: string[]) => Section, parent: Path) => {
    const s = make(listIds(sections))
    const holder = parent.length ? sectionAt(sections, parent) : undefined
    const index = parent.length ? (holder?.children || []).length : selected && selected.length === 1 ? selected[0] + 1 : sections.length
    change(withRootSections(page, at, insertAt(sections, parent, index, s)))
    setSelected([...parent, index])
    setTab('section')
    setAdding(null)
    announce(`${sectionTitle(cat, s)} added${holder ? ` to ${sectionTitle(cat, holder)}` : ''}`)
  }

  // A link from the gallery of presets: put the section in, once.
  const wanted = search.get('preset')
  const put = useRef('')
  useEffect(() => {
    if (!wanted || !presets.length || put.current === wanted) return
    put.current = wanted
    const p = presets.find((x) => x.id === wanted && x.section)
    if (p?.section) { insert((taken) => fresh(p.section!, taken), []); toast(`“${p.label}” added. Save to keep it.`) }
    const next = new URLSearchParams(search)
    next.delete('preset')
    setSearch(next, { replace: true })
  }, [wanted, presets]) // eslint-disable-line react-hooks/exhaustive-deps

  const goTo = (t: IssueTarget) => {
    if (t.where === 'section') { setVariant(t.variant); setSelected(t.path); setTab('section') }
    else setTab('page')
    setFocusForm((n) => n + 1)
  }

  const reset = async () => {
    const custom = source === 'custom'
    if (!await ask(custom
      ? { title: `Delete “${name}”?`, body: 'The page is taken out of the draft. Publishing the draft removes it for everyone; menu entries that open it need removing too.', confirm: 'Delete the page', danger: true }
      : { title: `Reset “${name}” to how it comes?`, body: 'Your version of the page is taken out of the draft, and the built-in page takes its place once you publish.', confirm: 'Reset to default', danger: true })) return
    try {
      await removePage(page.id, onConflict)
      await refresh()
      toast(custom ? 'The page was deleted from the draft' : 'The page is back to the built-in one, in the draft')
      leave()
      if (custom) navigate('/settings/pages'); else reload()
    } catch (e) { toast(message(e), { tone: 'danger' }) }
  }

  const openInApp = async () => {
    const route = pageRoute(cat, page, previewWith.current)
    if (!route) { toast('Choose what to preview the page with first, beside the preview.'); return }
    if (dirty && !await save()) return
    leave()
    await showInApp(route)
  }

  const more: MenuEntry[] = [
    { key: 'app', label: 'Open the draft in the app', hint: 'The whole app shows you the draft, on this device', onSelect: () => void openInApp() },
    { key: 'export', label: 'Export this page', hint: 'As saved to the draft, as a file', onSelect: () => { getExport(page.id, true).then((d) => download(exportName(page.id), d)).catch((e) => toast(message(e), { tone: 'danger' })) } },
  ]
  if (source !== 'builtin') more.push({ kind: 'separator', key: 'sep' }, { key: 'reset', label: source === 'custom' ? 'Delete this page' : 'Reset to default', danger: true, onSelect: () => void reset() })

  const ctx: FieldCtx = useMemo(() => ({ cat, libraries, pages: status.pages, sections: selected ? sectionChoices(cat, sections, selected) : [] }), [cat, libraries, status.pages, sections, selected])
  const entry = { kind: page.kind, match: page.match }
  const addingTo = adding && adding.length ? sectionAt(sections, adding) : undefined
  const note = limitNote(cat, page)
  const tabs = [
    { value: 'section' as Tab, label: 'Section' },
    { value: 'page' as Tab, label: 'Page' },
    { value: 'issues' as Tab, label: 'Problems', badge: issues.length ? <span className={'pe-count' + (errors ? ' error' : '')}>{issues.length}</span> : undefined },
    { value: 'json' as Tab, label: 'JSON' },
  ]

  return (
    <div className="pe-wide">
      <BackLink to="/settings/pages" label="Pages" />
      <PageHead title={name} desc={<><span className={'pe-source ' + source} title={SOURCE_HELP[source]}>{SOURCE_WORDS[source] || source}</span> {matchText(cat, entry, libraries)}</>} />
      <DraftBar route={pageRoute(cat, page, previewWith.current) || '/'} />

      <div className="pe-toolbar" role="toolbar" aria-label="This page">
        <Button size="sm" variant="ghost" disabled={!undo.past.length} onClick={doUndo} title="Ctrl or Cmd with Z">Undo</Button>
        <Button size="sm" variant="ghost" disabled={!undo.future.length} onClick={doRedo} title="Ctrl or Cmd with Shift and Z">Redo</Button>
        <span className="pe-toolbar-state" role="status">
          {dirty ? <><i className="pe-dot" aria-hidden="true" />Changes not saved yet</> : status.draft && source !== 'builtin' ? 'Saved to the draft' : 'Nothing changed'}
          {errors > 0 ? <button type="button" className="linkish" onClick={() => setTab('issues')}>{errors === 1 ? '1 problem' : `${errors} problems`}</button> : warnings > 0 ? <button type="button" className="linkish" onClick={() => setTab('issues')}>{warnings === 1 ? '1 thing worth a look' : `${warnings} things worth a look`}</button> : null}
        </span>
        <Menu label="More for this page" items={more} trigger={(p) => <button type="button" className="btn ghost sm square" aria-label="More for this page" {...p}><IconMore /></button>} />
        <Button size="sm" variant="primary" busy={saving} disabled={!dirty} onClick={() => void save()}>Save to draft</Button>
      </div>

      <div className="pe-editor" ref={grid}>
        <section className="pe-pane pe-pane-tree" aria-label="Sections">
          <div className="pe-pane-head">
            <h2>Sections</h2>
            <Button size="sm" icon={<IconPlus size={14} />} onClick={() => setAdding([])}>Add</Button>
          </div>
          {(page.variants?.length || 0) > 0 && (
            <Select inputSize="sm" aria-label="Whose sections these are" value={String(at)} onChange={(e) => { setVariant(Number(e.target.value)); setSelected(null) }}>
              <option value="-1">The page itself</option>
              {(page.variants || []).map((v, i) => <option key={i} value={String(i)}>Variant: {v.label || words(v.id)}</option>)}
            </Select>
          )}
          {sections.length === 0
            ? <p className="pe-pane-empty">{at < 0 ? 'This page has no sections yet.' : 'This variant has no sections yet.'} Add one, or choose a ready-made one from the same list.</p>
            : <SectionTree cat={cat} sections={sections} selected={selected} label={at < 0 ? 'The page’s sections' : 'The variant’s sections'} marks={marks}
                onSelect={select} onChange={(next, pick) => setSections(next, pick)} onAddInside={(parent) => setAdding(parent)} />}
          {note && <p className="hint">{note}</p>}
          <p className="hint">Drag a section by its handle, or use the arrow keys on the handle, or the Move buttons.</p>
        </section>

        <section className="pe-pane pe-pane-form" aria-label="Details" ref={form}>
          <Tabs label="What to edit" value={tab} onChange={setTab} items={tabs} panelId="pe-panel" />
          <div id="pe-panel" role="tabpanel" aria-label={tabs.find((t) => t.value === tab)?.label as string}>
            {tab === 'section' && (section && selected
              ? <SectionForm key={at + ':' + selected.join('.') + ':' + section.component} section={section} page={page} taken={listIds(sections)} ctx={ctx} issues={sectionIssues(at, selected)}
                  onChange={(next, tag) => change(withRootSections(page, at, updateAt(sections, selected, () => next)), tag)}
                  onRename={(id) => change(withRootSections(page, at, renameId(cat, sections, selected, id)))} />
              : <EmptyState title="Choose a section" text="Pick one from the list to change it, or add a new one." action={<Button icon={<IconPlus size={14} />} onClick={() => setAdding([])}>Add a section</Button>} />)}
            {tab === 'page' && (
              <>
                <PageForm page={page} source={source} ctx={ctx} issues={pageIssues} onChange={change} />
                <VariantsForm page={page} ctx={ctx} current={at} max={cat.limits?.maxVariants || 8} issuesOf={variantIssues} onChange={change} onEdit={(v) => { setVariant(v); setSelected(null) }} />
              </>
            )}
            {tab === 'issues' && (issues.length === 0
              ? <EmptyState title="Nothing wrong" text="The server checks the page as you change it. Anything it finds is listed here and beside the field it is about." icon={sduiIcon('check')} />
              : <ul className="pe-issue-list">
                  {located.map((l, i) => (
                    <li key={i}>
                      {l.at.where === 'other' ? <span className="pe-issue-where">Elsewhere ({l.issue.path || 'the whole configuration'})</span> : <button type="button" className="linkish pe-issue-where" onClick={() => goTo(l.at)}>{issueWhere(cat, page, l.at)}</button>}
                      <IssueNotes issues={[view(l)]} />
                    </li>
                  ))}
                </ul>)}
            {tab === 'json' && (
              <JsonView<Page> key={undo.past.length + ':' + undo.future.length} what="The page" value={tidy}
                parse={(text) => { const got = parsePageJson(text, page.id); return 'error' in got ? got : { value: got.page } }}
                check={(v) => validatePage(v)} onApply={(v) => { change(v); setSelected(null); toast('Applied to the form. Save to keep it.') }} />
            )}
          </div>
        </section>

        <section className="pe-pane pe-pane-preview" aria-label="Preview">
          <div className="pe-pane-head"><h2>Preview</h2></div>
          <Preview page={page} saved={saved} dirty={dirty} onSave={() => void save()} onParams={(p) => { previewWith.current = p }} />
        </section>
      </div>

      {adding && (
        <Dialog title={addingTo ? `Add a section inside ${sectionTitle(cat, addingTo)}` : 'Add a section'} size="wide" onClose={() => setAdding(null)} className="pe-add">
          <Palette parent={addingTo?.component || ''} pageKind={page.kind} presets={presets}
            onComponent={(type) => insert((taken) => newSection(cat, type, page.kind, taken), adding)}
            onPreset={(p) => insert((taken) => fresh(p.section!, taken), adding)} />
        </Dialog>
      )}
    </div>
  )
}

// What can be added: every component the catalogue lets an administrator place here, in
// the catalogue's own groups, and the ready-made sections that suit this kind of page.
function Palette({ parent, pageKind, presets, onComponent, onPreset }: { parent: string; pageKind: string; presets: Preset[]; onComponent: (type: string) => void; onPreset: (p: Preset) => void }) {
  const { cat } = useEditor()
  const groups = palette(cat, parent)
  const ready = presets.filter((p) => p.kind === 'section' && p.section && (!p.pageKinds?.length || p.pageKinds.includes(pageKind)) && canHold(cat, parent, p.section.component))
  return (
    <div className="pe-palette">
      {groups.map((g) => (
        <section key={g.category} aria-label={g.label}>
          <h3>{g.label}</h3>
          <div className="pe-palette-grid" data-nav-group>
            {g.items.map((c) => (
              <button type="button" key={c.type} className="pe-tile" onClick={() => onComponent(c.type)}>
                <span className="pe-tile-icon" aria-hidden="true">{sduiIcon(c.icon)}</span>
                <span className="pe-tile-text"><strong>{c.label}</strong><small>{c.description}</small></span>
              </button>
            ))}
          </div>
        </section>
      ))}
      {ready.length > 0 && (
        <section aria-label="Ready-made">
          <h3>Ready-made</h3>
          <div className="pe-palette-grid" data-nav-group>
            {ready.map((p) => (
              <button type="button" key={p.id} className="pe-tile" onClick={() => onPreset(p)}>
                <span className="pe-tile-icon" aria-hidden="true">{sduiIcon(p.icon)}</span>
                <span className="pe-tile-text"><strong>{p.label}</strong><small>{p.description} <em>{componentSpec(cat, p.section!.component)?.label}</em></small></span>
              </button>
            ))}
          </div>
        </section>
      )}
      {groups.length === 0 && ready.length === 0 && <p className="hint">Nothing can be added here.</p>}
    </div>
  )
}
