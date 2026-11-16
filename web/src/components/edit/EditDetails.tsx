// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, artPreviewUrl, type Item, type Show } from '../../api'
import { t, tk, tx } from '../../i18n'
import { IconClose, IconImage, IconLock, IconRefresh, IconTrash, IconUnlock } from '../../icons'
import { ask, Button, Dialog, IconButton } from '../../ui'
import { fmtClock, joinOf, listOf, parseClock, undoBatch, type ArtKind, type MetaField, type Target, type TitleEditor } from '../../metadata'
import { toast } from '../../state'
import LegalNote from '../LegalNote'
import RatingField from '../RatingField'
import { LogoRows } from './LogoChooser'
import MatchPanel from './MatchPanel'
import MarkersEditor from './MarkersEditor'
import TitleArtPicker from './TitleArtPicker'
import './edit.css'

// Edit details: one dialog for a film, an episode or a show. Everything about a title is
// here, in sections, with Delete, Cancel and Save always in view. Each field says where
// its value came from, can be locked so nothing changes it, and can go back to what the
// source said. Choosing a match or moving a title happens at once, and can be undone;
// typing waits for Save.

export type EditSection = 'match' | 'details' | 'artwork' | 'playback' | 'organise' | 'logos'

const SECTIONS: { id: EditSection; label: string }[] = [
  { id: 'match', label: tk('Match') }, { id: 'details', label: tk('Details') }, { id: 'artwork', label: tk('Artwork') },
  { id: 'playback', label: tk('Skipping') }, { id: 'organise', label: tk('Organise') }, { id: 'logos', label: tk('Logos') },
]

