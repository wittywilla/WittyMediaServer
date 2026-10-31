import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, artPreviewUrl, mediaUrl, type GameCandidate, type GameDetailsInfo, type GameMetaStatus, type GameSource, type Item } from '../api'
import { IconCheck, IconClose, IconDown, IconLock, IconRefresh, IconSearch, IconUp } from '../icons'
import { toast } from '../state'
import { Row, Section, Switch } from './settings/ui'

// What's known about a game, and where it comes from: its ratings and pictures, fixing
// a wrong match, and choosing sources and keys. Shared by the game's page, the library
// page, Settings and the setup wizard. Everything here is the server's work: pictures
// come through the server, and a source's address or key never reaches this page.

const SOURCE_NAMES: Record<string, string> = { libretro: 'libretro database', libretroart: 'libretro thumbnails', wikidata: 'Wikidata', igdb: 'IGDB', rawg: 'RAWG', tgdb: 'TheGamesDB', screenscraper: 'ScreenScraper' }
export const sourceName = (id: string) => SOURCE_NAMES[id] || id
const KIND_LABEL: Record<string, string> = { screenshot: 'Screenshot', title: 'Title screen', artwork: 'Artwork' }

// A rating out of 100, shown with who gave it. Colour never carries the meaning.
export function GameRatings({ d }: { d: GameDetailsInfo }) {
  if (d.scores.length === 0 && d.ages.length === 0) return null
  return (
    <div className="gm-ratings">
      {d.scores.length > 0 && (
        <ul className="gm-scores" aria-label="Scores">
          {d.scores.map((s) => (
            <li key={s.source + s.kind}>
              <b>{Math.round(s.value)}<small>/100</small></b>
              <span>{s.url ? <a href={s.url} target="_blank" rel="noreferrer">{s.source}</a> : s.source}</span>
              <small>{s.kind === 'critic' ? 'Critics' : 'Players'}{s.votes ? ` · ${s.votes.toLocaleString()} ${s.votes === 1 ? 'vote' : 'votes'}` : ''}</small>
            </li>
          ))}
        </ul>
      )}
      {d.ages.length > 0 && (
        <ul className="gm-ages" aria-label="Age ratings">
          {d.ages.map((a) => <li key={a.system}><span className="pill">{a.system} {a.value}</span></li>)}
        </ul>
      )}
    </div>
  )
}

// Screenshots: fetched by the server the first time someone looks, so nothing loads
// until the page scrolls to them.
export function GameShots({ d, title }: { d: GameDetailsInfo; title: string }) {
  const [open, setOpen] = useState<number | null>(null)
  const [broken, setBroken] = useState<Record<number, boolean>>({})
  const shots = d.art.map((a, i) => ({ ...a, i })).filter((a) => !broken[a.i])
  if (shots.length === 0) return null
  const shown = shots.find((a) => a.i === open)
  return (
    <section aria-label="Pictures">
      <h2 className="shelf-head">Pictures</h2>
      <ul className="gm-shots">
        {shots.map((a) => (
          <li key={a.i}>
            <button type="button" onClick={() => setOpen(a.i)} aria-label={`${KIND_LABEL[a.kind] || 'Picture'} from ${title}: open larger`}>
              <img src={mediaUrl(a.url)} alt="" loading="lazy" onError={() => setBroken((b) => ({ ...b, [a.i]: true }))} />
            </button>
          </li>
        ))}
      </ul>
      {shown && (
        <div className="modal-backdrop" onClick={() => setOpen(null)}>
          <div className="modal gm-shot-modal" onClick={(e) => e.stopPropagation()}>
            <h2>{KIND_LABEL[shown.kind] || 'Picture'}</h2>
            <img src={mediaUrl(shown.url)} alt={`${KIND_LABEL[shown.kind] || 'Picture'} from ${title}`} />
            <div className="actions"><button className="btn primary" onClick={() => setOpen(null)} autoFocus>Close</button></div>
          </div>
        </div>
      )}
    </section>
  )
}

