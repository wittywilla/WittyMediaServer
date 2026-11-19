// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "Customise this page": a profile's own order, hidden and pinned sections, density, card
// sizes and the tab a page opens on, as far as the page allows (document.personal). Each
// change is saved at once and the page behind redraws; Undo takes the last one back and
// Reset returns the page to how everyone has it. docs/SDUI.md, "Personalisation".

import { useEffect, useMemo, useRef, useState } from 'react'
import { announce } from '../a11y'
import { Button } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Select } from '../ui/Field'
import { ask, Sheet } from '../ui/Overlay'
import { toast } from '../ui/Toast'
import { ArrangeList } from './ArrangeList'
import { deletePersonal, getPersonal, putPersonal } from './client'
import { allows, body, draftOf, move, same, setCardSize, setDefaultTab, setDensity, setHidden, setPinned, type Draft } from './personal'
import type { Document, Node } from './types'
import { t, tk, tx } from '../i18n'

const DENSITY: Record<string, string> = { compact: tk('Compact'), comfortable: tk('Comfortable'), spacious: tk('Spacious') }
const SIZE_NAMES: Record<string, string> = { s: tk('Small cards'), m: tk('Medium cards'), l: tk('Large cards') }

export default function Customise({ doc, onClose }: { doc: Document; onClose: () => void }) {
  const info = doc.personal
  const url = info?.url || ''
  const [draft, setDraft] = useState<Draft>(() => draftOf(info))
  const [history, setHistory] = useState<Draft[]>([])
  const [saving, setSaving] = useState(false)
  const loaded = useRef(false)
  // The sizes and tabs chosen before aren't in the page itself: ask for them once.
  useEffect(() => {
    if (!url) return
    let live = true
    getPersonal(url).then((r) => {
      if (!live || loaded.current) return
      loaded.current = true
      setDraft((d) => ({ ...d, cardSize: { ...(r.personal?.cardSize || {}) }, defaultTab: { ...(r.personal?.defaultTab || {}) }, density: r.personal?.density || d.density }))
    }).catch(() => { /* the list still works */ })
    return () => { live = false }
  }, [url])
  // The page behind was drawn again with the new arrangement: its list is the truth, in
  // case the server placed something differently (a locked section, one that went).
  const sig = JSON.stringify(info?.sections || [])
  useEffect(() => { setDraft((d) => ({ ...d, rows: draftOf(info).rows })) }, [sig]) // eslint-disable-line react-hooks/exhaustive-deps

  const top = doc.root.children || []
  const withCards = useMemo(() => new Set(top.filter((n) => (n.type === 'Shelf' || n.type === 'Grid') && n.props?.card).map((n) => n.id)), [top])
  const tabSets = useMemo(() => {
    const out: { id: string; tabs: { id: string; label: string }[] }[] = []
    const walk = (n: Node) => {
      if (n.type === 'Tabs') out.push({ id: n.id, tabs: (n.children || []).filter((x) => x.type === 'Tab').map((x) => ({ id: x.id, label: tx(String(x.props?.label || x.id)) })) })
      n.children?.forEach(walk)
    }
    walk(doc.root)
    return out.filter((set) => set.tabs.length > 1)
  }, [doc.root])

  const save = async (next: Draft, was: Draft | null, said?: string) => {
    if (same(next, draft)) return
    setDraft(next)
    if (was) setHistory((h) => [...h.slice(-19), was])
    setSaving(true)
    try {
      await putPersonal(url, body(next, info))
      if (said) announce(said)
    } catch (e) {
      setDraft(draft)
      if (was) setHistory((h) => h.slice(0, -1))
      toast((e as Error).message || t('Could not save that'), { tone: 'danger' })
    } finally { setSaving(false) }
  }
  const change = (next: Draft, said?: string) => save(next, draft, said)
  const undo = () => {
    const was = history[history.length - 1]
    if (!was) return
    setHistory((h) => h.slice(0, -1))
    save(was, null, t('Undone'))
  }
  const reset = async () => {
    if (!await ask({ title: t('Put this page back as everyone has it?'), body: t('Your own order, hidden and pinned sections, card sizes and spacing for this page are forgotten.'), confirm: t('Reset the page') })) return
    setSaving(true)
    try {
      await deletePersonal(url)
      setHistory([])
      setDraft({ rows: draftOf(info).rows.map((r) => ({ ...r, hidden: undefined, pinned: undefined })), density: 'comfortable', cardSize: {}, defaultTab: {} })
      toast(t('The page is back as everyone has it'), { tone: 'ok' })
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setSaving(false) }
  }

  const canMove = allows(info, 'reorder'), canHide = allows(info, 'hide'), canPin = allows(info, 'pin'), canSize = allows(info, 'cardSize')
  const rows = draft.rows.map((r) => ({ id: r.id, title: tx(r.title), locked: r.locked, hidden: r.hidden, pinned: r.pinned, note: r.empty && !r.hidden ? t('nothing in it now') : undefined }))
  const pristine = !draft.rows.some((r) => r.hidden || r.pinned) && draft.density === 'comfortable' && !Object.keys(draft.cardSize).length && !Object.keys(draft.defaultTab).length && same({ ...draft, rows: [] }, { ...draftOf(undefined), rows: [] })
  return (
    <Sheet title={doc.page.title ? t('Customise {page}', { page: tx(doc.page.title) }) : t('Customise this page')} onClose={onClose} className="sdui-customise"
      actions={<>
        <Button variant="ghost" onClick={undo} disabled={!history.length || saving}>{t('Undo')}</Button>
        <Button variant="ghost" onClick={reset} disabled={saving || (pristine && !history.length)}>{t('Reset')}</Button>
        <Button variant="primary" onClick={onClose}>{t('Done')}</Button>
      </>}>
      <p className="muted sdui-customise-lede">{canMove ? t('This is only for your profile, on every device. Drag a section by its handle, or use the arrows. Changes are saved as you make them.') : t('This is only for your profile, on every device. Changes are saved as you make them.')}</p>
      {allows(info, 'density') && (
        <div className="nav-custom-opt"><span id="sdui-density">{t('Spacing')}</span>
          <Segmented label={t('Spacing')} value={draft.density} onChange={(v) => change(setDensity(draft, v), t('Spacing: {choice}', { choice: tx(DENSITY[v] || v) }))}
            options={[{ value: 'compact', label: t('Compact') }, { value: 'comfortable', label: t('Comfortable') }, { value: 'spacious', label: t('Spacious') }]} />
        </div>
      )}
      {allows(info, 'defaultTab') && tabSets.map((set) => (
        <div className="nav-custom-opt" key={set.id}><span>{t('Open on')}</span>
          <Select aria-label={t('The tab this page opens on')} value={draft.defaultTab[set.id] || ''} onChange={(e) => change(setDefaultTab(draft, set.id, e.target.value))}
            options={[{ value: '', label: t('The page’s own choice') }, ...set.tabs.map((x) => ({ value: x.id, label: x.label }))]} />
        </div>
      ))}
      {rows.length > 0 && (canMove || canHide || canPin || canSize) && (
        <ArrangeList rows={rows} label={t('Sections of this page, in order')} canMove={canMove} canHide={canHide} canPin={canPin}
          onMove={(id, to) => change(move(draft, id, to))}
          onHidden={(id, hidden) => change(setHidden(draft, id, hidden), hidden ? t('Hidden') : t('Shown'))}
          onPinned={(id, pinned) => change(setPinned(draft, id, pinned), pinned ? t('Pinned to the top') : t('Unpinned'))}
          extra={(r) => (canSize && withCards.has(r.id) && !r.hidden
            ? <Select inputSize="sm" aria-label={t('Card size for {name}', { name: r.title })} value={draft.cardSize[r.id] || 'm'} onChange={(e) => change(setCardSize(draft, r.id, e.target.value))}
                options={['s', 'm', 'l'].map((s) => ({ value: s, label: t(SIZE_NAMES[s]) }))} />
            : null)} />
      )}
    </Sheet>
  )
}
