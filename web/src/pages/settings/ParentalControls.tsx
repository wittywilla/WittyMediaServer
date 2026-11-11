// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useState, type KeyboardEvent } from 'react'
import { api, profileName, type Access, type KindLimit, type ParentalLists, type RatingSystem, type ScreenTime, type TimeRules, type UserView } from '../../api'
import { toast, useApp } from '../../state'
import { Button } from '../../ui/Button'
import { Field, Input, Select, Switch } from '../../ui/Field'
import { ask, Dialog } from '../../ui/Overlay'
import { AVATAR_COLOUR_NAMES, AVATAR_NAMES, AvatarShape, avatarColourOf, avatarLabel, parseAvatar } from '../../ui/avatars'
import './parental.css'

// The parts of a profile's limits beyond which titles it sees: an age for each kind of media,
// what it may do, when it may watch and for how long, and what a parent can see of its day.
// Everything here is only the form. The server applies every limit, on every route.

export const KIND_LABEL: Record<string, string> = { film: 'Films and videos', tv: 'TV shows', game: 'Games', music: 'Music and audio', book: 'Books' }

let lists: Promise<ParentalLists> | null = null
// The rating systems and the rest of what Settings offers. Fixed for a server, so asked for once.
export function useParentalLists() {
  const [v, setV] = useState<ParentalLists | null>(null)
  useEffect(() => {
    lists ??= api.get<ParentalLists>('/parental/ratings')
    lists.then(setV).catch(() => { lists = null })
  }, [])
  return v
}

// The ratings of each system that an age lets through, highest first: "PG-13, 12A, FSK 12".
function allowedAt(systems: RatingSystem[], kind: string, age: number) {
  const out: string[] = []
  for (const s of systems) {
    if (!s.kinds.includes(kind)) continue
    const ok = s.ratings.filter((r) => r.age <= age)
    if (ok.length) out.push(ok[ok.length - 1].label)
  }
  return [...new Set(out)].join(', ')
}

// The ages worth offering for a kind: each one where some system's next rating begins.
function agesFor(systems: RatingSystem[], kind: string) {
  const ages = new Set<number>()
  for (const s of systems) if (s.kinds.includes(kind)) for (const r of s.ratings) ages.add(r.age)
  return [...ages].sort((a, b) => a - b)
}

// An age limit of its own for each kind of media. A kind left alone follows the limit above.
export function KindLimits({ access, set, lists }: { access: Access; set: (p: Partial<Access>) => void; lists: ParentalLists | null }) {
  const kinds = access.kinds || {}
  const change = (kind: string, v: KindLimit | null) => {
    const next = { ...kinds }
    if (v) next[kind] = v
    else delete next[kind]
    set({ kinds: next })
  }
  if (!lists) return null
  return (
    <div className="pc-kinds">
      {lists.kinds.map((kind) => {
        const mine = kinds[kind]
        const value = mine ? String(mine.maxAge) : 'same'
        const ages = agesFor(lists.systems, kind)
        if (mine && mine.maxAge >= 0 && !ages.includes(mine.maxAge)) ages.push(mine.maxAge)
        ages.sort((a, b) => a - b)
        return (
          <div className="pc-kind" key={kind}>
            <Field label={KIND_LABEL[kind] || kind}>
              <Select value={value} onChange={(e) => {
                const v = e.target.value
                change(kind, v === 'same' ? null : { maxAge: Number(v), allowUnrated: mine?.allowUnrated ?? access.allowUnrated })
              }}>
                <option value="same">The same as everything else</option>
                {ages.map((a) => <option key={a} value={a}>{a === 0 ? 'For everyone' : `Up to age ${a}`}{allowedAt(lists.systems, kind, a) ? ` (${allowedAt(lists.systems, kind, a)})` : ''}</option>)}
                <option value="-1">No age limit</option>
              </Select>
            </Field>
            {mine && mine.maxAge >= 0 && (
              <label className="check-row"><input type="checkbox" checked={mine.allowUnrated} onChange={(e) => change(kind, { ...mine, allowUnrated: e.target.checked })} /> Also those with no rating</label>
            )}
          </div>
        )
      })}
    </div>
  )
}

