// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The pages of the editor that are lists: every page in force, the gallery of ready-made
// pages and sections, and the history with files in and out.

import { useEffect, useId, useMemo, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
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

const plural = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`

// ---------------------------------------------------------------- pages

export function PagesList() {
  const { cat, libraries, status, refresh, onConflict } = useEditor()
  const [query, setQuery] = useState('')
  const [making, setMaking] = useState<{ from?: PageEntry } | null>(null)
  const q = query.trim().toLowerCase()
  const shown = status.pages.filter((p) => !q || `${p.title} ${pageName(cat, p)} ${p.id} ${p.kind} ${SOURCE_WORDS[p.source] || ''}`.toLowerCase().includes(q))
  const mine = shown.filter((p) => p.source !== 'builtin')
  const builtin = shown.filter((p) => p.source === 'builtin')

  const remove = async (p: PageEntry) => {
    const custom = p.source === 'custom'
    if (!await ask(custom
      ? { title: `Delete “${pageName(cat, p)}”?`, body: 'The page is taken out of the draft. Publishing the draft removes it for everyone; menu entries that open it need removing too.', confirm: 'Delete the page', danger: true }
      : { title: `Reset “${pageName(cat, p)}” to how it comes?`, body: 'Your version of the page is taken out of the draft, and the built-in page takes its place once you publish.', confirm: 'Reset to default', danger: true })) return
    try {
      await removePage(p.id, onConflict)
      await refresh()
      toast(custom ? 'The page was deleted from the draft' : 'The page is back to the built-in one, in the draft')
    } catch (e) { toast(message(e), { tone: 'danger' }) }
  }
  const menu = (p: PageEntry): MenuEntry[] => {
    const items: MenuEntry[] = [
      { key: 'copy', label: 'Duplicate…', onSelect: () => setMaking({ from: p }) },
      { key: 'export', label: 'Export', hint: 'As a file', onSelect: () => { getExport(p.id, true).then((d) => download(exportName(p.id), d)).catch((e) => toast(message(e), { tone: 'danger' })) } },
    ]
    if (p.source !== 'builtin') items.push({ kind: 'separator', key: 'sep' }, { key: 'reset', label: p.source === 'custom' ? 'Delete' : 'Reset to default', danger: true, onSelect: () => void remove(p) })
    return items
  }
  const row = (p: PageEntry) => (
    <li key={p.id} className="pe-page">
      <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
      <div className="pe-page-text">
        <span className="pe-page-name"><Link to={editPath(p.id)}>{pageName(cat, p)}</Link><span className={'pe-source ' + p.source} title={SOURCE_HELP[p.source]}>{SOURCE_WORDS[p.source] || p.source}</span></span>
        <small>{matchText(cat, p, libraries)}</small>
        <small className="pe-page-facts">{p.id} · {plural(p.sections, 'section')}{p.variants ? ` · ${plural(p.variants, 'variant')}` : ''}</small>
      </div>
      <LinkButton size="sm" to={editPath(p.id)} aria-label={`Edit ${pageName(cat, p)}`}>Edit</LinkButton>
      <Menu label={`More for ${pageName(cat, p)}`} items={menu(p)} trigger={(t) => <button type="button" className="btn ghost sm square" aria-label={`More for ${pageName(cat, p)}`} {...t}><IconMore /></button>} />
    </li>
  )

  return (
    <>
      <div className="pe-list-bar">
        <Input type="search" aria-label="Find a page" placeholder="Find a page" value={query} onChange={(e) => setQuery(e.target.value)} />
        <Button variant="primary" icon={<IconPlus size={16} />} onClick={() => setMaking({})}>New page</Button>
      </div>
      {mine.length > 0 && (
        <Section title="Yours and changed" desc="Pages you made, and built-in pages you changed. Resetting a changed page brings the built-in one back.">
          <ul className="pe-pages">{mine.map(row)}</ul>
        </Section>
      )}
      <Section title="Built-in pages" desc="The pages the server comes with. Open one to change it for everyone; your version then takes its place.">
        {builtin.length ? <ul className="pe-pages">{builtin.map(row)}</ul> : <p className="pe-pane-empty">{q ? 'No built-in page matches.' : 'Every built-in page has been changed.'}</p>}
      </Section>
      {q && !shown.length && <EmptyState title="No page matches" text="Try its title, its ID or its kind." icon={sduiIcon('search')} />}
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
  const [title, setTitle] = useState(from ? (from.title.includes('{') ? from.title : `${from.title} (copy)`) : '')
  const [id, setId] = useState(from ? uniqueId(`${from.id}-copy`, taken) : '')
  const [ownId, setOwnId] = useState(!!from)
  const [kind, setKind] = useState(from?.kind || 'custom')
  const [match, setMatch] = useState<Match>(from?.match || {})
  const [busy, setBusy] = useState(false)
  const [issues, setIssues] = useState<Issue[]>([])
  const spec = pageKindSpec(cat, kind)
  const fields = spec?.match || []
  const idNow = ownId ? id : title.trim() ? uniqueId(slug(title, 'page'), taken) : ''
  const idProblem = !idNow ? '' : !validId(idNow) ? 'Lower-case letters, digits, - and _, starting with a letter.' : taken.includes(idNow) ? 'There is already a page with this ID.' : ''
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
      toast(`“${page.title}” is in the draft`)
      navigate(editPath(page.id))
    } catch (err) {
      const list = issuesOf(err)
      if (list) setIssues(list)
      else toast(isConflict(err) ? 'Not made. The draft changed in the meantime; try again.' : message(err), { tone: 'danger' })
    } finally { setBusy(false) }
  }

  return (
    <Dialog title={from ? `Duplicate “${pageName(cat, from)}”` : 'New page'} onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form={formId} busy={busy} disabled={!title.trim() || !idNow || !!idProblem}>{from ? 'Make the copy' : 'Make the page'}</Button></>}>
      <form id={formId} className="ui-dialog-form" onSubmit={create}>
        <Labelled label="Title" required hint="Shown at the top of the page and in the menu.">
          <Input value={title} maxLength={120} autoFocus onChange={(e) => setTitle(e.target.value)} />
        </Labelled>
        <Labelled label="ID" required hint={`What the page is known by, and the end of its address${own && kind === own ? ` (${(spec?.route || '').replace('{id}', idNow || 'family-night')})` : ''}. Lower-case letters, digits, - and _.`} error={idProblem || undefined}>
          <Input value={idNow} spellCheck={false} autoCapitalize="none" onChange={(e) => { setOwnId(true); setId(e.target.value) }} />
        </Labelled>
        <Labelled label="Kind of page" hint={spec ? `${spec.description}${own && kind !== own ? ' Your page takes the place of the built-in one wherever it fits at least as well.' : ''}` : undefined}>
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            {cat.pageKinds.map((k) => <option key={k.kind} value={k.kind}>{k.label}</option>)}
          </Select>
        </Labelled>
        {fields.includes('library') && (
          <Labelled label="Only for the library" hint="Leave on “Every library” for all of them.">
            <Select value={match.library ? String(match.library) : ''} onChange={(e) => setMatch({ ...match, library: Number(e.target.value) || undefined })}>
              <option value="">Every library</option>
              {libraries.map((l) => <option key={l.id} value={String(l.id)}>{l.name}</option>)}
            </Select>
          </Labelled>
        )}
        {fields.includes('libraryKind') && (
          <Labelled label="Only for libraries of the kind">
            <Select value={match.libraryKind || ''} onChange={(e) => setMatch({ ...match, libraryKind: e.target.value || undefined })}>
              <option value="">Every kind</option>
              {(cat.libraryKinds || []).map((k) => <option key={k} value={k}>{libraryKindLabel(cat, k)}</option>)}
            </Select>
          </Labelled>
        )}
        {fields.includes('mediaKind') && (
          <Labelled label="Only for titles of the kind">
            <Select value={match.mediaKind || ''} onChange={(e) => setMatch({ ...match, mediaKind: e.target.value || undefined })}>
              <option value="">Every kind</option>
              {(cat.mediaKinds || []).map((k) => <option key={k} value={k}>{words(k)}</option>)}
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
      title: `Use “${p.label}”?`,
      body: there ? `It goes into the draft as the page “${pageName(cat, there)}” (${page.id}), in place of what the draft has there now. Nobody sees it until you publish, and resetting the page brings the built-in one back.` : `It goes into the draft as a new page, “${page.title}” (${page.id}). Nobody sees it until you publish.`,
      confirm: 'Put it in the draft',
    })) return
    setBusy(p.id)
    try {
      await applyPreset(p.id, onConflict)
      await refresh()
      toast(`“${p.label}” is in the draft`)
      navigate(editPath(page.id))
    } catch (e) {
      toast(isConflict(e) ? 'Not done. The draft changed in the meantime; try again.' : message(e), { tone: 'danger' })
    } finally { setBusy('') }
  }

  if (!presets) return <p className="pe-pane-empty">Loading the gallery…</p>
  return (
    <>
      <Section title="Whole pages" desc="A page ready to use. It goes into the draft, where you can change it like any other before you publish.">
        <ul className="pe-presets">
          {pages.map((p) => (
            <li key={p.id} className="pe-preset">
              <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
              <div className="pe-page-text">
                <strong>{p.label}</strong>
                <small>{p.description}</small>
                <small className="pe-page-facts">{pageKindSpec(cat, p.page!.kind)?.label || p.page!.kind} · becomes the page “{p.page!.id}” · {plural((p.page!.sections || []).length, 'section')}{p.page!.variants?.length ? ` · ${plural(p.page!.variants.length, 'variant')}` : ''}</small>
              </div>
              <Button size="sm" busy={busy === p.id} disabled={!!busy} aria-label={`Use ${p.label}`} onClick={() => void use(p)}>Use</Button>
            </li>
          ))}
        </ul>
      </Section>
      <Section title="Sections" desc="One section to drop into a page. Choose the page, and it is added at the end for you to move and change. They are also in the list when you add a section while editing a page.">
        <ul className="pe-presets">
          {sections.map((p) => {
            const fits = status.pages.filter((x) => !p.pageKinds?.length || p.pageKinds.includes(x.kind))
            const chosen = target[p.id] || ''
            return (
              <li key={p.id} className="pe-preset">
                <span className="pe-page-icon" aria-hidden="true">{sduiIcon(p.icon) || sduiIcon('grid')}</span>
                <div className="pe-page-text">
                  <strong>{p.label}</strong>
                  <small>{p.description}</small>
                  <small className="pe-page-facts">{componentSpec(cat, p.section!.component)?.label || p.section!.component}{p.pageKinds?.length ? ` · for ${p.pageKinds.map((k) => (pageKindSpec(cat, k)?.label || k).toLowerCase()).join(', ')} pages` : ''}</small>
                </div>
                <span className="pe-preset-add">
                  <Select inputSize="sm" aria-label={`The page to add ${p.label} to`} value={chosen} onChange={(e) => setTarget((t) => ({ ...t, [p.id]: e.target.value }))}>
                    <option value="">Add to…</option>
                    {fits.map((x) => <option key={x.id} value={x.id}>{pageName(cat, x)} ({x.id})</option>)}
                  </Select>
                  <Button size="sm" disabled={!chosen} aria-label={`Add ${p.label} to the chosen page`} onClick={() => navigate(`${editPath(chosen)}?preset=${encodeURIComponent(p.id)}`)}>Add</Button>
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

const when = (iso?: string) => { const d = iso ? new Date(iso) : null; return d && !Number.isNaN(d.getTime()) ? d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : 'Not known' }

function Changes({ changes }: { changes: Change[] }) {
  const { cat, status } = useEditor()
  const byId = useMemo(() => new Map(status.pages.map((p) => [p.id, p])), [status.pages])
  const lines = describeChanges(changes, { titleOf: (id) => (byId.get(id) ? pageName(cat, byId.get(id)!) : id), isBuiltin: (id) => !!byId.get(id) && byId.get(id)!.source !== 'custom' })
  if (!lines.length) return <p className="pe-pane-empty">Nothing differs between the two.</p>
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
      <p className="hint">Line by line, as JSON:</p>
      <div className="pe-row-actions">
        {able.map((c) => <Button key={c.path} size="sm" variant={open === c.path ? 'primary' : 'ghost'} aria-expanded={open === c.path} onClick={() => void show(c.path)}>{c.path === '/navigation' ? 'The menu' : c.path.slice(7)}</Button>)}
      </div>
      {open && failed && <p className="pe-pane-empty" role="alert">That couldn’t be read just now.</p>}
      {open && text && (
        <pre className="pe-code pe-diff" tabIndex={0} aria-label="What is published, and what the draft makes of it. Lines the draft takes away begin with a minus, lines it adds with a plus.">
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
    if (!await ask({ title: `Go back to revision ${r.revision}?`, body: `Everyone gets the pages and the menu as they were on ${when(r.updatedAt)}, straight away. It is saved as a new revision, so you can come back to how things are now.`, confirm: 'Go back to it', danger: true })) return
    setBusy('back' + r.revision)
    try {
      await rollback(r.revision, history.etag)
      setPreview('')
      forgetPages(); bump()
      await Promise.all([load(), refresh()])
      toast(`Back to revision ${r.revision}. Everyone has it now.`, { tone: 'ok' })
    } catch (e) {
      if (isConflict(e)) { await load(); toast('Someone else saved in between, so nothing was changed. The list is up to date now: look again, then go back if you still want to.', { tone: 'warn' }) }
      else toast(message(e), { tone: 'danger' })
    } finally { setBusy('') }
  }
  const name = (v: string) => (v === 'current' ? 'what is published now' : v === 'draft' ? 'the draft' : `revision ${v}`)
  const options = (
    <>
      {(history?.revisions || []).map((r) => <option key={r.revision} value={String(r.revision)}>Revision {r.revision}{r.current ? ' (published now)' : ''}, {when(r.updatedAt)}</option>)}
      {status.draft && <option value="draft">The draft</option>}
    </>
  )

  return (
    <>
      <Section title="History" desc={`Every publish is kept as a revision: the last ${history?.keep || 10} of them. Going back to one makes it what everyone gets again, as a new revision.`}>
        {failed && <Notice tone="danger" action={<Button size="sm" onClick={() => void load()}>Try again</Button>}>The history couldn’t be read.</Notice>}
        {history && (
          <ul className="pe-revisions">
            {history.revisions.map((r) => (
              <li key={r.revision} className="pe-revision">
                <div className="pe-page-text">
                  <strong>Revision {r.revision}{r.current && <span className="pe-source changed">Published now</span>}</strong>
                  <small>{when(r.updatedAt)} · by {r.updatedBy || 'someone'}</small>
                  <small className="pe-page-facts">{r.pages?.length ? `Holds your version of: ${r.pages.join(', ')}` : 'Every page as it comes'}</small>
                </div>
                {!r.current && <Button size="sm" variant="ghost" aria-label={`See what changed since revision ${r.revision}`} onClick={() => { setFrom(String(r.revision)); setTo('current'); void compare(String(r.revision), 'current') }}>What changed since</Button>}
                {!r.current && <Button size="sm" busy={busy === 'back' + r.revision} disabled={!!busy} aria-label={`Go back to revision ${r.revision}`} onClick={() => void back(r)}>Go back to this</Button>}
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="Compare" desc="What differs between two revisions, or between what is published and the draft.">
        <div className="pe-compare">
          <Labelled label="From"><Select value={from} onChange={(e) => setFrom(e.target.value)}><option value="">Choose…</option>{options}</Select></Labelled>
          <Labelled label="To"><Select value={to} onChange={(e) => setTo(e.target.value)}><option value="current">What is published now</option>{options}</Select></Labelled>
          <Button busy={busy === 'diff'} disabled={!from || !!busy} onClick={() => void compare(from, to)}>Compare</Button>
        </div>
        {diff && (
          <div className="pe-diff-result" role="status">
            <p className="hint">From {name(diff.from)} to {name(diff.to)}:</p>
            {diff.changes === null ? <p className="pe-pane-empty">One of the two isn’t kept any more.</p> : <Changes changes={diff.changes} />}
            {diff.changes && ((diff.from === 'current' || String(history?.current) === diff.from) && diff.to === 'draft'
              ? <DraftDetails changes={diff.changes} />
              : diff.changes.length > 0 && <p className="hint">The server says what differs between old revisions, not their full text. Compare what is published with the draft to see every line.</p>)}
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
    if (f.size > 16 * 1024 * 1024) { setError('That file is too large to be one of these.'); return }
    const got = readExport(await f.text())
    if ('error' in got) { setError(got.error); return }
    setFile(got)
    await check(got, false)
  }
  const bring = async () => {
    if (!file) return
    setBusy(true)
    try {
      await importFile(file.body, theme, onConflict)
      await onDone()
      toast(`${file.name} is in the draft. Look it over, then publish.`, { tone: 'ok' })
      setFile(null); setDry(null)
    } catch (e) {
      const list = issuesOf(e)
      if (list) setDry({ ok: false, issues: list, changes: dry?.changes || [] })
      else toast(isConflict(e) ? 'Not brought in. The draft changed in the meantime; try again.' : message(e), { tone: 'danger' })
    } finally { setBusy(false) }
  }

  return (
    <>
      <Section title="Export" desc="One page, or every page and the menu, as a file to keep or to bring into another server.">
        <div className="pe-compare">
          <Labelled label="What">
            <Select value={what} onChange={(e) => setWhat(e.target.value)}>
              <option value="">Everything: every changed page and the menu</option>
              {status.pages.map((p) => <option key={p.id} value={p.id}>The page {pageName(cat, p)} ({p.id})</option>)}
            </Select>
          </Labelled>
          {status.draft && (
            <Labelled label="As it is in">
              <Select value={fromDraft ? 'draft' : 'published'} onChange={(e) => setFromDraft(e.target.value === 'draft')}>
                <option value="draft">The draft</option>
                <option value="published">What is published</option>
              </Select>
            </Labelled>
          )}
          <Button onClick={out}>Export</Button>
        </div>
      </Section>

      <Section title="Import" desc="A file that Export made, from this server or another. It is checked first and you are shown what it would change; then it goes into the draft, not straight to everyone.">
        <input ref={picker} type="file" accept="application/json,.json" hidden aria-label="A file exported from a server’s pages" onChange={(e) => void chosen(e.target.files)} />
        <div className="pe-row-actions"><Button onClick={() => picker.current?.click()}>Choose a file…</Button></div>
        {error && <Notice tone="danger">{error}</Notice>}
        {file && (
          <div className="pe-import" role="status">
            <p><strong>{file.name}</strong> · {file.kind === 'page' ? 'one page' : 'a whole configuration: its pages and its menu take the place of the draft’s'}</p>
            {busy && !dry && <p className="hint">Checking it…</p>}
            {dry && (
              <>
                {dry.issues.length > 0
                  ? <IssueNotes issues={dry.issues.map((issue) => ({ issue: { ...issue, message: `${issue.path || 'The file'}: ${issue.message}` }, fix: null }))} />
                  : <Notice tone="ok">Nothing wrong with it.</Notice>}
                <p className="hint">What it would change in the draft:</p>
                <Changes changes={dry.changes} />
                {file.kind === 'config' && (
                  <div className="pe-bool-row">
                    <span className="ui-field-label" id="pe-import-theme">Also bring its theme colours</span>
                    <Switch checked={theme} labelledBy="pe-import-theme" onChange={(v) => { setTheme(v); void check(file, v) }} />
                  </div>
                )}
                <div className="pe-row-actions">
                  <Button variant="ghost" onClick={() => { setFile(null); setDry(null) }}>Not now</Button>
                  <Button variant="primary" busy={busy} disabled={!dry.ok} onClick={() => void bring()}>Bring it into the draft</Button>
                </div>
                {!dry.ok && <p className="hint">It can’t be brought in until its problems are put right in the file.</p>}
              </>
            )}
          </div>
        )}
      </Section>
    </>
  )
}
