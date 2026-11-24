// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtDateTime } from '../../api'
import { Spinner } from '../../components/common'
import { num, t, tk, tn, tx } from '../../i18n'
import { toast, useApp } from '../../state'
import { Button, Dialog, Notice, Select } from '../../ui'
import { PageHead, Section } from './ui'
import './health.css'

// Library health: what in the libraries needs a look, in one place, with the fix for each where
// there is one (internal/library/health.go). It ties the tools that were apart together: Fix
// matches, a scan, the item editor, Repair. Nothing here renames, moves or deletes a media file.

type Member = { id: number; title: string; path: string; height?: number; size?: number }
type Row = { key: string; target: 'item' | 'show'; id: number; libraryId: number; title: string; year?: number; path?: string; detail?: string; members?: Member[]; fixes: string[] }
type SectionData = { id: string; count: number; rows: Row[]; checked?: number }
type Check = { running: boolean; done: number; total: number; at?: number; stopped?: boolean; error?: string }
type View = { sections: SectionData[]; setAside: number; scanning: number[]; check: Check }

const WORDS: Record<string, { title: string; desc: string; none: string }> = {
  missing: { title: tk('Files that have gone'), desc: tk('Titles whose files aren’t where they were. Everything known about them is kept in case they come back: scan the library once the drive is back, or forget them if they’re gone for good. Moved a drive? See Repair.'), none: tk('Every file is where it was.') },
  unmatched: { title: tk('No match'), desc: tk('Titles the server couldn’t find online, so they have no poster or plot. Choose the match in Fix matches, or leave them as they are.'), none: tk('Every title has a match.') },
  duplicates: { title: tk('Possible duplicates'), desc: tk('The same film more than once in a library, one episode in two files, or one show in two folders. Keep copies of a film as versions of it, so it shows once with a choice of copy.'), none: tk('Nothing is listed twice.') },
  names: { title: tk('Odd file names'), desc: tk('Names the scan couldn’t read well: episodes with no number, files in a shows library outside a show’s folder, and films titled with their release name. Naming rules can read them; the file itself is never renamed.'), none: tk('Every name could be read.') },
  nfo: { title: tk('NFO files that can’t be read'), desc: tk('NFO and metadata files beside your media with a mistake in them, so they’re ignored. These are your own files, and nothing here changes them. Checking reads every one, so it’s done when you ask.'), none: tk('Every NFO file that was checked can be read.') },
  artwork: { title: tk('No artwork'), desc: tk('Films and shows with no poster. Fetch their details again, or match them first if they have no match.'), none: tk('Every film and show has a poster.') },
  unreadable: { title: tk('Files that can’t be read'), desc: tk('Video files whose details couldn’t be read when they were scanned, so they may not play. Read one again once it’s fixed or copied over again. If none can be read, check FFmpeg in Repair.'), none: tk('Every video file could be read.') },
}

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))
const sizeText = (n = 0) => (n >= 1 << 30 ? `${num(n / (1 << 30), { maximumFractionDigits: 1 })} GB` : n >= 1 << 20 ? `${num(Math.round(n / (1 << 20)))} MB` : '')
const openPath = (r: { target: string; id: number }) => `/${r.target === 'show' ? 'show' : 'item'}/${r.id}?edit=details`

