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
import { api } from '../../api'
import { Button, Field, Input, Spinner } from '../../ui'
import { TvPart, useTvPartBlocked } from '../../tvmode/OnAComputer'
import { t } from '../../i18n'
import { TickRow, LaterNote, StepActions, StepHead, type StepProps } from './parts'

// What's been chosen on this page and not saved yet: going to another page and back keeps it.
let kept: BrandDraft | null = null

// This server, its name: the same editor as Settings, Appearance, tried in the corner of this page as it is made. On a TV
// the name can still be typed; colouring its letters and choosing a mark are for a computer.
export function Name({ info, next, back, onTry }: StepProps & { onTry: (t: { name: string; parts: StyledPart[] } | null) => void }) {
  const { brandName, brandColouring, brandLogo, setBrandName, setBrandLogo } = useApp()
  const [draft, setDraft] = useState<BrandDraft | null>(null)
  const [preview, setPreview] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [warn, setWarn] = useState<BrandWarning[]>([])
  const [readable, setReadable] = useState(true)
  const plain = useTvPartBlocked('appearance.brand')
  useEffect(() => {
    if (kept) { const d = kept; setDraft(d); onTry({ name: d.name.trim(), parts: partsFromCells(d.name.trim() || DEFAULT_NAME, d.cells) }); return }
    void loadBrandDraft(brandName, brandColouring).then(setDraft)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => onTry(null), []) // eslint-disable-line react-hooks/exhaustive-deps
  const logo = draft?.logo
  useEffect(() => {
    if (logo instanceof File) { const u = URL.createObjectURL(logo); setPreview(u); return () => URL.revokeObjectURL(u) }
    setPreview(logo === 'remove' || !brandLogo ? '' : logoUrl(brandLogo))
  }, [logo, brandLogo])
  const change = (d: BrandDraft) => { setDraft(d); kept = d; onTry({ name: d.name.trim(), parts: partsFromCells(d.name.trim() || DEFAULT_NAME, d.cells) }) }
  const save = async () => {
    if (!draft) { next(); return }
    setBusy(true); setErr('')
    try {
      const r = await saveBrandDraft(draft)
      // The server checked the colours against the theme: say so once, and let them carry on.
      if (r.warnings.length && readable && !warn.length) { setWarn(r.warnings); setBusy(false); return }
      kept = null
      const shown = draft.name.trim() || DEFAULT_NAME
      setBrandName(draft.name.trim(), 'auto')
      if (r.logo !== undefined) setBrandLogo(r.logo)
      setStatusBrandParts(partsFromCells(shown, draft.cells))
      await loadThemeOverrides()
      next()
    } catch (e) { setErr((e as Error).message); toast(t('The name couldn’t be saved'), { tone: 'danger' }) } finally { setBusy(false) }
  }
  return (
    <section>
      <StepHead title={t('Name your server')} lead={t('The name appears in every app, so everyone at home knows which server they’re on. Choose its colours, and what shows in small spaces.')} />
      {!draft ? <Spinner /> : plain
        ? <>
            <Field label={t('Name')}><Input value={draft.name} maxLength={40} placeholder={DEFAULT_NAME} onChange={(e) => change({ ...draft, name: e.target.value, cells: [] })} /></Field>
            <TvPart id="appearance.brand"><span /></TvPart>
          </>
        : <NameEditor draft={draft} onChange={change} logoUrl={preview} onLogoError={setErr} onCheck={setReadable} />}
      {warn.length > 0 && <div className="ne-note" role="status"><b>{t('Check:')}</b> {warn.slice(0, 3).map((w, i) => <span key={i}>{i > 0 && ' '}{warningWords(w)}.</span>)} {t('Words should be at least 4.5 to 1. It is saved; change the colours, or press Continue again to keep them.')}</div>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant={readable ? 'primary' : 'default'} size="big" busy={busy} onClick={() => void save()}>{readable ? t('Continue') : t('Continue anyway')}</Button></StepActions>
    </section>
  )
}

// This server, on the home network: whether apps find it by themselves, and its address, with the number at the end of
// it (the port) that can be changed. Encrypted connections are about reaching it from away, on the Away from home step.
export function Network({ info, next, back }: StepProps) {
  const net = useNetwork()
  const [find, setFind] = useState(true)
  const [lan, setLan] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<Record<string, string>>('/settings').then((v) => setFind(v.lanDiscovery !== '0')).catch(() => {})
    api.get<{ lanUrls?: string[] }>('/system').then((s) => setLan(s.lanUrls || [])).catch(() => {})
  }, [])
  // An address another device at home can open. This one may only be "localhost".
  const here = /^(localhost|127\.|\[::1\])/.test(location.hostname) || location.hostname.endsWith('.localhost') ? '' : location.origin
  const addrs = here ? [here] : lan
  const save = async () => {
    setBusy(true)
    try { await api.put('/settings', { lanDiscovery: find ? '1' : '0' }) } catch { toast(t('That couldn’t be saved. You can change it later in Settings, Network.'), { tone: 'danger' }) }
    setBusy(false)
    next()
  }
  return (
    <section>
      <StepHead title={t('Finding this server at home')} lead={t('Phones, TVs and computers on your home network (the same Wi-Fi or cable) reach the server at its address. Apps can also find it by themselves.')} />
      <div className="su-net">
        <TickRow kind="switch" checked={find} onChange={setFind} title={t('Find this server automatically')} text={t('Apps on your home network find it by its name, with no address to type.')} />
      </div>
      <h2 className="oobe-sub">{t('Its address')}</h2>
      {addrs.length > 0
        ? <p className="su-addr">{t('Other devices at home open:')} {addrs.slice(0, 2).map((a) => <code key={a}>{a}</code>)}</p>
        : <p className="muted">{t('Right now it answers only on this computer. Settings, Network says how to let other devices at home reach it.')}</p>}
      {net.view
        ? <div className="su-gap"><p className="muted">{t('The number at the end of the address is its port. Leave it unless another program already uses that number, or your network needs another.')}</p><PortSetting view={net.view} onSaved={net.setView} compact /></div>
        : <p className="muted">{net.failed ? t('The port couldn’t be read. It is under Settings, Network.') : t('Reading the port…')}</p>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={() => void save()}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
