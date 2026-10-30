import { useEffect, useState } from 'react'
import { api } from '../../api'
import { IconDown, IconEdit, IconPlug, IconPlus, IconTrash, IconUp } from '../../icons'
import { toast } from '../../state'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'

// Where titles, posters, plots and ratings come from: the built-in agents
// (some need a free API key), the owner's own custom agents, and what is
// written back next to the media.

type Agent = { id: string; name: string; url: string; header: string; kinds: string[]; enabled: boolean }
type AgentResult = { title: string; year?: number; overview?: string; genres?: string[]; rating?: number; poster?: string }
const KIND_NAMES: Record<string, string> = { movies: 'Movies', videos: 'Home videos', books: 'Books' }
const NO_KEY = 'No API key needed'

export default function MetadataSettings() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return null
  return (
    <>
      <PageHead title="Media Indexers and Metadata Agents"
        desc="Where titles, posters, plots and ratings come from. Information inside your files and NFO files always comes first; then your custom agents, in order; then the built-in agents." />

      <Section title="Built-in agents" desc="Agents marked “No API key needed” work straight away. The others need a free key from their website.">
        <Row label="TVmaze" badge={NO_KEY} desc="TV shows: episode titles, plots, posters and stills. Used until you add a TheTVDB key. Data shared under CC BY-SA 4.0.">
          <Switch label="Use TVmaze" checked={on('tvmaze')} onChange={(v) => set('tvmaze', v ? '1' : '0')} />
        </Row>
        <Row label="Wikidata and Wikipedia" badge={NO_KEY} desc="Movies: facts, ratings and plot summaries (no posters). Used until you add a TMDB key.">
          <Switch label="Use Wikidata" checked={on('wikidata')} onChange={(v) => set('wikidata', v ? '1' : '0')} />
        </Row>
        <Row label="AniList" badge={NO_KEY} desc="Anime libraries: titles in your chosen language, plots, posters and episode numbering.">
          <span className="set-pill">Always on</span>
        </Row>
        <KeyRow provider="tmdb" label="The Movie Database (TMDB)" desc={<>Posters, plots and ratings for movies and TV. Get a free key at <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">themoviedb.org</a>. This product uses the TMDB API but is not endorsed by TMDB.</>}
          value={s.tmdbKey || ''} onChange={(v) => set('tmdbKey', v)} placeholder="API key (v3)" />
        <KeyRow provider="tvdb" label="TheTVDB" desc={<>Detailed TV episode information. Get a free key at <a href="https://thetvdb.com/api-information" target="_blank" rel="noreferrer">thetvdb.com</a>. Metadata provided by TheTVDB.</>}
          value={s.tvdbKey || ''} onChange={(v) => set('tvdbKey', v)} placeholder="API key (v4)" pin={s.tvdbPin || ''} onPin={(v) => set('tvdbPin', v)} />
      </Section>

      <CustomAgents />

      <Section title="Anime">
        <Row label="Where anime from TV libraries appears" desc="Shows with the genre “Anime”. Needs an Anime library.">
          <select className="select" value={s.animeIn || 'both'} onChange={(e) => set('animeIn', e.target.value)}>
            <option value="both">Anime and TV libraries</option>
            <option value="anime">Only the Anime library</option>
          </select>
        </Row>
        <Row label="Title language">
          <select className="select" value={s.animeTitles || 'english'} onChange={(e) => set('animeTitles', e.target.value)}>
            <option value="english">English (Attack on Titan)</option>
            <option value="romaji">Rōmaji (Shingeki no Kyojin)</option>
            <option value="native">Japanese (進撃の巨人)</option>
          </select>
        </Row>
      </Section>

      <Section title="Saving information">
        <Row label="Save information and artwork next to your media" desc="Writes standard NFO files and poster.jpg / fanart.jpg that other media servers can read too. Existing NFO files are kept as .nfo.bak.">
          <Switch label="Save next to media" checked={on('saveMetadata', '0')} onChange={(v) => set('saveMetadata', v ? '1' : '0')} />
        </Row>
      </Section>
      <SaveBar {...st} />
    </>
  )
}

