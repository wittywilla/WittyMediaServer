// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { Spinner } from '../components/common'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { MONTHS, hours, yearSpec, type Review } from '../spec/specs/year'
import { toast } from '../state'
import '../gallery.css'
import { ask } from '../ui/Overlay'

// Year in review: this profile's year, added up on this server from its own
// record. Nobody else sees it, and nothing is sent anywhere (docs/COLLECTIONS.md).

export default function ReviewPage() {
  const [params, setParams] = useSearchParams()
  const year = Number(params.get('year')) || new Date().getFullYear()
  const [r, setR] = useState<Review | null>(null)
  const load = () => { api.get<Review>(`/me/review?year=${year}`).then(setR).catch(() => setR(null)) }
  useEffect(load, [year]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!r) return <Spinner />
  const peak = Math.max(1, ...r.byMonth)
  const clear = async () => {
    if (!await ask({ title: 'Forget what this profile watched and listened to?', body: 'Your year in review, the numbers behind For You and your history start again from nothing. Your places in videos and books stay.', confirm: 'Forget it all', danger: true })) return
    try { await api.del('/me/history'); load(); toast('History cleared') } catch (e: any) { toast(e.message) }
  }
  const page = yearSpec(r)
  const act = (id: string) => { if (id === 'clear') void clear() }
  const field = (id: string, v: string | number | boolean | File) => { if (id === 'year') setParams({ year: String(v) }) }
  const chart = (
    <div className="rv-months" role="img" aria-label={MONTHS.map((m, i) => `${m}: ${hours(r.byMonth[i] || 0)}`).join(', ')}>
      {r.byMonth.map((v, i) => <div key={i} className="rv-month"><i style={{ height: `${Math.max(v > 0 ? 3 : 0, (v / peak) * 100)}%` }} /><span>{MONTHS[i]}</span></div>)}
    </div>
  )
  const at = page.sections.findIndex((s) => s.h === 'Month by month')
  return <SpecPage page={page} handled={handledIds(page)} onAction={act} onField={field} after={at >= 0 ? { [at]: chart } : undefined} />
}