export default function EditDetails({ target, id, start, onClose, onChanged }: {
  target: Target; id: number; start?: EditSection; onClose: () => void
  // Something saved: the page behind should read the title again.
  onChanged: () => void
}) {
  const [ed, setEd] = useState<TitleEditor | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [locks, setLocks] = useState<Record<string, boolean>>({})
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [undo, setUndo] = useState<{ batch: string; what: string } | null>(null)
  const scroller = useRef<HTMLDivElement | null>(null)
  const nav = useNavigate()

  const load = useCallback(() => api.get<TitleEditor>(`/meta/${target}/${id}`).then(setEd).catch((e) => setLoadErr(e.message)), [target, id])
  useEffect(() => { load() }, [load])

  // Opened at a section: Artwork, or coming back from picking a time in the player.
  useEffect(() => {
    if (!ed || !start || start === 'match') return
    document.getElementById('ed-' + start)?.scrollIntoView({ block: 'start' })
    // Only on opening.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!ed])

  const dirty = useMemo(() => Object.keys(draft).length > 0 || Object.keys(locks).length > 0, [draft, locks])
  const val = (f: string) => (f in draft ? draft[f] : ed?.fields[f]?.value ?? '')
  const isLocked = (f: string) => (f in locks ? locks[f] : !!ed?.fields[f]?.locked)
  const set = (f: string, v: string) => {
    setDraft((d) => {
      const n = { ...d }
      if (v === (ed?.fields[f]?.value ?? '')) delete n[f]
      else n[f] = v
      return n
    })
    // What someone types is theirs, unless they unlock it.
    if (v !== (ed?.fields[f]?.value ?? '') && !ed?.fields[f]?.locked && !(f in locks)) setLocks((l) => ({ ...l, [f]: true }))
    if (v === (ed?.fields[f]?.value ?? '') && !ed?.fields[f]?.locked) setLocks((l) => { const n = { ...l }; delete n[f]; return n })
  }
  const toggleLock = (f: string) => setLocks((l) => {
    const now = f in l ? l[f] : !!ed?.fields[f]?.locked
    const next = !now
    const n = { ...l }
    if (next === !!ed?.fields[f]?.locked) delete n[f]
    else n[f] = next
    return n
  })

  // Something changed at once: show the new state, and keep what's been typed and not saved.
  const applied = (next: TitleEditor, batch?: string, what?: string) => {
    setEd(next)
    onChanged()
    if (batch && what) setUndo({ batch, what })
  }

  // The artwork picker applied something: what was waiting to be saved for that picture is
  // gone, as the picture now is what was chosen.
  const artApplied = (next: TitleEditor, batch: string, what: string) => {
    const moved = ART.map((a) => a.kind).filter((k) => next.fields[k]?.value !== ed?.fields[k]?.value)
    const drop = <T,>(m: Record<string, T>) => { const n = { ...m }; for (const k of moved) delete n[k]; return n }
    setDraft(drop); setLocks(drop)
    applied(next, batch, what)
  }

  const save = async () => {
    if (!ed) return
    const year = val('year')
    if (year && !/^\d{4}$/.test(year)) { setErr(t('A year has four digits.')); document.getElementById('ed-year')?.focus(); return }
    setBusy(true); setErr('')
    try {
      const lock = Object.entries(locks).filter(([, on]) => on).map(([f]) => f)
      const unlock = Object.entries(locks).filter(([, on]) => !on).map(([f]) => f)
      const r = await api.put<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}`, { fields: draft, lock, unlock })
      setEd(r.editor); setDraft({}); setLocks({})
      onChanged()
      toast(t('Saved'))
      onClose()
    } catch (e: any) { setErr(e.message); setBusy(false) }
  }
  // Save with a shortcut, from anywhere in the dialog.
  const keys = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'Enter')) { e.preventDefault(); save() }
  }
  const askClose = async () => {
    if (!dirty || await ask({ title: t('Discard your changes?'), body: t("You have changes that aren't saved."), confirm: t('Discard them'), cancel: t('Keep editing'), danger: true })) onClose()
  }
  const revert = async (f: string) => {
    setErr('')
    try {
      const r = await api.post<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}/revert`, { field: f })
      setDraft((d) => { const n = { ...d }; delete n[f]; return n })
      setLocks((l) => { const n = { ...l }; delete n[f]; return n })
      applied(r.editor)
    } catch (e: any) { setErr(e.message) }
  }
  const lockAll = async (on: boolean) => {
    try {
      const r = await api.put<{ editor: TitleEditor }>(`/meta/${target}/${id}`, on ? { lockAll: true } : { unlockAll: true })
      setLocks({}); applied(r.editor); toast(on ? t('Every field is locked') : t('Every field is unlocked'))
    } catch (e: any) { setErr(e.message) }
  }
  const remove = async () => {
    const body = target === 'show'
      ? t("Its files stay on disk, and it won't come back on a scan. You can bring it back from Fix matches, Removed.")
      : t("Its file stays on disk, and it won't come back on a scan. You can bring it back from Fix matches, Removed.")
    if (!await ask({ title: t('Take this out of its library?'), body, confirm: t('Take it out'), cancel: t('Keep it'), danger: true })) return
    try {
      await api.del(`/meta/${target}/${id}`)
      toast(t('Taken out of the library. Its file is still there.'))
      onChanged()
      onClose()
      nav(ed ? `/library/${ed.libraryId}` : '/')
    } catch (e: any) { setErr(e.message) }
  }

  const fieldProps = { val, set, isLocked, toggleLock, revert, ed: ed! }
  const allLocked = !!ed && ed.order.every((f) => ed.fields[f].locked)
  // What's asked of a remote: from the last section down to the buttons, and back.
  const navKeys = (e: React.KeyboardEvent<HTMLElement>) => {
    if (e.key !== 'ArrowDown') return
    const links = [...e.currentTarget.querySelectorAll<HTMLElement>('a')]
    if (document.activeElement !== links[links.length - 1]) return
    e.preventDefault()
    e.stopPropagation()
    e.currentTarget.closest('.ed-dialog')?.querySelector<HTMLElement>('.actions button:not([disabled])')?.focus()
  }
  return (
    <Dialog title={t('Edit details')} description={ed?.file} size="wide" className="ed-dialog" onClose={askClose}
      actions={<>
        <Button variant="ghost" className="ed-delete" disabled={!ed} icon={<IconTrash size={18} />} onClick={remove}>{t('Delete')}</Button>
        <Button variant="ghost" onClick={askClose}>{t('Cancel')}</Button>
        <Button variant="primary" busy={busy} disabled={!ed || !dirty} onClick={save} title="Ctrl+S">{t('Save')}</Button>
      </>}>
      <div className="ed-body" onKeyDown={keys}>
        {!ed ? (loadErr ? <div className="error ed-pad" role="alert">{loadErr}</div> : <div className="loading-box"><div className="spinner" /></div>) : (
          <div className="ed-layout">
            <nav className="ed-nav" aria-label={t('Sections')} data-nav-group onKeyDown={navKeys}>
              {SECTIONS.filter((s) => !(s.id === 'logos' && ed.kind === 'episode') && !(s.id === 'playback' && ed.disc)).map((s) => (
                <a key={s.id} href={`#ed-${s.id}`} onClick={(e) => { e.preventDefault(); const el = document.getElementById('ed-' + s.id); el?.scrollIntoView({ block: 'start' }); el?.querySelector<HTMLElement>('input, select, textarea, button')?.focus({ preventScroll: true }) }}>{tx(s.label)}</a>
              ))}
            </nav>
            <div className="ed-scroll" ref={scroller}>
              <MatchPanel ed={ed} target={target} id={id} onApplied={applied} onError={setErr} />
              <section id="ed-details" className="ed-section" aria-labelledby="ed-details-h">
                <div className="ed-section-head">
                  <h3 id="ed-details-h">{t('Details')}</h3>
                  <button type="button" className="btn sm ghost" onClick={() => lockAll(!allLocked)}>{allLocked ? <><IconUnlock size={16} /> {t('Unlock all')}</> : <><IconLock size={16} /> {t('Lock all')}</>}</button>
                </div>
                <p className="muted ed-help">{t('A locked field stays as it is: no scan, refresh, new match or file beside it can change it. Anything you type locks itself.')}</p>
                <DetailFields {...fieldProps} />
              </section>
              <ArtworkSection {...fieldProps} target={target} id={id} onApplied={artApplied} />
              {!ed.disc && <PlaybackSection {...fieldProps} target={target} id={id} dirty={dirty} onError={setErr} />}
              <OrganiseSection ed={ed} target={target} id={id} onApplied={applied} onError={setErr} />
              {ed.kind !== 'episode' && (
                <section id="ed-logos" className="ed-section" aria-labelledby="ed-logos-h">
                  <h3 id="ed-logos-h">{t('Logos')}</h3>
                  <p className="muted ed-help">{t('The logo each studio or network wears everywhere on this server and on your devices.')}</p>
                  <LogoRows logos={ed.brands} onChanged={load} />
                  {ed.brands.length === 0 && <p className="muted">{t('No studio or network is set for this yet. Add one under Details, or choose a match.')}</p>}
                </section>
              )}
              <LegalNote kind="metadata" />
            </div>
          </div>
        )}
        {undo && (
          <div className="ed-undo" role="status">
            <span>{undo.what}</span>
            <button type="button" className="btn sm" onClick={async () => {
              try { await undoBatch(undo.batch); setUndo(null); setDraft({}); setLocks({}); await load(); onChanged(); toast(t('Undone')) } catch (e: any) { setErr(e.message) }
            }}>{t('Undo')}</button>
            <IconButton size="sm" label={t('Dismiss')} hideTip onClick={() => setUndo(null)}><IconClose size={16} /></IconButton>
          </div>
        )}
        {err && <div className="error ed-err" role="alert">{err}</div>}
      </div>
    </Dialog>
  )
}