export default function LibraryHealth() {
  const { libraries } = useApp()
  const [library, setLibrary] = useState(0)
  const [view, setView] = useState<View | null>(null)
  const [err, setErr] = useState('')
  const [more, setMore] = useState<Record<string, SectionData>>({}) // a section shown in full
  const [busy, setBusy] = useState('')
  const [forget, setForget] = useState<Row | null>(null)
  const poll = useRef<number | undefined>(undefined)

  const load = useCallback(async () => {
    try {
      const v = await api.get<View>(`/health/library?library=${library}`)
      setView(v); setErr('')
      return v
    } catch (e) { setErr(message(e)); return null }
  }, [library])

  useEffect(() => { setMore({}); void load() }, [load])

  // While the NFO check runs, ask how it's going.
  useEffect(() => {
    window.clearTimeout(poll.current)
    if (view?.check.running) poll.current = window.setTimeout(() => void load(), 1500)
    return () => window.clearTimeout(poll.current)
  }, [view, load])

  const showAll = async (id: string) => {
    try {
      const v = await api.get<View>(`/health/library?library=${library}&section=${id}&limit=500`)
      if (v.sections[0]) setMore((m) => ({ ...m, [id]: v.sections[0] }))
    } catch (e) { toast(message(e)) }
  }

  const after = async (id?: string) => {
    const v = await load()
    if (v && id && more[id]) await showAll(id)
  }

  const fix = async (section: string, row: Row, what: string) => {
    setBusy(row.key + what)
    try {
      switch (what) {
        case 'scan':
          await api.post(`/libraries/${row.libraryId}/scan`, {})
          toast(t('Scanning {library}. Activity shows how it’s going.', { library: libName(row.libraryId) }))
          break
        case 'leave':
          await api.post(`/meta/${row.target}/${row.id}/leave`, {})
          toast(t('{title} is left with no match', { title: row.title }))
          break
        case 'refresh':
          await api.post(`/meta/${row.target}/${row.id}/refresh`, {})
          toast(t('Fetched {title} again', { title: row.title }))
          break
        case 'versions': {
          const r = await api.post<{ result: string }>('/health/library/fix', { action: 'versions', ids: (row.members || []).map((m) => m.id) })
          toast(tx(r.result))
          break
        }
        case 'probe': {
          const r = await api.post<{ result: string }>('/health/library/fix', { action: 'probe', id: row.id })
          toast(tx(r.result))
          break
        }
        case 'setaside':
          await api.post('/health/library/fix', { action: 'setaside', key: row.key })
          break
      }
      await after(section)
    } catch (e) { toast(message(e)) } finally { setBusy('') }
  }

  const doForget = async () => {
    if (!forget) return
    const row = forget
    setBusy(row.key + 'forget')
    try {
      await api.post('/health/library/fix', { action: 'forget', target: row.target, id: row.id })
      toast(t('{title} was forgotten', { title: row.title }))
      setForget(null)
      await after('missing')
    } catch (e) { toast(message(e)) } finally { setBusy('') }
  }

  const check = async () => {
    try { await api.post('/health/library/check'); await load() } catch (e) { toast(message(e)) }
  }
  const stopCheck = async () => {
    try { await api.del('/health/library/check'); await load() } catch (e) { toast(message(e)) }
  }
  const bringBack = async () => {
    try { await api.post('/health/library/fix', { action: 'bringback' }); await load() } catch (e) { toast(message(e)) }
  }
  const matchAll = async () => {
    try { await api.post('/match/suggest-all', { library }); toast(t('Looking for matches. Fix matches shows the suggestions as they come.')) } catch (e) { toast(message(e)) }
  }

  const libName = (id: number) => libraries.find((l) => l.id === id)?.name || t('the library')

  if (!view) return err ? <div role="alert"><Notice tone="danger">{err}</Notice></div> : <Spinner />
  const total = view.sections.reduce((n, s) => n + s.count, 0)

  return (
    <>
      <PageHead title={tk('Library health')} desc={t('What in your libraries needs a look, in one place, with a fix for each where there is one. Nothing here renames, moves or deletes a media file.')} />
      <div className="lh-bar">
        <Select aria-label={t('Library')} value={String(library)} onChange={(e) => setLibrary(Number(e.target.value))}
          options={[{ value: '0', label: t('All libraries') }, ...libraries.map((l) => ({ value: String(l.id), label: l.name }))]} />
        <Link className="btn" to="/fix-matches">{t('Fix matches')}</Link>
        <Link className="btn" to="/settings/repair">{t('Repair')}</Link>
      </div>
      {view.scanning.length > 0 && <Notice tone="info">{t('A scan is running, so its library’s numbers will change as it goes. Titles still being matched are left out until it ends.')}</Notice>}
      <p className="lh-quiet" role="status">
        {total === 0 ? t('Nothing needs a look.') : tn('{count} thing needs a look.', '{count} things need a look.', total)}
        {view.setAside > 0 && <> {tn('{count} set aside as fine.', '{count} set aside as fine.', view.setAside)} <button type="button" className="lh-link" onClick={() => void bringBack()}>{t('Bring them back')}</button></>}
      </p>

      {view.sections.map((s) => {
        const w = WORDS[s.id]
        const shown = more[s.id] || s
        const actions = <>
          {s.id === 'unmatched' && s.count > 0 && <><Button onClick={() => void matchAll()}>{t('Look for matches for all')}</Button><Link className="btn" to={`/fix-matches?tab=unmatched${library ? `&library=${library}` : ''}`}>{t('Open Fix matches')}</Link></>}
          {s.id === 'names' && <Link className="btn" to={`/fix-matches?page=rules${library ? `&library=${library}` : ''}`}>{t('Naming rules')}</Link>}
          {s.id === 'nfo' && (view.check.running
            ? <Button onClick={() => void stopCheck()}>{t('Stop')}</Button>
            : <Button variant={view.check.at ? undefined : 'primary'} onClick={() => void check()}>{view.check.at ? t('Check again') : t('Check NFO files')}</Button>)}
        </>
        return (
          <div key={s.id} id={`lh-${s.id}`} className="lh-section">
            <Section title={w?.title || s.id} desc={w ? tx(w.desc) : undefined} actions={actions}>
              {s.id === 'nfo' && <CheckLine check={view.check} />}
              {s.id === 'nfo' && !view.check.at && !view.check.running ? null
                : s.count === 0 ? <p className="lh-quiet lh-none">{w ? tx(w.none) : ''}</p>
                  : <ul className="lh-rows">{shown.rows.map((r) => <HealthRow key={r.key} row={r} section={s.id} busy={busy} libName={libName(r.libraryId)} onFix={(what) => what === 'forget' ? setForget(r) : void fix(s.id, r, what)} />)}</ul>}
              {s.count > shown.rows.length && <Button size="sm" onClick={() => void showAll(s.id)}>{shown.rows.length >= 500 ? t('The first 500 are shown') : t('Show all {count}', { count: num(s.count) })}</Button>}
            </Section>
          </div>
        )
      })}

      {forget && (
        <Dialog title={t('Forget {title}?', { title: forget.title })} onClose={() => setForget(null)}
          actions={<><Button onClick={() => setForget(null)}>{t('Cancel')}</Button><Button variant="danger" busy={busy === forget.key + 'forget'} onClick={() => void doForget()}>{t('Forget it')}</Button></>}>
          <p>{t('Its file has gone. Forgetting it takes it off the server with everything known about it, and everyone’s progress on it. If the file comes back, a scan adds it again as new.')}</p>
          <p className="lh-quiet">{t('Only titles whose files are still gone, on a drive that is plugged in, can be forgotten. To keep them in case the files come back, leave them here.')}</p>
        </Dialog>
      )}
    </>
  )
}

