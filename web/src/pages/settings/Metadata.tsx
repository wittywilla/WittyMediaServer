// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { IconDown, IconEdit, IconPlug, IconPlus, IconTrash, IconUp } from '../../icons'
import { toast } from '../../state'
import { MusicOnlineSettings } from '../MusicOnline'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'
import LegalNote from '../../components/LegalNote'
import { Link } from 'react-router-dom'
import { ListingSettings, LogoSettings } from './MetadataOptional'
import { ask, Dialog } from '../../ui/Overlay'
import { t, tk, tx } from '../../i18n'

// Where titles, posters, plots and ratings come from: the built-in agents
// (some need a free API key), the owner's own custom agents, and what is
// written back next to the media.

type Agent = { id: string; name: string; url: string; header: string; kinds: string[]; enabled: boolean }
type AgentResult = { title: string; year?: number; overview?: string; genres?: string[]; rating?: number; poster?: string }
const KIND_NAMES: Record<string, string> = { movies: tk('Movies'), discs: tk('DVDs & Blu-rays'), videos: tk('Home videos'), books: tk('Books') }
const NO_KEY = tk('No API key needed')

export default function MetadataSettings() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return st.failed ? <LoadError what={t('This page')} onRetry={st.retry} /> : null
  return (
    <>
      <PageHead title={tk('Metadata agents')}
        desc={t('Where titles, posters, plots and ratings come from. Information inside your files and NFO files always comes first; then your custom agents, in order; then the built-in agents.')}
        actions={<Link className="btn" to="/fix-matches">{t('Fix matches')}</Link>} />

      <Section title={tk('Built-in agents')} desc={t('Agents marked “No API key needed” work straight away. The others need a free key from their website.')}>
        <Row label="TVmaze" badge={tx(NO_KEY)} desc={t('TV shows: episode titles, plots, posters and stills. Used until you add a TheTVDB key. Data shared under CC BY-SA 4.0.')}>
          <Switch label={t('Use TVmaze')} checked={on('tvmaze')} onChange={(v) => set('tvmaze', v ? '1' : '0')} />
        </Row>
        <Row label={t('Wikidata and Wikipedia')} badge={tx(NO_KEY)} desc={t('Movies: facts, ratings and plot summaries (no posters). Used until you add a TMDB key.')}>
          <Switch label={t('Use Wikidata')} checked={on('wikidata')} onChange={(v) => set('wikidata', v ? '1' : '0')} />
        </Row>
        <Row label="Radio Browser" badge={tx(NO_KEY)} desc={t('Internet radio: a free community directory of stations to search, with logos, genres and countries. Your server contacts it when you search, and once a day for stations you added from it, sending only their IDs. It is never told what anyone listens to.')}>
          <Switch label={t('Use Radio Browser')} checked={s.radioDirectory !== '0'} onChange={(v) => set('radioDirectory', v ? '1' : '0')} />
        </Row>
        <Row label={t('Check radio stations in the background')} desc={t('Tries each station about once a week, so ones that have gone quiet are marked even if nobody has listened lately. This contacts stations while nobody is listening. Without it, a station is checked when someone plays it, when you ask, and through the directory for stations from there.')}>
          <Switch label={t('Check radio stations in the background')} checked={s.radioChecks === '1'} onChange={(v) => set('radioChecks', v ? '1' : '0')} />
        </Row>
        <Row label={t('MusicBrainz and the Cover Art Archive')} badge={tx(NO_KEY)} desc={t('Audio CD images: album and track names and covers. A disc is looked up by its table of contents, the track lengths and nothing else: no names, no audio. Album data is CC0.')}>
          <Switch label={t('Use MusicBrainz')} checked={on('musicbrainz')} onChange={(v) => set('musicbrainz', v ? '1' : '0')} />
        </Row>
        <Row label="AniList" badge={tx(NO_KEY)} desc={t('Anime libraries: titles in your chosen language, plots, posters and episode numbering. Anime titles are sent to AniList.')}>
          <span className="set-pill">{t('Always on')}</span>
        </Row>
        <KeyRow provider="tmdb" label={t('The Movie Database (TMDB)')} desc={<>{t('Posters, plots and ratings for movies and TV. Get a free key at')} <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">themoviedb.org</a>. {t('This product uses the TMDB API but is not endorsed or certified by TMDB.')}</>}
          value={s.tmdbKey || ''} onChange={(v) => set('tmdbKey', v)} placeholder={t('API key (v3)')} />
        <KeyRow provider="tvdb" label="TheTVDB" desc={<>{t('Detailed TV episode information. Get a key at')} <a href="https://thetvdb.com/api-information" target="_blank" rel="noreferrer">thetvdb.com</a>. {t('Metadata provided by')} <a href="https://thetvdb.com" target="_blank" rel="noreferrer">TheTVDB</a>.</>}
          value={s.tvdbKey || ''} onChange={(v) => set('tvdbKey', v)} placeholder={t('API key (v4)')} pin={s.tvdbPin || ''} onPin={(v) => set('tvdbPin', v)} />
      </Section>

      <MusicOnlineSettings s={s} set={set} on={on} />

      <CustomAgents />

      <LogoSettings />
      <ListingSettings />

      <Section title={tk('Anime')}>
        <Row label={t('Where anime from TV libraries appears')} desc={t('Shows with the genre “Anime”. Needs an Anime library.')}>
          <select className="select" value={s.animeIn || 'both'} onChange={(e) => set('animeIn', e.target.value)}>
            <option value="both">{t('Anime and TV libraries')}</option>
            <option value="anime">{t('Only the Anime library')}</option>
          </select>
        </Row>
        <Row label={t('Title language')} desc={t('How anime titles are written.')}>
          <select className="select" value={s.animeTitles || 'english'} onChange={(e) => set('animeTitles', e.target.value)}>
            <option value="english">{t('English')}</option>
            <option value="romaji">{t('Rōmaji')}</option>
            <option value="native">{t('Japanese')}</option>
          </select>
        </Row>
      </Section>

      <Section title={tk('Artwork')}>
        <Row label={t('When a film or video has no poster')} desc={t("What a title gets when nothing beside it or from a source has a poster. Titles already in your libraries keep what they have until they're scanned again or you choose another; any title's poster, including a title poster, can be changed from its page.")}>
          <select className="select" value={s.noPoster || 'frame'} onChange={(e) => set('noPoster', e.target.value)}>
            <option value="frame">{t('A frame from the video')}</option>
            <option value="title">{t('A poster made from its title')}</option>
            <option value="none">{t('Nothing: a plain card')}</option>
          </select>
        </Row>
      </Section>

      <Section title={tk('Saving information')}>
        <Row label={t('Save information and artwork next to your media')} desc={t('Writes standard NFO files and poster.jpg / fanart.jpg that other media servers can read too. Existing NFO files are kept as .nfo.bak.')}>
          <Switch label={t('Save next to media')} checked={on('saveMetadata', '0')} onChange={(v) => set('saveMetadata', v ? '1' : '0')} />
        </Row>
      </Section>
      <LegalNote kind="webResults" className="set-legal" />
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
    try { await api.post('/metadata/test', { provider, key: value, pin }); setState('ok'); setMsg(t('Key accepted')) } catch (e: any) { setState('bad'); setMsg(e.message) }
  }
  return (
    <Row label={label} desc={desc} badge={value ? (masked ? t('Key saved') : undefined) : undefined} stack>
      <div className="set-key">
        <input className="input" value={value} onChange={(e) => { onChange(e.target.value); setState('') }} placeholder={placeholder} aria-label={t('{name} API key', { name: label })} spellCheck={false} autoComplete="off" />
        {onPin && <input className="input pin" value={pin} onChange={(e) => onPin(e.target.value)} placeholder={t('PIN (if your key has one)')} aria-label={t('TheTVDB subscriber PIN')} autoComplete="off" />}
        <button className="btn sm" onClick={test} disabled={!value || masked || state === 'testing'} title={masked ? t('Saved keys are hidden; paste the key again to test it') : t('Check the key with the service')}>{state === 'testing' ? t('Testing…') : t('Test')}</button>
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
  const remove = async (a: Agent) => { if (await ask({ title: t('Remove the agent “{name}”?', { name: a.name }), confirm: t('Remove'), danger: true })) store(agents!.filter((x) => x.id !== a.id)) }
  const newAgent = (): Agent => ({ id: 'agent-' + Math.random().toString(36).slice(2, 8), name: '', url: '', header: '', kinds: ['movies'], enabled: true })
  return (
    <Section title={tk('Custom agents')} desc={t("Your own web services, for catalogues the built-in agents don't cover: a family archive, a club's list, a script of your own. They run in this order and only fill in what is still empty.")}
      actions={<button className="btn sm primary" onClick={() => setEditing(newAgent())}><IconPlus /> {t('Add agent')}</button>}>
      {agents === null ? <div className="spinner" /> : agents.length === 0 ? (
        <div className="set-empty">
          <IconPlug size={28} />
          <b>{t('No custom agents yet')}</b>
          <p>{t('An agent is an address like')} <code>https://my-catalogue.local/lookup?title={'{title}'}&amp;year={'{year}'}</code> {t('that answers with JSON. The format is in the')} <Link to="/help/metadata#custom-agents">{t('metadata guide')}</Link>.</p>
        </div>
      ) : (
        <ol className="set-list">
          {agents.map((a, i) => (
            <li key={a.id} className={'set-item' + (a.enabled ? '' : ' off')}>
              <span className="set-item-icon"><IconPlug /></span>
              <div className="set-item-text">
                <b>{a.name}</b>
                <span>{hostOf(a.url)} · {a.kinds.map((k) => tx(KIND_NAMES[k]) || k).join(', ')}{a.header ? ' · ' + t('with API key') : ''}</span>
              </div>
              <div className="set-item-actions">
                <button className="btn sm ghost square" disabled={i === 0} onClick={() => move(i, -1)} aria-label={t('Run {name} earlier', { name: a.name })} title={t('Earlier')}><IconUp /></button>
                <button className="btn sm ghost square" disabled={i === agents.length - 1} onClick={() => move(i, 1)} aria-label={t('Run {name} later', { name: a.name })} title={t('Later')}><IconDown /></button>
                <button className="btn sm ghost square" onClick={() => setEditing(a)} aria-label={t('Edit {name}', { name: a.name })} title={t('Edit')}><IconEdit /></button>
                <button className="btn sm ghost square danger-hover" onClick={() => remove(a)} aria-label={t('Remove {name}', { name: a.name })} title={t('Remove')}><IconTrash /></button>
                <Switch label={t('{name} on', { name: a.name })} checked={a.enabled} onChange={(v) => store(agents.map((x) => (x.id === a.id ? { ...x, enabled: v } : x)))} />
              </div>
            </li>
          ))}
        </ol>
      )}
      {editing && <AgentEditor agent={editing} onClose={() => setEditing(null)}
        onSave={async (a) => { const list = agents || []; const exists = list.some((x) => x.id === a.id); if (await store(exists ? list.map((x) => (x.id === a.id ? a : x)) : [...list, a])) { setEditing(null); toast(exists ? t('Agent saved') : t('Agent added')) } }} />}
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
    <Dialog title={<>{agent.name ? t('Edit {name}', { name: agent.name }) : t('Add a custom agent')}</>} onClose={onClose} className="set-modal" actions={<><button className="btn ghost" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn primary" disabled={!valid} onClick={() => onSave({ ...a, name: a.name.trim(), url: a.url.trim(), header: a.header.trim() })}>{agent.name ? t('Save agent') : t('Add agent')}</button></>}>
        <div className="set-form">
          <label>{t('Name')}<input className="input" value={a.name} onChange={(e) => up({ name: e.target.value })} placeholder={t('Family archive')} autoFocus /></label>
          <label>{t('Address')}
            <input className="input mono" value={a.url} onChange={(e) => up({ url: e.target.value })} placeholder="https://example.local/lookup?title={title}&year={year}&type={type}" spellCheck={false} />
            <small>{t('Use')} <code>{'{title}'}</code>, <code>{'{year}'}</code> {t('and')} <code>{'{type}'}</code> {t('(movie, video or book) where they go.')}</small>
          </label>
          <label>{t('API key header')} <span className="muted">{t('(optional)')}</span>
            <input className="input mono" value={a.header} onChange={(e) => up({ header: e.target.value })} placeholder="Authorization: Bearer …" spellCheck={false} autoComplete="off" />
          </label>
          <fieldset className="set-kinds">
            <legend>{t('Use it for')}</legend>
            {Object.entries(KIND_NAMES).map(([k, l]) => (
              <label key={k} className={'set-chip' + (a.kinds.includes(k) ? ' on' : '')}>
                <input type="checkbox" checked={a.kinds.includes(k)} onChange={(e) => up({ kinds: e.target.checked ? [...a.kinds, k] : a.kinds.filter((x) => x !== k) })} />{tx(l)}
              </label>
            ))}
          </fieldset>
          <div className="set-test">
            <b>{t('Try it')}</b>
            <div className="set-test-row">
              <input className="input" value={q.title} onChange={(e) => setQ({ ...q, title: e.target.value })} placeholder={t('A title')} aria-label={t('Title to look up')} />
              <input className="input year" value={q.year} onChange={(e) => setQ({ ...q, year: e.target.value.replace(/\D/g, '').slice(0, 4) })} placeholder={t('Year')} aria-label={t('Year')} inputMode="numeric" />
              <button className="btn sm" onClick={test} disabled={!valid || !q.title.trim() || result?.busy}>{result?.busy ? t('Asking…') : t('Look up')}</button>
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
    </Dialog>
  )
}