type FieldProps = { ed: TitleEditor; val: (f: string) => string; set: (f: string, v: string) => void; isLocked: (f: string) => boolean; toggleLock: (f: string) => void; revert: (f: string) => void }

function LockButton({ label, locked, onClick }: { label: string; locked: boolean; onClick: () => void }) {
  return <IconButton size="sm" className={'ed-lock' + (locked ? ' on' : '')} aria-pressed={locked} label={locked ? t('{label} is locked: nothing can change it', { label }) : t('Lock {label}', { label })} onClick={onClick}>{locked ? <IconLock size={16} /> : <IconUnlock size={16} />}</IconButton>
}

// Where a value came from, in words.
function sourceWords(m?: MetaField) {
  if (!m || !m.source) return ''
  if (m.source === 'you') return t('You typed this')
  if (m.source === 'file') return t('From the file name')
  if (m.source === 'nfo') return t('From an NFO file')
  if (m.source === 'saved') return t('Saved earlier')
  if (m.source === 'local') return t('From beside the file')
  if (m.source === 'frame') return t('A frame from the video')
  return t('From {source}', { source: m.sourceLabel })
}

function FieldShell({ name, label, children, p, wide, hint }: { name: string; label: string; children: (id: string, describedBy?: string) => React.ReactNode; p: FieldProps; wide?: boolean; hint?: string }) {
  const id = 'ed-' + name
  const m = p.ed.fields[name]
  const locked = p.isLocked(name)
  const hintId = hint ? id + '-hint' : undefined
  const unsaved = p.val(name) !== (m?.value ?? '')
  return (
    <div className={'ed-field' + (wide ? ' wide' : '') + (locked ? ' locked' : '')}>
      <div className="ed-field-label">
        <label htmlFor={id}>{label}</label>
        <span className="ed-source">{unsaved ? t('Not saved yet') : sourceWords(m)}</span>
      </div>
      <div className="ed-field-control">{children(id, hintId)}</div>
      {/* After the field in the tab order, though it's drawn above it: type, then lock. */}
      <div className="ed-field-tools">
        {m?.canRevert && !unsaved && (
          <button type="button" className="btn sm ghost ed-revert" onClick={() => p.revert(name)} title={m.revertTo ? t('Go back to: {value}', { value: m.revertTo }) : t('Go back to empty')}
            aria-label={t('Go back to what {source} said for {field}', { source: m.revertFrom || t('the source'), field: label })}><IconRefresh size={14} /> {t('Revert')}</button>
        )}
        <LockButton label={label} locked={locked} onClick={() => p.toggleLock(name)} />
      </div>
      {hint && <div className="ed-hint" id={hintId}>{hint}</div>}
    </div>
  )
}

