import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, artPreviewUrl } from '../api'
import { announce } from '../a11y'
import EditDetails from '../components/edit/EditDetails'
import { ConfidenceMeter, PreviewTable, ResultRow } from '../components/edit/MatchPanel'
import { EmptyState, LoadError, Spinner } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconCheck, IconPlus, IconTrash } from '../icons'
import { Button, Card, Chip, Chips, Dialog, IconButton, Notice, Select, Tabs } from '../ui'
import { undoBatch, type EpisodeMap, type EpisodeRow, type HistoryEntry, type MatchCandidate, type MatchPreview, type MatchRule, type QueueResult, type QueueRow, type RuleTry, type SearchResult, type Target } from '../metadata'
import { toast, useApp } from '../state'
import BulkEdit from './library/BulkEdit'
import './library/library.css'
import './fixmatches.css'

// Fix matches: everything that needs a look, worst first, with suggestions and reasons, and a
// key for each thing you'd do. Nothing here asks anyone anything until you open a title.

const TABS: [string, string][] = [['review', 'Needs a look'], ['unmatched', 'No match'], ['low', 'Not sure'], ['ok', 'Looks right'], ['manual', 'You chose'], ['none', 'Left alone'], ['all', 'Everything'], ['removed', 'Removed']]

const STATE_WORDS: Record<QueueRow['state'], string> = { unmatched: 'No match', low: 'Not sure', ok: 'Looks right', manual: 'You chose this', none: 'Left alone' }

