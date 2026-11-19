// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { fmtRuntime } from '../../api'
import { toast, useApp } from '../../state'
import { Button, Dialog, Input } from '../../ui'
import { channelVideos, type Bumper, type CommercialPick } from './tvApi'
import { num, t, tn } from '../../i18n'

// Choosing a channel's commercials and bumper libraries without ever listing a
// whole library: each library shows its count, and picking particular videos
// happens in a dialog that is searched and loaded a page at a time.

const PAGE = 25

// The commercials-kind libraries: where both commercials and bumpers live.
export function useShortLibraries() {
  const { libraries } = useApp()
  return libraries.filter((l) => l.type === 'commercials')
}

// LibraryPicks ticks libraries, each whole or only some of its videos.
export function LibraryPicks({ label, picks, onChange, note: noteOf }: { label: string; picks: CommercialPick[]; onChange: (next: CommercialPick[]) => void; note?: (id: number) => string }) {
  const libs = useShortLibraries()
  const pickOf = (id: number) => picks.find((p) => p.library === id)
  const toggle = (id: number, on: boolean) => onChange(on ? [...picks, { library: id }] : picks.filter((p) => p.library !== id))
  const change = (id: number, patch: Partial<CommercialPick>) => onChange(picks.map((p) => (p.library === id ? { ...p, ...patch } : p)))
  return (
    <ul className="tvs-blocks tve-libpicks" aria-label={label}>
      {libs.map((l) => {
        const p = pickOf(l.id)
        const note = !p && noteOf?.(l.id)
        return (
          <li key={l.id} className="tvs-block">
            <label className="check-row">
              <input type="checkbox" checked={!!p} onChange={(e) => toggle(l.id, e.target.checked)} /> <b>{l.name}</b>{' '}
              <span className="tvs-dim">{tn('{count} video', '{count} videos', l.itemCount)}{p?.only ? ' · ' + t('{count} chosen', { count: (p.items || []).length }) : ''}</span>
              {note && <span className="tvs-dim"> · {note}</span>}
            </label>
            {p && (
              <>
                <div className="tvs-days" role="radiogroup" aria-label={t('What to use from {name}', { name: l.name })}>
                  <button type="button" role="radio" aria-checked={!p.only} className={'tvs-day' + (!p.only ? ' on' : '')} onClick={() => change(l.id, { only: false })}>{t('All of them')}</button>
                  <button type="button" role="radio" aria-checked={!!p.only} className={'tvs-day' + (p.only ? ' on' : '')} onClick={() => change(l.id, { only: true })}>{t('Only these…')}</button>
                </div>
                {p.only && (
                  <>
                    <VideoPicker label={t('Videos from {name}', { name: l.name })} libraries={[l.id]} value={p.items || []} onChange={(items) => change(l.id, { items })} />
                    {!(p.items || []).length && <p className="tvs-note" role="status">{t('None chosen yet, so nothing from this library will play.')}</p>}
                  </>
                )}
              </>
            )}
          </li>
        )
      })}
    </ul>
  )
}

// VideoPicker shows how many are chosen, and a button that opens a dialog with a
// searchable list, a page at a time. Nothing loads until the dialog opens, so
// the editor stays light however big the libraries are.
export function VideoPicker({ label, libraries, own, value, onChange }: { label: string; libraries: number[]; own?: boolean; value: number[]; onChange: (v: number[]) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="tve-vpick">
      <span><b>{label}</b><span className="tvs-count"> · {t('{count} chosen', { count: value.length })}</span></span>
      <Button size="sm" aria-haspopup="dialog" onClick={() => setOpen(true)}>{value.length ? t('Change…') : t('Choose videos…')}</Button>
      {open && <PickerDialog label={label} libraries={libraries} own={own} value={value} onChange={onChange} onClose={() => setOpen(false)} />}
    </div>
  )
}

function PickerDialog({ label, libraries, own, value, onChange, onClose }: { label: string; libraries: number[]; own?: boolean; value: number[]; onChange: (v: number[]) => void; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [search, setSearch] = useState('')
  const [chosenOnly, setChosenOnly] = useState(false)
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<{ items: Bumper[]; total: number } | null>(null)
  const libKey = libraries.join(',')
  // Waits a moment after typing before asking.
  useEffect(() => {
    const timer = setTimeout(() => { setSearch(q.trim()); setOffset(0) }, 250)
    return () => clearTimeout(timer)
  }, [q])
  useEffect(() => { setOffset(0) }, [libKey, own, chosenOnly])
  const idsKey = chosenOnly ? value.join(',') : ''
  useEffect(() => {
    if (chosenOnly && value.length === 0) { setPage({ items: [], total: 0 }); return }
    let live = true
    channelVideos(chosenOnly ? { ids: value, search, offset, limit: PAGE } : { libraries, own, search, offset, limit: PAGE })
      .then((r) => { if (live) setPage(r) })
      .catch((e) => toast((e as Error).message))
    return () => { live = false }
  }, [libKey, own, search, offset, idsKey, chosenOnly])
  const set = (id: number, on: boolean) => onChange(on ? [...value, id] : value.filter((x) => x !== id))
  const total = page?.total || 0
  return (
    <Dialog className="tve-vpick-dialog" title={label + ' · ' + t('{count} chosen', { count: value.length })} onClose={onClose} actions={<Button variant="primary" onClick={onClose}>{t('Done')}</Button>}>
      <div className="tve-vpick-bar">
        <Input type="search" autoFocus aria-label={t('Find in {name}', { name: label })} placeholder={t('Find…')} value={q} onChange={(e) => setQ(e.target.value)} />
        <label className="check-row tvs-inline"><input type="checkbox" checked={chosenOnly} onChange={(e) => setChosenOnly(e.target.checked)} /> {t('Chosen only')}</label>
        {value.length > 0 && <Button size="sm" variant="ghost" onClick={() => onChange([])}>{t('Clear')}</Button>}
      </div>
      {!page && <p className="muted">{t('Loading…')}</p>}
      {page && page.items.length === 0 && <p className="muted">{search ? t('Nothing found.') : chosenOnly ? t('Nothing chosen yet.') : t('Nothing here yet. A library may still be scanning.')}</p>}
      {page && page.items.length > 0 && (
        <ul className="tve-vpick-list">
          {page.items.map((b) => (
            <li key={b.itemId}>
              <label className="check-row">
                <input type="checkbox" checked={value.includes(b.itemId)} onChange={(e) => set(b.itemId, e.target.checked)} />
                <span className="tve-vpick-title" title={b.title}>{b.title}</span>
                <small className="tvs-dim">{fmtRuntime(b.durationSec) || t('{n} s', { n: Math.round(b.durationSec) })} · {b.uploaded ? t('Uploaded') : b.library}</small>
              </label>
            </li>
          ))}
        </ul>
      )}
      {total > PAGE && (
        <div className="tve-vpick-pages">
          <Button size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>{t('Previous')}</Button>
          <span className="tvs-dim" aria-live="polite">{t('{from}–{to} of {total}', { from: num(offset + 1), to: num(Math.min(offset + PAGE, total)), total: num(total) })}</span>
          <Button size="sm" disabled={offset + PAGE >= total} onClick={() => setOffset(offset + PAGE)}>{t('Next')}</Button>
        </div>
      )}
    </Dialog>
  )
}