// A key for a built-in agent, with a test that checks it with the service.
function KeyRow({ provider, label, desc, value, onChange, placeholder, pin, onPin }: {
  provider: 'tmdb' | 'tvdb'; label: string; desc: React.ReactNode; value: string; onChange: (v: string) => void; placeholder: string; pin?: string; onPin?: (v: string) => void
}) {
  const [state, setState] = useState<'' | 'testing' | 'ok' | 'bad'>('')
  const [msg, setMsg] = useState('')
  const masked = value.includes('•')
  const test = async () => {
    setState('testing')
    try { await api.post('/metadata/test', { provider, key: value, pin }); setState('ok'); setMsg('Key accepted') } catch (e: any) { setState('bad'); setMsg(e.message) }
  }
  return (
    <Row label={label} desc={desc} badge={value ? (masked ? 'Key saved' : undefined) : undefined} stack>
      <div className="set-key">
        <input className="input" value={value} onChange={(e) => { onChange(e.target.value); setState('') }} placeholder={placeholder} aria-label={`${label} API key`} spellCheck={false} autoComplete="off" />
        {onPin && <input className="input pin" value={pin} onChange={(e) => onPin(e.target.value)} placeholder="PIN (if your key has one)" aria-label="TheTVDB subscriber PIN" autoComplete="off" />}
        <button className="btn sm" onClick={test} disabled={!value || masked || state === 'testing'} title={masked ? 'Saved keys are hidden; paste the key again to test it' : 'Check the key with the service'}>{state === 'testing' ? 'Testing…' : 'Test'}</button>
      </div>
      {state && state !== 'testing' && <div className={'set-note ' + state}>{msg}</div>}
    </Row>
  )
}

// ---------------------------------------------------------------- custom agents

function CustomAgents() {
  const [agents, setAgents] = useState<Agent[] | null>(null)
  const [editing, setEditing] = useState<Agent | null>(null)
  useEffect(() => { api.get<{ agents: Agent[] }>('/agents').then((r) => setAgents(r.agents || [])).catch(() => setAgents([])) }, [])
  const store = async (list: Agent[]) => {
    try { const r = await api.put<{ agents: Agent[] }>('/agents', { agents: list }); setAgents(r.agents || []); return true } catch (e: any) { toast(e.message); return false }
  }
  const move = (i: number, d: number) => { const l = [...agents!]; const [a] = l.splice(i, 1); l.splice(i + d, 0, a); store(l) }
  const remove = (a: Agent) => { if (confirm(`Remove the agent “${a.name}”?`)) store(agents!.filter((x) => x.id !== a.id)) }
  const newAgent = (): Agent => ({ id: 'agent-' + Math.random().toString(36).slice(2, 8), name: '', url: '', header: '', kinds: ['movies'], enabled: true })
  return (
    <Section title="Custom agents" desc="Your own web services, for catalogues the built-in agents don't cover: a family archive, a club's list, a script of your own. They run in this order and only fill in what is still empty."
      actions={<button className="btn sm primary" onClick={() => setEditing(newAgent())}><IconPlus /> Add agent</button>}>
      {agents === null ? <div className="spinner" /> : agents.length === 0 ? (
        <div className="set-empty">
          <IconPlug size={28} />
          <b>No custom agents yet</b>
          <p>An agent is an address like <code>https://my-catalogue.local/lookup?title={'{title}'}&amp;year={'{year}'}</code> that answers with JSON. The format is in the <a href="https://github.com/wittywilla/WittyMediaServer/blob/master/docs/METADATA.md#custom-agents" target="_blank" rel="noreferrer">metadata guide</a>.</p>
        </div>
      ) : (
        <ol className="set-list">
          {agents.map((a, i) => (
            <li key={a.id} className={'set-item' + (a.enabled ? '' : ' off')}>
              <span className="set-item-icon"><IconPlug /></span>
              <div className="set-item-text">
                <b>{a.name}</b>
                <span>{hostOf(a.url)} · {a.kinds.map((k) => KIND_NAMES[k] || k).join(', ')}{a.header ? ' · with API key' : ''}</span>
              </div>
              <div className="set-item-actions">
                <button className="btn sm ghost square" disabled={i === 0} onClick={() => move(i, -1)} aria-label={`Run ${a.name} earlier`} title="Earlier"><IconUp /></button>
                <button className="btn sm ghost square" disabled={i === agents.length - 1} onClick={() => move(i, 1)} aria-label={`Run ${a.name} later`} title="Later"><IconDown /></button>
                <button className="btn sm ghost square" onClick={() => setEditing(a)} aria-label={`Edit ${a.name}`} title="Edit"><IconEdit /></button>
                <button className="btn sm ghost square danger-hover" onClick={() => remove(a)} aria-label={`Remove ${a.name}`} title="Remove"><IconTrash /></button>
                <Switch label={`${a.name} on`} checked={a.enabled} onChange={(v) => store(agents.map((x) => (x.id === a.id ? { ...x, enabled: v } : x)))} />
              </div>
            </li>
          ))}
        </ol>
      )}
      {editing && <AgentEditor agent={editing} onClose={() => setEditing(null)}
        onSave={async (a) => { const list = agents || []; const exists = list.some((x) => x.id === a.id); if (await store(exists ? list.map((x) => (x.id === a.id ? a : x)) : [...list, a])) { setEditing(null); toast(exists ? 'Agent saved' : 'Agent added') } }} />}
    </Section>
  )
}

