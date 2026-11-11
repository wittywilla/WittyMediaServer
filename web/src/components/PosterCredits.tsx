// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../api'
import { WebLink } from './WebLink'

// Where each poster or backdrop from Wikimedia Commons came from, as the licences of those files ask: its
// page, licence and author. Films get them from there on a server with no TMDB key.

type Picture = { title: string; target: string; id: number; kind: 'poster' | 'backdrop'; page: string; license: string; licenseUrl?: string; credit?: string }

export default function PosterCredits() {
  const [list, setList] = useState<Picture[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<{ pictures: Picture[] }>('/artwork/credits').then((r) => setList(r.pictures)).catch((e) => setErr(e.message)) }, [])
  if (err) return <p className="muted">{err}</p>
  if (!list) return null
  if (list.length === 0) return <p className="muted">None yet.</p>
  return (
    <ul className="set-credits">
      {list.map((p) => (
        <li key={p.target + p.id + p.kind}>
          <b>{p.title}</b> <span className="muted">({p.kind === 'poster' ? 'Poster' : 'Backdrop'})</span>:{' '}
          <WebLink href={p.page}>Wikimedia Commons</WebLink>, {p.licenseUrl ? <WebLink href={p.licenseUrl}>{p.license}</WebLink> : p.license}{p.credit ? `, by ${p.credit}` : ''}.
        </li>
      ))}
    </ul>
  )
}
