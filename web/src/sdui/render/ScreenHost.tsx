// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app's own screens a page's button can open for a title (docs/SDUI.md, "Native
// screens"): the share dialog, the device picker and the watch party, as the web app has
// always had them.

import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { api, type Item } from '../../api'
import ShareDialog from '../../components/ShareDialog'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Overlay'
import { toast } from '../../ui/Toast'

const PlayOn = lazy(() => import('../../remote/PlayOn'))
const WatchParty = lazy(() => import('../../remote/WatchParty'))

export default function ScreenHost({ screen, item, title, onClose }: { screen: string; item: string; title: string; onClose: () => void }) {
  const loc = useLocation()
  const here = loc.pathname + loc.search
  const numeric = /^\d+$/.test(item) ? Number(item) : 0
  // A screen this app has no dialog for closes again at once rather than leaving nothing on screen.
  const known = screen === 'share' || ((screen === 'playOn' || screen === 'watchParty') && numeric > 0)
  useEffect(() => { if (!known) onClose() }, [known, onClose])
  if (screen === 'share') {
    // A link anyone can open needs one file to point at; a show, an album or a collection
    // is shared as its place on this server, for people who have an account here.
    if (numeric) return <ShareDialog kind="item" id={numeric} title={title} internalPath={here} onClose={onClose} />
    return <PlaceLink title={title} path={here} onClose={onClose} />
  }
  if (screen === 'playOn' && numeric) return <Suspense fallback={null}><PlayOn source={{ kind: 'item', itemId: numeric, title }} onClose={onClose} /></Suspense>
  if (screen === 'watchParty' && numeric) return <PartyFor id={numeric} title={title} onClose={onClose} />
  return null
}

function PlaceLink({ title, path, onClose }: { title: string; path: string; onClose: () => void }) {
  const link = location.origin + path
  const field = useRef<HTMLInputElement | null>(null)
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try { await navigator.clipboard.writeText(link); setCopied(true); toast('Link copied', { tone: 'ok' }) } catch { field.current?.select() }
  }
  return (
    <Dialog title={`Share ${title}`} onClose={onClose} size="sm" description="This link opens it for anyone who has a profile on this server."
      actions={<><Button variant="ghost" onClick={onClose}>Close</Button><Button variant="primary" onClick={copy}>{copied ? 'Copied' : 'Copy link'}</Button></>}>
      <input ref={field} className="input" readOnly aria-label="Link" value={link} onFocus={(e) => e.currentTarget.select()} />
    </Dialog>
  )
}

// A watch party starts from what the title is: an episode carries on through its season.
function PartyFor({ id, title, onClose }: { id: number; title: string; onClose: () => void }) {
  const [it, setIt] = useState<Item | null>(null)
  useEffect(() => {
    api.get<Item>(`/items/${id}`).then(setIt).catch((e) => { toast((e as Error).message, { tone: 'danger' }); onClose() })
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!it) return null
  return <Suspense fallback={null}><WatchParty item={{ id: it.id, title: it.title || title, kind: it.kind, showId: it.showId ?? undefined }} onClose={onClose} /></Suspense>
}