const hostOf = (u: string) => { try { return new URL(u.replace(/\{[a-z]+\}/g, 'x')).host } catch { return u } }

function AgentEditor({ agent, onClose, onSave }: { agent: Agent; onClose: () => void; onSave: (a: Agent) => void }) {
  const [a, setA] = useState<Agent>(agent)
  const [q, setQ] = useState({ title: '', year: '' })
  const [result, setResult] = useState<{ r?: AgentResult; err?: string; busy?: boolean } | null>(null)
  const up = (p: Partial<Agent>) => setA({ ...a, ...p })
  const test = async () => {
    setResult({ busy: true })
    try {
      const r = await api.post<AgentResult>('/agents/test', { agent: a, title: q.title, year: q.year ? Number(q.year) : null, kind: a.kinds[0] || 'movies' })
      setResult({ r })
    } catch (e: any) { setResult({ err: e.message }) }
  }
  const valid = a.name.trim() && /^https?:\/\//.test(a.url) && a.url.includes('{title}') && a.kinds.length > 0
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal set-modal" onClick={(e) => e.stopPropagation()} role="dialog" aria-label="Custom agent">
        <h2>{agent.name ? `Edit ${agent.name}` : 'Add a custom agent'}</h2>
        <div className="set-form">
          <label>Name<input className="input" value={a.name} onChange={(e) => up({ name: e.target.value })} placeholder="Family archive" autoFocus /></label>
          <label>Address
            <input className="input mono" value={a.url} onChange={(e) => up({ url: e.target.value })} placeholder="https://example.local/lookup?title={title}&year={year}&type={type}" spellCheck={false} />
            <small>Use <code>{'{title}'}</code>, <code>{'{year}'}</code> and <code>{'{type}'}</code> (movie, video or book) where they go.</small>
          </label>
          <label>API key header <span className="muted">(optional)</span>
            <input className="input mono" value={a.header} onChange={(e) => up({ header: e.target.value })} placeholder="Authorization: Bearer …" spellCheck={false} autoComplete="off" />
          </label>
          <fieldset className="set-kinds">
            <legend>Use it for</legend>
            {Object.entries(KIND_NAMES).map(([k, l]) => (
              <label key={k} className={'set-chip' + (a.kinds.includes(k) ? ' on' : '')}>
                <input type="checkbox" checked={a.kinds.includes(k)} onChange={(e) => up({ kinds: e.target.checked ? [...a.kinds, k] : a.kinds.filter((x) => x !== k) })} />{l}
              </label>
            ))}
          </fieldset>
          <div className="set-test">
            <b>Try it</b>
            <div className="set-test-row">
              <input className="input" value={q.title} onChange={(e) => setQ({ ...q, title: e.target.value })} placeholder="A title, e.g. The Lorax" aria-label="Title to look up" />
              <input className="input year" value={q.year} onChange={(e) => setQ({ ...q, year: e.target.value.replace(/\D/g, '').slice(0, 4) })} placeholder="Year" aria-label="Year" inputMode="numeric" />
              <button className="btn sm" onClick={test} disabled={!valid || !q.title.trim() || result?.busy}>{result?.busy ? 'Asking…' : 'Look up'}</button>
            </div>
            {result?.err && <div className="set-note bad">{result.err}</div>}
            {result?.r && (
              <div className="set-preview">
                {result.r.poster && <img src={result.r.poster} alt="" referrerPolicy="no-referrer" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} />}
                <div>
                  <b>{result.r.title}{result.r.year ? ` (${result.r.year})` : ''}</b>
                  {(result.r.genres?.length || result.r.rating) && <span className="muted">{[result.r.genres?.slice(0, 3).join(', '), result.r.rating ? `★ ${result.r.rating}` : ''].filter(Boolean).join(' · ')}</span>}
                  {result.r.overview && <p>{result.r.overview.slice(0, 260)}{result.r.overview.length > 260 ? '…' : ''}</p>}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="actions">
          <button className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" disabled={!valid} onClick={() => onSave({ ...a, name: a.name.trim(), url: a.url.trim(), header: a.header.trim() })}>{agent.name ? 'Save agent' : 'Add agent'}</button>
        </div>
      </div>
    </div>
  )
}
