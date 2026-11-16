// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, profileName, type User } from '../../api'
import { bump, sduiAsk, SDUI_BASE } from '../../sdui/client'
import { loadThemeOverrides } from '../../theme'
import { toast } from '../../state'
import { Button } from '../../ui'
import { AddProfile } from '../Profiles'
import { getLooks, type LooksAnswer } from '../settings/pages/api'
import type { ThemeMeta } from '../../theme'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'
import { t } from '../../i18n'

// Step: People and rules. Profiles (a kids profile has limits of its own), and a look or theme to require of everyone
// or of kids, which then shows locked for them (docs/SDUI.md, "Experiences").
export function People({ info, next, back }: StepProps) {
  const [add, setAdd] = useState<'' | 'adult' | 'kids'>('')
  const [looks, setLooks] = useState<LooksAnswer | null>(null)
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  type Req = { experience: string; theme: string }
  const [req, setReq] = useState<{ everyone: Req; kids: Req; profiles: Record<string, Req> }>({ everyone: { experience: '', theme: '' }, kids: { experience: '', theme: '' }, profiles: {} })
  const [people, setPeople] = useState<User[]>([])
  const loadPeople = () => api.get<User[]>('/users').then(setPeople).catch(() => {})
  useEffect(() => { void loadPeople() }, [])
  useEffect(() => {
    getLooks(false).then((l) => {
      setLooks(l)
      const r = l.experiences?.required
      setReq({ everyone: { experience: r?.everyone?.experience || '', theme: r?.everyone?.theme || '' }, kids: { experience: r?.kids?.experience || '', theme: r?.kids?.theme || '' }, profiles: Object.fromEntries(Object.entries(r?.profiles || {}).map(([id, v]) => [id, { experience: v.experience || '', theme: v.theme || '' }])) })
    }).catch(() => {})
    api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  }, [])
  const save = async () => {
    try {
      if (looks) {
        const clean = (r: { experience: string; theme: string }) => ({ ...(r.experience ? { experience: r.experience } : {}), ...(r.theme ? { theme: r.theme } : {}) })
        const profiles = Object.fromEntries(Object.entries(req.profiles).filter(([, v]) => v.experience || v.theme).map(([id, v]) => [id, clean(v)]))
        const required = { ...(req.everyone.experience || req.everyone.theme ? { everyone: clean(req.everyone) } : {}), ...(req.kids.experience || req.kids.theme ? { kids: clean(req.kids) } : {}), ...(Object.keys(profiles).length ? { profiles } : {}) }
        const policy = { ...(looks.experiences || {}), required: Object.keys(required).length ? required : undefined }
        await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: policy, headers: { 'If-Match': looks.etag } })
        // A look or theme required of someone shows at once, in this very page, not after a reload.
        bump(); void loadThemeOverrides()
      }
    } catch { toast(t('The required look couldn’t be saved. You can set it in Settings, Pages & menu, Looks.'), { tone: 'danger' }) }
    next()
  }
  const get = (who: string): Req => (who === 'everyone' || who === 'kids' ? req[who] : req.profiles[who] || { experience: '', theme: '' })
  const put = (who: string, patch: Partial<Req>) => setReq((r) => (who === 'everyone' || who === 'kids' ? { ...r, [who]: { ...r[who], ...patch } } : { ...r, profiles: { ...r.profiles, [who]: { ...get(who), ...patch } } }))
  const row = (who: string, label: string) => (
    <div className="su-req" role="group" aria-label={t('Required for {who}', { who: label })}>
      <b>{label}</b>
      <select className="select" aria-label={t('Look required for {who}', { who: label })} value={get(who).experience} onChange={(e) => put(who, { experience: e.target.value })}>
        <option value="">{t('Free to choose a look')}</option>
        {looks?.options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
      </select>
      <select className="select" aria-label={t('Theme required for {who}', { who: label })} value={get(who).theme} onChange={(e) => put(who, { theme: e.target.value })}>
        <option value="">{t('Free to choose a theme')}</option>
        {themes.map((x) => <option key={x.id} value={x.id}>{x.name.replace(/^WittyMediaServer /, '')}</option>)}
      </select>
    </div>
  )
  return (
    <section>
      <StepHead title={t('People and rules')} lead={t('Everyone at home can have a profile of their own, with their own place in what they’re watching. A kids profile only sees what you allow. You can require a look or theme of everyone, or of kids, and it shows locked for them.')} />
      {people.length > 0 && <><h2 className="oobe-sub">{t('Profiles')}</h2><ul className="tvs-list" aria-label={t('Profiles')}>{people.map((u) => <li key={u.id} className="tvs-item"><b>{profileName(u)}</b> <span className="muted">{u.isAdmin ? t('Administrator') : u.kind === 'kids' ? t('Kids') : u.kind === 'guest' ? t('Guest') : t('Adult')}</span></li>)}</ul></>}
      <div className="su-gap">
        <Button onClick={() => setAdd('adult')}>{t('Add a profile')}</Button>{' '}
        <Button onClick={() => setAdd('kids')}>{t('Add a kids profile')}</Button>
      </div>
      {add && <AddProfile admin initialKind={add} onClose={() => setAdd('')} onDone={() => { setAdd(''); void loadPeople() }} />}
      {looks && <>
        <h2 className="oobe-sub">{t('Required looks and themes')}</h2>
        <p className="muted">{t('A required look or theme is locked for that profile, with your name beside it. A profile with nothing required chooses freely.')}</p>
        {row('everyone', t('Everyone'))}{row('kids', t('Kids profiles'))}
        {people.filter((u) => !u.isAdmin).map((u) => row(String(u.id), profileName(u)))}
      </>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
