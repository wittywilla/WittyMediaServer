// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { DEFAULT_NAME } from '../../brandName'
import { partsFromCells, type StyledPart } from '../../brandStyle.ts'
import { loadBrandDraft, saveBrandDraft, warningWords, type BrandWarning } from '../../components/BrandDialog'
import { logoUrl } from '../../components/common'
import { NameEditor, type BrandDraft } from '../../components/NameEditor'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { toast, useApp } from '../../state'
import { loadThemeOverrides, setStatusBrandParts } from '../../theme'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { Button, Field, Select, Spinner } from '../../ui'
import { timeZoneChoices, httpsWords, type HttpsStatus } from './network'
import { TickRow, LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Step: Name and address. The server's name, its colours and its mark (the same editor as Settings, Appearance), tried in the
// corner of this page as it is made, and the port it answers on.
export function Name({ info, next, back, onTry }: StepProps & { onTry: (t: { name: string; parts: StyledPart[] } | null) => void }) {
  const { brandName, brandColouring, brandLogo, setBrandName, setBrandLogo } = useApp()
  const [draft, setDraft] = useState<BrandDraft | null>(null)
  const [preview, setPreview] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const net = useNetwork()
  // Time zone, finding the server from apps on the home network, and whether connections are encrypted (Remote Access sets that up).
  const [zone, setZone] = useState('')
  const [zones, setZones] = useState<{ computer: string; now: string }>({ computer: '', now: '' })
  const [find, setFind] = useState(true)
  const [https, setHttps] = useState<HttpsStatus | null>(null)
  const [warn, setWarn] = useState<BrandWarning[]>([])
  useEffect(() => {
    api.get<Record<string, string>>('/settings').then((v) => { setZone(v.timeZone || ''); setZones({ computer: v.timeZoneComputer || '', now: v.timeZoneNow || '' }); setFind(v.lanDiscovery !== '0') }).catch(() => {})
    api.get<{ https?: HttpsStatus }>('/remote').then((r) => setHttps(r.https || null)).catch(() => {})
  }, [])
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
      await api.put('/settings', { timeZone: zone, lanDiscovery: find ? '1' : '0' })
      const r = await saveBrandDraft(draft)
      // The server checked the colours against the theme: say so once, and let them carry on.
      if (r.warnings.length && !warn.length) { setWarn(r.warnings); setBusy(false); return }
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
      {warn.length > 0 && <div className="ne-note" role="status"><b>Check:</b> {warn.slice(0, 3).map((w, i) => <span key={i}>{i > 0 && ' '}{warningWords(w)}.</span>)} Words should be at least 4.5 to 1. It is saved; change the colours, or press Continue again to keep them.</div>}
      <h2 className="oobe-sub">Network and time</h2>
      <div className="su-net">
        <Field label="Time zone" hint={zone ? 'Guide days, schedules, recordings and the logs follow this.' : `The computer’s own${zones.computer ? ` (${zones.computer})` : ''}. Guide days, schedules and recordings follow it.`}>
          <Select value={zone} onChange={(e) => setZone(e.target.value)} options={timeZoneChoices(zones.computer, zone)} />
        </Field>
        <TickRow kind="switch" checked={find} onChange={setFind} title="Find this server automatically" text="Apps on your home network find it by name, with no address to type." />
        <p className="su-https"><b>Encrypted connections.</b> {httpsWords(https)} <Link to="/settings/remote">Settings, Remote Access</Link></p>
      </div>
      {net.view
        ? <div className="su-gap"><p className="muted">The port this server answers on. Apps find it by name on your home network; the setup page is always at this address.</p><PortSetting view={net.view} onSaved={net.setView} compact /></div>
        : <p className="muted">{net.failed ? 'The port couldn’t be read. It is under Settings, Network.' : 'Reading the port…'}</p>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={() => void save()}>Continue</Button></StepActions>
    </section>
  )
}
