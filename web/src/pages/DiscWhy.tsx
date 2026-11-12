// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, fmtTime, type Item } from '../api'
import { Button, Notice, Select, ask, toast } from '../ui'
import { IconPlay } from '../icons'

// What the disc's engine made of it, from GET /items/{id}/disc/analysis: which title
// (DVD) or playlist (Blu-ray) is the film, why, and how sure it is. Anyone who may see the
// disc reads it; an admin can choose another title for everyone, which the server keeps
// until it's cleared, and can ask for the ranking again.

export type DiscCandidate = { ttn: number; durationSec: number; chapters: number; class: string; score: number; reasons?: string[] }
export type DiscVersion = { kind: 'angle' | 'parental' | 'cut' | 'edition' | string; ttn: number; angle?: number; durationSec: number; label: string; main?: boolean }
export type DiscEpisode = { index: number; ttn: number; durationSec: number; startSec: number; chapter?: number; item?: number; part?: boolean }
export type DiscIssue = { severity: string; kind: string; ttn: number; what: string }
export type DiscRank = {
  engine: string; main: number; confidence: number; level: string; why: string; reasons: string[]; against?: string[]; runnerUp: number
  feature?: { titles: number[]; durationSec: number; source: string; assembled: boolean }
  versions?: DiscVersion[]; episodes?: DiscEpisode[]; playAll: number; candidates: DiscCandidate[]; decoys: number; hidden: number
  warnings?: string[]; repairs?: string[]; repairCount: number; issues?: DiscIssue[]; issueCount: number
  paddedSectors: number; readRetries: number; protected?: boolean
  secondOpinion?: { source: string; agrees: boolean; main: number; differences?: string[]; more?: number; note?: string }
}
export type DiscAnalysis = {
  kind: 'dvd' | 'bluray'; ranked: boolean; pending: boolean; main: number; engineMain: number
  chosen: { ttn: number; by: number; at: number } | null; chosenBy?: string; rank: DiscRank | null
  canChoose: boolean; protected: boolean
  secondOpinion?: { enabled: boolean; available: boolean; version?: string; reason?: string }
}

const sure = (c: number) => `${Math.round(c * 100)}% sure`
const levelWord = (l: string) => (l === 'high' ? 'High confidence' : l === 'medium' ? 'Medium confidence' : l === 'low' ? 'Low confidence' : '')

export function useDiscAnalysis(id: number) {
  const [a, setA] = useState<DiscAnalysis | null>(null)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let gone = false
    let timer: ReturnType<typeof setTimeout> | undefined
    api.get<DiscAnalysis>(`/items/${id}/disc/analysis`).then((v) => {
      if (gone) return
      setA(v)
      // Worked out behind on a server that goes easy on its machine: ask again shortly.
      if (v.pending) timer = setTimeout(() => setTick((t) => t + 1), 4000)
    }).catch(() => { if (!gone) setA(null) })
    return () => { gone = true; if (timer) clearTimeout(timer) }
  }, [id, tick])
  return [a, setA] as const
}

