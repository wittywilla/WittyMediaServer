// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { PageHelp } from '../components/HelpLink'
import { Link, useSearchParams } from 'react-router-dom'
import { api, artPreviewUrl } from '../api'
import { announce } from '../a11y'
import EditDetails from '../components/edit/EditDetails'
import { ConfidenceMeter, PreviewTable, sourceName } from '../components/edit/MatchPanel'
import { EmptyState, LoadError, Spinner } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconCheck, IconPlus, IconTrash } from '../icons'
import { Button, Card, Dialog, IconButton, Select, Tabs } from '../ui'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { detailSpec, tabsSpec, toolsSpec } from '../spec/specs/fixMatches'

import { undoBatch, type EpisodeMap, type EpisodeRow, type HistoryEntry, type MatchCandidate, type MatchPreview, type MatchRule, type QueueResult, type QueueRow, type RuleTry, type SearchResult, type Target } from '../metadata'
import { toast, useApp } from '../state'
import { date, t, tk, tn, tx, bidi } from '../i18n'
import BulkEdit from './library/BulkEdit'
import './library/library.css'
import './fixmatches.css'

// Fix matches: everything that needs a look, worst first, with suggestions and reasons, and a
// key for each thing you'd do. Nothing here asks anyone anything until you open a title.

const TABS: [string, string][] = [['review', tk('Needs a look')], ['unmatched', tk('No match')], ['low', tk('Not sure')], ['ok', tk('Looks right')], ['manual', tk('You chose')], ['none', tk('Left alone')], ['all', tk('Everything')], ['removed', tk('Removed')]]

const STATE_WORDS: Record<QueueRow['state'], string> = { unmatched: tk('No match'), low: tk('Not sure'), ok: tk('Looks right'), manual: tk('You chose this'), none: tk('Left alone') }

export default function FixMatches() {
  const { user, libraries } = useApp()
  const [params, setParams] = useSearchParams()
  const page = params.get('page') || 'review'
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const matchable = libraries.filter((l) => ['movies', 'discs', 'shows', 'anime'].includes(l.type))
  if (!user?.isAdmin) return <EmptyState title={t('This page is for administrators')} />
  return (
    <div className="fm">
      <div className="toolbar">
        <h1>{t('Fix matches')}</h1>
        <Tabs label={t('Fix matches')} value={page} onChange={(k) => set('page', k === 'review' ? '' : k)} items={[{ value: 'review', label: t('Review') }, { value: 'rules', label: t('Naming rules') }, { value: 'history', label: t('History') }]} />
        <div className="grow" />
        <PageHelp to="fixing-metadata" />
        <Select aria-label={t('Library')} value={params.get('library') || ''} onChange={(e) => set('library', e.target.value)}
          options={[{ value: '', label: page === 'rules' ? t('Choose a library') : t('All libraries') }, ...matchable.map((l) => ({ value: String(l.id), label: l.name }))]} />
      </div>
      {page === 'rules' ? <RulesPage library={Number(params.get('library') || 0)} />
        : page === 'history' ? <HistoryPage />
        : <ReviewPage library={Number(params.get('library') || 0)} tab={params.get('tab') || 'review'} setTab={(t) => set('tab', t === 'review' ? '' : t)} />}
    </div>
  )
}

