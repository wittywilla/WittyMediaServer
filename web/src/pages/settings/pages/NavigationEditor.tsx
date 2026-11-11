// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The menu everyone gets: the tab bar of a phone, the side menu of the web app, the top
// menu of a television, from one list of entries. Each entry opens a page, a library, one
// of the app's own screens or a place in the app, and says where it sits on each kind of
// device. Settings is always in it: it can be moved, never hidden or given a condition
// (the server holds to that; this page just doesn't offer it). Saved to the draft, like a page.

import { useCallback, useEffect, useMemo, useState } from 'react'
import { announce } from '../../../a11y'
import { IconPlus } from '../../../icons'
import { ArrangeList, type ArrangeRow } from '../../../sdui/ArrangeList'
import { sduiIcon } from '../../../sdui/render/icons'
import { ask, Button, EmptyState, Field as Labelled, LoadError, Menu, Select, Spinner, Tabs, toast } from '../../../ui'
import { PageHead } from '../ui'
import { getNavigation, isConflict, issuesOf, message, resetNavigation, saveNavigation, validateNavigation } from './api'
import { DraftBar, useEditor } from './context'
import { ConditionEditor, IconPicker, IssueNotes, PropField, type FieldCtx, type IssueView } from './fields'
import { IdBox, JsonView } from './forms'
import { useEditKeys, useUnsavedGuard } from './guard'
import {
  NAV_TYPES, applyFix, conditionText, fieldFor, formFactorLabel, isSettingsEntry, locateIssue, moveItem, navScreens, navTitle, newNavItem, pageName, parseFix, parseNavigationJson,
  placementOf, sameNavigation, setPlacement, slug, tidyNavigation, undoBack, undoForward, undoPush, undoStart, uniqueId,
  type Issue, type NavItem, type Navigation,
} from './editorModel.ts'

type Tab = 'entry' | 'issues' | 'json'

export default function NavigationEditor() {
  const [loaded, setLoaded] = useState<{ navigation: Navigation; source: string } | null>(null)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let live = true
    setLoaded(null); setFailed(false)
    getNavigation(true).then((a) => { if (live) setLoaded({ navigation: a.navigation, source: a.source }) }).catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [attempt])
  if (failed) return <><PageHead title="The menu" /><LoadError what="The menu" onRetry={() => setAttempt((n) => n + 1)} /></>
  if (!loaded) return <Spinner />
  return <Editor key={attempt} start={loaded.navigation} source={loaded.source} reload={() => setAttempt((n) => n + 1)} />
}

