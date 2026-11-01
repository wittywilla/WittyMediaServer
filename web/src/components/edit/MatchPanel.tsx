import { useEffect, useRef, useState } from 'react'
import { api, artPreviewUrl } from '../../api'
import { IconStar } from '../../icons'
import type { FieldChange, MatchCandidate, MatchPreview, SearchResult, Target, TitleEditor } from '../../metadata'
import { ask, Button, Notice, Progress } from '../../ui'
import LegalNote from '../LegalNote'

// Search and match: pick the right title. Choosing a result shows what would change
// first, field by field, before anything does.

export const confidenceWord = (n: number) => (n >= 90 ? 'Very likely' : n >= 70 ? 'Likely' : n >= 40 ? 'Not sure' : 'Unlikely')

export function ConfidenceMeter({ value, label }: { value: number; label?: string }) {
  return <span className="ed-conf"><Progress value={value} showValue tone={value >= 70 ? 'ok' : value >= 40 ? 'warn' : 'danger'} label={`${label || confidenceWord(value)}: ${value} out of 100`} /></span>
}

// Shared with the fix-matches page.
export function ResultRow({ c, onChoose, chosen }: { c: MatchCandidate; onChoose: () => void; chosen?: boolean }) {
  const [showWhy, setShowWhy] = useState(false)
  return (
    <div className={'ed-result' + (chosen ? ' on' : '')}>
      <button type="button" className="ed-result-main" onClick={onChoose} aria-describedby={`why-${c.source}-${c.id}`}>
        <span className="ed-result-pic">{c.poster ? <img src={artPreviewUrl(c.poster)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : <span aria-hidden="true">?</span>}</span>
        <span className="ed-result-text">
          <b>{c.title}</b>
          <span className="meta">
            {c.year && <span>{c.year}</span>}
            {c.rating ? <span className="star"><IconStar size={14} /> {c.rating.toFixed(1)}</span> : null}
            <span>{c.kind === 'show' ? 'Show' : 'Film'}</span>
            <span>{sourceName(c.source)}</span>
          </span>
          {c.overview && <span className="ed-result-over">{c.overview}</span>}
        </span>
      </button>
      <div className="ed-result-side" id={`why-${c.source}-${c.id}`}>
        <ConfidenceMeter value={c.score} />
        {c.reasons?.length ? <button type="button" className="ed-why" aria-expanded={showWhy} onClick={() => setShowWhy(!showWhy)}>Why?</button> : null}
      </div>
      {showWhy && <ul className="ed-why-list">{c.reasons?.map((r) => <li key={r}>{r}</li>)}</ul>}
    </div>
  )
}

const sourceName = (s: string) => ({ tmdb: 'TMDB', 'tmdb-tv': 'TMDB', tvdb: 'TheTVDB', tvmaze: 'TVmaze', anilist: 'AniList', wikidata: 'Wikidata' } as Record<string, string>)[s] || (s.startsWith('agent:') ? 'Your agent' : s)

export default function MatchPanel({ ed, target, id, onApplied, onError }: { ed: TitleEditor; target: Target; id: number; onApplied: (e: TitleEditor, batch?: string, what?: string) => void; onError: (s: string) => void }) {
  const [query, setQuery] = useState('')
  const [res, setRes] = useState<SearchResult | null>(null)
  const [searching, setSearching] = useState(false)
  const [preview, setPreview] = useState<MatchPreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const list = useRef<HTMLDivElement | null>(null)
  const results = useRef<HTMLHeadingElement | null>(null)

  const search = async () => {
    setSearching(true); setPreview(null); onError('')
    try { setRes(await api.post<SearchResult>(`/meta/${target}/${id}/search`, { query })) } catch (e: any) { onError(e.message); setRes(null) } finally { setSearching(false) }
  }
  useEffect(() => { if (res && results.current) results.current.focus({ preventScroll: true }) }, [res])

  const showPreview = (p: MatchPreview) => {
    setPreview(p)
    setPicked(new Set(p.changes.filter((c) => c.apply).map((c) => c.field)))
  }
  const choose = async (c: MatchCandidate) => {
    setPreviewing(true); onError('')
    try { showPreview(await api.post<MatchPreview>(`/meta/${target}/${id}/preview`, { source: c.source, id: c.id })) } catch (e: any) { onError(e.message) } finally { setPreviewing(false) }
  }
  const refresh = async () => {
    setPreviewing(true); onError(''); setRes(null)
    try { showPreview(await api.post<MatchPreview>(`/meta/${target}/${id}/preview`, {})) } catch (e: any) { onError(e.message) } finally { setPreviewing(false) }
  }
  const apply = async () => {
    if (!preview) return
    onError('')
    try {
      const r = await api.post<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}/match`, { source: preview.source, id: preview.id, fields: [...picked] })
      setPreview(null); setRes(null)
      onApplied(r.editor, r.batch, `Matched with ${preview.title}.`)
    } catch (e: any) { onError(e.message) }
  }
  const removeMatch = async () => {
    onError('')
    if (!await ask({ title: 'Remove this match?', body: `Everything ${ed.match.sourceLabel} gave goes, and it's named after its file again. What you typed and what's beside the file stays. Nothing is looked up for it until you choose a match.`, confirm: 'Remove the match', cancel: 'Keep it', danger: true })) return
    try {
      const r = await api.del<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}/match`)
      onApplied(r.editor, r.batch, 'Match removed.')
    } catch (e: any) { onError(e.message) }
  }
  // Up and down along the results, as well as Tab.
  const arrows = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const items = [...(list.current?.querySelectorAll<HTMLButtonElement>('.ed-result-main') || [])]
    const i = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = items[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) { e.preventDefault(); e.stopPropagation(); next.focus() }
  }
  const m = ed.match
  return (
    <section id="ed-match" className="ed-section" aria-labelledby="ed-match-h">
      <h3 id="ed-match-h" className="sr-only">Match</h3>
      {m.source ? (
        <div className="ed-matched">
          <p>Matched with {m.url ? <a href={m.url} target="_blank" rel="noreferrer">{m.sourceLabel}</a> : m.sourceLabel}{m.state === 'manual' ? ', chosen by you' : ''}.</p>
          {m.state !== 'manual' && <ConfidenceMeter value={m.confidence} />}
        </div>
      ) : m.state === 'none' ? (
        <p className="muted">You said this has no match, so nothing is looked up for it. Choosing one below changes that.</p>
      ) : (
        <p className="muted">Nothing is matched yet. Search below and pick the right title.</p>
      )}
      {m.reasons.length > 0 && m.state === '' && m.source !== '' && <ul className="ed-why-list">{m.reasons.map((r) => <li key={r}>{r}</li>)}</ul>}
      {m.source && <p>Wrong film or show? <button type="button" className="ed-link" onClick={removeMatch}>Remove this match</button> and keep only what you typed.</p>}
      <div className="ed-panel">
        <h4>Search and match: pick the right title</h4>
        <form className="field-row" onSubmit={(e) => { e.preventDefault(); search() }}>
          <input className="input" aria-label="Search for the right title, or paste an id or address" autoFocus value={query} placeholder={ed.name} onChange={(e) => setQuery(e.target.value)} />
          <Button type="submit" busy={searching}>Search</Button>
          {m.source && m.state !== 'none' && <Button variant="ghost" onClick={refresh} disabled={previewing}>Check for changes</Button>}
        </form>
        <p className="muted ed-help">A name, or an id or address from TMDB, IMDb, TheTVDB, TVmaze or Wikidata, such as tt0000123 or tmdb:12.</p>
        {previewing && <div className="loading-box"><div className="spinner" /></div>}
        {preview ? <PreviewTable p={preview} picked={picked} setPicked={setPicked} onApply={apply} onBack={() => setPreview(null)} /> : res && (
          <>
            <h5 ref={results} tabIndex={-1} className="ed-results-h">{res.results.length ? `${res.results.length} ${res.results.length === 1 ? 'result' : 'results'}, most likely first` : 'Nothing found'}</h5>
            {res.warnings.map((w) => <Notice key={w} tone="warn">{w}</Notice>)}
            {res.results.length === 0 && !res.warnings.length && <p className="muted">Try fewer words, the original title, or an id. Asked: {res.sources.join(', ') || 'no sources are turned on'}.</p>}
            <div className="ed-results" ref={list} onKeyDown={arrows} data-nav-group>
              {res.results.map((c) => <ResultRow key={c.source + c.id} c={c} onChoose={() => choose(c)} />)}
            </div>
            {res.results.length > 0 && <LegalNote kind="webResults" />}
          </>
        )}
      </div>
    </section>
  )
}