const FEATURES: { id: string; label: string; hint: string; adultOnly?: boolean; kidsLabel?: string }[] = [
  { id: 'games', label: 'Games', hint: 'Play games here or on a game PC. Off hides game libraries too.' },
  { id: 'recordings', label: 'Recording Live TV', hint: 'Record programmes and keep recording rules.' },
  { id: 'downloads', label: 'Downloads', hint: 'Save a copy of a photo or video to their device.' },
  { id: 'sharing', label: 'Share links', hint: 'Make links that open a photo, video or album without signing in.', adultOnly: true },
  { id: 'comments', label: 'Comments', hint: 'Read and write comments.', kidsLabel: 'Read comments. Kids and Guest profiles never write them.' },
  { id: 'remote', label: 'Use away from home', hint: 'Off keeps this profile to your home network: it is signed out everywhere else.' },
  { id: 'editProfile', label: 'Change their own name and picture', hint: 'Their username and password stay with whoever manages them.' },
]

// What the profile may do, one switch each. Share links are for grown-up accounts whatever is chosen.
export function FeatureSwitches({ access, set, kind }: { access: Access; set: (p: Partial<Access>) => void; kind: string }) {
  const id = useId()
  const off = access.off || []
  const flip = (f: string, on: boolean) => set({ off: on ? off.filter((x) => x !== f) : [...new Set([...off, f])] })
  const grownUp = kind === 'adult'
  return (
    <div className="pc-switches">
      {FEATURES.map((f) => {
        const never = !!f.adultOnly && !grownUp
        return (
          <div className="pc-switch" key={f.id}>
            <div>
              <div id={id + f.id}>{f.label}</div>
              <div className="hint">{never ? 'Only for adult accounts.' : !grownUp && f.kidsLabel ? f.kidsLabel : f.hint}</div>
            </div>
            <Switch labelledBy={id + f.id} disabled={never} checked={!never && !off.includes(f.id)} onChange={(on) => flip(f.id, on)} />
          </div>
        )
      })}
    </div>
  )
}

const NO_TIME: TimeRules = { schoolDayMinutes: 0, weekendMinutes: 0, from: '', schoolNightUntil: '', weekendUntil: '' }
const ALLOWANCES = [0, 30, 45, 60, 90, 120, 180, 240, 360]
const allowanceLabel = (m: number) => m === 0 ? 'No limit' : m < 60 ? `${m} minutes` : m % 60 === 0 ? `${m / 60} ${m === 60 ? 'hour' : 'hours'}` : `${Math.floor(m / 60)} h ${m % 60} min`

// When the profile may watch, and for how long each day.
export function TimeRulesEditor({ access, set }: { access: Access; set: (p: Partial<Access>) => void }) {
  const t = access.time || NO_TIME
  const change = (p: Partial<TimeRules>) => set({ time: { ...t, ...p } })
  const allowance = (label: string, hint: string, key: 'schoolDayMinutes' | 'weekendMinutes') => {
    const opts = ALLOWANCES.includes(t[key]) ? ALLOWANCES : [...ALLOWANCES, t[key]].sort((a, b) => a - b)
    return (
      <Field label={label} hint={hint}>
        <Select value={String(t[key])} onChange={(e) => change({ [key]: Number(e.target.value) })}>
          {opts.map((m) => <option key={m} value={m}>{allowanceLabel(m)}</option>)}
        </Select>
      </Field>
    )
  }
  const time = (label: string, hint: string, key: 'from' | 'schoolNightUntil' | 'weekendUntil') => (
    <Field label={label} hint={hint}><Input type="time" value={t[key]} onChange={(e) => change({ [key]: e.target.value })} /></Field>
  )
  return (
    <div className="pc-time">
      <p className="muted">Time is counted by the server while this profile is playing something, on any device. When it runs out, what is playing stops and the profile is told why. A parent can give more.</p>
      <div className="pc-grid">
        {allowance('School days', 'Monday to Friday.', 'schoolDayMinutes')}
        {allowance('Weekends', 'Saturday and Sunday.', 'weekendMinutes')}
      </div>
      <div className="pc-grid">
        {time('Not before', 'Leave empty for no earliest time. With a bedtime and nothing here, the day starts at 6:00.', 'from')}
        {time('Bedtime on school nights', 'Sunday to Thursday evenings.', 'schoolNightUntil')}
        {time('Bedtime on Fridays and Saturdays', 'Leave empty for none.', 'weekendUntil')}
      </div>
      <p className="muted">School holidays aren’t known to the server: change these when they start.</p>
    </div>
  )
}