export default function FixMatches() {
  const { user, libraries } = useApp()
  const [params, setParams] = useSearchParams()
  const page = params.get('page') || 'review'
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const matchable = libraries.filter((l) => ['movies', 'discs', 'shows', 'anime'].includes(l.type))
  if (!user?.isAdmin) return <EmptyState title="This page is for administrators" />
  return (
    <div className="fm">
      <div className="toolbar">
        <h1>Fix matches</h1>
        <Tabs label="Fix matches" value={page} onChange={(k) => set('page', k === 'review' ? '' : k)} items={[{ value: 'review', label: 'Review' }, { value: 'rules', label: 'Naming rules' }, { value: 'history', label: 'History' }]} />
        <div className="grow" />
        <Select aria-label="Library" value={params.get('library') || ''} onChange={(e) => set('library', e.target.value)}
          options={[{ value: '', label: page === 'rules' ? 'Choose a library' : 'All libraries' }, ...matchable.map((l) => ({ value: String(l.id), label: l.name }))]} />
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
    const t = setTimeout(() => api.get<NonNullable<typeof job>>('/match/suggest-all').then((j) => {
      setJob(j)
      if (j.running) return
      if (j.total === 0) toast('They all have suggestions already')
      setTick((n) => n + 1)
    }).catch(() => {}), 2000)
    return () => clearTimeout(t)
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
      decided(out.batches[0], `${r.title}: matched with ${c.title}.`)
    } catch (e: any) { toast(e.message) }
  }, [])
  const leave = useCallback(async (r: QueueRow, keep: boolean) => {
    try {
      const out = await api.post<{ batch: string }>(`/meta/${r.target}/${r.id}/leave`, { keep })
      decided(out.batch, keep ? `${r.title}: kept as it is.` : `${r.title}: left with no match.`)
    } catch (e: any) { toast(e.message) }
  }, [])
  const undo = useCallback(async () => {
    if (!lastUndo) return
    try { await undoBatch(lastUndo.batch); setLastUndo(null); announce('Undone'); toast('Undone'); setTick((n) => n + 1) } catch (e: any) { toast(e.message) }
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
    if (!choices.length) { toast('None of those has a suggestion yet'); return }
    try {
      const out = await api.post<{ done: number; batches: string[]; failed: string[] }>('/match/accept', { choices })
      setChecked(new Set())
      toast(`Matched ${out.done}${out.failed.length ? `, ${out.failed.length} couldn't be` : ''}`)
      setTick((n) => n + 1)
    } catch (e: any) { toast(e.message) }
  }
  const sure = rows.filter((r) => (r.suggestions?.[0]?.score ?? 0) >= 90)
  return (
    <>
      <Chips label="Which titles" className="fm-tabs">
        {TABS.map(([k, l]) => <Chip key={k} selected={tab === k} onClick={() => setTab(k)}>{l}{q && k !== 'removed' && q.counts[k] != null ? ` ${q.counts[k]}` : ''}</Chip>)}
      </Chips>
      {tab === 'removed' ? <RemovedList library={library} /> : (
        <>
          <div className="fm-actions">
            <input className="input fm-search" aria-label="Find a title in this list" placeholder="Find a title" value={search} onChange={(e) => setSearch(e.target.value)} />
            {job?.running
              ? <span className="fm-job" role="status">Finding suggestions: {job.done} of {job.total}{job.current ? `, ${job.current}` : ''}
                  <Button size="sm" variant="ghost" onClick={() => api.del('/match/suggest-all').then(() => setJob(null))}>Stop</Button></span>
              : <Button onClick={() => api.post<{ job: NonNullable<typeof job> }>('/match/suggest-all', { library }).then((r) => setJob(r.job)).catch((e) => toast(e.message))}>Find suggestions for all</Button>}
            {sure.length > 1 && <Button onClick={() => { setChecked(new Set(sure.map(keyOf))) }}>Choose the {sure.length} that are 90% sure</Button>}
          </div>
          {!q ? (failed ? <LoadError what="The list" onRetry={load} /> : <Spinner />) : rows.length === 0 ? (
            <EmptyState icon={<IconCheck />} title={tab === 'review' ? 'Nothing needs a look' : 'Nothing here'} text={tab === 'review' ? 'Everything here is matched, and matched well. A new scan may add more.' : undefined} />
          ) : (
            <div className="fm-split">
              <ul className="fm-list" ref={list} aria-label="Titles" data-nav-group>
                {rows.map((r, i) => {
                  const on = i === Math.min(sel, rows.length - 1)
                  return (
                    <li key={keyOf(r)} className={'fm-row' + (on ? ' on' : '')}>
                      <input type="checkbox" aria-label={`Choose ${r.title}`} checked={checked.has(keyOf(r))} onChange={() => setChecked((c) => { const n = new Set(c); if (n.has(keyOf(r))) n.delete(keyOf(r)); else n.add(keyOf(r)); return n })} />
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
                          <span className="fm-row-state"><ConfidenceMeter value={r.confidence} label={STATE_WORDS[r.state]} /> <span className="muted">{STATE_WORDS[r.state]}</span></span>
                        </span>
                      </button>
                    </li>
                  )
                })}
                {q.total > rows.length && <li className="muted fm-more">The worst {rows.length} of {q.total}. Fix these and the rest move up.</li>}
              </ul>
              {cur && <Detail key={keyOf(cur)} ref_={detail} r={cur} topFor={topFor} setTopFor={setTopFor} onAccept={accept} onLeave={leave} onEdit={() => setEditing(cur)} onRenumber={() => setRenumber(cur)} onDecided={decided} />}
            </div>
          )}
          <p className="muted fm-keys">Keys: <kbd>J</kbd> <kbd>K</kbd> next and last, <kbd>A</kbd> accept the first suggestion, <kbd>1</kbd> to <kbd>6</kbd> accept that one, <kbd>N</kbd> no match, <kbd>E</kbd> edit, <kbd>X</kbd> choose, <kbd>/</kbd> search, <kbd>U</kbd> undo.</p>
          {(lastUndo || chosen.length > 0) && (
            <div className="lib-bulk fm-bar" role="region" aria-label="Actions">
              {chosen.length > 0 && <>
                <span>{chosen.length} chosen</span>
                <Button variant="primary" onClick={acceptChosen}>Accept their best suggestions</Button>
                <Button onClick={() => setBulk(true)}>Edit together</Button>
                <Button variant="ghost" onClick={() => setChecked(new Set())}>Clear</Button>
              </>}
              {lastUndo && <>
                <span className="fm-bar-what">{lastUndo.what}</span>
                <Button onClick={undo}>Undo</Button>
              </>}
            </div>
          )}
        </>
      )}
      <LegalNote kind="webResults" />
      {editing && <EditDetails target={editing.target as Target} id={editing.id} onClose={() => setEditing(null)} onChanged={() => setTick((n) => n + 1)} />}
      {bulk && <BulkEdit titles={chosen.map((r) => ({ target: r.target, id: r.id, title: r.title }))} onClose={() => setBulk(false)} onDone={() => { setBulk(false); setChecked(new Set()); setTick((n) => n + 1) }} />}
      {renumber && <RenumberDialog showId={renumber.id} title={renumber.title} onClose={() => setRenumber(null)} onDone={(batch) => { setRenumber(null); decided(batch, `${renumber.title}: renumbered.`) }} />}
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
      onDecided(out.batch, `${r.title}: matched with ${preview.p.title}.`)
    } catch (e: any) { setErr(e.message) }
  }
  const list = results?.results ?? known ?? []
  return (
    <div className="fm-detail" ref={ref_} aria-label={`Fixing ${r.title}`} role="region">
      <header className="fm-detail-head">
        <div>
          <h2>{r.title}{r.year ? ` (${r.year})` : ''}</h2>
          <p className="muted fm-file" title={r.file}>{r.file}</p>
        </div>
        <div className="fm-detail-btns">
          <Button onClick={onEdit}>Edit details</Button>
          {r.target === 'show' && <Button onClick={onRenumber}>Renumber episodes</Button>}
          {(r.state === 'unmatched' || r.state === 'low') && <Button variant="ghost" onClick={() => onLeave(r, r.state === 'low')}>{r.state === 'low' ? 'It\'s right: stop asking' : 'It has no match'}</Button>}
        </div>
      </header>
      <div className="fm-why">
        {r.provider ? <span>Matched with {r.url ? <a href={r.url} target="_blank" rel="noreferrer">{r.sourceLabel}</a> : r.sourceLabel}.</span> : null}
        <ul>{r.reasons.map((w) => <li key={w}>{w}</li>)}</ul>
      </div>
      <form className="field-row fm-find" onSubmit={(e) => { e.preventDefault(); search() }}>
        <input className="input" aria-label="Search for the right title, or paste an id or address" placeholder="A name, or an id or address from TMDB, IMDb, TheTVDB, TVmaze or Wikidata" value={query} onChange={(e) => setQuery(e.target.value)} />
        <Button type="submit" disabled={busy}>Search</Button>
      </form>
      {err && <div className="error" role="alert">{err}</div>}
      {results?.warnings.map((w) => <Notice key={w} tone="warn">{w}</Notice>)}
      {busy && <Spinner />}
      {preview ? (
        <PreviewTable p={preview.p} picked={picked} setPicked={setPicked} onApply={apply} onBack={() => setPreview(null)} />
      ) : (
        <>
          {list.length > 0 && <h3 className="fm-sugg-h">{results ? 'Results' : 'Suggestions'}</h3>}
          <ol className="fm-sugg" data-nav-group>
            {list.map((c, i) => (
              <li key={c.source + c.id}>
                <span className="fm-num" aria-hidden="true">{i < 6 ? i + 1 : ''}</span>
                <ResultRow c={c} onChoose={() => look(c)} />
                <Button size="sm" onClick={() => onAccept(r, c)} aria-label={`Accept ${c.title}`}>Accept</Button>
              </li>
            ))}
          </ol>
          {!busy && list.length === 0 && r.state !== 'manual' && r.state !== 'none' && <p className="muted">No source has anything by this name. Try another name, or an id, above.</p>}
        </>
      )}
    </div>
  )
}