const isArt = (f: string) => f === 'poster' || f === 'backdrop' || f === 'banner' || f === 'logo'
const showValue = (c: FieldChange, v: string) => (isArt(c.field) ? v : v.replace(/\|/g, ', '))

export function PreviewTable({ p, picked, setPicked, onApply, onBack, applyLabel = 'Use this match' }: { p: MatchPreview; picked: Set<string>; setPicked: (s: Set<string>) => void; onApply: () => void; onBack: () => void; applyLabel?: string }) {
  const toggle = (f: string, on: boolean) => { const n = new Set(picked); if (on) n.add(f); else n.delete(f); setPicked(n) }
  const head = useRef<HTMLHeadingElement | null>(null)
  useEffect(() => { head.current?.focus({ preventScroll: true }) }, [p])
  return (
    <div className="ed-preview">
      <h5 ref={head} tabIndex={-1}>
        {p.title} <span className="muted">from {p.url ? <a href={p.url} target="_blank" rel="noreferrer">{p.sourceLabel}</a> : p.sourceLabel}</span>
      </h5>
      {p.changes.length === 0 ? <p className="muted">Nothing would change: everything already says what this says.</p> : (
        <>
          <p className="muted ed-help">Here's what would change. Untick anything to keep what's there now.</p>
          <ul className="ed-changes">
            {p.changes.map((c) => (
              <li key={c.field} className={c.locked ? 'locked' : ''}>
                <label className="ed-change-pick">
                  <input type="checkbox" disabled={c.locked} checked={picked.has(c.field)} onChange={(e) => toggle(c.field, e.target.checked)} />
                  <b>{c.label}</b>
                </label>
                <div className="ed-change-vals">
                  {isArt(c.field) ? (
                    <>
                      <span className="ed-change-art">{c.current ? <img src={artPreviewUrl(c.current)} alt="Now" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} /> : <span className="muted">None</span>}</span>
                      <span aria-hidden="true">→</span>
                      <span className="ed-change-art">{c.incoming ? <img src={artPreviewUrl(c.incoming)} alt="New" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} /> : <span className="muted">None</span>}</span>
                    </>
                  ) : (
                    <>
                      <span className="ed-old"><span className="sr-only">Now: </span>{showValue(c, c.current) || <span className="muted">Empty</span>}</span>
                      <span className="ed-new"><span className="sr-only">New: </span>{showValue(c, c.incoming) || <span className="muted">Empty</span>}</span>
                    </>
                  )}
                </div>
                {c.note && <div className="ed-hint">{c.note}</div>}
              </li>
            ))}
          </ul>
        </>
      )}
      {p.episodes && (
        <p className="ed-help">
          {p.source && <>It knows {p.episodes.known} of the {p.episodes.total} numbered episodes here{p.episodes.known < p.episodes.total ? ', so check the numbering under Fix matches' : ''}.</>}
          {p.episodes.unknown.length > 0 && <span className="muted"> Not known: {p.episodes.unknown.slice(0, 12).join(', ')}{p.episodes.unknown.length > 12 ? '…' : ''}.</span>}
        </p>
      )}
      <div className="ed-art-btns">
        <Button variant="primary" onClick={onApply}>{applyLabel}</Button>
        <Button variant="ghost" onClick={onBack}>Back</Button>
      </div>
    </div>
  )
}