// A list of short things: chips, with a box for the next one.
export function TokenField({ id, value, onChange, placeholder, describedBy, suggestions }: { id: string; value: string[]; onChange: (v: string[]) => void; placeholder?: string; describedBy?: string; suggestions?: string[] }) {
  const [text, setText] = useState('')
  const listId = useId()
  const add = (raw: string) => {
    const parts = raw.split(/[,|]/).map((s) => s.trim()).filter(Boolean)
    const next = [...value]
    for (const p of parts) if (!next.some((x) => x.toLowerCase() === p.toLowerCase())) next.push(p)
    if (next.length !== value.length) onChange(next)
    setText('')
  }
  return (
    <div className="ed-tokens">
      {value.map((v) => (
        <span key={v} className="ed-token">{v}
          <button type="button" aria-label={t('Remove {name}', { name: v })} onClick={() => onChange(value.filter((x) => x !== v))}><IconClose size={14} /></button>
        </span>
      ))}
      <input id={id} className="ed-token-input" value={text} placeholder={value.length ? '' : placeholder} aria-describedby={describedBy} list={suggestions?.length ? listId : undefined}
        onChange={(e) => { const v = e.target.value; if (v.includes(',')) add(v); else setText(v) }}
        onBlur={() => add(text)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && text.trim() && !e.ctrlKey && !e.metaKey) { e.preventDefault(); add(text) }
          if (e.key === 'Backspace' && !text && value.length) onChange(value.slice(0, -1))
        }} />
      {suggestions?.length ? <datalist id={listId}>{suggestions.map((s) => <option key={s} value={s} />)}</datalist> : null}
    </div>
  )
}