function ReviewPage({ library, tab, setTab }: { library: number; tab: string; setTab: (t: string) => void }) {
  const [q, setQ] = useState<QueueResult | null>(null)
  const [failed, setFailed] = useState(false)
  const [sel, setSel] = useState(0)
  const [checked, setChecked] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [lastUndo, setLastUndo] = useState<{ batch: string; what: string } | null>(null)
  const [job, setJob] = useState<{ running: boolean; done: number; total: number; current?: string } | null>(null)
  const [editing, setEditing] = useState<QueueRow | null>(null)
  const [bulk, setBulk] = useState(false)
  const [renumber, setRenumber] = useState<QueueRow | null>(null)
  const [tick, setTick] = useState(0)
  const list = useRef<HTMLUListElement | null>(null)
  const detail = useRef<HTMLDivElement | null>(null)
  const load = useCallback(() => {
    setFailed(false)
    return api.get<QueueResult>(`/match/queue?library=${library}&state=${tab}&search=${encodeURIComponent(search)}&limit=500`).then(setQ).catch(() => setFailed(true))
  }, [library, tab, search])
  useEffect(() => { if (tab !== 'removed') load() }, [load, tab, tick])
  useEffect(() => { setSel(0); setChecked(new Set()) }, [library, tab])
  // While suggestions are being found, and only then, look in on them.
  useEffect(() => {
    if (!job?.running) return
    const timer = setTimeout(() => api.get<NonNullable<typeof job>>('/match/suggest-all').then((j) => {
      setJob(j)
      if (j.running) return
      if (j.total === 0) toast(t('They all have suggestions already'))
      setTick((n) => n + 1)
    }).catch(() => {}), 2000)
    return () => clearTimeout(timer)
  }, [job])
  useEffect(() => { api.get<NonNullable<typeof job>>('/match/suggest-all').then((j) => { if (j.running) setJob(j) }).catch(() => {}) }, [])

  const rows = useMemo(() => q?.rows ?? [], [q])
  const cur = rows[Math.min(sel, rows.length - 1)]
  const keyOf = (r: QueueRow) => r.target + r.id
  const decided = (batch: string, what: string) => {
    setLastUndo({ batch, what })
    announce(what)
    setTick((n) => n + 1)
  }
  const accept = useCallback(async (r: QueueRow, c: MatchCandidate) => {
    try {
      const out = await api.post<{ batches: string[]; failed: string[] }>('/match/accept', { choices: [{ target: r.target, id: r.id, source: c.source, sourceId: c.id }] })
      if (out.failed.length) { toast(out.failed[0]); return }
      decided(out.batches[0], t('{title}: matched with {match}.', { title: r.title, match: c.title }))
    } catch (e: any) { toast(e.message) }
  }, [])
  const leave = useCallback(async (r: QueueRow, keep: boolean) => {
    try {
      const out = await api.post<{ batch: string }>(`/meta/${r.target}/${r.id}/leave`, { keep })
      decided(out.batch, keep ? t('{title}: kept as it is.', { title: r.title }) : t('{title}: left with no match.', { title: r.title }))
    } catch (e: any) { toast(e.message) }
  }, [])
  const undo = useCallback(async () => {
    if (!lastUndo) return
    try { await undoBatch(lastUndo.batch); setLastUndo(null); announce(t('Undone')); toast(t('Undone')); setTick((n) => n + 1) } catch (e: any) { toast(e.message) }
  }, [lastUndo])

  const [topFor, setTopFor] = useState<Record<string, MatchCandidate[]>>({})
  const top = cur ? (topFor[keyOf(cur)] ?? cur.suggestions ?? null) : null

  // One key for each thing you'd do. Only when typing isn't what's going on.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t.matches('input, textarea, select, [contenteditable=true]') || e.ctrlKey || e.metaKey || e.altKey) return
      if (document.querySelector('.modal-backdrop')) return
      if (!cur) return
      const move = (d: number) => {
        e.preventDefault()
        const n = Math.max(0, Math.min(rows.length - 1, sel + d))
        setSel(n)
        list.current?.querySelectorAll<HTMLElement>('.fm-row')[n]?.scrollIntoView({ block: 'nearest' })
      }
      switch (e.key) {
        case 'j': return move(1)
        case 'k': return move(-1)
        case 'a': if (top?.[0]) { e.preventDefault(); accept(cur, top[0]) } return
        case 'x': e.preventDefault(); setChecked((c) => { const n = new Set(c); const k = keyOf(cur); if (n.has(k)) n.delete(k); else n.add(k); return n }); return
        case 'n': e.preventDefault(); leave(cur, false); return
        case 'e': e.preventDefault(); setEditing(cur); return
        case 'u': if (lastUndo) { e.preventDefault(); undo() } return
        case '/': e.preventDefault(); detail.current?.querySelector<HTMLInputElement>('input')?.focus(); return
      }
      if (/^[1-6]$/.test(e.key) && top?.[Number(e.key) - 1]) { e.preventDefault(); accept(cur, top[Number(e.key) - 1]) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [cur, rows.length, sel, top, lastUndo, accept, leave, undo])

  const chosen = rows.filter((r) => checked.has(keyOf(r)))
  const acceptChosen = async () => {
    const choices = chosen.flatMap((r) => { const c = (topFor[keyOf(r)] ?? r.suggestions ?? [])[0]; return c ? [{ target: r.target, id: r.id, source: c.source, sourceId: c.id }] : [] })
    if (!choices.length) { toast(t('None of those has a suggestion yet')); return }
    try {
      const out = await api.post<{ done: number; batches: string[]; failed: string[] }>('/match/accept', { choices })
      setChecked(new Set())
      toast(out.failed.length ? t('Matched {done}, {failed} couldn’t be', { done: out.done, failed: out.failed.length }) : t('Matched {done}', { done: out.done }))
      setTick((n) => n + 1)
    } catch (e: any) { toast(e.message) }
  }
  const sure = rows.filter((r) => (r.suggestions?.[0]?.score ?? 0) >= 90)
  const tabsPage = useMemo(() => tabsSpec(TABS.map(([k, l]) => [k, t(l)] as [string, string]), q?.counts, tab), [q, tab])
  const toolsPage = useMemo(() => toolsSpec(search, job, sure.length), [search, job, sure.length])
  return (
    <>
      <SpecPage embedded className="fm-tabs" page={tabsPage} handled={handledIds(tabsPage)} onAction={(id) => setTab(id.slice(4))} />
      {tab === 'removed' ? <RemovedList library={library} /> : (
        <>
          <SpecPage embedded className="fm-actions" page={toolsPage} handled={handledIds(toolsPage)} onField={(_, v) => setSearch(String(v))}
            onAction={(id) => {
              if (id === 'stop') api.del('/match/suggest-all').then(() => setJob(null))
              else if (id === 'find') api.post<{ job: NonNullable<typeof job> }>('/match/suggest-all', { library }).then((r) => setJob(r.job)).catch((e) => toast(e.message))
              else if (id === 'sure') setChecked(new Set(sure.map(keyOf)))
            }} />
          {!q ? (failed ? <LoadError what={t('The list')} onRetry={load} /> : <Spinner />) : rows.length === 0 ? (
            <EmptyState icon={<IconCheck />} title={tab === 'review' ? t('Nothing needs a look') : t('Nothing here')} text={tab === 'review' ? t('Everything here is matched, and matched well. A new scan may add more.') : undefined} />
          ) : (
            <div className="fm-split">
              <ul className="fm-list" ref={list} aria-label={t('Titles')} data-nav-group>
                {rows.map((r, i) => {
                  const on = i === Math.min(sel, rows.length - 1)
                  return (
                    <li key={keyOf(r)} className={'fm-row' + (on ? ' on' : '')}>
                      <label className="fm-row-tick"><input type="checkbox" aria-label={t('Choose {title}', { title: r.title })} checked={checked.has(keyOf(r))} onChange={() => setChecked((c) => { const n = new Set(c); if (n.has(keyOf(r))) n.delete(keyOf(r)); else n.add(keyOf(r)); return n })} /></label>
                      <button type="button" className="fm-row-main" aria-current={on}
                        // Moving along the list with a remote or the keyboard shows each title as it goes.
                        onFocus={() => setSel(i)}
                        onClick={(e) => {
                          setSel(i)
                          requestAnimationFrame(() => {
                            detail.current?.scrollIntoView({ block: 'nearest' })
                            // OK on a remote or Enter moves on to what can be done with it.
                            if (e.detail === 0) detail.current?.querySelector<HTMLElement>('.fm-sugg .btn, button')?.focus()
                          })
                        }}>
                        <span className="fm-row-pic">{r.poster ? <img src={artPreviewUrl(r.poster)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : null}</span>
                        <span className="fm-row-text">
                          <b>{r.title}{r.year ? ` (${r.year})` : ''}</b>
                          <span className="muted">{r.file}</span>
                          <span className="fm-row-state"><ConfidenceMeter value={r.confidence} label={tx(STATE_WORDS[r.state])} /> <span className="muted">{tx(STATE_WORDS[r.state])}</span></span>
                        </span>
                      </button>
                    </li>
                  )
                })}
                {q.total > rows.length && <li className="muted fm-more">{t('The worst {shown} of {total}. Fix these and the rest move up.', { shown: rows.length, total: q.total })}</li>}
              </ul>
              {cur && <Detail key={keyOf(cur)} ref_={detail} r={cur} topFor={topFor} setTopFor={setTopFor} onAccept={accept} onLeave={leave} onEdit={() => setEditing(cur)} onRenumber={() => setRenumber(cur)} onDecided={decided} />}
            </div>
          )}
          <p className="muted fm-keys">{t('Keys:')} <kbd>J</kbd> <kbd>K</kbd> {t('next and last')}, <kbd>A</kbd> {t('accept the first suggestion')}, <kbd>1</kbd>–<kbd>6</kbd> {t('accept that one')}, <kbd>N</kbd> {t('no match')}, <kbd>E</kbd> {t('edit')}, <kbd>X</kbd> {t('choose')}, <kbd>/</kbd> {t('search')}, <kbd>U</kbd> {t('undo')}.</p>
          {(lastUndo || chosen.length > 0) && (
            <div className="lib-bulk fm-bar" role="region" aria-label={t('Actions')}>
              {chosen.length > 0 && <>
                <span>{tn('{count} chosen', '{count} chosen', chosen.length)}</span>
                <Button variant="primary" onClick={acceptChosen}>{t('Accept their best suggestions')}</Button>
                <Button onClick={() => setBulk(true)}>{t('Edit together')}</Button>
                <Button variant="ghost" onClick={() => setChecked(new Set())}>{t('Clear')}</Button>
              </>}
              {lastUndo && <>
                <span className="fm-bar-what">{lastUndo.what}</span>
                <Button onClick={undo}>{t('Undo')}</Button>
              </>}
            </div>
          )}
        </>
      )}
      <LegalNote kind="webResults" />
      {editing && <EditDetails target={editing.target as Target} id={editing.id} onClose={() => setEditing(null)} onChanged={() => setTick((n) => n + 1)} />}
      {bulk && <BulkEdit titles={chosen.map((r) => ({ target: r.target, id: r.id, title: r.title }))} onClose={() => setBulk(false)} onDone={() => { setBulk(false); setChecked(new Set()); setTick((n) => n + 1) }} />}
      {renumber && <RenumberDialog showId={renumber.id} title={renumber.title} onClose={() => setRenumber(null)} onDone={(batch) => { setRenumber(null); decided(batch, t('{title}: renumbered.', { title: renumber.title })) }} />}
    </>
  )
}

function Detail({ r, ref_, topFor, setTopFor, onAccept, onLeave, onEdit, onRenumber, onDecided }: {
  r: QueueRow; ref_: React.RefObject<HTMLDivElement | null>; topFor: Record<string, MatchCandidate[]>; setTopFor: (f: (m: Record<string, MatchCandidate[]>) => Record<string, MatchCandidate[]>) => void
  onAccept: (r: QueueRow, c: MatchCandidate) => void; onLeave: (r: QueueRow, keep: boolean) => void; onEdit: () => void; onRenumber: () => void; onDecided: (batch: string, what: string) => void
}) {
  const k = r.target + r.id
  const known = topFor[k] ?? r.suggestions ?? null
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult | null>(null)
  const [preview, setPreview] = useState<{ p: MatchPreview; c: MatchCandidate } | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [err, setErr] = useState('')
  // Suggestions are asked for when a title is opened, and kept.
  useEffect(() => {
    if (known || r.state === 'manual' || r.state === 'none') return
    let on = true
    setBusy(true); setErr('')
    // Going down the list with a remote passes over titles: only one it stays on asks anybody.
    const t = setTimeout(() => {
      api.post<{ suggestions: MatchCandidate[] }>(`/meta/${r.target}/${r.id}/suggest`)
        .then((out) => { if (on) setTopFor((m) => ({ ...m, [k]: out.suggestions })) })
        .catch((e) => { if (on) setErr(e.message) })
        .finally(() => { if (on) setBusy(false) })
    }, 400)
    return () => { on = false; clearTimeout(t) }
    // Once for each title.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [k])
  const search = async () => {
    setBusy(true); setErr(''); setPreview(null)
    try { setResults(await api.post<SearchResult>(`/meta/${r.target}/${r.id}/search`, { query })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const look = async (c: MatchCandidate) => {
    setBusy(true); setErr('')
    try {
      const p = await api.post<MatchPreview>(`/meta/${r.target}/${r.id}/preview`, { source: c.source, id: c.id })
      setPreview({ p, c })
      setPicked(new Set(p.changes.filter((x) => x.apply).map((x) => x.field)))
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const apply = async () => {
    if (!preview) return
    try {
      const out = await api.post<{ batch: string }>(`/meta/${r.target}/${r.id}/match`, { source: preview.p.source, id: preview.p.id, fields: [...picked] })
      onDecided(out.batch, t('{title}: matched with {match}.', { title: r.title, match: preview.p.title }))
    } catch (e: any) { setErr(e.message) }
  }
  const list = results?.results ?? known ?? []
  const dpage = detailSpec({
    state: r.state, target: r.target, reasons: r.reasons, matchedWith: r.provider ? r.sourceLabel : undefined, matchedUrl: r.provider ? r.url : undefined,
    query, busy, err, warnings: results?.warnings ?? [], results: !!results,
    list: list.map((c) => ({ title: c.title, year: c.year, kind: c.kind, rating: c.rating, score: c.score, source: c.source, sourceName: sourceName(c.source) })),
  })
  const press = (id: string) => {
    if (id === 'edit') onEdit()
    else if (id === 'renumber') onRenumber()
    else if (id === 'leave') onLeave(r, r.state === 'low')
    else if (id === 'go') search()
    else if (id.startsWith('look:')) look(list[Number(id.slice(5))])
    else if (id.startsWith('accept:')) onAccept(r, list[Number(id.slice(7))])
  }
  return (
    <div className="fm-detail" ref={ref_} aria-label={t('Fixing {title}', { title: r.title })} role="region">
      <header className="fm-detail-head">
        <div>
          <h2>{r.title}{r.year ? ` (${r.year})` : ''}</h2>
          <p className="muted fm-file" title={r.file}>{r.file}</p>
        </div>
      </header>
      <div onKeyDown={(e) => { if (e.key === 'Enter' && (e.target as HTMLElement).matches('input[type=text], input:not([type])')) { e.preventDefault(); search() } }}>
        {!preview && <SpecPage bare className="fm-spec" page={dpage} handled={handledIds(dpage)} onField={(_, v) => setQuery(String(v))} onAction={press} />}
      </div>
      {busy && <Spinner />}
      {preview && <PreviewTable p={preview.p} picked={picked} setPicked={setPicked} onApply={apply} onBack={() => setPreview(null)} />}
    </div>
  )
}

function RemovedList({ library }: { library: number }) {
  const [list, setList] = useState<{ path: string; title: string; libraryId: number; at: number }[] | null>(null)
  const load = useCallback(() => { api.get<{ removed: { path: string; title: string; libraryId: number; at: number }[] }>(`/meta/removed?library=${library}`).then((r) => setList(r.removed)).catch(() => setList([])) }, [library])
  useEffect(load, [load])
  if (!list) return <Spinner />
  if (list.length === 0) return <EmptyState icon={<IconTrash />} title={t('Nothing has been taken out')} text={t('Titles taken out of a library wait here. Their files stay on disk.')} />
  return (
    <ul className="fm-simple">
      {list.map((r) => (
        <li key={r.path}>
          <div><b>{r.title}</b><div className="muted fm-file">{r.path}</div></div>
          <Button onClick={() => api.post('/meta/removed/restore', { path: r.path }).then(() => { toast(t('Back in its library')); load() }).catch((e) => toast(e.message))}>{t('Bring it back')}</Button>
        </li>
      ))}
    </ul>
  )
}

function HistoryPage() {
  const [list, setList] = useState<HistoryEntry[] | null>(null)
  const load = useCallback(() => { api.get<{ history: HistoryEntry[] }>('/meta/history?limit=100').then((r) => setList(r.history)).catch(() => setList([])) }, [])
  useEffect(load, [load])
  if (!list) return <Spinner />
  if (list.length === 0) return <EmptyState title={t('No changes yet')} text={t('Every match, edit and renumbering is kept here, and can be undone.')} />
  return (
    <ul className="fm-simple">
      {list.map((h) => (
        <li key={h.batch}>
          <div><b>{tx(h.action)}</b>: {h.count > 1 ? tn('{title} and {count} more', '{title} and {count} more', h.count - 1, { title: h.title }) : h.title}<div className="muted fm-file">{date(h.at * 1000, { dateStyle: 'medium', timeStyle: 'short' })}{h.user ? ` ${t('by {user}', { user: h.user })}` : ''}</div></div>
          {h.undone ? <span className="muted">{t('Undone')}</span> : <Button onClick={() => undoBatch(h.batch).then(() => { toast(t('Undone')); load() }).catch((e) => toast(e.message))}>{t('Undo')}</Button>}
        </li>
      ))}
    </ul>
  )
}

const LOOKUPS: [string, string][] = [['auto', tk('Look everywhere')], ['none', tk('Don’t look it up')], ['tmdb', tk('TMDB only')], ['tvmaze', tk('TVmaze only')], ['tvdb', tk('TheTVDB only')], ['wikidata', tk('Wikidata only')], ['anilist', tk('AniList only')]]
const EXTRAS: [string, string][] = [['trailer', tk('trailer')], ['featurette', tk('featurette')], ['behind-the-scenes', tk('behind the scenes')], ['deleted-scene', tk('deleted scene')], ['interview', tk('interview')], ['short', tk('short')], ['other', tk('other')]]

function RulesPage({ library }: { library: number }) {
  const [rules, setRules] = useState<MatchRule[] | null>(null)
  const [tried, setTried] = useState<RuleTry[] | null>(null)
  const [err, setErr] = useState('')
  const [saved, setSaved] = useState(true)
  useEffect(() => {
    setRules(null); setTried(null); setErr(''); setSaved(true)
    if (library) api.get<{ rules: MatchRule[] }>(`/match/rules?library=${library}`).then((r) => setRules(r.rules)).catch((e) => setErr(e.message))
  }, [library])
  const up = (i: number, p: Partial<MatchRule>) => { setRules(rules!.map((r, j) => (j === i ? { ...r, ...p } : r))); setSaved(false); setTried(null) }
  const save = async () => {
    setErr('')
    try { const r = await api.put<{ rules: MatchRule[] }>('/match/rules', { library, rules }); setRules(r.rules); setSaved(true); toast(t('Rules saved. They apply to new and changed files on the next scan.')) } catch (e: any) { setErr(e.message) }
  }
  const tryThem = async () => {
    setErr('')
    try { const r = await api.post<{ matches: RuleTry[] }>('/match/rules/try', { library, rules }); setTried(r.matches) } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="fm-rules">
      <Card title={t('Names that say what a file is')} desc={<>{t('An ID in a file or folder’s name always wins over a search, and doesn’t appear in the title:')} <code>My Film (2001) [tmdbid-123].mkv</code>, <code>{'{imdb-tt0000123}'}</code>, <code>[tvdb-456]</code>, <code>[tvmaze-78]</code>.</>} />
      {!library ? <EmptyState title={t('Choose a library')} text={t('Each library has its own rules.')} /> : !rules ? (err ? <div className="error" role="alert">{err}</div> : <Spinner />) : (
        <Card title={t('This library’s rules')} desc={<>{t('For folders with a naming of their own. The first rule that fits a file decides. You can use:')} <code>{'{title}'}</code>, <code>{'{year}'}</code>, <code>{'{edition}'}</code>, <code>{'{tmdb}'}</code>, <code>{'{imdb}'}</code>, <code>{'{tvdb}'}</code>, <code>{'{any}'}</code>, <code>*</code> ({t('anything within a folder’s name')}). {t('For example:')} <code>Harbour Studio/{'{year}'} - {'{title}'}</code></>}>
          {rules.length === 0 && <p className="muted">{t('No rules yet.')}</p>}
          <ol className="fm-rule-list">
            {rules.map((r, i) => (
              <li key={i}>
                <label className="fm-rule-pattern">{t('How the path reads')}
                  <input className="input mono" spellCheck={false} value={r.pattern} placeholder={t('Folder/{year} - {title}')} onChange={(e) => up(i, { pattern: e.target.value })} />
                </label>
                <label>{t('Where to look')}
                  <select className="select" value={r.lookup || 'auto'} onChange={(e) => up(i, { lookup: e.target.value })}>{LOOKUPS.map(([k, l]) => <option key={k} value={k}>{tx(l)}</option>)}</select>
                </label>
                <label>{t('Add tags')} <span className="muted">{t('(commas)')}</span>
                  <input className="input" value={r.tags.join(', ')} onChange={(e) => up(i, { tags: e.target.value.split(',').map((s) => s.trimStart()) })} onBlur={(e) => up(i, { tags: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} />
                </label>
                <label>{t('These are bonus features')}
                  <select className="select" value={r.extra} onChange={(e) => up(i, { extra: e.target.value })}>
                    <option value="">{t('No')}</option>
                    {EXTRAS.map(([x, l]) => <option key={x} value={x}>{tx(l)}</option>)}
                  </select>
                </label>
                <label className="lib-check"><input type="checkbox" checked={r.enabled} onChange={(e) => up(i, { enabled: e.target.checked })} /> {t('On')}</label>
                <IconButton size="sm" className="danger-hover" label={t('Remove the rule {pattern}', { pattern: r.pattern })} onClick={() => { setRules(rules.filter((_, j) => j !== i)); setSaved(false) }}><IconTrash /></IconButton>
              </li>
            ))}
          </ol>
          {err && <div className="error" role="alert">{err}</div>}
          <div className="panel-actions">
            <Button icon={<IconPlus />} onClick={() => { setRules([...rules, { pattern: '', lookup: 'auto', tags: [], extra: '', enabled: true }]); setSaved(false) }}>{t('Add a rule')}</Button>
            <Button disabled={rules.length === 0 || rules.some((r) => !r.pattern.trim())} onClick={tryThem}>{t('Try them')}</Button>
            <Button variant="primary" disabled={saved || rules.some((r) => !r.pattern.trim())} onClick={save}>{t('Save')}</Button>
          </div>
          {tried && (
            <div className="fm-try">
              <h3>{tried.length ? tn('{count} file fits', '{count} files fit', tried.length) : t('No file fits')}</h3>
              <p className="muted">{t('Nothing has changed. This is what each would become on its next scan.')}</p>
              <ul>{tried.slice(0, 50).map((one) => (
                <li key={one.file}><span className="mono">{one.file}</span><span>{[one.result.title, one.result.year, one.result.source && `${one.result.source} ${one.result.id}`, one.result.lookup === 'none' && t('not looked up'), one.result.extra && extraWords(one.result.extra), one.result.tags?.join(', ')].filter(Boolean).map(bidi).join(' · ')}</span></li>
              ))}</ul>
            </div>
          )}
        </Card>
      )}
      <p className="muted"><Link to="/fix-matches">{t('Back to the review list')}</Link></p>
    </div>
  )
}

// What a rule's bonus feature is called, in a sentence about a file.
const extraWords = (x: string) => { const l = EXTRAS.find(([k]) => k === x)?.[1]; return t('a {kind}', { kind: l ? tx(l) : x }) }

// Season and episode numbers beside what the source lists, with quick ways to shift a whole season.
function RenumberDialog({ showId, title, onClose, onDone }: { showId: number; title: string; onClose: () => void; onDone: (batch: string) => void }) {
  const [map, setMap] = useState<EpisodeMap | null>(null)
  const [rows, setRows] = useState<EpisodeRow[]>([])
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [season, setSeason] = useState('all')
  const [shift, setShift] = useState('1')
  useEffect(() => { api.get<EpisodeMap>(`/meta/show/${showId}/episodes`).then((m) => { setMap(m); setRows(m.rows) }).catch((e) => setErr(e.message)) }, [showId])
  const names = useMemo(() => { const n = new Map<string, string>(); map?.listed.forEach((l) => n.set(`${l.season}|${l.episode}`, l.title)); return n }, [map])
  const seasons = [...new Set(rows.map((r) => r.season).filter((s): s is number => s != null))].sort((a, b) => a - b)
  const inScope = (r: EpisodeRow) => season === 'all' || r.season === Number(season)
  const change = (id: number, p: Partial<EpisodeRow>) => setRows(rows.map((r) => (r.id === id ? { ...r, ...p } : r)))
  const num = (s: string) => (s === '' ? null : Number(s))
  const save = async () => {
    setBusy(true); setErr('')
    try {
      const r = await api.put<{ batch: string }>(`/meta/show/${showId}/episodes`, { episodes: rows.map((x) => ({ id: x.id, season: x.season ?? null, episode: x.episode ?? null, episodeEnd: x.episodeEnd ?? null })) })
      onDone(r.batch)
    } catch (e: any) { setErr(e.message); setBusy(false) }
  }
  const unknown = rows.filter((r) => r.season != null && r.episode != null && !names.has(`${r.season}|${r.episode}`)).length
  return (
    <Dialog title={t('Renumber {title}', { title })} size="wide" className="fm-renumber" onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button>
        <Button variant="primary" busy={busy} disabled={!map} onClick={save}>{t('Save')}</Button>
      </>}>
        {!map ? (err ? <div className="error" role="alert">{err}</div> : <Spinner />) : (
          <>
            {map.note ? <p className="muted">{map.note}</p> : <p className="muted">{t('Each file beside what {source} calls that number.', { source: map.sourceLabel })}{unknown ? ` ${tn('{count} number isn’t listed there.', '{count} numbers aren’t listed there.', unknown)}` : ''} {t('Numbers you change are locked, so a scan keeps them.')}</p>}
            <div className="fm-shift">
              <label>{t('Season')}
                <select className="select" value={season} onChange={(e) => setSeason(e.target.value)}>
                  <option value="all">{t('All')}</option>
                  {seasons.map((s) => <option key={s} value={s}>{s === 0 ? t('Specials') : t('Season {n}', { n: s })}</option>)}
                </select>
              </label>
              <label>{t('Move episodes by')}
                <input className="input" inputMode="numeric" value={shift} onChange={(e) => setShift(e.target.value)} />
              </label>
              <Button onClick={() => setRows(rows.map((r) => (inScope(r) && r.episode != null ? { ...r, episode: Math.max(0, r.episode + Number(shift || 0)), episodeEnd: r.episodeEnd != null ? Math.max(0, r.episodeEnd + Number(shift || 0)) : r.episodeEnd } : r)))}>{t('Move them')}</Button>
              <Button onClick={() => setRows(rows.map((r) => (inScope(r) && r.season != null ? { ...r, season: Math.max(0, r.season + Number(shift || 0)) } : r)))}>{t('Move seasons instead')}</Button>
              <Button variant="ghost" onClick={() => { let n = 0; setRows(rows.map((r) => (inScope(r) ? { ...r, episode: ++n } : r))) }}>{t('Number them in order')}</Button>
            </div>
            <div className="fm-ep-wrap">
              <table className="titles-table fm-eps">
                <thead><tr><th>{t('File')}</th><th>{t('Season')}</th><th>{t('Episode')}</th><th>{t('{source} calls it', { source: map.sourceLabel || t('Source') })}</th></tr></thead>
                <tbody>
                  {rows.filter(inScope).map((r) => (
                    <tr key={r.id}>
                      <td className="fm-file" title={r.file}>{r.file}</td>
                      <td><input className="input fm-n" inputMode="numeric" aria-label={t('Season of {file}', { file: r.file })} value={r.season ?? ''} onChange={(e) => change(r.id, { season: num(e.target.value) })} /></td>
                      <td><input className="input fm-n" inputMode="numeric" aria-label={t('Episode of {file}', { file: r.file })} value={r.episode ?? ''} onChange={(e) => change(r.id, { episode: num(e.target.value) })} /></td>
                      <td className={names.has(`${r.season}|${r.episode}`) ? '' : 'muted'}>{names.get(`${r.season}|${r.episode}`) || t('Not listed')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
        {map && err && <div className="error" role="alert">{err}</div>}
    </Dialog>
  )
}