function RemovedList({ library }: { library: number }) {
  const [list, setList] = useState<{ path: string; title: string; libraryId: number; at: number }[] | null>(null)
  const load = useCallback(() => { api.get<{ removed: { path: string; title: string; libraryId: number; at: number }[] }>(`/meta/removed?library=${library}`).then((r) => setList(r.removed)).catch(() => setList([])) }, [library])
  useEffect(load, [load])
  if (!list) return <Spinner />
  if (list.length === 0) return <EmptyState icon={<IconTrash />} title="Nothing has been taken out" text="Titles taken out of a library wait here. Their files stay on disk." />
  return (
    <ul className="fm-simple">
      {list.map((r) => (
        <li key={r.path}>
          <div><b>{r.title}</b><div className="muted fm-file">{r.path}</div></div>
          <Button onClick={() => api.post('/meta/removed/restore', { path: r.path }).then(() => { toast('Back in its library'); load() }).catch((e) => toast(e.message))}>Bring it back</Button>
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
  if (list.length === 0) return <EmptyState title="No changes yet" text="Every match, edit and renumbering is kept here, and can be undone." />
  return (
    <ul className="fm-simple">
      {list.map((h) => (
        <li key={h.batch}>
          <div><b>{h.action}</b>: {h.title}{h.count > 1 ? ` and ${h.count - 1} more` : ''}<div className="muted fm-file">{new Date(h.at * 1000).toLocaleString()}{h.user ? ` by ${h.user}` : ''}</div></div>
          {h.undone ? <span className="muted">Undone</span> : <Button onClick={() => undoBatch(h.batch).then(() => { toast('Undone'); load() }).catch((e) => toast(e.message))}>Undo</Button>}
        </li>
      ))}
    </ul>
  )
}

const LOOKUPS: [string, string][] = [['auto', 'Look everywhere'], ['none', "Don't look it up"], ['tmdb', 'TMDB only'], ['tvmaze', 'TVmaze only'], ['tvdb', 'TheTVDB only'], ['wikidata', 'Wikidata only'], ['anilist', 'AniList only']]

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
    try { const r = await api.put<{ rules: MatchRule[] }>('/match/rules', { library, rules }); setRules(r.rules); setSaved(true); toast('Rules saved. They apply to new and changed files on the next scan.') } catch (e: any) { setErr(e.message) }
  }
  const tryThem = async () => {
    setErr('')
    try { const r = await api.post<{ matches: RuleTry[] }>('/match/rules/try', { library, rules }); setTried(r.matches) } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="fm-rules">
      <Card title="Names that say what a file is" desc={<>An id in a file or folder's name always wins over a search, and doesn't appear in the title: <code>My Film (2001) [tmdbid-123].mkv</code>, <code>{'{imdb-tt0000123}'}</code>, <code>[tvdb-456]</code>, <code>[tvmaze-78]</code>.</>} />
      {!library ? <EmptyState title="Choose a library" text="Each library has its own rules." /> : !rules ? (err ? <div className="error" role="alert">{err}</div> : <Spinner />) : (
        <Card title="This library's rules" desc={<>For folders with a naming of their own. The first rule that fits a file decides. Use <code>{'{title}'}</code>, <code>{'{year}'}</code>, <code>{'{edition}'}</code>, <code>{'{tmdb}'}</code>, <code>{'{imdb}'}</code>, <code>{'{tvdb}'}</code>, <code>{'{any}'}</code>, and <code>*</code> for anything within a folder's name. For example, <code>Harbour Studio/{'{year}'} - {'{title}'}</code>.</>}>
          {rules.length === 0 && <p className="muted">No rules yet.</p>}
          <ol className="fm-rule-list">
            {rules.map((r, i) => (
              <li key={i}>
                <label className="fm-rule-pattern">How the path reads
                  <input className="input mono" spellCheck={false} value={r.pattern} placeholder="Folder/{year} - {title}" onChange={(e) => up(i, { pattern: e.target.value })} />
                </label>
                <label>Where to look
                  <select className="select" value={r.lookup || 'auto'} onChange={(e) => up(i, { lookup: e.target.value })}>{LOOKUPS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select>
                </label>
                <label>Add tags <span className="muted">(commas)</span>
                  <input className="input" value={r.tags.join(', ')} onChange={(e) => up(i, { tags: e.target.value.split(',').map((s) => s.trimStart()) })} onBlur={(e) => up(i, { tags: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} />
                </label>
                <label>These are bonus features
                  <select className="select" value={r.extra} onChange={(e) => up(i, { extra: e.target.value })}>
                    <option value="">No</option>
                    {['trailer', 'featurette', 'behind-the-scenes', 'deleted-scene', 'interview', 'short', 'other'].map((x) => <option key={x} value={x}>{x.replace(/-/g, ' ')}</option>)}
                  </select>
                </label>
                <label className="lib-check"><input type="checkbox" checked={r.enabled} onChange={(e) => up(i, { enabled: e.target.checked })} /> On</label>
                <IconButton size="sm" className="danger-hover" label={`Remove the rule ${r.pattern}`} onClick={() => { setRules(rules.filter((_, j) => j !== i)); setSaved(false) }}><IconTrash /></IconButton>
              </li>
            ))}
          </ol>
          {err && <div className="error" role="alert">{err}</div>}
          <div className="panel-actions">
            <Button icon={<IconPlus />} onClick={() => { setRules([...rules, { pattern: '', lookup: 'auto', tags: [], extra: '', enabled: true }]); setSaved(false) }}>Add a rule</Button>
            <Button disabled={rules.length === 0 || rules.some((r) => !r.pattern.trim())} onClick={tryThem}>Try them</Button>
            <Button variant="primary" disabled={saved || rules.some((r) => !r.pattern.trim())} onClick={save}>Save</Button>
          </div>
          {tried && (
            <div className="fm-try">
              <h3>{tried.length ? `${tried.length} ${tried.length === 1 ? 'file fits' : 'files fit'}` : 'No file fits'}</h3>
              <p className="muted">Nothing has changed. This is what each would become on its next scan.</p>
              <ul>{tried.slice(0, 50).map((t) => (
                <li key={t.file}><span className="mono">{t.file}</span><span>{[t.result.title, t.result.year, t.result.source && `${t.result.source} ${t.result.id}`, t.result.lookup === 'none' && 'not looked up', t.result.extra && `a ${t.result.extra}`, t.result.tags?.join(', ')].filter(Boolean).join(' · ')}</span></li>
              ))}</ul>
            </div>
          )}
        </Card>
      )}
      <p className="muted">Back to <Link to="/fix-matches">the review list</Link>.</p>
    </div>
  )
}

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
    <Dialog title={`Renumber ${title}`} size="wide" className="fm-renumber" onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" busy={busy} disabled={!map} onClick={save}>Save</Button>
      </>}>
        {!map ? (err ? <div className="error" role="alert">{err}</div> : <Spinner />) : (
          <>
            {map.note ? <p className="muted">{map.note}</p> : <p className="muted">Each file beside what {map.sourceLabel} calls that number{unknown ? `. ${unknown} ${unknown === 1 ? 'number isn\'t' : 'numbers aren\'t'} listed there` : ''}. Numbers you change are locked, so a scan keeps them.</p>}
            <div className="fm-shift">
              <label>Season
                <select className="select" value={season} onChange={(e) => setSeason(e.target.value)}>
                  <option value="all">All</option>
                  {seasons.map((s) => <option key={s} value={s}>{s === 0 ? 'Specials' : `Season ${s}`}</option>)}
                </select>
              </label>
              <label>Move episodes by
                <input className="input" inputMode="numeric" value={shift} onChange={(e) => setShift(e.target.value)} />
              </label>
              <Button onClick={() => setRows(rows.map((r) => (inScope(r) && r.episode != null ? { ...r, episode: Math.max(0, r.episode + Number(shift || 0)), episodeEnd: r.episodeEnd != null ? Math.max(0, r.episodeEnd + Number(shift || 0)) : r.episodeEnd } : r)))}>Move them</Button>
              <Button onClick={() => setRows(rows.map((r) => (inScope(r) && r.season != null ? { ...r, season: Math.max(0, r.season + Number(shift || 0)) } : r)))}>Move seasons instead</Button>
              <Button variant="ghost" onClick={() => { let n = 0; setRows(rows.map((r) => (inScope(r) ? { ...r, episode: ++n } : r))) }}>Number them in order</Button>
            </div>
            <div className="fm-ep-wrap">
              <table className="titles-table fm-eps">
                <thead><tr><th>File</th><th>Season</th><th>Episode</th><th>{map.sourceLabel || 'Source'} calls it</th></tr></thead>
                <tbody>
                  {rows.filter(inScope).map((r) => (
                    <tr key={r.id}>
                      <td className="fm-file" title={r.file}>{r.file}</td>
                      <td><input className="input fm-n" inputMode="numeric" aria-label={`Season of ${r.file}`} value={r.season ?? ''} onChange={(e) => change(r.id, { season: num(e.target.value) })} /></td>
                      <td><input className="input fm-n" inputMode="numeric" aria-label={`Episode of ${r.file}`} value={r.episode ?? ''} onChange={(e) => change(r.id, { episode: num(e.target.value) })} /></td>
                      <td className={names.has(`${r.season}|${r.episode}`) ? '' : 'muted'}>{names.get(`${r.season}|${r.episode}`) || 'Not listed'}</td>
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