// Who to thank, as each source's terms ask.
export function GameCredits({ d }: { d: GameDetailsInfo }) {
  if (d.credits.length === 0) return null
  return (
    <p className="fine-print gm-credits">
      Details from{' '}
      {d.credits.map((c, i) => (
        <span key={c.source}>
          {i > 0 ? '; ' : ''}<a href={c.url || c.site} target="_blank" rel="noreferrer">{c.name}</a> ({c.license})
        </span>
      ))}.
    </p>
  )
}

const methodWords = (d: GameDetailsInfo) => {
  if (!d.matchSource) return 'No match found'
  const who = sourceName(d.matchSource)
  switch (d.matchMethod) {
    case 'checksum': return `Recognised by its exact file (${who})`
    case 'serial': return `Recognised by its disc's product code (${who})`
    case 'manual': return `Chosen by hand (${who})`
  }
  return `Matched by name, ${d.confidence}% sure (${who})`
}

// The administrator's view of a match, with what can be done about it.
export function GameMatchBar({ it, d, onChanged }: { it: Item; d: GameDetailsInfo; onChanged: () => void }) {
  const [fixing, setFixing] = useState(false)
  const lock = async (on: boolean) => {
    try { await api.put(`/games/${it.id}/metadata/match`, { lock: on }); toast(on ? 'Match locked' : 'Match unlocked'); onChanged() } catch (e: any) { toast(e.message) }
  }
  const refresh = async () => {
    try { await api.post(`/games/${it.id}/metadata/refresh`); toast('Looking this game up again'); onChanged() } catch (e: any) { toast(e.message) }
  }
  return (
    <section className="gm-match" aria-label="Match">
      <p>
        {d.locked && <IconLock size={14} />} {d.status === 'pending' && !d.matchSource ? 'Being looked up…' : methodWords(d)}{d.locked ? ', locked' : ''}.
        {d.crcs && <small className="muted"> Checksum {d.crcs.split(',')[0]}{d.serial ? `, code ${d.serial}` : ''}</small>}
      </p>
      <div className="gm-match-actions">
        <button className="btn sm" onClick={() => setFixing(true)}><IconSearch /> {d.matchSource ? 'Fix match' : 'Find a match'}</button>
        {d.matchSource && <button className="btn sm ghost" onClick={() => lock(!d.locked)}><IconLock /> {d.locked ? 'Unlock' : 'Lock'}</button>}
        <button className="btn sm ghost" onClick={refresh}><IconRefresh /> Look up again</button>
      </div>
      {fixing && <GameMatchDialog it={it} onClose={() => setFixing(false)} onDone={() => { setFixing(false); onChanged() }} />}
    </section>
  )
}

