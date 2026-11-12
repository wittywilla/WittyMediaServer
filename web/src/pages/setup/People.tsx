// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { sduiAsk, SDUI_BASE } from '../../sdui/client'
import { toast } from '../../state'
import { Button } from '../../ui'
import { AddProfile } from '../Profiles'
import { getLooks, type LooksAnswer } from '../settings/pages/api'
import type { ThemeMeta } from '../../theme'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Step: People and rules. Profiles (a kids profile has limits of its own), and a look or theme to require of everyone
// or of kids, which then shows locked for them (docs/SDUI.md, "Experiences").
export function People({ info, next, back }: StepProps) {
  const [add, setAdd] = useState<'' | 'adult' | 'kids'>('')
  const [made, setMade] = useState<string[]>([])
  const [looks, setLooks] = useState<LooksAnswer | null>(null)
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [req, setReq] = useState<{ everyone: { experience: string; theme: string }; kids: { experience: string; theme: string } }>({ everyone: { experience: '', theme: '' }, kids: { experience: '', theme: '' } })
  useEffect(() => {
    getLooks(false).then((l) => {
      setLooks(l)
      const r = l.experiences?.required
      setReq({ everyone: { experience: r?.everyone?.experience || '', theme: r?.everyone?.theme || '' }, kids: { experience: r?.kids?.experience || '', theme: r?.kids?.theme || '' } })
    }).catch(() => {})
    api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  }, [])
  const save = async () => {
    try {
      if (looks) {
        const clean = (r: { experience: string; theme: string }) => ({ ...(r.experience ? { experience: r.experience } : {}), ...(r.theme ? { theme: r.theme } : {}) })
        const required = { ...(req.everyone.experience || req.everyone.theme ? { everyone: clean(req.everyone) } : {}), ...(req.kids.experience || req.kids.theme ? { kids: clean(req.kids) } : {}) }
        const policy = { ...(looks.experiences || {}), required: Object.keys(required).length ? required : undefined }
        await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: policy, headers: { 'If-Match': looks.etag } })
      }
    } catch { toast('The required look couldn’t be saved. You can set it in Settings, Pages & menu, Looks.', { tone: 'danger' }) }
    next()
  }
  const row = (who: 'everyone' | 'kids', label: string) => (
    <div className="su-gap" role="group" aria-label={label}>
      <b>{label}</b>
      <select className="select" aria-label={`Look required for ${label}`} value={req[who].experience} onChange={(e) => setReq((r) => ({ ...r, [who]: { ...r[who], experience: e.target.value } }))}>
        <option value="">Free to choose a look</option>
        {looks?.options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
      </select>
      <select className="select" aria-label={`Theme required for ${label}`} value={req[who].theme} onChange={(e) => setReq((r) => ({ ...r, [who]: { ...r[who], theme: e.target.value } }))}>
        <option value="">Free to choose a theme</option>
        {themes.map((t) => <option key={t.id} value={t.id}>{t.name.replace(/^WittyMediaServer /, '')}</option>)}
      </select>
    </div>
  )
  return (
    <section>
      <StepHead title="People and rules" lead="Everyone at home can have a profile of their own, with their own place in what they’re watching. A kids profile only sees what you allow. You can require a look or theme of everyone, or of kids, and it shows locked for them." />
      {made.length > 0 && <ul className="tvs-list" aria-label="Added">{made.map((n) => <li key={n} className="tvs-item"><b>{n}</b></li>)}</ul>}
      <div className="su-gap">
        <Button onClick={() => setAdd('adult')}>Add a profile</Button>{' '}
        <Button onClick={() => setAdd('kids')}>Add a kids profile</Button>
      </div>
      {add && <AddProfile admin initialKind={add} onClose={() => setAdd('')} onDone={() => { setMade((m) => [...m, add === 'kids' ? 'A kids profile' : 'A profile']); setAdd('') }} />}
      {looks && <>{row('everyone', 'Everyone')}{row('kids', 'Kids profiles')}</>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>Continue</Button></StepActions>
    </section>
  )
}