// DiscWhy is the panel under the feature: why this title, and for an admin, another choice.
export default function DiscWhy({ item, analysis, setAnalysis, unit }: {
  item: Item; analysis: DiscAnalysis; setAnalysis: (a: DiscAnalysis) => void; unit: string
}) {
  const [busy, setBusy] = useState('')
  const [pick, setPick] = useState('')
  const a = analysis
  const rk = a.rank
  const name = (n: number) => `${unit} ${a.kind === 'bluray' ? String(n).padStart(5, '0') : n}`
  const run = async (what: string, f: () => Promise<DiscAnalysis>, done: string) => {
    setBusy(what)
    try { setAnalysis(await f()); toast(done, { tone: 'ok' }) } catch (e: any) { toast(e.message, { tone: 'danger' }) } finally { setBusy('') }
  }
  const choose = (ttn: number) => run('choose', () => api.put<DiscAnalysis>(`/items/${item.id}/disc/choice`, { ttn }), `${name(ttn)} is now this disc's film`)
  const clear = async () => {
    if (!(await ask({ title: 'Use the engine’s pick again?', body: `Play will play ${a.engineMain >= 0 ? name(a.engineMain) : 'what the engine picks'} again, for everyone.`, confirm: 'Use the engine’s pick' }))) return
    run('clear', () => api.del<DiscAnalysis>(`/items/${item.id}/disc/choice`), 'The engine’s pick is the film again')
  }
  const again = (secondOpinion: boolean) => run(secondOpinion ? 'second' : 'again', () => api.post<DiscAnalysis>(`/items/${item.id}/disc/analysis`, { secondOpinion }),
    secondOpinion ? 'libbluray’s reading was compared' : 'The disc was ranked again')

  if (a.pending && !rk) return <Notice title="Working out which title is the film">This disc is being looked at in the background. This page updates by itself.</Notice>
  if (!a.ranked || !rk) {
    return (
      <div className="disc-why">
        <p className="muted">The disc module in use is from before ranking, so the film is the longest sensible {unit.toLowerCase()}. Updating the module explains its pick.</p>
        {a.chosen && <ChosenLine a={a} name={name} busy={busy} clear={clear} />}
      </div>
    )
  }
  const others = rk.candidates.filter((c) => c.ttn !== a.main)
  return (
    <div className="disc-why" aria-labelledby="why-h">
      <h3 className="section-title" id="why-h">Why this {unit.toLowerCase()}</h3>
      {a.chosen
        ? <ChosenLine a={a} name={name} busy={busy} clear={clear} />
        : <p><span className={`pill why-level ${rk.level}`}>{levelWord(rk.level) || sure(rk.confidence)}</span> {rk.why || `The engine picked ${name(rk.main)}.`} <span className="muted">({sure(rk.confidence)})</span></p>}
      {rk.protected && <Notice tone="warn">This disc is encrypted, so it won’t play here. Its structure was still read, to say what is on it.</Notice>}
      <details className="disc-why-more">
        <summary>How the engine decided</summary>
        {rk.reasons.length > 0 && <><h4>For {name(rk.main)}</h4><ul>{rk.reasons.map((r, i) => <li key={i}>{r}</li>)}</ul></>}
        {rk.against && rk.against.length > 0 && <><h4>Against it</h4><ul>{rk.against.map((r, i) => <li key={i}>{r}</li>)}</ul></>}
        {rk.feature?.assembled && <p>The film runs through {rk.feature.titles.length > 1 ? `titles ${rk.feature.titles.join(', ')}` : `title ${rk.feature.titles[0]}`} ({fmtTime(rk.feature.durationSec)}), which play here as one.</p>}
        {(rk.decoys > 0 || rk.hidden > 0) && <p className="muted">{rk.decoys > 0 ? `${rk.decoys} ${unit.toLowerCase()}s look like decoys and aren’t listed. ` : ''}{rk.hidden > 0 ? `${rk.hidden} more aren’t shown.` : ''}</p>}
        {rk.warnings && rk.warnings.length > 0 && <><h4>Worth knowing</h4><ul>{rk.warnings.map((w, i) => <li key={i}>{w}</li>)}</ul></>}
        {(rk.repairCount > 0 || rk.issueCount > 0 || rk.paddedSectors > 0) && (
          <>
            <h4>The copy</h4>
            <ul>
              {rk.repairs?.map((r, i) => <li key={'r' + i}>{r}</li>)}
              {rk.repairCount > (rk.repairs?.length || 0) && <li className="muted">and {rk.repairCount - (rk.repairs?.length || 0)} more mended</li>}
              {rk.issues?.map((x, i) => <li key={'i' + i}>{x.severity === 'error' ? 'Won’t play: ' : ''}{x.what}{x.ttn >= 0 ? ` (${name(x.ttn)})` : ''}</li>)}
              {rk.paddedSectors > 0 && <li>{rk.paddedSectors} unreadable sectors were passed over as blank while reading the disc’s index.</li>}
            </ul>
          </>
        )}
        {rk.secondOpinion && (
          <p>{rk.secondOpinion.note
            ? `No second opinion: ${rk.secondOpinion.note}`
            : rk.secondOpinion.agrees ? `${rk.secondOpinion.source} agrees.` : `${rk.secondOpinion.source} reads it differently${rk.secondOpinion.main >= 0 ? ` and picks ${name(rk.secondOpinion.main)}` : ''}.`}
          {rk.secondOpinion.differences && rk.secondOpinion.differences.length > 0 && <ul>{rk.secondOpinion.differences.map((d, i) => <li key={i}>{d}</li>)}</ul>}</p>
        )}
        <p className="muted small">{rk.engine}</p>
      </details>
      {a.canChoose && (
        <div className="disc-why-actions">
          {others.length > 0 && (
            <>
              <Select inputSize="sm" aria-label={`Another ${unit.toLowerCase()} to play as the film`} value={pick} onChange={(e) => setPick(e.target.value)}
                options={[{ value: '', label: `Another ${unit.toLowerCase()}…` }, ...others.map((c) => ({ value: String(c.ttn), label: `${name(c.ttn)} · ${fmtTime(c.durationSec)} · ${c.class}` }))]} />
              <Button size="sm" disabled={!pick} busy={busy === 'choose'} onClick={() => choose(Number(pick))}>Make it the film</Button>
            </>
          )}
          <Button size="sm" variant="ghost" busy={busy === 'again'} onClick={() => again(false)}>Rank again</Button>
          {a.kind === 'bluray' && a.secondOpinion?.enabled && a.secondOpinion.available && !a.protected && (
            <Button size="sm" variant="ghost" busy={busy === 'second'} onClick={() => again(true)}>Compare with libbluray</Button>
          )}
        </div>
      )}
    </div>
  )
}