type CollectionRow = { id: number; name: string; kind: string; owner: string; count: number; system?: string }

// Collections whose titles are always or never allowed. Only collections that list their titles.
export function CollectionRules({ access, set }: { access: Access; set: (p: Partial<Access>) => void }) {
  const [rows, setRows] = useState<CollectionRow[] | null>(null)
  useEffect(() => { api.get<CollectionRow[]>('/collections').then((r) => setRows(r.filter((c) => c.kind === 'manual' && !c.system))).catch(() => setRows([])) }, [])
  if (!rows) return null
  if (rows.length === 0) return <p className="muted">You have no collections yet. Make one under Collections, add titles to it, and it can be allowed or hidden here as a whole.</p>
  const allow = access.allowCollections || [], block = access.blockCollections || []
  const choose = (id: number, v: string) => set({
    allowCollections: v === 'allow' ? [...new Set([...allow, id])] : allow.filter((x) => x !== id),
    blockCollections: v === 'block' ? [...new Set([...block, id])] : block.filter((x) => x !== id),
  })
  return (
    <div className="pc-collections">
      {rows.map((c) => (
        <div className="pc-switch" key={c.id}>
          <div><div>{c.name}</div><div className="hint">{c.count} {c.count === 1 ? 'title' : 'titles'}</div></div>
          <Select inputSize="sm" aria-label={`Titles in ${c.name}`} value={block.includes(c.id) ? 'block' : allow.includes(c.id) ? 'allow' : ''} onChange={(e) => choose(c.id, e.target.value)}
            options={[{ value: '', label: 'By their rating' }, { value: 'allow', label: 'Always allow' }, { value: 'block', label: 'Never allow' }]} />
        </div>
      ))}
    </div>
  )
}

// Tags of videos to hide, typed with commas between.
export function TagRules({ access, set }: { access: Access; set: (p: Partial<Access>) => void }) {
  const [text, setText] = useState((access.blockTags || []).join(', '))
  return (
    <Field label="Hide videos with these tags" hint="For video libraries with tags. Commas between them.">
      <Input value={text} onChange={(e) => { setText(e.target.value); set({ blockTags: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) }) }} />
    </Field>
  )
}

// Choosing a picture and its colour.
export function AvatarPicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const chosen = parseAvatar(value)
  const colour = chosen?.colour || 'blue'
  return (
    <fieldset className="pc-avatars">
      <legend>Picture</legend>
      <div className="pc-avatar-grid" role="radiogroup" aria-label="Picture">
        <button type="button" role="radio" aria-checked={!chosen} className={'pc-avatar none' + (!chosen ? ' on' : '')} onClick={() => onChange('')}>Their initial</button>
        {AVATAR_NAMES.map((n) => {
          const a = parseAvatar(n + ':' + colour)!
          return (
            <button key={n} type="button" role="radio" aria-checked={chosen?.name === n} aria-label={avatarLabel(n)} className={'pc-avatar' + (chosen?.name === n ? ' on' : '')} onClick={() => onChange(n + ':' + colour)}>
              <span className="avatar-tile" style={{ background: a.background }}><AvatarShape shape={a.shape} /></span>
            </button>
          )
        })}
      </div>
      {chosen && (
        <div className="pc-avatar-grid" role="radiogroup" aria-label="Colour">
          {AVATAR_COLOUR_NAMES.map((c) => (
            <button key={c} type="button" role="radio" aria-checked={colour === c} aria-label={avatarLabel(c)} className={'pc-colour' + (colour === c ? ' on' : '')} onClick={() => onChange(chosen.name + ':' + c)}>
              <span style={{ background: avatarColourOf(c) }} />
            </button>
          ))}
        </div>
      )}
    </fieldset>
  )
}