function CheckLine({ check }: { check: Check }) {
  if (check.running) {
    return <p className="lh-quiet" role="status"><Spinner /> {check.total ? t('Checking: {done} of {total} titles', { done: num(check.done), total: num(check.total) }) : t('Checking…')}</p>
  }
  if (check.error) return <Notice tone="warn">{check.error}</Notice>
  if (!check.at) return <p className="lh-quiet">{t('Not checked yet.')}</p>
  return <p className="lh-quiet">{t('Checked {when}.', { when: fmtDateTime(check.at * 1000) })}{check.stopped ? ' ' + t('It stopped early, to keep from running too long: check again to read the rest.') : ''}</p>
}

const FIX_WORDS: Record<string, string> = {
  scan: tk('Scan again'), forget: tk('Forget'), match: tk('Choose the match'), leave: tk('Leave it unmatched'), versions: tk('Keep as versions'),
  open: tk('Edit'), rules: tk('Naming rules'), refresh: tk('Fetch again'), probe: tk('Read it again'), setaside: tk('It’s fine'),
}

function HealthRow({ row, section, busy, libName, onFix }: { row: Row; section: string; busy: string; libName: string; onFix: (what: string) => void }) {
  const title = row.title + (row.year ? ` (${row.year})` : '')
  return (
    <li className="lh-row">
      <div className="lh-row-text">
        <b><Link to={`/${row.target === 'show' ? 'show' : 'item'}/${row.id}`}>{title}</Link></b>
        <span className="lh-quiet">{libName}{row.detail ? ' · ' + tx(row.detail) : ''}</span>
        {row.path && <small className="mono lh-path">{row.path}</small>}
        {row.members && row.members.length > 0 && (
          <ul className="lh-members">
            {row.members.map((m, i) => (
              <li key={m.id}>
                <span className="mono lh-path">{m.path}</span>
                <span className="lh-quiet">{[m.height ? `${m.height}p` : '', sizeText(m.size), i === 0 && section === 'duplicates' && row.fixes.includes('versions') ? t('kept as the main copy') : ''].filter(Boolean).join(' · ')}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <div className="lh-row-actions">
        {row.fixes.map((f) => {
          const label = tx(FIX_WORDS[f] || f)
          const named = t('{action}: {title}', { action: label, title: row.title })
          if (f === 'open') return <Link key={f} className="btn sm" to={openPath(row)} aria-label={named}>{label}</Link>
          if (f === 'match') return <Link key={f} className="btn sm" to={`/fix-matches?tab=unmatched&library=${row.libraryId}`} aria-label={named}>{label}</Link>
          if (f === 'rules') return <Link key={f} className="btn sm" to={`/fix-matches?page=rules&library=${row.libraryId}`} aria-label={named}>{label}</Link>
          return <Button key={f} size="sm" variant={f === 'setaside' ? 'ghost' : f === 'forget' ? 'danger' : undefined} busy={busy === row.key + f} onClick={() => onFix(f)} aria-label={named}>{label}</Button>
        })}
      </div>
    </li>
  )
}
