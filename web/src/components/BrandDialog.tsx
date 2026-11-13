// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, getToken, serverBase } from '../api'
import { DEFAULT_NAME } from '../brandName'
import { cellsFromColouring, cellsFromSpans, markFromSetting, markSetting, PRESETS, sameColor, spanCount, spansFromCells, MAX_SPANS, partsFromCells, type BrandSpan, type Mark } from '../brandStyle.ts'
import { toast, useApp } from '../state'
import { loadThemeOverrides, setStatusBrandParts } from '../theme'
import { Button } from '../ui'
import { Dialog } from '../ui/Overlay'
import { logoUrl as logoAddress } from './common'
import { NameEditor, type BrandDraft } from './NameEditor'

// The name editor in a window: Settings, Appearance, Name and logo, and the layout editor's "Edit name…". Setup draws the
// same editor on its own page (pages/setup/Name.tsx) and keeps the draft until there is an administrator to save it.

export const startDraft = (name: string, colouring: string, spans: BrandSpan[], mark: Mark = { kind: 'image' }): BrandDraft => {
  const shown = name.trim() || DEFAULT_NAME
  return { name, cells: spans.length ? cellsFromSpans(Array.from(shown).length, spans) : cellsFromColouring(shown, colouring), mark, logo: null }
}

// What the server has now, as a draft.
export async function loadBrandDraft(name: string, colouring: string): Promise<BrandDraft> {
  const s = await api.get<Record<string, string>>('/settings').catch(() => ({} as Record<string, string>))
  let spans: BrandSpan[] = []
  try { spans = s.brandSpans ? JSON.parse(s.brandSpans) : [] } catch { /* the automatic colouring */ }
  return startDraft(name, colouring, spans, s.brandMark === undefined ? undefined : markFromSetting(s.brandMark))
}

// Saves a draft: the name, its colours (the automatic colouring is kept as the server's own, anything else as spans), the
// mark, and the logo when there is a new one or it was removed. Returns the logo's version ('' for none) when it changed.
export type BrandWarning = { text: string; surface: string; where: string; ratio: number; need: number; variant?: string }
export const warningWords = (w: BrandWarning) => `“${w.text}” is ${w.ratio} to 1 on the ${w.where}${w.variant ? ` (${w.variant} look)` : ''}`
export async function saveBrandDraft(d: BrandDraft): Promise<{ logo?: string; warnings: BrandWarning[] }> {
  const shown = d.name.trim() || DEFAULT_NAME
  const auto = PRESETS[0].cells(shown)
  const isAuto = d.cells.length === auto.length && d.cells.every((c, i) => sameColor(c, auto[i]))
  if (!isAuto && spanCount(d.cells) > MAX_SPANS) throw new Error('The name has too many changes of colour.')
  let logo: string | undefined
  if (d.logo === 'remove') { await api.del('/branding/logo'); logo = '' } else if (d.logo) logo = await uploadLogo(d.logo)
  const saved = await api.put<{ brandWarnings?: BrandWarning[] }>('/settings', { brandName: d.name.trim(), brandColouring: 'auto', brandSpans: isAuto ? '' : JSON.stringify(spansFromCells(d.cells)), brandMark: markSetting(d.mark) })
  return { logo, warnings: saved?.brandWarnings || [] }
}

export default function BrandDialog({ onClose }: { onClose: () => void }) {
  const { brandName, brandColouring, brandLogo, setBrandName, setBrandLogo } = useApp()
  const [draft, setDraft] = useState<BrandDraft | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  // Words under 4.5 to 1 are said beside the colour controls; saving them is a second, plain choice.
  const [readable, setReadable] = useState(true)
  const [preview, setPreview] = useState('')
  useEffect(() => { void loadBrandDraft(brandName, brandColouring).then(setDraft) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const logo = draft?.logo
  useEffect(() => {
    if (logo instanceof File) { const u = URL.createObjectURL(logo); setPreview(u); return () => URL.revokeObjectURL(u) }
    setPreview(logo === 'remove' || !brandLogo ? '' : logoAddress(brandLogo))
  }, [logo, brandLogo])

  const save = async () => {
    if (!draft) return
    setBusy(true); setErr('')
    try {
      const r = await saveBrandDraft(draft)
      const shown = draft.name.trim() || DEFAULT_NAME
      setBrandName(draft.name.trim(), 'auto')
      if (r.logo !== undefined) setBrandLogo(r.logo)
      setStatusBrandParts(partsFromCells(shown, draft.cells))
      await loadThemeOverrides()
      if (r.warnings.length) toast('Saved. Check: ' + r.warnings.slice(0, 2).map(warningWords).join('; ') + '. Words should be at least 4.5 to 1.', { tone: 'warn' })
      else toast('Saved', { tone: 'ok' })
      onClose()
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const reset = () => draft && setDraft({ ...draft, name: '', cells: PRESETS[0].cells(DEFAULT_NAME), mark: { kind: 'letter' } })
  return (
    <Dialog title="Your server’s name" size="wide" className="brand-dialog" onClose={onClose} actions={<>
      <Button variant={readable ? 'primary' : 'default'} busy={busy} disabled={!draft} onClick={() => void save()}>{readable ? 'Save' : 'Save anyway'}</Button>
      <Button variant="ghost" disabled={!draft} onClick={reset}>Reset to default</Button>
      <Button variant="ghost" onClick={onClose}>Cancel</Button>
    </>}>
      <p className="muted">Settings · Appearance · Name and logo. Shown in the menu, on the sign-in page and in the browser tab.</p>
      {draft ? <NameEditor draft={draft} onChange={setDraft} logoUrl={preview} onLogoError={setErr} onCheck={setReadable} /> : <p className="muted">Loading…</p>}
      {err && <div className="error" role="alert">{err}</div>}
    </Dialog>
  )
}

// uploadLogo stores a new logo on the server and returns its version.
export async function uploadLogo(f: File): Promise<string> {
  const headers: Record<string, string> = { 'Content-Type': f.type }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(`${serverBase()}/api/branding/logo`, { method: 'PUT', headers, body: f, credentials: 'same-origin' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || 'The logo could not be saved')
  return data.brandLogo as string
}
