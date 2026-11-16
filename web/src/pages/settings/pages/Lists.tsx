// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The pages of the editor that are lists: every page in force, the gallery of ready-made
// pages and sections, and the history with files in and out.

import { useEffect, useId, useMemo, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { date, t, tn, tx } from '../../../i18n'
import { IconMore, IconPlus } from '../../../icons'
import { forgetPages, bump, setPreview } from '../../../sdui/client'
import { sduiIcon } from '../../../sdui/render/icons'
import { ask, Button, Dialog, EmptyState, Field as Labelled, Input, LinkButton, Menu, Notice, Select, Switch, toast, type MenuEntry } from '../../../ui'
import { Section } from '../ui'
import {
  applyPreset, getDiff, getExport, getHistory, getNavigation, getPage, importDryRun, importFile, isConflict, issuesOf, message, removePage, rollback, savePage, validatePage,
  type DryRun, type Revision,
} from './api'
import { useEditor } from './context'
import { IssueNotes } from './fields'
import { download } from './guard'
import {
  SOURCE_HELP, SOURCE_WORDS, componentSpec, copyPage, describeChanges, editPath, exportName, foldDiff, libraryKindLabel, lineDiff, matchText, newPage, pageKindSpec, pageName, pretty,
  readExport, slug, tidyNavigation, tidyPage, uniqueId, validId, words,
  type Change, type Issue, type Match, type PageEntry, type Preset,
} from './editorModel.ts'

const sectionCount = (n: number) => tn('{count} section', '{count} sections', n)
const variantCount = (n: number) => tn('{count} variant', '{count} variants', n)

// ---------------------------------------------------------------- pages

export function PagesList() {
  const { cat, libraries, status, refresh, onConflict } = useEditor()
  const [query, setQuery] = useState('')
  const [making, setMaking] = useState<{ from?: PageEntry } | null>(null)
  const q = query.trim().toLowerCase()
  const shown = status.pages.filter((p) => !q || `${p.title} ${pageName(cat, p)} ${p.id} ${p.kind} ${SOURCE_WORDS[p.source] || ''} ${tx(SOURCE_WORDS[p.source])}`.toLowerCase().includes(q))
  const mine = shown.filter((p) => p.source !== 'builtin')
  const builtin = shown.filter((p) => p.source === 'builtin')

  const remove = async (p: PageEntry) => {
    const custom = p.source === 'custom'
    if (!await ask(custom
      ? { title: t('Delete “{name}”?', { name: pageName(cat, p) }), body: t('The page is taken out of the draft. Publishing the draft removes it for everyone; menu entries that open it need removing too.'), confirm: t('Delete the page'), danger: true }
      : { title: t('Reset “{name}” to how it comes?', { name: pageName(cat, p) }), body: t('Your version of the page is taken out of the draft, and the built-in page takes its place once you publish.'), confirm: t('Reset to default'), danger: true })) return
    try {
      await removePage(p.id, onConflict)
      await refresh()
      toast(custom ? t('The page was deleted from the draft') : t('The page is back to the built-in one, in the draft'))
    } catch (e) { toast(message(e), { tone: 'danger' }) }
  }
  const menu = (p: PageEntry): MenuEntry[] => {
    const items: MenuEntry[] = [
      { key: 'copy', label: t('Duplicate…'), onSelect: () => setMaking({ from: p }) },
      { key: 'export', label: t('Export'), hint: t('As a file'), onSelect: () => { getExport(p.id, true).then((d) => download(exportName(p.id), d)).catch((e) => toast(message(e), { tone: 'danger' })) } },
    ]
    if (p.source !== 'builtin') items.push({ kind: 'separator', key: 'sep' }, { key: 'reset', label: p.source === 'custom' ? t('Delete') : t('Reset to default'), danger: true, onSelect: () => void remove(p) })
    return items
  }
  const row = (p: PageEntry) => (
    <li key={p.id} className="pe-page">
      <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
      <div className="pe-page-text">
        <span className="pe-page-name"><Link to={editPath(p.id)}>{pageName(cat, p)}</Link><span className={'pe-source ' + p.source} title={tx(SOURCE_HELP[p.source])}>{tx(SOURCE_WORDS[p.source]) || p.source}</span></span>
        <small>{matchText(cat, p, libraries)}</small>
        <small className="pe-page-facts">{p.id} · {sectionCount(p.sections)}{p.variants ? ` · ${variantCount(p.variants)}` : ''}</small>
      </div>
      <LinkButton size="sm" to={editPath(p.id)} aria-label={t('Edit {name}', { name: pageName(cat, p) })}>{t('Edit')}</LinkButton>
      <Menu label={t('More for {name}', { name: pageName(cat, p) })} items={menu(p)} trigger={(tr) => <button type="button" className="btn ghost sm square" aria-label={t('More for {name}', { name: pageName(cat, p) })} {...tr}><IconMore /></button>} />
    </li>
  )

  return (
    <>
      <div className="pe-list-bar">
        <Input type="search" aria-label={t('Find a page')} placeholder={t('Find a page')} value={query} onChange={(e) => setQuery(e.target.value)} />
        <Button variant="primary" icon={<IconPlus size={16} />} onClick={() => setMaking({})}>{t('New page')}</Button>
      </div>
      {mine.length > 0 && (
        <Section title={t('Yours and changed')} desc={t('Pages you made, and built-in pages you changed. Resetting a changed page brings the built-in one back.')}>
          <ul className="pe-pages">{mine.map(row)}</ul>
        </Section>
      )}
      <Section title={t('Built-in pages')} desc={t('The pages the server comes with. Open one to change it for everyone; your version then takes its place.')}>
        {builtin.length ? <ul className="pe-pages">{builtin.map(row)}</ul> : <p className="pe-pane-empty">{q ? t('No built-in page matches.') : t('Every built-in page has been changed.')}</p>}
      </Section>
      {q && !shown.length && <EmptyState title={t('No page matches')} text={t('Try its title, its ID or its kind.')} icon={sduiIcon('search')} />}
      {making && <NewPage from={making.from} onClose={() => setMaking(null)} />}
    </>
  )
}

// A new page, or a copy of one: what it is called, what it is known by, and what it is for.
function NewPage({ from, onClose }: { from?: PageEntry; onClose: () => void }) {
  const { cat, libraries, status, refresh, onConflict } = useEditor()
  const navigate = useNavigate()
  const formId = useId()
  const taken = status.pages.map((p) => p.id)
  const [title, setTitle] = useState(from ? (from.title.includes('{') ? from.title : t('{title} (copy)', { title: from.title })) : '')
  const [id, setId] = useState(from ? uniqueId(`${from.id}-copy`, taken) : '')
  const [ownId, setOwnId] = useState(!!from)
  const [kind, setKind] = useState(from?.kind || 'custom')
  const [match, setMatch] = useState<Match>(from?.match || {})
  const [busy, setBusy] = useState(false)
  const [issues, setIssues] = useState<Issue[]>([])
  const spec = pageKindSpec(cat, kind)
  const fields = spec?.match || []
  const idNow = ownId ? id : title.trim() ? uniqueId(slug(title, 'page'), taken) : ''
  const idProblem = !idNow ? '' : !validId(idNow) ? t('Lower-case letters, digits, - and _, starting with a letter.') : taken.includes(idNow) ? t('There is already a page with this ID.') : ''
  const own = cat.pageKinds.find((k) => k.route.includes('{id}'))?.kind

  const create = async (e: FormEvent) => {
    e.preventDefault()
    if (!title.trim() || !idNow || idProblem) return
    setBusy(true)
    try {
      const kept: Match = {}
      if (fields.includes('library') && match.library) kept.library = match.library
      if (fields.includes('libraryKind') && match.libraryKind) kept.libraryKind = match.libraryKind
      if (fields.includes('mediaKind') && match.mediaKind) kept.mediaKind = match.mediaKind
      let page = newPage(idNow, title.trim(), kind, kept)
      if (from) {
        const src = (await getPage(from.id, true)).page
        page = { ...copyPage(src, idNow, title.trim()), kind }
        if (Object.keys(kept).length) page.match = kept; else delete page.match
      }
      page = tidyPage(page)
      const check = await validatePage(page)
      if (!check.ok) { setIssues(check.issues); return }
      await savePage(page, onConflict)
      await refresh()
      toast(t('“{title}” is in the draft', { title: page.title }))
      navigate(editPath(page.id))
    } catch (err) {
      const list = issuesOf(err)
      if (list) setIssues(list)
      else toast(isConflict(err) ? t('Not made. The draft changed in the meantime; try again.') : message(err), { tone: 'danger' })
    } finally { setBusy(false) }
  }

  const address = own && kind === own ? (spec?.route || '').replace('{id}', idNow || 'family-night') : ''
  return (
    <Dialog title={from ? t('Duplicate “{name}”', { name: pageName(cat, from) }) : t('New page')} onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" type="submit" form={formId} busy={busy} disabled={!title.trim() || !idNow || !!idProblem}>{from ? t('Make the copy') : t('Make the page')}</Button></>}>
      <form id={formId} className="ui-dialog-form" onSubmit={create}>
        <Labelled label={t('Title')} required hint={t('Shown at the top of the page and in the menu.')}>
          <Input value={title} maxLength={120} autoFocus onChange={(e) => setTitle(e.target.value)} />
        </Labelled>
        <Labelled label={t('ID')} required hint={address ? t('What the page is known by, and the end of its address ({address}). Lower-case letters, digits, - and _.', { address }) : t('What the page is known by, and the end of its address. Lower-case letters, digits, - and _.')} error={idProblem || undefined}>
          <Input value={idNow} spellCheck={false} autoCapitalize="none" onChange={(e) => { setOwnId(true); setId(e.target.value) }} />
        </Labelled>
        <Labelled label={t('Kind of page')} hint={spec ? tx(spec.description) + (own && kind !== own ? ' ' + t('Your page takes the place of the built-in one wherever it fits at least as well.') : '') : undefined}>
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            {cat.pageKinds.map((k) => <option key={k.kind} value={k.kind}>{tx(k.label)}</option>)}
          </Select>
        </Labelled>
        {fields.includes('library') && (
          <Labelled label={t('Only for the library')} hint={t('Leave on “Every library” for all of them.')}>
            <Select value={match.library ? String(match.library) : ''} onChange={(e) => setMatch({ ...match, library: Number(e.target.value) || undefined })}>
              <option value="">{t('Every library')}</option>
              {libraries.map((l) => <option key={l.id} value={String(l.id)}>{l.name}</option>)}
            </Select>
          </Labelled>
        )}
        {fields.includes('libraryKind') && (
          <Labelled label={t('Only for libraries of the kind')}>
            <Select value={match.libraryKind || ''} onChange={(e) => setMatch({ ...match, libraryKind: e.target.value || undefined })}>
              <option value="">{t('Every kind')}</option>
              {(cat.libraryKinds || []).map((k) => <option key={k} value={k}>{libraryKindLabel(cat, k)}</option>)}
            </Select>
          </Labelled>
        )}
        {fields.includes('mediaKind') && (
          <Labelled label={t('Only for titles of the kind')}>
            <Select value={match.mediaKind || ''} onChange={(e) => setMatch({ ...match, mediaKind: e.target.value || undefined })}>
              <option value="">{t('Every kind')}</option>
              {(cat.mediaKinds || []).map((k) => <option key={k} value={k}>{tx(words(k))}</option>)}
            </Select>
          </Labelled>
        )}
        <IssueNotes issues={issues.map((issue) => ({ issue, fix: null }))} />
      </form>
    </Dialog>
  )
}

// ---------------------------------------------------------------- presets

export function PresetsGallery({ presets }: { presets: Preset[] | null }) {
  const { cat, status, refresh, onConflict } = useEditor()
  const navigate = useNavigate()
  const [busy, setBusy] = useState('')
  const [target, setTarget] = useState<Record<string, string>>({})
  const pages = (presets || []).filter((p) => p.kind === 'page' && p.page)
  const sections = (presets || []).filter((p) => p.kind === 'section' && p.section)

  const use = async (p: Preset) => {
    const page = p.page!
    const there = status.pages.find((x) => x.id === page.id)
    if (!await ask({
      title: t('Use “{name}”?', { name: tx(p.label) }),
      body: there ? t('It goes into the draft as the page “{name}” ({id}), in place of what the draft has there now. Nobody sees it until you publish, and resetting the page brings the built-in one back.', { name: pageName(cat, there), id: page.id }) : t('It goes into the draft as a new page, “{title}” ({id}). Nobody sees it until you publish.', { title: page.title, id: page.id }),
      confirm: t('Put it in the draft'),
    })) return
    setBusy(p.id)
    try {
      await applyPreset(p.id, onConflict)
      await refresh()
      toast(t('“{name}” is in the draft', { name: tx(p.label) }))
      navigate(editPath(page.id))
    } catch (e) {
      toast(isConflict(e) ? t('Not done. The draft changed in the meantime; try again.') : message(e), { tone: 'danger' })
    } finally { setBusy('') }
  }

  if (!presets) return <p className="pe-pane-empty">{t('Loading the gallery…')}</p>
  return (
    <>
      <Section title={t('Whole pages')} desc={t('A page ready to use. It goes into the draft, where you can change it like any other before you publish.')}>
        <ul className="pe-presets">
          {pages.map((p) => (
            <li key={p.id} className="pe-preset">
              <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
              <div className="pe-page-text">
                <strong>{tx(p.label)}</strong>
                <small>{tx(p.description)}</small>
                <small className="pe-page-facts">{tx(pageKindSpec(cat, p.page!.kind)?.label) || p.page!.kind} · {t('becomes the page “{id}”', { id: p.page!.id })} · {sectionCount((p.page!.sections || []).length)}{p.page!.variants?.length ? ` · ${variantCount(p.page!.variants.length)}` : ''}</small>
              </div>
              <Button size="sm" busy={busy === p.id} disabled={!!busy} aria-label={t('Use {name}', { name: tx(p.label) })} onClick={() => void use(p)}>{t('Use')}</Button>
            </li>
          ))}
        </ul>
      </Section>
      <Section title={t('Sections')} desc={t('One section to drop into a page. Choose the page, and it is added at the end for you to move and change. They are also in the list when you add a section while editing a page.')}>
        <ul className="pe-presets">
          {sections.map((p) => {
            const fits = status.pages.filter((x) => !p.pageKinds?.length || p.pageKinds.includes(x.kind))
            const chosen = target[p.id] || ''
            return (
              <li key={p.id} className="pe-preset">
                <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
                <div className="pe-page-text">
                  <strong>{tx(p.label)}</strong>
                  <small>{tx(p.description)}</small>
                  <small className="pe-page-facts">{tx(componentSpec(cat, p.section!.component)?.label) || p.section!.component}{p.pageKinds?.length ? ' · ' + t('for {kinds} pages', { kinds: p.pageKinds.map((k) => (tx(pageKindSpec(cat, k)?.label) || k).toLowerCase()).join(', ') }) : ''}</small>
                </div>
                <span className="pe-preset-add">
                  <Select inputSize="sm" aria-label={t('The page to add {name} to', { name: tx(p.label) })} value={chosen} onChange={(e) => setTarget((tg) => ({ ...tg, [p.id]: e.target.value }))}>
                    <option value="">{t('Add to…')}</option>
                    {fits.map((x) => <option key={x.id} value={x.id}>{pageName(cat, x)} ({x.id})</option>)}
                  </Select>
                  <Button size="sm" disabled={!chosen} aria-label={t('Add {name} to the chosen page', { name: tx(p.label) })} onClick={() => navigate(`${editPath(chosen)}?preset=${encodeURIComponent(p.id)}`)}>{t('Add')}</Button>
                </span>
              </li>
            )
          })}
        </ul>
      </Section>
    </>
  )
}

// ---------------------------------------------------------------- history and files

const when = (iso?: string) => { const d = iso ? new Date(iso) : null; return d && !Number.isNaN(d.getTime()) ? date(d, { dateStyle: 'medium', timeStyle: 'short' }) : t('Not known') }

function Changes({ changes }: { changes: Change[] }) {
  const { cat, status } = useEditor()
  const byId = useMemo(() => new Map(status.pages.map((p) => [p.id, p])), [status.pages])
  const lines = describeChanges(changes, { titleOf: (id) => (byId.get(id) ? pageName(cat, byId.get(id)!) : id), isBuiltin: (id) => !!byId.get(id) && byId.get(id)!.source !== 'custom' })
  if (!lines.length) return <p className="pe-pane-empty">{t('Nothing differs between the two.')}</p>
  return <ul className="pe-changes">{lines.map((l) => <li key={l.title}>{l.title}{l.details.length > 0 && <ul>{l.details.map((d) => <li key={d}>{d}</li>)}</ul>}</li>)}</ul>
}

// What the draft would change, line by line: the one pair of versions whose full text the server gives out.
function DraftDetails({ changes }: { changes: Change[] }) {
  const [open, setOpen] = useState('')
  const [text, setText] = useState<{ was: string; now: string } | null>(null)
  const [failed, setFailed] = useState(false)
  const show = async (path: string) => {
    if (open === path) { setOpen(''); return }
    setOpen(path); setText(null); setFailed(false)
    try {
      if (path === '/navigation') {
        const [a, b] = await Promise.all([getNavigation(false), getNavigation(true)])
        setText({ was: pretty(tidyNavigation(a.navigation)), now: pretty(tidyNavigation(b.navigation)) })
      } else {
        const id = path.slice(7)
        const [a, b] = await Promise.all([getPage(id, false).catch(() => null), getPage(id, true).catch(() => null)])
        setText({ was: a ? pretty(tidyPage(a.page)) : '', now: b ? pretty(tidyPage(b.page)) : '' })
      }
    } catch { setFailed(true) }
  }
  const able = changes.filter((c) => c.path === '/navigation' || c.path.startsWith('/pages/'))
  if (!able.length) return null
  return (
    <div className="pe-diff-pick">
      <p className="hint">{t('Line by line, as JSON:')}</p>
      <div className="pe-row-actions">
        {able.map((c) => <Button key={c.path} size="sm" variant={open === c.path ? 'primary' : 'ghost'} aria-expanded={open === c.path} onClick={() => void show(c.path)}>{c.path === '/navigation' ? t('The menu') : c.path.slice(7)}</Button>)}
      </div>
      {open && failed && <p className="pe-pane-empty" role="alert">{t('That couldn’t be read just now.')}</p>}
      {open && text && (
        <pre className="pe-code pe-diff" tabIndex={0} aria-label={t('What is published, and what the draft makes of it. Lines the draft takes away begin with a minus, lines it adds with a plus.')}>
          {foldDiff(lineDiff(text.was, text.now)).map((l, i) => <span key={i} className={l.op === '+' ? 'add' : l.op === '-' ? 'del' : l.op === '…' ? 'gap' : undefined}>{l.op === '…' ? `   … ${l.text}` : `${l.op} ${l.text}`}{'\n'}</span>)}
        </pre>
      )}
    </div>
  )
}

export function HistoryAndFiles() {
  const { status, refresh, onConflict } = useEditor()
  const [history, setHistory] = useState<{ current: number; etag: string; keep: number; revisions: Revision[] } | null>(null)
  const [failed, setFailed] = useState(false)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('current')
  const [diff, setDiff] = useState<{ from: string; to: string; changes: Change[] | null } | null>(null)
  const [busy, setBusy] = useState('')

  const load = async () => {
    try {
      const h = await getHistory()
      setHistory(h); setFailed(false)
      // The usual question first: what did the last save change?
      setFrom((f) => f || String(h.revisions.find((r) => !r.current)?.revision || ''))
    } catch { setFailed(true) }
  }
  useEffect(() => { void load() }, [status.draft]) // eslint-disable-line react-hooks/exhaustive-deps

  const compare = async (a: string, b: string) => {
    setBusy('diff')
    try { setDiff({ from: a, to: b, changes: await getDiff(a, b) }) } catch (e) { toast(message(e), { tone: 'danger' }) } finally { setBusy('') }
  }
  const back = async (r: Revision) => {
    if (!history) return
    if (!await ask({ title: t('Go back to revision {revision}?', { revision: r.revision }), body: t('Everyone gets the pages and the menu as they were on {when}, straight away. It is saved as a new revision, so you can come back to how things are now.', { when: when(r.updatedAt) }), confirm: t('Go back to it'), danger: true })) return
    setBusy('back' + r.revision)
    try {
      await rollback(r.revision, history.etag)
      setPreview('')
      forgetPages(); bump()
      await Promise.all([load(), refresh()])
      toast(t('Back to revision {revision}. Everyone has it now.', { revision: r.revision }), { tone: 'ok' })
    } catch (e) {
      if (isConflict(e)) { await load(); toast(t('Someone else saved in between, so nothing was changed. The list is up to date now: look again, then go back if you still want to.'), { tone: 'warn' }) }
      else toast(message(e), { tone: 'danger' })
    } finally { setBusy('') }
  }
  const name = (v: string) => (v === 'current' ? t('what is published now') : v === 'draft' ? t('the draft') : t('revision {revision}', { revision: v }))
  const options = (
    <>
      {(history?.revisions || []).map((r) => <option key={r.revision} value={String(r.revision)}>{r.current ? t('Revision {revision} (published now), {when}', { revision: r.revision, when: when(r.updatedAt) }) : t('Revision {revision}, {when}', { revision: r.revision, when: when(r.updatedAt) })}</option>)}
      {status.draft && <option value="draft">{t('The draft')}</option>}
    </>
  )

  return (
    <>
      <Section title={t('History')} desc={t('Every publish is kept as a revision: the last {count} of them. Going back to one makes it what everyone gets again, as a new revision.', { count: history?.keep || 10 })}>
        {failed && <Notice tone="danger" action={<Button size="sm" onClick={() => void load()}>{t('Try again')}</Button>}>{t('The history couldn’t be read.')}</Notice>}
        {history && (
          <ul className="pe-revisions">
            {history.revisions.map((r) => (
              <li key={r.revision} className="pe-revision">
                <div className="pe-page-text">
                  <strong>{t('Revision {revision}', { revision: r.revision })}{r.current && <span className="pe-source changed">{t('Published now')}</span>}</strong>
                  <small>{t('{when} · by {who}', { when: when(r.updatedAt), who: r.updatedBy || t('someone') })}</small>
                  <small className="pe-page-facts">{r.pages?.length ? t('Holds your version of: {pages}', { pages: r.pages.join(', ') }) : t('Every page as it comes')}</small>
                </div>
                {!r.current && <Button size="sm" variant="ghost" aria-label={t('See what changed since revision {revision}', { revision: r.revision })} onClick={() => { setFrom(String(r.revision)); setTo('current'); void compare(String(r.revision), 'current') }}>{t('What changed since')}</Button>}
                {!r.current && <Button size="sm" busy={busy === 'back' + r.revision} disabled={!!busy} aria-label={t('Go back to revision {revision}', { revision: r.revision })} onClick={() => void back(r)}>{t('Go back to this')}</Button>}
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title={t('Compare')} desc={t('What differs between two revisions, or between what is published and the draft.')}>
        <div className="pe-compare">
          <Labelled label={t('From')}><Select value={from} onChange={(e) => setFrom(e.target.value)}><option value="">{t('Choose…')}</option>{options}</Select></Labelled>
          <Labelled label={t('To')}><Select value={to} onChange={(e) => setTo(e.target.value)}><option value="current">{t('What is published now')}</option>{options}</Select></Labelled>
          <Button busy={busy === 'diff'} disabled={!from || !!busy} onClick={() => void compare(from, to)}>{t('Compare')}</Button>
        </div>
        {diff && (
          <div className="pe-diff-result" role="status">
            <p className="hint">{t('From {from} to {to}:', { from: name(diff.from), to: name(diff.to) })}</p>
            {diff.changes === null ? <p className="pe-pane-empty">{t('One of the two isn’t kept any more.')}</p> : <Changes changes={diff.changes} />}
            {diff.changes && ((diff.from === 'current' || String(history?.current) === diff.from) && diff.to === 'draft'
              ? <DraftDetails changes={diff.changes} />
              : diff.changes.length > 0 && <p className="hint">{t('The server says what differs between old revisions, not their full text. Compare what is published with the draft to see every line.')}</p>)}
          </div>
        )}
      </Section>

      <Files onDone={async () => { await Promise.all([refresh(), load()]) }} onConflict={onConflict} />
    </>
  )
}

function Files({ onDone, onConflict }: { onDone: () => Promise<void>; onConflict: () => Promise<boolean> }) {
  const { cat, status } = useEditor()
  const picker = useRef<HTMLInputElement | null>(null)
  const [what, setWhat] = useState('')
  const [fromDraft, setFromDraft] = useState(true)
  const [file, setFile] = useState<{ kind: 'page' | 'config'; name: string; body: unknown } | null>(null)
  const [dry, setDry] = useState<DryRun | null>(null)
  const [theme, setTheme] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const out = () => {
    getExport(what || undefined, fromDraft && status.draft).then((d) => download(exportName(what || undefined), d)).catch((e) => toast(message(e), { tone: 'danger' }))
  }
  const check = async (f: NonNullable<typeof file>, withTheme: boolean) => {
    setBusy(true); setDry(null)
    try { setDry(await importDryRun(f.body, withTheme)) } catch (e) { setError(message(e)); setFile(null) } finally { setBusy(false) }
  }
  const chosen = async (list: FileList | null) => {
    const f = list?.[0]
    if (picker.current) picker.current.value = ''
    if (!f) return
    setError(''); setFile(null); setDry(null); setTheme(false)
    if (f.size > 16 * 1024 * 1024) { setError(t('That file is too large to be one of these.')); return }
    const got = readExport(await f.text())
    if ('error' in got) { setError(tx(got.error)); return }
    setFile(got)
    await check(got, false)
  }
  const bring = async () => {
    if (!file) return
    setBusy(true)
    try {
      await importFile(file.body, theme, onConflict)
      await onDone()
      toast(t('{name} is in the draft. Look it over, then publish.', { name: file.name }), { tone: 'ok' })
      setFile(null); setDry(null)
    } catch (e) {
      const list = issuesOf(e)
      if (list) setDry({ ok: false, issues: list, changes: dry?.changes || [] })
      else toast(isConflict(e) ? t('Not brought in. The draft changed in the meantime; try again.') : message(e), { tone: 'danger' })
    } finally { setBusy(false) }
  }

  return (
    <>
      <Section title={t('Export')} desc={t('One page, or every page and the menu, as a file to keep or to bring into another server.')}>
        <div className="pe-compare">
          <Labelled label={t('What')}>
            <Select value={what} onChange={(e) => setWhat(e.target.value)}>
              <option value="">{t('Everything: every changed page and the menu')}</option>
              {status.pages.map((p) => <option key={p.id} value={p.id}>{t('The page {name} ({id})', { name: pageName(cat, p), id: p.id })}</option>)}
            </Select>
          </Labelled>
          {status.draft && (
            <Labelled label={t('As it is in')}>
              <Select value={fromDraft ? 'draft' : 'published'} onChange={(e) => setFromDraft(e.target.value === 'draft')}>
                <option value="draft">{t('The draft')}</option>
                <option value="published">{t('What is published')}</option>
              </Select>
            </Labelled>
          )}
          <Button onClick={out}>{t('Export')}</Button>
        </div>
      </Section>

      <Section title={t('Import')} desc={t('A file that Export made, from this server or another. It is checked first and you are shown what it would change; then it goes into the draft, not straight to everyone.')}>
        <input ref={picker} type="file" accept="application/json,.json" hidden aria-label={t('A file exported from a server’s pages')} onChange={(e) => void chosen(e.target.files)} />
        <div className="pe-row-actions"><Button onClick={() => picker.current?.click()}>{t('Choose a file…')}</Button></div>
        {error && <Notice tone="danger">{error}</Notice>}
        {file && (
          <div className="pe-import" role="status">
            <p><strong>{file.name}</strong> · {file.kind === 'page' ? t('one page') : t('a whole configuration: its pages and its menu take the place of the draft’s')}</p>
            {busy && !dry && <p className="hint">{t('Checking it…')}</p>}
            {dry && (
              <>
                {dry.issues.length > 0
                  ? <IssueNotes issues={dry.issues.map((issue) => ({ issue: { ...issue, message: t('{where}: {problem}', { where: issue.path || t('The file'), problem: issue.message }) }, fix: null }))} />
                  : <Notice tone="ok">{t('Nothing wrong with it.')}</Notice>}
                <p className="hint">{t('What it would change in the draft:')}</p>
                <Changes changes={dry.changes} />
                {file.kind === 'config' && (
                  <div className="pe-bool-row">
                    <span className="ui-field-label" id="pe-import-theme">{t('Also bring its theme colours')}</span>
                    <Switch checked={theme} labelledBy="pe-import-theme" onChange={(v) => { setTheme(v); void check(file, v) }} />
                  </div>
                )}
                <div className="pe-row-actions">
                  <Button variant="ghost" onClick={() => { setFile(null); setDry(null) }}>{t('Not now')}</Button>
                  <Button variant="primary" busy={busy} disabled={!dry.ok} onClick={() => void bring()}>{t('Bring it into the draft')}</Button>
                </div>
                {!dry.ok && <p className="hint">{t('It can’t be brought in until its problems are put right in the file.')}</p>}
              </>
            )}
          </div>
        )}
      </Section>
    </>
  )
}
