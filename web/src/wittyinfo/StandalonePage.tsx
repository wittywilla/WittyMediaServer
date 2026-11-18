// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../api'
import InfoEditor, { LivePreview, type PlayerLink } from './InfoEditor'
import type { InfoAnswer } from './model'
import { draftMarkers } from './model'
import './info.css'

// The editor as the wittyinfo program serves it (cmd/wittyinfo, "wittyinfo edit"): one
// .wittyinfo on this computer, with its video playing beside it when the browser can play
// it. The program answers the same API as the server, as /wittyinfo/file/0.

const BASE = '/wittyinfo/file/0'

export default function StandalonePage() {
  const [ans, setAns] = useState<InfoAnswer | null>(null)
  const [err, setErr] = useState('')
  const [link, setLink] = useState<PlayerLink | null>(null)
  const [now, setNow] = useState(0)
  const [draft, setDraft] = useState<ReturnType<typeof draftMarkers> | null>(null)
  useEffect(() => { document.title = 'wittyinfo editor'; api.get<InfoAnswer>(BASE).then(setAns).catch((e) => setErr((e as Error).message)) }, [])
  if (err) return <main className="wi-page"><h1>wittyinfo</h1><p className="error" role="alert">{err}. Open the address the wittyinfo program printed, key and all.</p></main>
  if (!ans) return <main className="wi-page"><p className="muted">Reading the file…</p></main>
  const name = ans.path ? ans.path.split(/[\\/]/).pop() : 'the file'
  const scope = /(^|[\\/])show\.wittyinfo$/i.test(ans.path || '') ? 'show' : /(^|[\\/])season(\d+)?\.wittyinfo$/i.test(ans.path || '') ? 'season' : 'item'
  return (
    <main className="wi-page">
      <header>
        <h1>{name}</h1>
        <p className="muted wi-help">{ans.videoName ? <>For <b>{ans.videoName}</b>. </> : 'No video found beside it, so there is no preview. '}Saving writes the file straight away; the old one is never left half-written.</p>
      </header>
      {ans.video && <LivePreview src={ans.video} markers={draft} duration={ans.durationSec || 0} onLink={(l, t) => { setLink(l); setNow(t) }} />}
      <InfoEditor base={BASE} scope={scope} player={link} now={now} onDraft={setDraft} />
    </main>
  )
}
