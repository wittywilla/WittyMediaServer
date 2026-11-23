// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { DEFAULT_NAME } from '../../brandName'
import { partsFromCells, type StyledPart } from '../../brandStyle.ts'
import { loadBrandDraft, saveBrandDraft, warningWords, type BrandWarning } from '../../components/BrandDialog'
import { logoUrl } from '../../components/common'
import { NameEditor, type BrandDraft } from '../../components/NameEditor'
import { toast, useApp } from '../../state'
import { loadThemeOverrides, setStatusBrandParts } from '../../theme'
import { Button, Field, Input, Spinner } from '../../ui'
import { TvPart, useTvPartBlocked } from '../../tvmode/OnAComputer'
import { t } from '../../i18n'
import { StepActions, StepHead, type StepProps } from './parts'

// What's been chosen on this page and not saved yet: going to another page and back keeps it.
let kept: BrandDraft | null = null

// Advanced setup: the server's name with its colours and the mark for small spaces, the same editor as Settings, Appearance,
// tried in the corner of this page as it is made. On a TV the name can still be typed; colouring its letters and choosing
// a mark are for a computer.
export function ServerName({ next, back, onTry }: StepProps & { onTry: (t: { name: string; parts: StyledPart[] } | null) => void }) {
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
      <StepHead title={t('Your server’s name')} lead={t('It’s shown in every app, so everyone knows which server they’re on.')} />
      {!draft ? <Spinner /> : plain
        ? <>
            <Field label={t('Name')}><Input value={draft.name} maxLength={40} placeholder={DEFAULT_NAME} onChange={(e) => change({ ...draft, name: e.target.value, cells: [] })} /></Field>
            <TvPart id="appearance.brand"><span /></TvPart>
          </>
        : <NameEditor draft={draft} onChange={change} logoUrl={preview} onLogoError={setErr} onCheck={setReadable} />}
      {warn.length > 0 && <div className="ne-note" role="status"><b>{t('Hard to read:')}</b> {warn.slice(0, 3).map((w, i) => <span key={i}>{i > 0 && ' '}{warningWords(w)}.</span>)} {t('Change the colours, or press Continue again to keep them.')}</div>}
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <StepActions back={back}><Button variant={readable ? 'primary' : 'default'} size="big" busy={busy} onClick={() => void save()}>{readable ? t('Continue') : t('Continue anyway')}</Button></StepActions>
    </section>
  )
}