function Editor({ start, source, reload }: { start: Navigation; source: string; reload: () => void }) {
  const { cat, libraries, status, refresh, onConflict } = useEditor()
  const [undo, setUndo] = useState(() => undoStart<Navigation>({ items: start.items || [] }))
  const [baseline, setBaseline] = useState(undo.present)
  const [changed, setChanged] = useState(source === 'changed')
  const [selected, setSelected] = useState<number>(-1)
  const [tab, setTab] = useState<Tab>('entry')
  const [issues, setIssues] = useState<Issue[]>([])
  const [saving, setSaving] = useState(false)
  const nav = undo.present
  const items = nav.items
  const dirty = !sameNavigation(nav, baseline)
  const tidy = useMemo(() => tidyNavigation(nav), [nav])
  const leave = useUnsavedGuard(dirty, 'the menu')
  const change = useCallback((next: Navigation, tag = '') => setUndo((u) => undoPush(u, next, tag, Date.now())), [])
  const ctx: FieldCtx = useMemo(() => ({ cat, libraries, pages: status.pages }), [cat, libraries, status.pages])
  const formFactors = cat.formFactors || []
  const placements = cat.placements || []

  // The check asks the server about pages as they are published: one that so far is only
  // in the draft exists as far as this menu is concerned, and isn't called missing.
  const inDraft = useMemo(() => new Set(status.pages.map((p) => p.id)), [status.pages])
  const real = useCallback((list: Issue[]) => list.filter((i) => {
    const at = locateIssue(i.path, '', true)
    return !(at.field === 'page' && at.item >= 0 && inDraft.has(tidy.items[at.item]?.page || '') && /^there is no page called/.test(i.message))
  }), [inDraft, tidy])

  useEffect(() => {
    const stop = new AbortController()
    const t = window.setTimeout(() => { validateNavigation(tidy, stop.signal).then((r) => setIssues(real(r.issues))).catch(() => { /* the next change asks again */ }) }, 450)
    return () => { window.clearTimeout(t); stop.abort() }
  }, [tidy, real])

  const located = useMemo(() => issues.map((issue) => ({ issue, at: locateIssue(issue.path, '', true) })), [issues])
  const view = (l: (typeof located)[number]): IssueView => {
    const fix = l.at.where === 'nav' ? parseFix(l.issue, tidy, l.at.pointer) : null
    return { issue: l.issue, fix, apply: fix ? () => change(applyFix(nav, l.at.pointer, fix)) : undefined }
  }
  const at = (i: number, field: string) => located.filter((l) => l.at.item === i && (l.at.field === field || l.at.field.startsWith(field + '/'))).map(view)
  const restOf = (i: number, shown: string[]) => located.filter((l) => l.at.item === i && !shown.some((f) => l.at.field === f || l.at.field.startsWith(f + '/'))).map(view)
  const errors = issues.filter((i) => i.severity !== 'warning').length

  const save = async () => {
    if (saving) return
    setSaving(true)
    try {
      const check = await validateNavigation(tidy)
      const found = real(check.issues)
      setIssues(found)
      if (found.some((i) => i.severity !== 'warning')) { setTab('issues'); toast('Not saved yet. There are problems to put right first.', { tone: 'warn' }); return }
      await saveNavigation(tidy, onConflict)
      setBaseline(nav); setChanged(true)
      await refresh()
      toast('Saved to the draft')
      announce('Saved to the draft')
    } catch (e) {
      const list = issuesOf(e)
      if (list) { setIssues(list); setTab('issues'); toast('Not saved. The server found problems, listed under Problems.', { tone: 'danger' }) }
      else if (isConflict(e)) toast('Not saved. The draft changed in the meantime; your changes are still here.', { tone: 'warn' })
      else toast(message(e), { tone: 'danger' })
    } finally { setSaving(false) }
  }
  const doUndo = () => { if (undo.past.length) { setUndo(undoBack); announce('Undone') } }
  const doRedo = () => { if (undo.future.length) { setUndo(undoForward); announce('Redone') } }
  useEditKeys({ undo: doUndo, redo: doRedo, save: () => void save() })

  const reset = async () => {
    if (!await ask({ title: 'Reset the menu to default?', body: 'Your menu is taken out of the draft, and the built-in one takes its place once you publish.', confirm: 'Reset the menu', danger: true })) return
    try {
      await resetNavigation(onConflict)
      await refresh()
      toast('The menu is back to the built-in one, in the draft')
      leave()
      reload()
    } catch (e) { toast(message(e), { tone: 'danger' }) }
  }

  const setItem = (i: number, it: NavItem, tag = '') => change({ items: items.map((x, n) => (n === i ? it : x)) }, tag ? `${i}/${tag}` : '')
  const add = (type: string) => {
    const it = newNavItem(type, items.map((x) => x.id))
    // New entries go in above Settings when it is last, which is where it likes to be.
    const end = items.length && isSettingsEntry(items[items.length - 1]) ? items.length - 1 : items.length
    const next = [...items]
    next.splice(end, 0, it)
    change({ items: next })
    setSelected(end); setTab('entry')
    announce('Entry added. Choose what it opens.')
  }
  const remove = (i: number) => { change({ items: items.filter((_, n) => n !== i) }); setSelected(-1); announce('Entry removed') }

  // Rows are known by their place as well as their ID, so two entries that (wrongly) share an ID still move one at a time.
  const rows: ArrangeRow[] = items.map((it, i) => {
    const where = formFactors.map((f) => placementOf(cat, it, f))
    const hiddenOn = formFactors.filter((_, n) => where[n] === 'hidden').map((f) => formFactorLabel(cat, f).toLowerCase())
    const problems = located.filter((l) => l.at.item === i && l.issue.severity !== 'warning').length
    return {
      id: `${i}:${it.id}`, title: navTitle(cat, it, status.pages, libraries), icon: sduiIcon(it.icon || iconOf(it)), hideFixed: isSettingsEntry(it),
      note: [NAV_TYPES.find((t) => t.value === it.type)?.label.toLowerCase(), it.when && Object.keys(it.when).length ? 'sometimes' : '', hiddenOn.length ? `not on ${hiddenOn.join(', ')}` : '', problems ? (problems === 1 ? '1 problem' : `${problems} problems`) : ''].filter(Boolean).join(' · '),
    }
  })
  function iconOf(it: NavItem): string {
    if (it.type === 'page') return status.pages.find((p) => p.id === it.page)?.icon || ''
    if (it.type === 'native') return cat.nativeScreens?.find((s) => s.screen === it.screen)?.icon || ''
    return it.type === 'route' ? '' : 'library'
  }
  const indexOf = (rowId: string) => Number(rowId.split(':')[0])

  const it = selected >= 0 ? items[selected] : undefined
  const settings = !!it && isSettingsEntry(it)
  const screens = navScreens(cat)
  const ids = items.map((x) => x.id)
  const tabs = [
    { value: 'entry' as Tab, label: 'Entry' },
    { value: 'issues' as Tab, label: 'Problems', badge: issues.length ? <span className={'pe-count' + (errors ? ' error' : '')}>{issues.length}</span> : undefined },
    { value: 'json' as Tab, label: 'JSON' },
  ]
  const fixedShown = ['id', 'type', 'label', 'icon', 'page', 'library', 'screen', 'route', 'when', 'placement']

  return (
    <div className="pe-box">
      <PageHead title="The menu" desc="The app’s main menu, for everyone: what is in it, in what order, and where each entry sits on a phone, a tablet, a television and a computer. People can still arrange their own from Appearance, as far as that goes." />
      <DraftBar />
      <div className="pe-toolbar" role="toolbar" aria-label="The menu">
        <Button size="sm" variant="ghost" disabled={!undo.past.length} onClick={doUndo} title="Ctrl or Cmd with Z">Undo</Button>
        <Button size="sm" variant="ghost" disabled={!undo.future.length} onClick={doRedo} title="Ctrl or Cmd with Shift and Z">Redo</Button>
        <span className="pe-toolbar-state" role="status">
          {dirty ? <><i className="pe-dot" aria-hidden="true" />Changes not saved yet</> : changed ? 'Your menu, as saved' : 'The built-in menu'}
          {errors > 0 && <button type="button" className="linkish" onClick={() => setTab('issues')}>{errors === 1 ? '1 problem' : `${errors} problems`}</button>}
        </span>
        <Button size="sm" variant="ghost" disabled={!changed} onClick={() => void reset()}>Reset the menu to default</Button>
        <Button size="sm" variant="primary" busy={saving} disabled={!dirty} onClick={() => void save()}>Save to draft</Button>
      </div>

      <div className="pe-editor two">
        <section className="pe-pane pe-pane-tree" aria-label="Entries">
          <div className="pe-pane-head">
            <h2>Entries</h2>
            <Menu label="Add an entry" items={NAV_TYPES.map((t) => ({ key: t.value, label: t.label, hint: t.help, onSelect: () => add(t.value) }))}
              trigger={(p) => <button type="button" className="btn sm" {...p}><IconPlus size={14} /> Add</button>} />
          </div>
          {items.length === 0
            ? <p className="pe-pane-empty">The menu is empty. Settings is put back by the server whatever you do; add the rest.</p>
            : <ArrangeList rows={rows} label="Menu entries, in order" canMove canHide={false}
                onMove={(id, to) => { const from = indexOf(id); change({ items: moveItem(items, from, to) }); if (selected === from) setSelected(to); else setSelected(-1) }}
                onHidden={() => { /* where an entry shows is set per device, in its form */ }}
                extra={(row) => <Button size="sm" variant={indexOf(row.id) === selected ? 'primary' : 'ghost'} aria-pressed={indexOf(row.id) === selected} aria-label={`Edit ${row.title}`} onClick={() => { setSelected(indexOf(row.id)); setTab('entry') }}>Edit</Button>} />}
          <p className="hint">Drag an entry by its handle, or use the arrow keys on the handle, or the Move buttons. A bar only holds so many: past its limit, the last entries move behind More by themselves.</p>
        </section>

        <section className="pe-pane pe-pane-form" aria-label="Details">
          <Tabs label="What to edit" value={tab} onChange={setTab} items={tabs} panelId="pe-nav-panel" />
          <div id="pe-nav-panel" role="tabpanel" aria-label={tabs.find((t) => t.value === tab)?.label as string}>
            {tab === 'entry' && (!it
              ? <EmptyState title="Choose an entry" text="Pick one from the list to change it, or add a new one." icon={sduiIcon('list')} />
              : <div className="pe-form" key={selected}>
                  <header className="pe-form-head">
                    <span className="pe-form-icon" aria-hidden="true">{sduiIcon(it.icon || iconOf(it))}</span>
                    <div><h2 tabIndex={-1}>{navTitle(cat, it, status.pages, libraries)}</h2><p>{settings ? 'Settings is always in the menu. It can be moved and renamed, not removed, hidden or given a condition.' : NAV_TYPES.find((t) => t.value === it.type)?.help}</p></div>
                  </header>
                  <IssueNotes issues={restOf(selected, fixedShown)} />

                  <div className="pe-field">
                    <Labelled label="What it opens" className={at(selected, 'type').length ? 'invalid' : undefined}>
                      <Select value={it.type} disabled={settings} onChange={(e) => setItem(selected, { id: it.id, type: e.target.value, label: it.label, icon: it.icon, when: it.when, placement: it.placement })}>
                        {NAV_TYPES.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
                        {!NAV_TYPES.some((t) => t.value === it.type) && <option value={it.type}>{it.type || 'Not chosen'} (not a kind of entry)</option>}
                      </Select>
                    </Labelled>
                    <IssueNotes issues={at(selected, 'type')} />
                  </div>
                  {it.type === 'page' && (
                    <div className="pe-field">
                      <Labelled label="Page" required className={at(selected, 'page').length ? 'invalid' : undefined}>
                        <Select value={it.page || ''} onChange={(e) => {
                          const page = e.target.value
                          // An entry still called "entry" is named after the page it first opens.
                          const id = /^entry(-[0-9]+)?$/.test(it.id) && page ? uniqueId(slug(page, it.id), ids.filter((x) => x !== it.id)) : it.id
                          setItem(selected, { ...it, id, page: page || undefined })
                        }}>
                          <option value="">Choose a page…</option>
                          {status.pages.map((p) => <option key={p.id} value={p.id}>{pageName(cat, p)} ({p.id})</option>)}
                          {it.page && !status.pages.some((p) => p.id === it.page) && <option value={it.page}>{it.page} (no such page)</option>}
                        </Select>
                      </Labelled>
                      <IssueNotes issues={at(selected, 'page')} />
                    </div>
                  )}
                  {it.type === 'library' && <PropField field={fieldFor(cat, { name: 'library', type: 'library', label: 'Library', required: true })} value={it.library} ctx={ctx} issues={at(selected, 'library')} onChange={(v) => setItem(selected, { ...it, library: typeof v === 'number' ? v : undefined })} />}
                  {it.type === 'native' && (
                    <div className="pe-field">
                      <Labelled label="Screen" required hint={screens.find((s) => s.screen === it.screen)?.description} className={at(selected, 'screen').length ? 'invalid' : undefined}>
                        <Select value={it.screen || ''} disabled={settings} onChange={(e) => setItem(selected, { ...it, screen: e.target.value || undefined })}>
                          <option value="">Choose a screen…</option>
                          {screens.filter((s) => settings || s.screen !== 'settings' || !items.some(isSettingsEntry)).map((s) => <option key={s.screen} value={s.screen}>{s.label}</option>)}
                          {it.screen && !screens.some((s) => s.screen === it.screen) && <option value={it.screen}>{it.screen} (not a screen an entry can open)</option>}
                        </Select>
                      </Labelled>
                      <IssueNotes issues={at(selected, 'screen')} />
                    </div>
                  )}
                  {it.type === 'route' && <PropField field={fieldFor(cat, { name: 'route', type: 'route', label: 'Place in the app', required: true })} value={it.route} ctx={ctx} issues={at(selected, 'route')} onChange={(v) => setItem(selected, { ...it, route: typeof v === 'string' ? v : undefined }, 'route')} />}

                  <PropField field={fieldFor(cat, { name: 'label', type: 'string', label: 'Name in the menu', maxLength: 40, required: it.type === 'route', help: it.type === 'route' ? 'What the entry says.' : 'Leave empty to use the page’s, the library’s or the screen’s own name.' })}
                    value={it.label} ctx={ctx} issues={at(selected, 'label')} onChange={(v) => setItem(selected, { ...it, label: typeof v === 'string' ? v : undefined }, 'label')} />
                  <div className="pe-field">
                    <Labelled label="Icon" hint="Leave on the usual one to use the page’s, or the library’s kind."><IconPicker cat={cat} value={it.icon} none="The usual one" onChange={(v) => setItem(selected, { ...it, icon: typeof v === 'string' ? v : undefined })} /></Labelled>
                    <IssueNotes issues={at(selected, 'icon')} />
                  </div>

                  <section className="pe-group" aria-label="Where it sits">
                    <h3>Where it sits</h3>
                    <p className="pe-group-desc">On each kind of device: in the bar itself, behind More, at the far end, or not at all.</p>
                    <div className="pe-place" role="group" aria-label="Where it sits on each kind of device">
                      {formFactors.map((ff) => (
                        <Labelled key={ff} label={formFactorLabel(cat, ff)}>
                          <Select inputSize="sm" value={placementOf(cat, it, ff)} onChange={(e) => setItem(selected, setPlacement(cat, it, ff, e.target.value))}>
                            {placements.map((p) => <option key={p.value} value={p.value} disabled={settings && p.value === 'hidden'}>{p.label}</option>)}
                          </Select>
                        </Labelled>
                      ))}
                    </div>
                    <IssueNotes issues={at(selected, 'placement')} />
                  </section>

                  {!settings && (
                    <section className="pe-group" aria-label="When it is shown">
                      <h3>When it is shown</h3>
                      <p className="pe-group-desc">Leave this alone to show the entry to everyone. Now: {conditionText(cat, it.when).toLowerCase()}.</p>
                      <ConditionEditor value={it.when} ctx={ctx} name={navTitle(cat, it, status.pages, libraries)} issues={at(selected, 'when')} onChange={(c) => setItem(selected, { ...it, when: c }, 'when')} />
                    </section>
                  )}

                  <section className="pe-group" aria-label="More">
                    <h3>More</h3>
                    <IdBox value={it.id} taken={ids.filter((_, n) => n !== selected)} what="entry" hint="What the entry is known by: each person’s own order of the menu refers to it. Lower-case letters, digits, - and _." onCommit={(id) => setItem(selected, { ...it, id })} />
                    <IssueNotes issues={at(selected, 'id')} />
                    {!settings && <div className="pe-row-actions"><Button size="sm" variant="danger" onClick={() => remove(selected)}>Remove this entry</Button></div>}
                  </section>
                </div>)}
            {tab === 'issues' && (issues.length === 0
              ? <EmptyState title="Nothing wrong" text="The server checks the menu as you change it." icon={sduiIcon('check')} />
              : <ul className="pe-issue-list">
                  {located.map((l, i) => (
                    <li key={i}>
                      {l.at.item >= 0 && items[l.at.item]
                        ? <button type="button" className="linkish pe-issue-where" onClick={() => { setSelected(l.at.item); setTab('entry') }}>{navTitle(cat, items[l.at.item], status.pages, libraries)}{l.at.field ? `, ${l.at.field.replace('/', ' on ')}` : ''}</button>
                        : <span className="pe-issue-where">{l.issue.path || 'The menu'}</span>}
                      <IssueNotes issues={[view(l)]} />
                    </li>
                  ))}
                </ul>)}
            {tab === 'json' && (
              <JsonView<Navigation> key={undo.past.length + ':' + undo.future.length} what="The menu" value={tidy}
                parse={(text) => { const got = parseNavigationJson(text); return 'error' in got ? got : { value: got.navigation } }}
                check={async (v) => { const r = await validateNavigation(v); return { ok: r.ok, issues: r.issues } }}
                onApply={(v) => { change(v); setSelected(-1); toast('Applied to the form. Save to keep it.') }} />
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
