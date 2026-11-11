// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { CardRow } from '../../components/common'
import { IconRight } from '../../icons'
import { fmtLeft, type GuideChannel, type GuidePage } from './tvApi'
import './onnow.css'

// "On TV now" on Home: the channels, favourites first, with what is on and a
// bar for how far in. It asks the server once, when Home opens.

const LIMIT = 16

export default function OnNowRow() {
  const [rows, setRows] = useState<GuideChannel[] | null>(null)
  useEffect(() => {
    let on = true
    api.get<GuidePage>(`/guide?hours=1&limit=${LIMIT}`).then((g) => { if (on) setRows(g.channels) }).catch(() => { if (on) setRows([]) })
    return () => { on = false }
  }, [])
  if (!rows || rows.length === 0) return null
  const now = Date.now() / 1000
  const list = [...rows].sort((a, b) => Number(!!b.favourite) - Number(!!a.favourite))
  return (
    <section className="hub" aria-label="On TV now">
      <div className="hub-head"><h2>On TV now</h2><Link to="/tv">Guide <IconRight /></Link></div>
      <CardRow>
        {list.map((c) => {
          const p = c.programmes.find((x) => x.start <= now && x.end > now)
          const pct = p ? ((now - p.start) / (p.end - p.start)) * 100 : 0
          return (
            <Link key={c.key} className="onnow-card" to={`/tv/watch/${c.key}`} aria-label={`Channel ${c.number}, ${c.name}: ${p?.title || 'nothing listed'}`}>
              <span className="onnow-art">
                {p?.itemId ? <img src={mediaUrl(`/api/items/${p.itemId}/backdrop`)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} /> : null}
                <span className="onnow-num">{c.number}</span>
                <i className="onnow-bar"><u style={{ width: pct + '%' }} /></i>
              </span>
              <b>{p?.title || (c.kind === 'library' ? 'Off air' : 'No guide data')}</b>
              <small>{c.name}{p ? ` · ${fmtLeft(p.end - now)} left` : ''}</small>
            </Link>
          )
        })}
      </CardRow>
    </section>
  )
}
