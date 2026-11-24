// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The menu everyone gets: the tab bar of a phone, the side menu of the web app, the top
// menu of a television, from one list of entries. Each entry opens a page, a library, one
// of the app's own screens or a place in the app, and says where it sits on each kind of
// device. Settings is always in it: it can be moved, never hidden or given a condition
// (the server holds to that; this page just doesn't offer it). Saved to the draft, like a page.

import { useCallback, useEffect, useMemo, useState } from 'react'
import { announce } from '../../../a11y'
import { t, tk, tn, tx, bidi } from '../../../i18n'
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
  if (failed) return <><PageHead title={t('The menu')} /><LoadError what={t('The menu')} onRetry={() => setAttempt((n) => n + 1)} /></>
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
  const leave = useUnsavedGuard(dirty, t('the menu'))
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
    const timer = window.setTimeout(() => { validateNavigation(tidy, stop.signal).then((r) => setIssues(real(r.issues))).catch(() => { /* the next change asks again */ }) }, 450)
    return () => { window.clearTimeout(timer); stop.abort() }
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
      if (found.some((i) => i.severity !== 'warning')) { setTab('issues'); toast(t('Not saved yet. There are problems to put right first.'), { tone: 'warn' }); return }
      await saveNavigation(tidy, onConflict)
      setBaseline(nav); setChanged(true)
      await refresh()
      toast(t('Saved to the draft'))
      announce(t('Saved to the draft'))
    } catch (e) {
      const list = issuesOf(e)
      if (list) { setIssues(list); setTab('issues'); toast(t('Not saved. The server found problems, listed under Problems.'), { tone: 'danger' }) }
      else if (isConflict(e)) toast(t('Not saved. The draft changed in the meantime; your changes are still here.'), { tone: 'warn' })
      else toast(message(e), { tone: 'danger' })
    } finally { setSaving(false) }
  }
  const doUndo = () => { if (undo.past.length) { setUndo(undoBack); announce(t('Undone')) } }
  const doRedo = () => { if (undo.future.length) { setUndo(undoForward); announce(t('Redone')) } }
  useEditKeys({ undo: doUndo, redo: doRedo, save: () => void save() })

  const reset = async () => {
    if (!await ask({ title: t('Reset the menu to default?'), body: t('Your menu is taken out of the draft, and the built-in one takes its place once you publish.'), confirm: t('Reset the menu'), danger: true })) return
    try {
      await resetNavigation(onConflict)
      await refresh()
      toast(t('The menu is back to the built-in one, in the draft'))
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
    announce(t('Entry added. Choose what it opens.'))
  }
  const remove = (i: number) => { change({ items: items.filter((_, n) => n !== i) }); setSelected(-1); announce(t('Entry removed')) }

  // Rows are known by their place as well as their ID, so two entries that (wrongly) share an ID still move one at a time.
  const rows: ArrangeRow[] = items.map((it, i) => {
    const where = formFactors.map((f) => placementOf(cat, it, f))
    const hiddenOn = formFactors.filter((_, n) => where[n] === 'hidden').map((f) => tx(formFactorLabel(cat, f)).toLowerCase())
    const problems = located.filter((l) => l.at.item === i && l.issue.severity !== 'warning').length
    return {
      id: `${i}:${it.id}`, title: navTitle(cat, it, status.pages, libraries), icon: sduiIcon(it.icon || iconOf(it)), hideFixed: isSettingsEntry(it),
      note: [tx(NAV_TYPES.find((n) => n.value === it.type)?.label).toLowerCase(), it.when && Object.keys(it.when).length ? t('sometimes') : '', hiddenOn.length ? t('not on {devices}', { devices: hiddenOn.join(', ') }) : '', problems ? tn('{count} problem', '{count} problems', problems) : ''].filter(Boolean).map(bidi).join(' · '),
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
    { value: 'entry' as Tab, label: t('Entry') },
    { value: 'issues' as Tab, label: t('Problems'), badge: issues.length ? <span className={'pe-count' + (errors ? ' error' : '')}>{issues.length}</span> : undefined },
    { value: 'json' as Tab, label: t('JSON') },
  ]
  const fixedShown = ['id', 'type', 'label', 'icon', 'page', 'library', 'screen', 'route', 'when', 'placement']

  return (
    <div className="pe-box">
      <PageHead title={t('The menu')} desc={t('The app’s main menu, for everyone: what is in it, in what order, and where each entry sits on a phone, a tablet, a television and a computer. People can still arrange their own from Appearance, as far as that goes.')} />
      <DraftBar />
      <div className="pe-toolbar" role="toolbar" aria-label={t('The menu')}>
        <Button size="sm" variant="ghost" disabled={!undo.past.length} onClick={doUndo} title={t('Ctrl or Cmd with Z')}>{t('Undo')}</Button>
        <Button size="sm" variant="ghost" disabled={!undo.future.length} onClick={doRedo} title={t('Ctrl or Cmd with Shift and Z')}>{t('Redo')}</Button>
        <span className="pe-toolbar-state" role="status">
          {dirty ? <><i className="pe-dot" aria-hidden="true" />{t('Changes not saved yet')}</> : changed ? t('Your menu, as saved') : t('The built-in menu')}
          {errors > 0 && <button type="button" className="linkish" onClick={() => setTab('issues')}>{tn('{count} problem', '{count} problems', errors)}</button>}
        </span>
        <Button size="sm" variant="ghost" disabled={!changed} onClick={() => void reset()}>{t('Reset the menu to default')}</Button>
        <Button size="sm" variant="primary" busy={saving} disabled={!dirty} onClick={() => void save()}>{t('Save to draft')}</Button>
      </div>

      <div className="pe-editor two">
        <section className="pe-pane pe-pane-tree" aria-label={t('Entries')}>
          <div className="pe-pane-head">
            <h2>{t('Entries')}</h2>
            <Menu label={t('Add an entry')} items={NAV_TYPES.map((n) => ({ key: n.value, label: tx(n.label), hint: tx(n.help), onSelect: () => add(n.value) }))}
              trigger={(p) => <button type="button" className="btn sm" {...p}><IconPlus size={14} /> {t('Add')}</button>} />
          </div>
          {items.length === 0
            ? <p className="pe-pane-empty">{t('The menu is empty. Settings is put back by the server whatever you do; add the rest.')}</p>
            : <ArrangeList rows={rows} label={t('Menu entries, in order')} canMove canHide={false}
                onMove={(id, to) => { const from = indexOf(id); change({ items: moveItem(items, from, to) }); if (selected === from) setSelected(to); else setSelected(-1) }}
                onHidden={() => { /* where an entry shows is set per device, in its form */ }}
                extra={(row) => <Button size="sm" variant={indexOf(row.id) === selected ? 'primary' : 'ghost'} aria-pressed={indexOf(row.id) === selected} aria-label={t('Edit {name}', { name: row.title })} onClick={() => { setSelected(indexOf(row.id)); setTab('entry') }}>{t('Edit')}</Button>} />}
          <p className="hint">{t('Drag an entry by its handle, or use the arrow keys on the handle, or the Move buttons. A bar only holds so many: past its limit, the last entries move behind More by themselves.')}</p>
        </section>

        <section className="pe-pane pe-pane-form" aria-label={t('Details')}>
          <Tabs label={t('What to edit')} value={tab} onChange={setTab} items={tabs} panelId="pe-nav-panel" />
          <div id="pe-nav-panel" role="tabpanel" aria-label={tabs.find((x) => x.value === tab)?.label}>
            {tab === 'entry' && (!it
              ? <EmptyState title={t('Choose an entry')} text={t('Pick one from the list to change it, or add a new one.')} icon={sduiIcon('list')} />
              : <div className="pe-form" key={selected}>
                  <header className="pe-form-head">
                    <span className="pe-form-icon" aria-hidden="true">{sduiIcon(it.icon || iconOf(it))}</span>
                    <div><h2 tabIndex={-1}>{navTitle(cat, it, status.pages, libraries)}</h2><p>{settings ? t('Settings is always in the menu. It can be moved and renamed, not removed, hidden or given a condition.') : tx(NAV_TYPES.find((n) => n.value === it.type)?.help)}</p></div>
                  </header>
                  <IssueNotes issues={restOf(selected, fixedShown)} />

                  <div className="pe-field">
                    <Labelled label={t('What it opens')} className={at(selected, 'type').length ? 'invalid' : undefined}>
                      <Select value={it.type} disabled={settings} onChange={(e) => setItem(selected, { id: it.id, type: e.target.value, label: it.label, icon: it.icon, when: it.when, placement: it.placement })}>
                        {NAV_TYPES.map((n) => <option key={n.value} value={n.value}>{tx(n.label)}</option>)}
                        {!NAV_TYPES.some((n) => n.value === it.type) && <option value={it.type}>{t('{type} (not a kind of entry)', { type: it.type || t('Not chosen') })}</option>}
                      </Select>
                    </Labelled>
                    <IssueNotes issues={at(selected, 'type')} />
                  </div>
                  {it.type === 'page' && (
                    <div className="pe-field">
                      <Labelled label={t('Page')} required className={at(selected, 'page').length ? 'invalid' : undefined}>
                        <Select value={it.page || ''} onChange={(e) => {
                          const page = e.target.value
                          // An entry still called "entry" is named after the page it first opens.
                          const id = /^entry(-[0-9]+)?$/.test(it.id) && page ? uniqueId(slug(page, it.id), ids.filter((x) => x !== it.id)) : it.id
                          setItem(selected, { ...it, id, page: page || undefined })
                        }}>
                          <option value="">{t('Choose a page…')}</option>
                          {status.pages.map((p) => <option key={p.id} value={p.id}>{t('{name} ({id})', { name: pageName(cat, p), id: p.id })}</option>)}
                          {it.page && !status.pages.some((p) => p.id === it.page) && <option value={it.page}>{t('{id} (no such page)', { id: it.page })}</option>}
                        </Select>
                      </Labelled>
                      <IssueNotes issues={at(selected, 'page')} />
                    </div>
                  )}
                  {it.type === 'library' && <PropField field={fieldFor(cat, { name: 'library', type: 'library', label: tk('Library'), required: true })} value={it.library} ctx={ctx} issues={at(selected, 'library')} onChange={(v) => setItem(selected, { ...it, library: typeof v === 'number' ? v : undefined })} />}
                  {it.type === 'native' && (
                    <div className="pe-field">
                      <Labelled label={t('Screen')} required hint={tx(screens.find((s) => s.screen === it.screen)?.description) || undefined} className={at(selected, 'screen').length ? 'invalid' : undefined}>
                        <Select value={it.screen || ''} disabled={settings} onChange={(e) => setItem(selected, { ...it, screen: e.target.value || undefined })}>
                          <option value="">{t('Choose a screen…')}</option>
                          {screens.filter((s) => settings || s.screen !== 'settings' || !items.some(isSettingsEntry)).map((s) => <option key={s.screen} value={s.screen}>{tx(s.label)}</option>)}
                          {it.screen && !screens.some((s) => s.screen === it.screen) && <option value={it.screen}>{t('{screen} (not a screen an entry can open)', { screen: it.screen })}</option>}
                        </Select>
                      </Labelled>
                      <IssueNotes issues={at(selected, 'screen')} />
                    </div>
                  )}
                  {it.type === 'route' && <PropField field={fieldFor(cat, { name: 'route', type: 'route', label: tk('Place in the app'), required: true })} value={it.route} ctx={ctx} issues={at(selected, 'route')} onChange={(v) => setItem(selected, { ...it, route: typeof v === 'string' ? v : undefined }, 'route')} />}

                  <PropField field={fieldFor(cat, { name: 'label', type: 'string', label: tk('Name in the menu'), maxLength: 40, required: it.type === 'route', help: it.type === 'route' ? tk('What the entry says.') : tk('Leave empty to use the page’s, the library’s or the screen’s own name.') })}
                    value={it.label} ctx={ctx} issues={at(selected, 'label')} onChange={(v) => setItem(selected, { ...it, label: typeof v === 'string' ? v : undefined }, 'label')} />
                  <div className="pe-field">
                    <Labelled label={t('Icon')} hint={t('Leave on the usual one to use the page’s, or the library’s kind.')}><IconPicker cat={cat} value={it.icon} none={t('The usual one')} onChange={(v) => setItem(selected, { ...it, icon: typeof v === 'string' ? v : undefined })} /></Labelled>
                    <IssueNotes issues={at(selected, 'icon')} />
                  </div>

                  <section className="pe-group" aria-label={t('Where it sits')}>
                    <h3>{t('Where it sits')}</h3>
                    <p className="pe-group-desc">{t('On each kind of device: in the bar itself, behind More, at the far end, or not at all.')}</p>
                    <div className="pe-place" role="group" aria-label={t('Where it sits on each kind of device')}>
                      {formFactors.map((ff) => (
                        <Labelled key={ff} label={tx(formFactorLabel(cat, ff))}>
                          <Select inputSize="sm" value={placementOf(cat, it, ff)} onChange={(e) => setItem(selected, setPlacement(cat, it, ff, e.target.value))}>
                            {placements.map((p) => <option key={p.value} value={p.value} disabled={settings && p.value === 'hidden'}>{tx(p.label)}</option>)}
                          </Select>
                        </Labelled>
                      ))}
                    </div>
                    <IssueNotes issues={at(selected, 'placement')} />
                  </section>

                  {!settings && (
                    <section className="pe-group" aria-label={t('When it is shown')}>
                      <h3>{t('When it is shown')}</h3>
                      <p className="pe-group-desc">{t('Leave this alone to show the entry to everyone. Now: {condition}.', { condition: conditionText(cat, it.when).toLowerCase() })}</p>
                      <ConditionEditor value={it.when} ctx={ctx} name={navTitle(cat, it, status.pages, libraries)} issues={at(selected, 'when')} onChange={(c) => setItem(selected, { ...it, when: c }, 'when')} />
                    </section>
                  )}

                  <section className="pe-group" aria-label={t('More')}>
                    <h3>{t('More')}</h3>
                    <IdBox value={it.id} taken={ids.filter((_, n) => n !== selected)} what="entry" hint={t('What the entry is known by: each person’s own order of the menu refers to it. Lower-case letters, digits, - and _.')} onCommit={(id) => setItem(selected, { ...it, id })} />
                    <IssueNotes issues={at(selected, 'id')} />
                    {!settings && <div className="pe-row-actions"><Button size="sm" variant="danger" onClick={() => remove(selected)}>{t('Remove this entry')}</Button></div>}
                  </section>
                </div>)}
            {tab === 'issues' && (issues.length === 0
              ? <EmptyState title={t('Nothing wrong')} text={t('The server checks the menu as you change it.')} icon={sduiIcon('check')} />
              : <ul className="pe-issue-list">
                  {located.map((l, i) => (
                    <li key={i}>
                      {l.at.item >= 0 && items[l.at.item]
                        ? <button type="button" className="linkish pe-issue-where" onClick={() => { setSelected(l.at.item); setTab('entry') }}>{l.at.field ? t('{name}, {field}', { name: navTitle(cat, items[l.at.item], status.pages, libraries), field: l.at.field.includes('/') ? t('{a} on {b}', { a: l.at.field.split('/')[0], b: l.at.field.split('/').slice(1).join('/') }) : l.at.field }) : navTitle(cat, items[l.at.item], status.pages, libraries)}</button>
                        : <span className="pe-issue-where">{l.issue.path || t('The menu')}</span>}
                      <IssueNotes issues={[view(l)]} />
                    </li>
                  ))}
                </ul>)}
            {tab === 'json' && (
              <JsonView<Navigation> key={undo.past.length + ':' + undo.future.length} what={t('The menu')} value={tidy}
                parse={(text) => { const got = parseNavigationJson(text); return 'error' in got ? got : { value: got.navigation } }}
                check={async (v) => { const r = await validateNavigation(v); return { ok: r.ok, issues: r.issues } }}
                onApply={(v) => { change(v); setSelected(-1); toast(t('Applied to the form. Save to keep it.')) }} />
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
