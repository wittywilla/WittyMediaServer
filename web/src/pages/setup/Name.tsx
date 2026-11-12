// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { DEFAULT_NAME } from '../../brandName'
import { partsFromCells, type StyledPart } from '../../brandStyle.ts'
import { loadBrandDraft, saveBrandDraft } from '../../components/BrandDialog'
import { logoUrl } from '../../components/common'
import { NameEditor, type BrandDraft } from '../../components/NameEditor'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { toast, useApp } from '../../state'
import { loadThemeOverrides, setStatusBrandParts } from '../../theme'
import { Button, Spinner } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Step: Name and address. The server's name, its colours and its mark (the same editor as Settings, Appearance), tried in the
// corner of this page as it is made, and the port it answers on.
export function Name({ info, next, back, onTry }: StepProps & { onTry: (t: { name: string; parts: StyledPart[] } | null) => void }) {
  const { brandName, brandColouring, brandLogo, setBrandName, setBrandLogo } = useApp()
  const [draft, setDraft] = useState<BrandDraft | null>(null)
  const [preview, setPreview] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const net = useNetwork()
  useEffect(() => { void loadBrandDraft(brandName, brandColouring).then(setDraft) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => onTry(null), []) // eslint-disable-line react-hooks/exhaustive-deps
  const logo = draft?.logo
  useEffect(() => {
    if (logo instanceof File) { const u = URL.createObjectURL(logo); setPreview(u); return () => URL.revokeObjectURL(u) }
    setPreview(logo === 'remove' || !brandLogo ? '' : logoUrl(brandLogo))
  }, [logo, brandLogo])
  const change = (d: BrandDraft) => { setDraft(d); onTry({ name: d.name.trim(), parts: partsFromCells(d.name.trim() || DEFAULT_NAME, d.cells) }) }
  const save = async () => {
    if (!draft) { next(); return }
    setBusy(true); setErr('')
    try {
      const r = await saveBrandDraft(draft)
      const shown = draft.name.trim() || DEFAULT_NAME
      setBrandName(draft.name.trim(), 'auto')
      if (r.logo !== undefined) setBrandLogo(r.logo)
      setStatusBrandParts(partsFromCells(shown, draft.cells))
      await loadThemeOverrides()
      next()
    } catch (e) { setErr((e as Error).message); toast('The name couldn’t be saved', { tone: 'danger' }) } finally { setBusy(false) }
  }
  return (
    <section>
      <StepHead title="Name your server" lead="The name appears in every app. Choose its colours, and what shows in small spaces." />
      {draft ? <NameEditor draft={draft} onChange={change} logoUrl={preview} onLogoError={setErr} /> : <Spinner />}
      <h2 className="oobe-sub">Network</h2>
      {net.view
        ? <div className="su-gap"><p className="muted">The port this server answers on. Apps find it by name on your home network; the setup page is always at this address.</p><PortSetting view={net.view} onSaved={net.setView} compact /></div>
        : <p className="muted">{net.failed ? 'The port couldn’t be read. It is under Settings, Network.' : 'Reading the port…'}</p>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={() => void save()}>Continue</Button></StepActions>
    </section>
  )
}