function DetailFields(p: FieldProps) {
  const has = (f: string) => p.ed.order.includes(f)
  const text = (name: string, label: string, opts: { placeholder?: string; hint?: string; inputMode?: 'numeric'; wide?: boolean; maxLength?: number } = {}) => has(name) && (
    <FieldShell name={name} label={label} p={p} wide={opts.wide} hint={opts.hint}>
      {(id, d) => <input id={id} className="input" value={p.val(name)} placeholder={opts.placeholder} aria-describedby={d} inputMode={opts.inputMode} maxLength={opts.maxLength} onChange={(e) => p.set(name, e.target.value)} />}
    </FieldShell>
  )
  const list = (name: string, label: string, placeholder: string, hint?: string) => has(name) && (
    <FieldShell name={name} label={label} p={p} wide hint={hint}>
      {(id, d) => <TokenField id={id} value={listOf(p.val(name))} onChange={(v) => p.set(name, joinOf(v))} placeholder={placeholder} describedBy={d} />}
    </FieldShell>
  )
  return (
    <div className="ed-fields">
      {text('title', t('Title'), { wide: true })}
      {text('sortTitle', t('Sort name'), { placeholder: t('Where this should sit in A to Z lists'), wide: true })}
      {text('year', t('Year'), { inputMode: 'numeric', maxLength: 4 })}
      {text('director', p.ed.kind === 'show' ? t('Created by') : t('Director'))}
      {p.ed.kind === 'episode' && text('season', t('Season'), { inputMode: 'numeric' })}
      {p.ed.kind === 'episode' && text('episode', t('Episode'), { inputMode: 'numeric' })}
      {list('edition', t('Edition'), t("Director's Cut, Remastered"), t('Separate them with commas. Shows on its card, and can be filtered by.'))}
      {text('series', t('Collection'), { placeholder: t('The film series it belongs to'), wide: true, hint: t('Two or more with the same name appear as a collection on the library page.') })}
      {text('tagline', t('Tagline'), { wide: true })}
      {has('overview') && (
        <FieldShell name="overview" label={t('Overview')} p={p} wide>
          {(id) => <textarea id={id} className="input" rows={5} value={p.val('overview')} onChange={(e) => p.set('overview', e.target.value)} />}
        </FieldShell>
      )}
      {list('genres', t('Genres'), t('Comedy, Family'), t('These make the categories on the library page.'))}
      {list('tags', t('Tags'), t('Add your own words'), t('Your own groups, for filtering and saved views.'))}
      {has('contentRating') && (
        <div className={'ed-field wide' + (p.isLocked('contentRating') ? ' locked' : '')}>
          <div className="ed-field-head">
            <span className="ed-source">{sourceWords(p.ed.fields.contentRating)}</span>
            <div className="ed-field-tools">
              <LockButton label={t('Content rating')} locked={p.isLocked('contentRating')} onClick={() => p.toggleLock('contentRating')} />
            </div>
          </div>
          <RatingField tv={p.ed.kind !== 'movie'} value={p.val('contentRating')} onChange={(v) => p.set('contentRating', v)} />
        </div>
      )}
      {text('rating', t('Score out of 10'), { inputMode: 'numeric' })}
      {text('network', t('Network'), { placeholder: t('The network it first aired on') })}
      {text('status', t('Status'), { placeholder: t('Continuing, Ended') })}
      {list('studios', t('Studios'), t('Who made it'))}
    </div>
  )
}

const ART: { kind: ArtKind; label: string; change: string; hint: string; wide?: boolean }[] = [
  { kind: 'poster', label: tk('Poster'), change: tk('Change the poster'), hint: tk('The tall picture on cards.') },
  { kind: 'backdrop', label: tk('Backdrop'), change: tk('Change the backdrop'), hint: tk('Behind the title on its page.'), wide: true },
  { kind: 'banner', label: tk('Banner'), change: tk('Change the banner'), hint: tk('A wide strip for rows and guides.'), wide: true },
  { kind: 'logo', label: tk('Title logo'), change: tk('Change the title logo'), hint: tk('The name as a picture, over its backdrop.'), wide: true },
]

type ArtProps = FieldProps & { target: Target; id: number; onApplied: (e: TitleEditor, b: string, w: string) => void }

function ArtworkSection(p: ArtProps) {
  const [picking, setPicking] = useState<ArtKind | null>(null)
  const kinds = ART.filter((a) => p.ed.order.includes(a.kind))
  return (
    <section id="ed-artwork" className="ed-section" aria-labelledby="ed-artwork-h">
      <h3 id="ed-artwork-h">{t('Artwork')}</h3>
      <p className="muted ed-help">{t('Change opens the picture picker: what each source has, a file of your own, an address or a frame from the video. A picture is used as soon as you apply it, and can be undone.')}</p>
      <div className="ed-art-grid">
        {kinds.map((a) => <ArtSlot key={a.kind} a={a} p={p} onChange={() => setPicking(a.kind)} />)}
      </div>
      {picking && <TitleArtPicker target={p.target} id={p.id} ed={p.ed} start={picking} onApplied={p.onApplied} onClose={() => setPicking(null)} />}
    </section>
  )
}