function GameMatchDialog({ it, onClose, onDone }: { it: Item; onClose: () => void; onDone: () => void }) {
  const [q, setQ] = useState(it.title)
  const [results, setResults] = useState<GameCandidate[] | null>(null)
  const [failed, setFailed] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [lock, setLock] = useState(true)
  const search = async () => {
    setBusy(true)
    try {
      const r = await api.get<{ results: GameCandidate[]; failed: string[] }>(`/games/${it.id}/metadata/search?q=${encodeURIComponent(q)}`)
      setResults(r.results)
      setFailed(r.failed)
    } catch (e: any) { toast(e.message); setResults([]) } finally { setBusy(false) }
  }
  useEffect(() => { search() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const pick = async (c: GameCandidate) => {
    setBusy(true)
    try { await api.put(`/games/${it.id}/metadata/match`, { source: c.source, id: c.id, lock }); toast('Match saved'); onDone() } catch (e: any) { toast(e.message); setBusy(false) }
  }
  const none = async () => {
    try { await api.del(`/games/${it.id}/metadata/match`); toast('Match removed'); onDone() } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal gm-match-modal" onClick={(e) => e.stopPropagation()}>
        <h2>Which game is this?</h2>
        <form className="field-row" onSubmit={(e) => { e.preventDefault(); search() }}>
          <input className="input" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Game's name" autoFocus />
          <button className="btn" disabled={busy || !q.trim()}><IconSearch /> Search</button>
        </form>
        <label className="gm-check"><input type="checkbox" checked={lock} onChange={(e) => setLock(e.target.checked)} /> Lock this match, so it stays as chosen</label>
        {failed.length > 0 && <div className="set-note bad" role="status">{failed.join(', ')} didn’t answer. Try again in a while for {failed.length === 1 ? 'its' : 'their'} results.</div>}
        {results === null ? <div className="spinner" /> : results.length === 0
          ? <p className="muted" role="status">Nothing found. Try fewer words, or the game’s name in another region.</p>
          : (
            <ul className="gm-results" aria-label="Results">
              {results.map((c) => (
                <li key={c.source + c.id}>
                  <button type="button" disabled={busy} onClick={() => pick(c)}>
                    <span className="gm-result-art">{c.poster && <img src={artPreviewUrl(c.poster)} alt="" loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} />}</span>
                    <span className="gm-result-text"><b>{c.title}</b><small className="muted">{[c.year, sourceName(c.source), `${c.score}% alike`].filter(Boolean).join(' · ')}</small></span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        <div className="actions">
          <button className="btn ghost" onClick={none}>It has no match</button>
          <button className="btn" onClick={onClose}>Cancel</button>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- the library page

const dismissKey = 'wms.gameKeyHint'
const hintDismissed = () => { try { return localStorage.getItem(dismissKey) === '1' } catch { return false } }

// For administrators: how the lookup is going, and a nudge toward a key. Tells the page
// when there's more to show.
export function GameMetaNotice({ onProgress }: { onProgress: () => void }) {
  const [st, setSt] = useState<GameMetaStatus | null>(null)
  const [hidden, setHidden] = useState(hintDismissed)
  useEffect(() => {
    let stop = false
    let timer = 0
    let was = -1
    const tick = async () => {
      const r = await api.get<GameMetaStatus>('/gamemeta/status').catch(() => null)
      if (stop || !r) return
      setSt(r)
      const left = r.pending + r.pictures
      if (was !== -1 && left !== was) onProgress()
      was = left
      if (r.running || left > 0) timer = window.setTimeout(tick, r.running ? 5000 : 30000)
    }
    void tick()
    return () => { stop = true; clearTimeout(timer) }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  if (!st || st.total === 0) return null
  const busy = st.pending > 0
  const unmatched = st.total - st.matched
  return (
    <>
      {(busy || st.pictures > 0) && (
        <p className="gm-progress muted" role="status">
          {busy ? `Finding details: ${(st.total - st.pending).toLocaleString()} of ${st.total.toLocaleString()} games` : `Fetching pictures: ${st.pictures.toLocaleString()} games to go`}
          {st.paused.length > 0 && <> · {st.paused.map((p) => sourceName(p.source)).join(', ')} paused</>}
        </p>
      )}
      {!st.keysSet && !hidden && !busy && (
        <div className="warn-box gm-hint" role="status">
          <span><strong>Add a key for better matches.</strong> {unmatched > 0 ? `${unmatched.toLocaleString()} of your games have no match yet. ` : ''}A free key adds scores, age ratings and art for more games.{' '}
            <Link to="/settings/metadata#games">Choose a source</Link></span>
          <button className="btn sm ghost square" aria-label="Don't show this again" title="Don't show this again" onClick={() => { try { localStorage.setItem(dismissKey, '1') } catch { /* it just comes back */ } setHidden(true) }}><IconClose /></button>
        </div>
      )}
    </>
  )
}

// ---------------------------------------------------------------- keys

// The fields a source needs, with a test that says plainly whether they work. A saved
// value isn't shown: the field says it's saved, and stays empty unless it's replaced.
export function GameKeyFields({ source, values, onChange }: { source: GameSource; values: Record<string, string>; onChange: (setting: string, v: string) => void }) {
  const [state, setState] = useState<{ ok?: boolean; msg?: string; busy?: boolean }>({})
  const fields = source.fields.filter((f) => !f.hint || source.unavailable)
  const test = async () => {
    setState({ busy: true })
    const send: Record<string, string> = {}
    for (const f of source.fields) if (values[f.setting]?.trim()) send[f.setting] = values[f.setting].trim()
    try {
      const r = await api.post<{ ok: boolean; error?: string; message?: string }>(`/gamemeta/sources/${source.id}/test`, { fields: send })
      setState({ ok: r.ok, msg: r.ok ? r.message : r.error })
    } catch (e: any) { setState({ ok: false, msg: e.message }) }
  }
  const canTest = fields.every((f) => f.hint || values[f.setting]?.trim() || source.saved.includes(f.setting))
  return (
    <div className="gm-key">
      {source.keyUrl && <p><a href={source.keyUrl} target="_blank" rel="noreferrer">Get a key</a> · {source.cost}</p>}
      {source.unavailable && <div className="set-note bad">{source.unavailable}</div>}
      {source.steps && <ol className="op-steps">{source.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>}
      <div className="gm-key-fields">
        {fields.map((f) => (
          <label key={f.setting}>{f.label}
            <input className="input" type={/password|secret/i.test(f.setting) ? 'password' : 'text'} value={values[f.setting] || ''} spellCheck={false} autoComplete="off"
              placeholder={source.saved.includes(f.setting) ? 'Saved. Type to replace it' : f.hint || ''}
              onChange={(e) => { onChange(f.setting, e.target.value); setState({}) }} />
          </label>
        ))}
        <button type="button" className="btn" disabled={!canTest || state.busy} onClick={test}>{state.busy ? 'Testing…' : 'Test'}</button>
      </div>
      {state.msg && <div className={'set-note ' + (state.ok ? 'ok' : 'bad')} role="status">{state.ok && <IconCheck size={14} />} {state.msg}</div>}
    </div>
  )
}

// ---------------------------------------------------------------- settings

export function GameSourcesSettings() {
  const [sources, setSources] = useState<GameSource[] | null>(null)
  const [status, setStatus] = useState<GameMetaStatus | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [open, setOpen] = useState<string | null>(null)
  const load = () => api.get<{ sources: GameSource[] }>('/gamemeta/sources').then((r) => setSources(r.sources)).catch((e) => toast(e.message))
  useEffect(() => { load(); api.get<GameMetaStatus>('/gamemeta/status').then(setStatus).catch(() => {}) }, [])
  // Arriving from the Games page's "Add a key".
  const anchored = useRef(false)
  useEffect(() => {
    if (sources && !anchored.current && window.location.hash === '#games') {
      anchored.current = true
      document.getElementById('games')?.scrollIntoView()
    }
  }, [sources])
  const store = async (list: GameSource[]) => {
    setSources(list)
    try {
      const r = await api.put<{ sources: GameSource[] }>('/gamemeta/sources', { sources: list.map((s) => ({ id: s.id, on: s.on })) })
      setSources(r.sources)
    } catch (e: any) { toast(e.message); load() }
  }
  const move = (i: number, d: number) => { const l = [...sources!]; const [x] = l.splice(i, 1); l.splice(i + d, 0, x); store(l) }
  const saveKeys = async (s: GameSource, clear = false) => {
    const body: Record<string, string> = {}
    for (const f of s.fields) {
      if (clear) body[f.setting] = ''
      else if (values[f.setting]?.trim()) body[f.setting] = values[f.setting].trim()
    }
    try {
      await api.put('/settings', body)
      setValues((v) => { const n = { ...v }; for (const f of s.fields) delete n[f.setting]; return n })
      toast(clear ? 'Key removed' : 'Key saved. Your games will be looked up again in the background.')
      load()
    } catch (e: any) { toast(e.message) }
  }
  const refetch = async () => {
    if (!confirm('Forget what every source said about every game, and ask again? Games you locked stay as they are.')) return
    try { await api.post('/gamemeta/refetch'); toast('Looking up every game again') } catch (e: any) { toast(e.message) }
  }
  const consoles = status ? Object.entries(status.consoles).filter(([, c]) => c.total > 0).sort((a, b) => b[1].total - a[1].total) : []
  return (
    <Section id="games" title="Video games" desc="Sources run in the background, batched and at each source's own pace. Your game files never leave this server: only their names, checksums or sizes are looked up, as each source says below. Higher in the list is believed first."
      actions={<button className="btn sm ghost" onClick={refetch}><IconRefresh /> Look up all again</button>}>
      {sources === null ? <div className="spinner" /> : (
        <ol className="gm-sources">
          {sources.map((s, i) => {
            const needsKey = s.fields.length > 0
            const shown = open === s.id
            return (
              <li key={s.id} className={'gm-source' + (s.on ? '' : ' off')}>
                <div className="gm-source-text">
                  <div className="gm-source-head"><b>{s.name}</b><span className="pill">{!needsKey ? 'No API key needed' : s.ready ? 'Key saved' : 'Needs a key'}</span><span className="muted">{s.cost}</span></div>
                  <p>{s.why}</p>
                  {s.unavailable && <div className="set-note bad">{s.unavailable}</div>}
                  <details>
                    <summary>What it sends and its limits</summary>
                    <dl>
                      <dt>Sends</dt><dd>{s.sends}.</dd>
                      <dt>Limit</dt><dd>{s.limit}{s.allowance ? ` Used ${(s.used || 0).toLocaleString()} of ${s.allowance.toLocaleString()} ${s.period === 'day' ? 'today' : `this ${s.period}`}.` : ''}</dd>
                      <dt>Licence</dt><dd>{s.license}.</dd>
                      <dt>Gives</dt><dd>{s.gives.join(', ')}.</dd>
                    </dl>
                    <a href={s.site} target="_blank" rel="noreferrer">{s.name} website</a>
                  </details>
                </div>
                <div className="gm-source-actions">
                  {needsKey && <button className="btn sm" aria-expanded={shown} onClick={() => setOpen(shown ? null : s.id)}>{s.ready ? 'Change key' : 'Add a key'}</button>}
                  <button className="btn sm ghost square" disabled={i === 0} onClick={() => move(i, -1)} aria-label={`Believe ${s.name} sooner`} title="Sooner"><IconUp /></button>
                  <button className="btn sm ghost square" disabled={i === sources.length - 1} onClick={() => move(i, 1)} aria-label={`Believe ${s.name} later`} title="Later"><IconDown /></button>
                  <Switch label={`${s.name} on`} checked={s.on} onChange={(v) => store(sources.map((x) => (x.id === s.id ? { ...x, on: v } : x)))} />
                </div>
                {shown && (
                  <div className="gm-source-key">
                    <GameKeyFields source={s} values={values} onChange={(k, v) => setValues((c) => ({ ...c, [k]: v }))} />
                    <div className="gm-key-save">
                      <button className="btn sm primary" disabled={!s.fields.some((f) => values[f.setting]?.trim())} onClick={() => saveKeys(s)}>Save key</button>
                      {s.saved.length > 0 && <button className="btn sm ghost" onClick={() => saveKeys(s, true)}>Remove key</button>}
                    </div>
                  </div>
                )}
              </li>
            )
          })}
        </ol>
      )}
      {status && status.total > 0 && (
        <Row label="How it's going" desc={status.pending > 0 ? `${status.pending.toLocaleString()} still to look up.` : status.pictures > 0 ? `${status.pictures.toLocaleString()} still to fetch pictures for.` : 'Finished for now.'} stack>
          <table className="gm-tally">
            <thead><tr><th scope="col">Console</th><th scope="col">Games</th><th scope="col">Matched</th><th scope="col">By exact file</th></tr></thead>
            <tbody>
              {consoles.map(([id, c]) => <tr key={id}><td>{id.toUpperCase()}</td><td>{c.total.toLocaleString()}</td><td>{c.matched.toLocaleString()}</td><td>{c.byChecksum.toLocaleString()}</td></tr>)}
            </tbody>
          </table>
        </Row>
      )}
    </Section>
  )
}
