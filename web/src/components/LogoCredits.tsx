// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import '../pages/settings/metadataoptional.css'
import { useEffect, useState } from 'react'
import { api } from '../api'
import { LOGO_KIND_NAMES } from './edit/LogoChooser'
import type { LogoInfo } from './BrandLogo'
import { WebLink } from './WebLink'

// Where every logo on this server came from, as the licences of those files ask: its source,
// licence and author. Channels with a picture from elsewhere are listed too, so nothing on the
// screen is missing from here.

type Credits = { logos: LogoInfo[]; channels?: { name: string; source: 'owner' | 'list' }[] }

export default function LogoCredits() {
  const [c, setC] = useState<Credits | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<Credits>('/logos/credits').then(setC).catch((e) => setErr(e.message)) }, [])
  if (err) return <p className="muted">{err}</p>
  if (!c) return null
  const others = c.channels || []
  if (c.logos.length === 0 && others.length === 0) return <p className="muted">None yet.</p>
  return (
    <ul className="set-credits">
      {c.logos.map((l) => (
        <li key={l.id}>
          <b>{l.name}</b> <span className="muted">({LOGO_KIND_NAMES[l.kind]})</span>:{' '}
          {l.source === 'you' ? 'uploaded by this server\'s owner.' : <>
            <WebLink href={l.sourceUrl}>Wikimedia Commons</WebLink>, {l.licenseUrl ? <WebLink href={l.licenseUrl}>{l.license}</WebLink> : l.license}{l.credit ? `, by ${l.credit}` : ''}. Recoloured here to white and to dark.
          </>}
        </li>
      ))}
      {others.map((o) => (
        <li key={o.name + o.source}><b>{o.name}</b> <span className="muted">(Channel)</span>: {o.source === 'owner' ? 'uploaded by this server\'s owner.' : 'named by the channel list this server\'s owner added. It states no licence.'}</li>
      ))}
    </ul>
  )
}
