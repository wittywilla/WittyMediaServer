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
import { LANGUAGES, browserLanguage } from '../../languages'
import { language as appLanguage, languageNameHere, list, nativeName, setLanguage, t, tn } from '../../i18n'
import { TickRow, LaterNote, StepActions, StepHead, type StepProps } from './parts'

// What's been chosen on this step and not saved yet. Picking the pages' language redraws the whole app in it, this step
// included, so what was chosen is kept here to come back to.
type Kept = { draft: BrandDraft | null; ui: string; tracks: string; zone: string; find: boolean }
let kept: Kept | null = null

// Step: Name and address. The server's name, its colours and its mark (the same editor as Settings, Appearance), tried in the
// corner of this page as it is made, and the port it answers on.
export function Name({ info, next, back, onTry }: StepProps & { onTry: (t: { name: string; parts: StyledPart[] } | null) => void }) {
  const { brandName, brandColouring, brandLogo, setBrandName, setBrandLogo } = useApp()
  const [draft, setDraft] = useState<BrandDraft | null>(null)
  const [preview, setPreview] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const net = useNetwork()
  // Language and time zone, finding the server from apps on the home network, and whether connections are encrypted (Remote Access sets that up).
  // ui: the language of these pages; tracks: disc menus and sound tracks.
  const [ui, setUi] = useState(kept?.ui ?? appLanguage())
  const [tracks, setTracks] = useState(kept?.tracks ?? '')
  const [zone, setZone] = useState(kept?.zone ?? '')
  const [zones, setZones] = useState<{ computer: string; now: string }>({ computer: '', now: '' })
  const [find, setFind] = useState(kept?.find ?? true)
  const [https, setHttps] = useState<HttpsStatus | null>(null)
  const [warn, setWarn] = useState<BrandWarning[]>([])
  const [readable, setReadable] = useState(true)
  useEffect(() => {
    api.get<Record<string, string>>('/settings').then((v) => {
      setZones({ computer: v.timeZoneComputer || '', now: v.timeZoneNow || '' })
      if (kept) return
      // A language already chosen, else the browser's: menus and sound tracks start from it.
      setTracks(v.audioLanguage || (v.menuLanguage && v.menuLanguage !== 'en' ? v.menuLanguage : '') || browserLanguage() || v.menuLanguage || '')
      if (v.uiLanguage) setUi(v.uiLanguage)
      setZone(v.timeZone || ''); setFind(v.lanDiscovery !== '0')
    }).catch(() => {})
    api.get<{ https?: HttpsStatus }>('/remote').then((r) => setHttps(r.https || null)).catch(() => {})
  }, [])
  useEffect(() => {
    if (kept?.draft) { const d = kept.draft; setDraft(d); onTry({ name: d.name.trim(), parts: partsFromCells(d.name.trim() || DEFAULT_NAME, d.cells) }); return }
    void loadBrandDraft(brandName, brandColouring).then(setDraft)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  // The pages switch as soon as a language is picked; it's saved with the step.
  const pickUi = (code: string) => {
    setUi(code)
    kept = { draft, ui: code, tracks, zone, find }
    void setLanguage(code)
  }
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
    const language = tracks
    try {
      // Subtitles stay as they are: a subtitle language would turn them on for discs.
      await api.put('/settings', { timeZone: zone, lanDiscovery: find ? '1' : '0', audioLanguage: language, uiLanguage: ui, ...(language ? { menuLanguage: language } : {}) })
      kept = null
      const r = await saveBrandDraft(draft)
      // The server checked the colours against the theme: say so once, and let them carry on.
      if (r.warnings.length && readable && !warn.length) { setWarn(r.warnings); setBusy(false); return }
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
      <StepHead title={t('Name your server')} lead={t('The name appears in every app. Choose its colours, and what shows in small spaces.')} />
      {draft ? <NameEditor draft={draft} onChange={change} logoUrl={preview} onLogoError={setErr} onCheck={setReadable} /> : <Spinner />}
      {warn.length > 0 && <div className="ne-note" role="status"><b>{t('Check:')}</b> {warn.slice(0, 3).map((w, i) => <span key={i}>{i > 0 && ' '}{warningWords(w)}.</span>)} {t('Words should be at least 4.5 to 1. It is saved; change the colours, or press Continue again to keep them.')}</div>}
      <h2 className="oobe-sub">{t('Language and time')}</h2>
      <div className="su-net">
        <Field label={t('Language')} hint={t('Everything on these pages, for everyone who uses this server.')}>
          <Select value={ui} onChange={(e) => pickUi(e.target.value)} options={LANGUAGES.map(([value]) => ({ value, label: value === ui ? nativeName(value) : `${nativeName(value)} (${languageNameHere(value)})` }))} />
        </Field>
        <Field label={t('Disc menus and sound tracks')} hint={t('Disc menus, and the sound track a film or show starts with when it has one in this language. Subtitles stay off until someone turns them on.')}>
          <Select value={tracks} onChange={(e) => setTracks(e.target.value)} options={[{ value: '', label: t('The original language') }, ...LANGUAGES.map(([value]) => ({ value, label: languageNameHere(value) }))]} />
        </Field>
        <Field label={t('Time zone')} hint={`${zone ? t('Guide days, schedules, recordings and the logs follow this.') : zones.computer && zones.computer !== 'Local' ? t('The computer’s own ({zone}). Guide days, schedules and recordings follow it.', { zone: zones.computer }) : t('The computer’s own. Guide days, schedules and recordings follow it.')}${zone !== '' && zone !== zones.now ? ` ${t('The server restarts once to use it, when setup is finished.')}` : ''}`}>
          <Select value={zone} onChange={(e) => setZone(e.target.value)} options={timeZoneChoices(zones.computer, zone, undefined, { t, tn, list })} />
        </Field>
      </div>
      <h2 className="oobe-sub">{t('Network')}</h2>
      <div className="su-net">
        <TickRow kind="switch" checked={find} onChange={setFind} title={t('Find this server automatically')} text={t('Apps on your home network find it by name, with no address to type.')} />
        <p className="su-https"><b>{t('Encrypted connections.')}</b> {httpsWords(https, { t, tn, list })} <Link to="/settings/remote">{t('Settings, Remote Access')}</Link></p>
      </div>
      {net.view
        ? <div className="su-gap"><p className="muted">{t('The port this server answers on. Apps find it by name on your home network; the setup page is always at this address.')}</p><PortSetting view={net.view} onSaved={net.setView} compact /></div>
        : <p className="muted">{net.failed ? t('The port couldn’t be read. It is under Settings, Network.') : t('Reading the port…')}</p>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant={readable ? 'primary' : 'default'} size="big" busy={busy} onClick={() => void save()}>{readable ? t('Continue') : t('Continue anyway')}</Button></StepActions>
    </section>
  )
}