function ArtSlot({ a, p, onChange }: { a: (typeof ART)[number]; p: ArtProps; onChange: () => void }) {
  const [broken, setBroken] = useState(false)
  const value = p.val(a.kind)
  useEffect(() => setBroken(false), [value])
  const showable = value && value !== 'lazy' && !broken
  return (
    <div className={'ed-art' + (a.kind === 'poster' ? ' tall' : ' wide')}>
      <div className="ed-art-pic">
        {showable ? <img src={artPreviewUrl(value)} alt="" onError={() => setBroken(true)} /> : <span className="muted">{t('None yet')}</span>}
      </div>
      <div className="ed-art-info">
        <div className="ed-field-head">
          <b>{tx(a.label)}</b>
          <LockButton label={tx(a.label)} locked={p.isLocked(a.kind)} onClick={() => p.toggleLock(a.kind)} />
        </div>
        <span className="ed-source">{value !== (p.ed.fields[a.kind]?.value ?? '') ? t('Not saved yet') : sourceWords(p.ed.fields[a.kind]) || tx(a.hint)}</span>
        <div className="ed-art-btns">
          <button type="button" className="btn sm" aria-haspopup="dialog" aria-label={tx(a.change)} onClick={onChange}><IconImage size={16} /> {t('Change')}</button>
          {value && <button type="button" className="btn sm ghost" onClick={() => p.set(a.kind, '')}>{t('Remove')}</button>}
          {p.ed.fields[a.kind]?.canRevert && <button type="button" className="btn sm ghost" onClick={() => p.revert(a.kind)}>{t('Revert')}</button>}
        </div>
      </div>
    </div>
  )
}

function PlaybackSection(p: FieldProps & { target: Target; id: number; dirty: boolean; onError: (s: string) => void }) {
  const nav = useNavigate()
  const end = p.val('introEnd')
  const [text, setText] = useState(end ? fmtClock(Number(end)) : '')
  const [bad, setBad] = useState(false)
  useEffect(() => { setText(end ? fmtClock(Number(end)) : '') }, [end])
  const commit = (s: string) => {
    const secs = parseClock(s)
    setBad(secs !== null && Number.isNaN(secs))
    if (secs === null) p.set('introEnd', '')
    else if (!Number.isNaN(secs)) p.set('introEnd', String(secs))
  }
  const pick = async () => {
    if (p.dirty) { p.onError(t('Save or undo your changes first: picking a time leaves this dialog.')); return }
    if (p.target === 'item') { nav(`/play/${p.id}?pickIntro=1`); return }
    // For a show: its first episode.
    try {
      const d = await api.get<{ show: Show; episodes: Item[] }>(`/shows/${p.id}`)
      const first = d.episodes[0]
      if (first) nav(`/play/${first.id}?pickIntro=show`)
      else p.onError(t('This show has no episodes to pick a time from.'))
    } catch (e: any) { p.onError(e.message) }
  }
  // An episode or film has every part, set through its markers. The older single time stays
  // for a show (its own intro, for every episode without one), and for a title that still
  // has one typed in, so that it can be seen and taken away.
  const typed = !!p.ed.fields.introEnd?.value && !p.ed.introAuto
  const single = p.target === 'show' || typed
  return (
    <section id="ed-playback" className="ed-section" aria-labelledby="ed-playback-h">
      <h3 id="ed-playback-h">{t('Skipping')}</h3>
      {single && (
        <FieldShell name="introEnd" label={t('Where the intro ends')} p={p} wide hint={p.target === 'show' ? t("Used for every episode that has no intro of its own. Blank means work it out from each file's chapters. An episode's own recap, credits and advert breaks are set from that episode's Edit.") : t('An older, single time. The intro set below wins over it; blank it to leave the times below in charge.')}>
          {(id, d) => (
            <div className="field-row">
              <input id={id} className="input" value={text} placeholder={t('auto')} inputMode="numeric" aria-describedby={d} aria-invalid={bad} onChange={(e) => setText(e.target.value)} onBlur={(e) => commit(e.target.value)} />
              {p.target === 'show' && <button type="button" className="btn" onClick={pick}>{t('Pick in player')}</button>}
            </div>
          )}
        </FieldShell>
      )}
      {bad && <div className="error" role="alert">{t('Type the time as minutes and seconds, such as 1:05.')}</div>}
      {p.target === 'show' && p.ed.introAuto && !bad && <p className="muted ed-help">{t("Found in the file's chapters.")}</p>}
      {p.target === 'item' && <MarkersEditor id={p.id} dirty={p.dirty} onError={p.onError} />}
    </section>
  )
}

