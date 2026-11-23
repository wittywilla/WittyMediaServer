// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import ConvertPanel from '../../wittyinfo/ConvertPanel'
import { useEffect, useState } from 'react'
import { api } from '../../api'
import LegalNote from '../../components/LegalNote'
import { IconCheck } from '../../icons'
import { PartNote } from '../../tvmode/OnAComputer'
import { useTvMode } from '../../tvmode/mode'
import { Fill, type StepProps } from './parts'
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

// Posters and details, in the Ready step's Customize: the free sources that work already, the two that take a key, and
// the details already beside the media. A key is saved as soon as it checks out.
export function Details({ facts }: Pick<StepProps, 'facts'>) {
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [state, setState] = useState<Record<string, { ok?: boolean; msg?: string; busy?: boolean }>>({})
  const [open, setOpen] = useState<string | null>(null)
  // Asked on the Libraries step, before the first scan: whether details go beside the media files.
  const [beside, setBeside] = useState(true)
  // Pasting a key with a remote is no way to spend an evening: on a TV it's for a computer.
  const tv = useTvMode()
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setBeside(s.saveMetadata !== '0')).catch(() => {}) }, [])
  const test = async (p: (typeof PROVIDERS)[number]) => {
    setState((s) => ({ ...s, [p.id]: { busy: true } }))
    try {
      const r = await api.post<{ ok: boolean; error?: string }>('/metadata/test', { provider: p.id, key: keys[p.key] || '', pin: 'pinKey' in p ? keys[p.pinKey] || '' : '' })
      if (r.ok) await api.put('/settings', { [p.key]: (keys[p.key] || '').trim(), ...('pinKey' in p && keys[p.pinKey]?.trim() ? { [p.pinKey]: keys[p.pinKey].trim() } : {}) })
      setState((s) => ({ ...s, [p.id]: { ok: r.ok, msg: r.ok ? t('Connected') : r.error } }))
    } catch (e: any) { setState((s) => ({ ...s, [p.id]: { ok: false, msg: e.message } })) }
  }
  const sources = facts.modules.includes('wittyplay') ? [...FREE, ...FREE_GAMES] : FREE
  return (
    <div>
      <p className="muted oobe-hint">{t('These work already, for free. Names from your files and folders are sent to them to look things up.')}</p>
      <ul className="oobe-agents su-plain" aria-label={t('Sources that work already')}>
        {sources.map((a) => <li key={a.name + a.what} className="oobe-agent"><IconCheck size={16} /><b>{a.name}</b><span>{t(a.what)}</span><em>{t('Nothing to set up')}</em></li>)}
      </ul>
      <h3 className="oobe-sub">{t('More posters and better matches')}</h3>
      <p className="muted oobe-hint">{t('Each gives you a free key in about two minutes.')}</p>
      {tv ? <PartNote title={t('Adding keys')} path="/settings/metadata" /> : <div className="oobe-providers">
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
      </div>}
      <h3 className="oobe-sub">{t('Details already beside your media')}</h3>
      <ConvertPanel beforeStart={() => api.put('/settings', { saveMetadata: beside ? '1' : '0' })} />
      <LegalNote kind="webResults" newTab className="su-gap" />
    </div>
  )
}
