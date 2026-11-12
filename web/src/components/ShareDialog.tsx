// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api, getPrefs } from '../api'
import { copyText, type ShareLink } from '../gallery'
import { toast } from '../state'
import { useEffect } from 'react'
import LegalNote from './LegalNote'
import '../gallery.css'
import { Dialog } from '../ui/Overlay'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { shareSpec } from '../spec/specs/share'

// Sharing something: a link inside this server for other profiles, or (pictures,
// videos and albums) a link for someone with no account, with its own end date.

export default function ShareDialog({ kind, id, title, onClose, internalPath }: { kind: 'item' | 'album'; id: number; title: string; onClose: () => void; internalPath: string }) {
  const [days, setDays] = useState(30)
  const [download, setDownload] = useState(false)
  const [link, setLink] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [allowed, setAllowed] = useState(true)
  useEffect(() => { getPrefs().then((p) => setAllowed(p.shareLinks !== false)) }, [])
  const internal = location.origin + internalPath
  const make = async () => {
    setBusy(true); setErr('')
    try {
      const l = await api.post<ShareLink>('/shares', { kind, id, days, allowDownload: download })
      setLink(location.origin + '/s/' + l.token)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const copy = async (t: string) => { toast((await copyText(t)) ? 'Link copied' : "Couldn't copy the link: select it and copy") }
  const page = shareSpec({ title, internal, allowed, link, days, download, busy, error: err })
  const press = (id: string) => { if (id === 'copyInternal') copy(internal); else if (id === 'copyLink') copy(link); else if (id === 'make') make() }
  const field = (id: string, v: string | number | boolean | File) => { if (id === 'days') setDays(Number(v)); else if (id === 'download') setDownload(!!v) }
  return (
    <Dialog title={<>Share “{title}”</>} className="share-modal" onClose={onClose}>
      <LegalNote kind="sharing" />
      <SpecPage page={page} embedded handled={handledIds(page)} onAction={press} onField={field} />
    </Dialog>
  )
}
