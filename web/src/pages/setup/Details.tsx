// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import ConvertPanel from '../../wittyinfo/ConvertPanel'
import { useEffect, useState } from 'react'
import { api } from '../../api'
import LegalNote from '../../components/LegalNote'
import { IconCheck } from '../../icons'
import { Button } from '../../ui'
import { Fill, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { t, tk } from '../../i18n'

// Where posters, plots and ratings come from. These all work with nothing to set up.
const FREE = [
  { name: 'TVmaze', what: tk('TV shows') },
  { name: 'Wikidata and Wikipedia', what: tk('Movies') },
  { name: 'AniList', what: tk('Anime') },
]
const FREE_GAMES = [{ name: 'libretro', what: tk('Game pictures') }, { name: 'Wikidata and Wikipedia', what: tk('Game details') }]

export const PROVIDERS = [
  {
    id: 'tmdb', name: 'The Movie Database (TMDB)', for: tk('Movies, discs and TV'), key: 'tmdbKey',
    // A function, so the steps are in the language the page is in when they're shown.
    steps: () => [
      <Fill text={t('Make a free account at {site}.')} parts={{ site: <a href="https://www.themoviedb.org/signup" target="_blank" rel="noreferrer">themoviedb.org</a> }} />,
      <Fill text={t('Open its {link} and ask for a key. Choose “Personal use”.')} parts={{ link: <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">{t('API settings')}</a> }} />,
      <Fill text={t('Copy the {key} and paste it below.')} parts={{ key: <b>{t('API Key')}</b> }} />,
    ],
    note: tk('This product uses the TMDB API but is not endorsed or certified by TMDB.'),
  },
  {
    id: 'tvdb', name: 'TheTVDB', for: tk('TV shows'), key: 'tvdbKey', pinKey: 'tvdbPin',
    steps: () => [
      <Fill text={t('Make an account at {site}.')} parts={{ site: <a href="https://thetvdb.com/auth/register" target="_blank" rel="noreferrer">thetvdb.com</a> }} />,
      <Fill text={t('Open its {link} page and make a key.')} parts={{ link: <a href="https://thetvdb.com/api-information" target="_blank" rel="noreferrer">{t('API information')}</a> }} />,
      <Fill text={t('Paste the key below. If TheTVDB gave you a {pin}, add that too.')} parts={{ pin: <b>{t('subscriber PIN')}</b> }} />,
    ],
    note: tk('Metadata provided by TheTVDB (thetvdb.com).'),
  },
] as const

export function Details({ info, facts, next, back }: StepProps) {
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [state, setState] = useState<Record<string, { ok?: boolean; msg?: string; busy?: boolean }>>({})
  const [open, setOpen] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  // Whether anything is written into someone's media folders is worth asking, not assuming.
  const [beside, setBeside] = useState(true)
  // A conversion chosen and not finished holds Continue (ConvertPanel).
  const [converting, setConverting] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setBeside(s.saveMetadata !== '0')).catch(() => {}) }, [])
  const test = async (p: (typeof PROVIDERS)[number]) => {
    setState((s) => ({ ...s, [p.id]: { busy: true } }))
    try {
      const r = await api.post<{ ok: boolean; error?: string }>('/metadata/test', { provider: p.id, key: keys[p.key] || '', pin: 'pinKey' in p ? keys[p.pinKey] || '' : '' })
      setState((s) => ({ ...s, [p.id]: { ok: r.ok, msg: r.ok ? t('Connected') : r.error } }))
    } catch (e: any) { setState((s) => ({ ...s, [p.id]: { ok: false, msg: e.message } })) }
  }
  const save = async () => {
    setSaving(true)
    const body: Record<string, string> = { saveMetadata: beside ? '1' : '0' }
    for (const [k, v] of Object.entries(keys)) if (v.trim()) body[k] = v.trim()
    try { await api.put('/settings', body) } catch { /* all of this can be changed later */ }
    setSaving(false)
    next()
  }
  const any = Object.values(keys).some((v) => v.trim())
  const sources = facts.modules.includes('wittyplay') ? [...FREE, ...FREE_GAMES] : FREE
  return (
    <section>
      <StepHead title={t('Posters and details')} lead={t('Your server looks up posters, plots and ratings for what you own. These sources work already, for free, with nothing to set up. Names taken from your files and folders are sent to them to look things up. Your files stay on this computer.')} />
      <ul className="oobe-agents su-plain" aria-label={t('Sources that work already')}>
        {sources.map((a) => <li key={a.name + a.what} className="oobe-agent"><IconCheck size={16} /><b>{a.name}</b><span>{t(a.what)}</span><em>{t('Nothing to set up')}</em></li>)}
      </ul>
      <div className="su-gap">
        <TickRow checked={beside} onChange={setBeside} title={t('Save posters and details beside your files')}
          text={t('They stay with your media if you ever move it. Your videos and music aren’t changed. Untick this to leave your media folders exactly as they are, and keep it all on this server instead.')} />
      </div>
      <h2 className="oobe-sub">{t('Optional: more posters and better matches')}</h2>
      <p className="muted oobe-hint">{t('These two give you a free key: a code that lets your server ask them for things. Each takes about two minutes. It’s fine to skip this, and to do it another day.')}</p>
      <div className="oobe-providers">
        {PROVIDERS.map((p) => {
          const st = state[p.id] || {}
          const shown = open === p.id || !!keys[p.key]
          return (
            <div key={p.id} className={'oobe-provider' + (st.ok ? ' ok' : '')}>
              <button type="button" className="op-head" onClick={() => setOpen(open === p.id ? null : p.id)} aria-expanded={shown}>
                <b>{p.name}</b><span className="pill">{t(p.for)}</span>
                {st.ok ? <span className="op-ok"><IconCheck size={14} /> {t('Connected')}</span> : <span className="op-add">{shown ? t('Hide') : t('Add a key')}</span>}
              </button>
              {shown && (
                <>
                  <ol className="op-steps">{p.steps().map((s, i) => <li key={i}>{s}</li>)}</ol>
                  <div className="field-row">
                    <input className="input" aria-label={t('{name} key', { name: p.name })} placeholder={t('Paste your key')} value={keys[p.key] || ''} spellCheck={false} autoComplete="off"
                      onChange={(e) => { setKeys((k) => ({ ...k, [p.key]: e.target.value })); setState((s) => ({ ...s, [p.id]: {} })) }} />
                    {'pinKey' in p && <input className="input pin" aria-label={t('Subscriber PIN')} placeholder={t('PIN (optional)')} value={keys[p.pinKey] || ''} onChange={(e) => setKeys((k) => ({ ...k, [p.pinKey]: e.target.value }))} />}
                    <button type="button" className="btn" aria-busy={st.busy || undefined} onClick={() => { if (keys[p.key]?.trim() && !st.busy) void test(p) }}>{st.busy ? t('Checking…') : t('Test')}</button>
                  </div>
                  {st.msg && !st.ok && <div className="error" role="alert">{st.msg}</div>}
                  {st.ok && <span className="sr-only" role="status">{t('Connected')}</span>}
                  <div className="muted op-note">{t(p.note)}</div>
                </>
              )}
            </div>
          )
        })}
      </div>
      <h2 className="oobe-sub">{t('What your libraries already have')}</h2>
      <p className="muted oobe-hint">{t('NFO files, posters, lyrics and game details already beside your media can be gathered into WittyMediaServer’s own files, or the other way round. Choose for each kind and press Convert now: Continue waits until it is done.')}</p>
      <ConvertPanel onBusy={setConverting} beforeStart={() => api.put('/settings', { saveMetadata: beside ? '1' : '0' })} />
      <LegalNote kind="webResults" newTab className="su-gap" />
      <LaterNote info={info} />
      {converting && <p className="muted oobe-hint" role="status">{t('Convert what you chose, or choose “Leave them as they are”, to continue.')}</p>}
      <StepActions back={back}><Button variant="primary" size="big" busy={saving} disabled={converting} onClick={save}>{any ? t('Save and continue') : t('Continue')}</Button></StepActions>
    </section>
  )
}