function OrganiseSection({ ed, target, id, onApplied, onError }: { ed: TitleEditor; target: Target; id: number; onApplied: (e: TitleEditor, b?: string, w?: string) => void; onError: (s: string) => void }) {
  const [attach, setAttach] = useState<'' | 'extra' | 'version'>('')
  const [moveTo, setMoveTo] = useState('')
  const call = async (fn: () => Promise<TitleEditor>, ok: string) => {
    onError('')
    try { onApplied(await fn()); toast(ok) } catch (e: any) { onError(e.message) }
  }
  const unlink = (itemId: number, ok: string) => call(() => api.del<TitleEditor>(`/meta/item/${itemId}/link`).then(() => api.get<TitleEditor>(`/meta/${target}/${id}`)), ok)
  return (
    <section id="ed-organise" className="ed-section" aria-labelledby="ed-organise-h">
      <h3 id="ed-organise-h">{t('Organise')}</h3>
      {target === 'item' && (
        <div className="ed-block">
          <h4>{t('Bonus feature')}</h4>
          {ed.link ? (
            <>
              <p>{ed.link.link === 'version'
                ? t("This is another version of {name}, so it isn't listed by itself.", { name: ed.link.parentName })
                : t("This is a {kind} of {name}, so it isn't listed by itself.", { kind: ed.link.extraType ? tx(ed.link.extraType.replace(/-/g, ' ')) : t('bonus feature'), name: ed.link.parentName })}</p>
              <button type="button" className="btn" onClick={() => unlink(id, t('It has its own place in the library again'))}>{t('Make it a title of its own')}</button>
            </>
          ) : (
            <>
              <p>{ed.kind === 'episode' ? t('This shows as an episode of its own.') : t('This shows as a film of its own.')}</p>
              <p className="muted ed-help">{t("If it's really a trailer, a featurette or another extra, put it with the film or show it belongs to.")}</p>
              {attach === 'extra'
                ? <AttachPicker ed={ed} target={target} id={id} link="extra" onDone={(e) => { setAttach(''); onApplied(e); toast(t('Moved to its page')) }} onCancel={() => setAttach('')} onError={onError} />
                : <button type="button" className="btn" onClick={() => setAttach('extra')}>{t("It's a bonus feature")}</button>}
            </>
          )}
        </div>
      )}
      {ed.extras.length > 0 && (
        <div className="ed-block">
          <h4>{t('Bonus features here')}</h4>
          <ul className="ed-attached">
            {ed.extras.map((x) => (
              <li key={x.id}><span className="ed-attached-name">{x.title}</span><span className="muted">{tx((x.extraType || '').replace(/-/g, ' '))}</span>
                <button type="button" className="btn sm ghost" onClick={() => unlink(x.id, t('It has its own place in the library again'))}>{t('Make it a title of its own')}</button></li>
            ))}
          </ul>
        </div>
      )}
      {target === 'item' && ed.kind === 'movie' && !ed.link && (
        <div className="ed-block">
          <h4>{t('Versions')}</h4>
          <p className="muted ed-help">{t('Two copies of the same film, such as a DVD and a 4K file, can be one title with a choice of version.')}</p>
          {ed.versions.length > 0 && (
            <ul className="ed-attached">
              {ed.versions.map((x) => (
                <li key={x.id}><span className="ed-attached-name">{x.label || x.title}</span><span className="muted">{x.height ? `${x.height}p` : ''}</span>
                  <button type="button" className="btn sm ghost" onClick={() => unlink(x.id, t('Split into a title of its own'))}>{t('Split it off')}</button></li>
              ))}
            </ul>
          )}
          {attach === 'version'
            ? <AttachPicker ed={ed} target={target} id={id} link="version" onDone={(e) => { setAttach(''); onApplied(e); toast(t('Merged')) }} onCancel={() => setAttach('')} onError={onError} />
            : <button type="button" className="btn" onClick={() => setAttach('version')}>{t('This is another version of…')}</button>}
        </div>
      )}
      <div className="ed-block">
        <h4>{t('In the wrong library?')}</h4>
        {ed.move.moved && <p className="muted ed-help">{t('Moved here from {name}.', { name: ed.move.moved.name })}</p>}
        {ed.move.can ? (
          <div className="field-row">
            <select className="select" aria-label={t('Library to move it to')} value={moveTo} onChange={(e) => setMoveTo(e.target.value)}>
              <option value="">{t('Choose a library')}</option>
              {ed.move.options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
            </select>
            <button type="button" className="btn" disabled={!moveTo} onClick={() => call(() => api.post<TitleEditor>(`/meta/${target}/${id}/move`, { library: Number(moveTo) }), t('Moved. Its files stay where they are.'))}>{t('Move to')}</button>
          </div>
        ) : (
          <>
            <button type="button" className="btn" disabled>{t('Not right? Move to')}</button>
            <p className="muted ed-help">{tx(ed.move.why)}</p>
          </>
        )}
      </div>
    </section>
  )
}

// Choosing what to attach a title to, from a search of its library.
function AttachPicker({ ed, target, id, link, onDone, onCancel, onError }: { ed: TitleEditor; target: Target; id: number; link: 'extra' | 'version'; onDone: (e: TitleEditor) => void; onCancel: () => void; onError: (s: string) => void }) {
  const [q, setQ] = useState('')
  const [rows, setRows] = useState<{ type: 'item' | 'show'; id: number; title: string; year?: number | null }[] | null>(null)
  const [extraType, setExtraType] = useState(ed.extraTypes[0] || 'trailer')
  const [label, setLabel] = useState('')
  useEffect(() => {
    const t = setTimeout(() => {
      const scope = link === 'version' ? `&library=${ed.libraryId}&kind=&` : ''
      api.get<{ results: ({ type: 'item'; item: Item } | { type: 'show'; show: Show })[] }>(`/browse?search=${encodeURIComponent(q)}&limit=8&sort=title${scope}`)
        .then((r) => setRows(r.results.map((x) => (x.type === 'item' ? { type: 'item' as const, id: x.item.id, title: x.item.title, year: x.item.year } : { type: 'show' as const, id: x.show.id, title: x.show.title, year: x.show.year }))
          .filter((x) => !(x.type === 'item' && x.id === id) && (link !== 'version' || x.type === 'item'))))
        .catch(() => setRows([]))
    }, q ? 250 : 0)
    return () => clearTimeout(t)
  }, [q, id, link, ed.libraryId])
  const pick = async (r: { type: 'item' | 'show'; id: number }) => {
    onError('')
    try {
      onDone(await api.post<TitleEditor>(`/meta/${target}/${id}/link`, { link, extraType, label, parentItem: r.type === 'item' ? r.id : undefined, parentShow: r.type === 'show' ? r.id : undefined }))
    } catch (e: any) { onError(e.message) }
  }
  return (
    <div className="ed-picker">
      {link === 'extra' ? (
        <label>{t('What it is')}
          <select className="select" value={extraType} onChange={(e) => setExtraType(e.target.value)}>
            {ed.extraTypes.map((x) => <option key={x} value={x}>{tx(x.replace(/-/g, ' ').replace(/^./, (c) => c.toUpperCase()))}</option>)}
          </select>
        </label>
      ) : (
        <label>{t('What to call this version')} <span className="muted">{t('(optional)')}</span>
          <input className="input" value={label} placeholder={t("Director's Cut, 4K")} onChange={(e) => setLabel(e.target.value)} />
        </label>
      )}
      <label>{link === 'extra' ? t('The film or show it belongs to') : t('The film it is a version of')}
        <input className="input" autoFocus value={q} placeholder={t('Search your library')} onChange={(e) => setQ(e.target.value)} />
      </label>
      <ul className="ed-pick-list" aria-label={t('Results')}>
        {rows?.map((r) => <li key={r.type + r.id}><button type="button" className="btn ghost" onClick={() => pick(r)}>{r.title}{r.year ? ` (${r.year})` : ''}</button></li>)}
        {rows?.length === 0 && <li className="muted">{t('Nothing by that name.')}</li>}
      </ul>
      <button type="button" className="btn sm ghost" onClick={onCancel}>{t('Never mind')}</button>
    </div>
  )
}
