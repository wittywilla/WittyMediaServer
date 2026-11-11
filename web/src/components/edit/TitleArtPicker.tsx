// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { IconImage } from '../../icons'
import { sendPicture, undoBatch, type ArtChoice, type Target, type TitleEditor } from '../../metadata'
import { Dialog, Menu, toast } from '../../ui'
import ArtworkPicker, { ART_KINDS } from '../ArtworkPicker'

// The artwork picker for a film, an episode or a show: from the edit dialog's Artwork
// section, and from the poster on a title's page. Apply is one change, undone in one go.

export default function TitleArtPicker({ target, id, start, ed: given, onApplied, onClose }: {
  target: Target; id: number; start?: string
  // The editor, when the edit dialog already has it.
  ed?: TitleEditor | null
  onApplied: (ed: TitleEditor, batch: string, what: string) => void
  onClose: () => void
}) {
  const [loaded, setEd] = useState<TitleEditor | null>(null)
  const ed = given || loaded
  const [err, setErr] = useState('')
  const [beside, setBeside] = useState<boolean | null>(null)
  useEffect(() => { if (!given) api.get<TitleEditor>(`/meta/${target}/${id}`).then(setEd).catch((e) => setErr(e.message)) }, [target, id, given])
  // Read each time: it's the owner's to change in Settings while the app is open.
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setBeside(s.saveMetadata !== '0')).catch(() => {}) }, [])
  if (!ed) {
    return err ? <Dialog title="Change artwork" onClose={onClose}><div className="error" role="alert">{err}</div></Dialog> : null
  }
  const kinds = ART_KINDS.filter((k) => ed.order.includes(k.kind))
  const value = (k: string) => ed.fields[k]?.value ?? ''
  return (
    <ArtworkPicker kinds={kinds} start={start} title={`Artwork for ${ed.name}`} onClose={onClose}
      current={value}
      load={(k) => api.get<{ candidates: ArtChoice[]; warnings: string[] }>(`/meta/${target}/${id}/artwork?kind=${k}`)
        .then((r) => ({ candidates: r.candidates.map((c) => (c.source === 'you' ? { url: c.url, sourceLabel: 'Chosen by you', own: true } : { url: c.url, sourceLabel: c.sourceLabel || 'This server' })), warnings: r.warnings }))}
      apply={async (changes) => {
        const r = await api.put<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}`, { fields: changes })
        const names = kinds.filter((k) => k.kind in changes).map((k) => k.label)
        onApplied(r.editor, r.batch, names.length === 1 ? `${names[0]} changed.` : 'Artwork changed.')
      }}
      stage={(k, f) => sendPicture<{ url: string }>(`/meta/${target}/${id}/art/${k}?stage=1`, f).then((r) => r.url)}
      frame={target === 'item' ? () => api.post<{ url: string }>(`/meta/item/${id}/frame`).then((r) => r.url) : undefined}
      maker={kinds.some((k) => k.kind === 'poster') ? {
        title: value('title') || ed.name, line: value('year'), hasVideo: target === 'item',
        make: (o) => api.post<{ url: string }>(`/meta/${target}/${id}/titleposter`, o),
      } : undefined}
      note={beside === null ? undefined : beside
        ? 'Saving beside the media is on, so what you apply is also written next to it (poster.jpg, fanart.jpg or <name>-poster.jpg), where other media software can find it.'
        : 'Saving beside the media is off, so what you apply is kept on this server only.'} />
  )
}

// Applied from a title's page: the page reads the title again, and the toast can undo it.
export function appliedOnPage(onChanged: () => void) {
  return (_: TitleEditor, batch: string, what: string) => {
    onChanged()
    toast(what, { action: { label: 'Undo', run: () => { undoBatch(batch).then(() => { onChanged(); toast('Undone') }).catch((e) => toast(e.message, { tone: 'danger' })) } } })
  }
}

// The menu on a title's poster, for administrators: straight to the picker, at the
// picture chosen.
export function PosterMenu({ target, id, onChanged }: { target: Target; id: number; onChanged: () => void }) {
  const [open, setOpen] = useState('')
  return (
    <>
      <Menu label="Artwork" align="start" className="poster-menu-list"
        items={[
          { key: 'poster', label: 'Change poster', onSelect: () => setOpen('poster') },
          { key: 'backdrop', label: 'Change backdrop', onSelect: () => setOpen('backdrop') },
        ]}
        trigger={(p) => <button type="button" {...p} ref={p.ref as React.Ref<HTMLButtonElement>} className="btn sm poster-menu" aria-label="Change artwork"><IconImage size={16} /> Change</button>} />
      {open && <TitleArtPicker target={target} id={id} start={open} onClose={() => setOpen('')} onApplied={appliedOnPage(onChanged)} />}
    </>
  )
}