const hm = (sec: number) => {
  const m = Math.round(sec / 60)
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${m % 60 ? `${m % 60} min` : ''}`.trim()
}
const dayText = (day: number) => new Date(Math.floor(day / 10000), Math.floor(day / 100) % 100 - 1, day % 100).toLocaleDateString([], { weekday: 'long', day: 'numeric', month: 'long' })
const REFUSED: Record<string, string> = { blocked: 'Asked for a hidden title', time: 'Tried to play when time was up', feature: 'Tried something that is turned off', extra: 'Was given more time' }
const WHY: Record<string, string> = { allowance: 'the day’s time was used up', bedtime: 'it was past bedtime', early: 'it was too early', games: 'games', recordings: 'recording', downloads: 'downloads', sharing: 'share links', comments: 'comments' }

type Activity = {
  days: { day: number; screenSec: number; extraSec: number }[]
  watched: { day: number; itemId: number; title: string; showTitle?: string; kind: string; seconds: number; lastAt: number }[]
  events: { at: number; kind: string; title: string; detail: string }[]
  screenTime: ScreenTime
}

// What a profile watched and when, what it was refused, and more time for today.
// The report scrolls inside its dialog, and a remote or a keyboard has only arrows to read it
// with. Each day, and the list of what was refused, takes the focus as a whole; when one is
// longer than the window shows, the arrow reads on through it before moving to the next.
function readOn(e: KeyboardEvent<HTMLDivElement>) {
  const block = e.target as HTMLElement
  const down = e.key === 'ArrowDown'
  if ((!down && e.key !== 'ArrowUp') || !block.classList?.contains('pc-block')) return
  const view = e.currentTarget, v = view.getBoundingClientRect(), b = block.getBoundingClientRect()
  const more = down ? b.bottom > v.bottom + 4 && view.scrollTop < view.scrollHeight - view.clientHeight - 1 : b.top < v.top - 4 && view.scrollTop > 0
  if (!more) return
  e.preventDefault()
  view.scrollBy({ top: (down ? 1 : -1) * Math.max(80, view.clientHeight * 0.6) })
}

export function ActivityDialog({ user, onClose }: { user: UserView; onClose: () => void }) {
  const [a, setA] = useState<Activity | null>(null)
  const [err, setErr] = useState('')
  const name = profileName(user)
  const load = () => api.get<Activity>(`/users/${user.id}/activity?days=14`).then(setA).catch((e) => setErr((e as Error).message))
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const give = async (m: number) => {
    try { await api.post(`/users/${user.id}/screen-time/extra`, { minutes: m }); toast(`${name} has ${m} more minutes today`, { tone: 'ok' }); load() } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const clear = async () => {
    if (!await ask({ title: `Clear what’s written down about ${name}?`, body: 'What they watched and what was refused is forgotten. Today’s screen time stays.', confirm: 'Clear', danger: true })) return
    try { await api.del(`/users/${user.id}/activity`); load() } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const st = a?.screenTime
  const days = a ? [...new Set([...a.days.map((d) => d.day), ...a.watched.map((w) => w.day)])].sort((x, y) => y - x) : []
  return (
    <Dialog title={`${name}: the last two weeks`} size="wide" onClose={onClose}
      actions={<><Button variant="ghost" onClick={clear} disabled={!a}>Clear</Button><Button variant="primary" onClick={onClose}>Done</Button></>}>
      {err && <div className="error" role="alert">{err}</div>}
      {!a && !err && <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>}
      {a && (
        <div className="pc-activity" onKeyDown={readOn}>
          <section>
            <h3>Today</h3>
            <p>{st && st.allowanceSec >= 0 ? `${hm(st.usedSec)} of ${hm(st.allowanceSec)} used.` : `${hm(st?.usedSec || 0)} of screen time.`}{st && !st.allowed ? ` ${st.message}` : ''}</p>
            <div className="pc-give" role="group" aria-label={`Give ${name} more time today`}>
              <span className="muted">Give more time today:</span>
              {[15, 30, 60].map((m) => <Button key={m} size="sm" onClick={() => give(m)}>{m === 60 ? '1 hour' : `${m} minutes`}</Button>)}
            </div>
          </section>
          <section>
            <h3>What they watched</h3>
            {days.length === 0 && <p className="muted">Nothing yet. This is kept only for profiles with limits, and they can’t clear it themselves.</p>}
            {days.map((day) => {
              const total = a.days.find((d) => d.day === day)
              return (
                <div className="pc-day pc-block" key={day} tabIndex={0} role="group" aria-label={`${dayText(day)}${total ? `, ${hm(total.screenSec)} of screen time` : ''}`}>
                  <div className="pc-day-head"><b>{dayText(day)}</b><span className="muted">{total ? `${hm(total.screenSec)} of screen time${total.extraSec ? `, with ${hm(total.extraSec)} extra given` : ''}` : ''}</span></div>
                  <ul className="pc-rows">
                    {a.watched.filter((w) => w.day === day).map((w) => (
                      <li key={w.itemId}><span>{w.showTitle ? `${w.showTitle}: ` : ''}{w.title || 'A title that has gone'}</span><span className="muted">{hm(w.seconds)}</span></li>
                    ))}
                  </ul>
                </div>
              )
            })}
          </section>
          <section>
            <h3>What was refused, and extra time</h3>
            {a.events.length === 0 && <p className="muted">Nothing.</p>}
            <ul className={'pc-rows' + (a.events.length ? ' pc-block' : '')} tabIndex={a.events.length ? 0 : undefined} aria-label="What was refused, and extra time">
              {a.events.map((e, i) => (
                <li key={i}>
                  <span>{REFUSED[e.kind] || e.kind}{e.kind === 'blocked' ? `: ${e.title}${e.detail ? ` (${e.detail})` : ''}` : e.kind === 'extra' ? `: ${e.detail} minutes, by ${e.title === 'PIN' ? 'the parents’ PIN' : e.title}` : WHY[e.detail] ? `: ${WHY[e.detail]}` : ''}</span>
                  <span className="muted">{new Date(e.at * 1000).toLocaleString([], { weekday: 'short', hour: 'numeric', minute: '2-digit' })}</span>
                </li>
              ))}
            </ul>
          </section>
        </div>
      )}
    </Dialog>
  )
}

// The parents' PIN: what a parent types on a child's device to give more time.
export function ParentPin() {
  const { user } = useApp()
  const [isSet, setIsSet] = useState<boolean | null>(null)
  const [pin, setPin] = useState('')
  const [current, setCurrent] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<{ set: boolean }>('/parental/pin').then((r) => setIsSet(r.set)).catch(() => setIsSet(false)) }, [])
  const save = async (value: string) => {
    setErr(''); setBusy(true)
    try {
      const r = await api.put<{ set: boolean }>('/parental/pin', { pin: value, current, code })
      setIsSet(r.set); setPin(''); setCurrent(''); setCode('')
      toast(r.set ? 'The parents’ PIN is set' : 'The parents’ PIN is removed', { tone: 'ok' })
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const ready = /^\d{4,12}$/.test(pin) && !!current && (!user?.twoFactor || !!code)
  return (
    <div className="panel" id="parents-pin">
      <h2>Parents’ PIN</h2>
      <p className="muted">When a profile’s screen time runs out, a parent can type this PIN on that device to give it more time today. It does nothing else, so it can be short. {isSet ? 'A PIN is set.' : 'No PIN is set: a parent’s own username and password work instead.'}</p>
      <form className="field-row" onSubmit={(e) => { e.preventDefault(); if (ready && !busy) save(pin) }}>
        <Field label={isSet ? 'New PIN' : 'PIN'} hint="4 to 12 digits."><Input type="password" inputMode="numeric" autoComplete="off" value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 12))} /></Field>
        <Field label="Your password" error={err || undefined}><Input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
        {user?.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
        <Button type="submit" busy={busy} disabled={!ready}>{isSet ? 'Change PIN' : 'Set PIN'}</Button>
        {isSet && <Button variant="ghost" disabled={busy || !current} onClick={() => save('')}>Remove it</Button>}
      </form>
    </div>
  )
}