function ChosenLine({ a, name, busy, clear }: { a: DiscAnalysis; name: (n: number) => string; busy: string; clear: () => void }) {
  return (
    <p>
      <span className="pill">Chosen</span> {name(a.main)} was chosen as this disc’s film{a.chosenBy ? ` by ${a.chosenBy}` : ''}
      {a.engineMain >= 0 && a.engineMain !== a.main ? `; the engine picks ${name(a.engineMain)}` : ''}.
      {a.canChoose && <> <Button size="sm" variant="ghost" busy={busy === 'clear'} onClick={clear}>Use the engine’s pick</Button></>}
    </p>
  )
}

// DiscChoices lists a TV disc's episodes and the film's other versions, each to play.
export function DiscChoices({ item, rank, playable, unit }: { item: Item; rank: DiscRank; playable: boolean; unit: string }) {
  const nav = useNavigate()
  const go = (q: string) => nav(`/play/${item.id}?mode=title&${q}`)
  const versions = (rank.versions || []).filter((v) => !v.main)
  return (
    <>
      {rank.episodes && rank.episodes.length > 0 && (
        <section aria-labelledby="episodes-h">
          <h2 className="section-title" id="episodes-h">Episodes</h2>
          <ol className="disc-episodes">
            {rank.episodes.map((e) => (
              <li key={e.index}>
                <Button size="sm" icon={<IconPlay />} disabled={!playable} onClick={() => go(`episode=${e.index}`)}>Episode {e.index}</Button>
                <span className="muted"> {fmtTime(e.durationSec)}{e.part ? ` · from ${fmtTime(e.startSec)} of ${unit.toLowerCase()} ${e.ttn}` : ` · ${unit.toLowerCase()} ${e.ttn}`}</span>
              </li>
            ))}
          </ol>
        </section>
      )}
      {versions.length > 0 && (
        <section aria-labelledby="versions-h">
          <h2 className="section-title" id="versions-h">Other versions</h2>
          <ul className="disc-versions">
            {versions.map((v, i) => (
              <li key={i}>
                <Button size="sm" icon={<IconPlay />} disabled={!playable}
                  onClick={() => go(`ttn=${v.ttn}${v.kind === 'angle' && v.angle ? `&angle=${v.angle}` : ''}`)}>{v.label || `${unit} ${v.ttn}`}</Button>
                <span className="muted"> {fmtTime(v.durationSec)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
