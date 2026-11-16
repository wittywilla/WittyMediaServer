// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, profileName, type Access, type Item, type Library, type Show, type User, type UserView } from '../api'
import { IconTrash } from '../icons'
import { toast, useApp } from '../state'
import { Button, IconButton } from '../ui/Button'
import { Segmented, Tabs } from '../ui/Choice'
import { Field, Input, Switch } from '../ui/Field'
import { ask, Dialog } from '../ui/Overlay'
import { Avatar } from './AuthPage'
import LegalNote from '../components/LegalNote'
import { ActivityDialog, AvatarPicker, CollectionRules, FeatureSwitches, KindLimits, ParentPin, TagRules, TimeRulesEditor, useParentalLists } from './settings/ParentalControls'
import './profiles.css'
import { t, tk, tn, tx } from '../i18n'

// Settings → Users: your account, the profiles you may manage, and their
// limits. The admin manages everyone; adults manage Kids profiles unless the
// admin keeps that to themself; Kids and Guest profiles manage nothing.

export const AGE_LEVELS: { age: number; label: string; hint: string }[] = [
  { age: 0, label: tk('Little kids'), hint: 'G, U, TV-Y, TV-G' },
  { age: 7, label: tk('Kids 7+'), hint: tk('adds TV-Y7') },
  { age: 10, label: tk('Older kids 10+'), hint: tk('adds PG, TV-PG') },
  { age: 13, label: tk('Teens 13+'), hint: tk('adds PG-13, 12A') },
  { age: 16, label: tk('Older teens 16+'), hint: tk('adds TV-14, 15, FSK 16') },
  { age: -1, label: tk('Everything'), hint: tk('no age limit') },
]

const KIND_LABEL: Record<string, string> = { adult: tk('Adult'), kids: tk('Kids'), guest: tk('Guest') }

export default function Profiles() {
  const { user, setUser } = useApp()
  const [users, setUsers] = useState<UserView[]>([])
  const [mypw, setMypw] = useState('')
  const [curpw, setCurpw] = useState('')
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<UserView | null>(null)
  const [removing, setRemoving] = useState<User | null>(null)
  const [naming, setNaming] = useState<UserView | null>(null)
  const [settingPw, setSettingPw] = useState<UserView | null>(null)
  const [watching, setWatching] = useState<UserView | null>(null)
  const [mine, setMine] = useState<{ features: Record<string, boolean> } | null>(null)
  useEffect(() => { api.get<{ features: Record<string, boolean> }>('/me/limits').then(setMine).catch(() => {}) }, [user?.id])
  const load =() => api.get<UserView[]>('/users').then(setUsers).catch(() => setUsers([]))
  useEffect(() => { load() }, [])
  const changePw = async () => { try { await api.put(`/users/${user!.id}/password`, { password: mypw, current: curpw }); setMypw(''); setCurpw(''); toast(t('Password changed; other devices are signed out')) } catch (e: any) { toast(e.message) } }
  const del = async (u: User) => {
    if (u.isAdmin) { setRemoving(u); return } // this one asks for your password
    if (!await ask({ title: t('Remove “{name}”?', { name: profileName(u) }), body: t('Their watch history is deleted too. This can’t be undone.'), confirm: t('Remove profile'), danger: true })) return
    try { await api.del(`/users/${u.id}`); load() } catch (e: any) { toast(e.message) }
  }
  const restricted = user?.kind !== 'adult'
  return (
    <>
      <div className="panel">
        <h2>{t('Your account')}</h2>
        <div className="form">
          <div className="profile-me"><Avatar name={user ? profileName(user) : '?'} avatar={user?.avatar} /> <b>{user && profileName(user)}</b> <span className="pill">{user?.isAdmin ? t('Administrator') : tx(KIND_LABEL[user?.kind || 'adult'])}</span></div>
          {user && <YourNames key={user.name + (user.displayName || '') + (user.avatar || '')} canRename={!restricted && mine?.features.editProfile !== false} canLook={user.isAdmin || !!mine?.features.editProfile} onSaved={setUser} />}
          {restricted
            ? <p className="muted">{t("This profile's password and limits are managed by the adults of this server.")}</p>
            : <form className="field-row" onSubmit={(e) => { e.preventDefault(); changePw() }}>
                <label className="field-label">{t('Current password')}<input className="input" type="password" autoComplete="current-password" value={curpw} onChange={(e) => setCurpw(e.target.value)} /></label>
                <label className="field-label">{t('New password')}<input className="input" type="password" autoComplete="new-password" value={mypw} onChange={(e) => setMypw(e.target.value)} /></label>
                <button className="btn" disabled={mypw.length < 8 || !curpw}>{t('Change password')}</button>
              </form>}
        </div>
      </div>

      {!restricted && users.length > 0 && (
        <div className="panel">
          <h2>{user?.isAdmin ? t('Everyone on this server') : t('Kids profiles you manage')}</h2>
          <div className="list">
            {users.map((u) => (
              <div className="list-item profile-row" key={u.id}>
                <Avatar name={profileName(u)} avatar={u.avatar} />
                <div className="grow">
                  <b>{profileName(u)}</b> <span className={'pill kind-' + u.kind}>{u.isAdmin ? t('Administrator') : tx(KIND_LABEL[u.kind])}</span>
                  <div className="sub">{u.displayName && u.displayName !== u.name ? t('Signs in as {name}', { name: u.name }) + ' · ' : ''}{u.isAdmin ? t('No limits') : summary(u)}</div>
                </div>
                {u.id !== user!.id && (
                  <div className="profile-actions">
                    <Button size="sm" aria-label={t('Name and picture for {name}', { name: profileName(u) })} onClick={() => setNaming(u)}>{t('Name and picture')}</Button>
                    <Button size="sm" aria-label={u.kind === 'adult' ? t('Password for {name}', { name: profileName(u) }) : t('Password or PIN for {name}', { name: profileName(u) })} onClick={() => setSettingPw(u)}>{passwordWord(u)}</Button>
                    {!u.isAdmin && <Button size="sm" aria-label={t('Limits for {name}', { name: profileName(u) })} onClick={() => setEditing(u)}>{t('Limits')}</Button>}
                    {!u.isAdmin && <Button size="sm" aria-label={t('Report for {name}: what they watched, and more time', { name: profileName(u) })} onClick={() => setWatching(u)}>{t('Report')}</Button>}
                    <IconButton label={t('Remove {name}', { name: profileName(u) })} size="sm" variant="danger" onClick={() => del(u)}><IconTrash /></IconButton>
                  </div>
                )}
              </div>
            ))}
          </div>
          <div className="panel-actions"><button className="btn primary" onClick={() => setAdding(true)}>{t('Add profile')}</button></div>
        </div>
      )}
      {!restricted && users.length === 0 && !user?.isAdmin && (
        <div className="panel"><h2>{t('Kids profiles')}</h2><button className="btn primary" onClick={() => setAdding(true)}>{t('Add a Kids profile')}</button></div>
      )}
      {adding && <AddProfile admin={!!user?.isAdmin} onClose={() => setAdding(false)} onDone={() => { setAdding(false); load() }} />}
      {editing && <LimitsEditor user={editing} admin={!!user?.isAdmin} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load() }} />}
      {removing && <RemoveAdmin user={removing} onClose={() => setRemoving(null)} onDone={() => { setRemoving(null); load() }} />}
      {naming && <NamesDialog user={naming} onClose={() => setNaming(null)} onSaved={() => { setNaming(null); load() }} />}
      {settingPw && <PasswordDialog user={settingPw} onClose={() => setSettingPw(null)} onSaved={() => { setSettingPw(null); load() }} />}
      {watching && <ActivityDialog user={watching} onClose={() => setWatching(null)} />}
      {user?.isAdmin && <ParentPin />}
    </>
  )
}

// YourPassword asks again for the password of whoever is signed in, and their code if they
// use one, before a change a stolen session must not be able to make.
function YourPassword({ password, code, setPassword, setCode, autoFocus }: { password: string; code: string; setPassword: (s: string) => void; setCode: (s: string) => void; autoFocus?: boolean }) {
  const { user } = useApp()
  return (
    <>
      <label>{t('Your password')}<input className="input" type="password" autoFocus={autoFocus} autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>
      {user?.twoFactor && <label>{t('Code from your authenticator app')}<input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></label>}
    </>
  )
}

// Your own names, and which one the corner of the screen shows.
function YourNames({ canRename, canLook, onSaved }: { canRename: boolean; canLook: boolean; onSaved: (u: User) => void }) {
  const { user } = useApp()
  const [picking, setPicking] = useState(false)
  const [display, setDisplay] = useState(user?.displayName || '')
  const [username, setUsername] = useState(user?.name || '')
  const [err, setErr] = useState('')
  if (!user) return null
  const changed = display !== (user.displayName || '') || username !== user.name
  const save = async () => {
    setErr('')
    try { onSaved(await api.put<User>(`/users/${user.id}/profile`, { displayName: display, username })); toast(t('Names saved'), { tone: 'ok' }) } catch (e) { setErr((e as Error).message) }
  }
  const choose = async (on: boolean) => {
    try { onSaved(await api.put<User>(`/users/${user.id}/profile`, { showUsername: on })) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const picture = async (avatar: string) => {
    try { onSaved(await api.put<User>(`/users/${user.id}/profile`, { avatar })) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  return (
    <>
      {canRename && (
        <form className="field-row" onSubmit={(e) => { e.preventDefault(); save() }}>
          <Field label={t('Profile name')} hint={t('What you’re called around the app. Anyone can have the same one.')} error={err}><Input value={display} maxLength={60} placeholder={user.name} onChange={(e) => setDisplay(e.target.value)} /></Field>
          <Field label={t('Username')} hint={t('What you sign in with. It has to be yours alone.')}><Input value={username} maxLength={40} autoComplete="username" onChange={(e) => setUsername(e.target.value)} /></Field>
          <Button disabled={!changed || !username.trim()}>{t('Save names')}</Button>
        </form>
      )}
      {canLook && (picking
        ? <AvatarPicker value={user.avatar || ''} onChange={picture} />
        : <div><Button size="sm" onClick={() => setPicking(true)}>{t('Change your picture')}</Button></div>)}
      <div className="set-row-inline">
        <span id="show-username">{t('Show my username at the top right, not my profile name')}</span>
        <Switch labelledBy="show-username" checked={!!user.showUsername} onChange={choose} />
      </div>
    </>
  )
}

function RemoveAdmin({ user: target, onClose, onDone }: { user: User; onClose: () => void; onDone: () => void }) {
  const { user } = useApp()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const remove = async () => {
    setErr(''); setBusy(true)
    try {
      await api.del(`/users/${target.id}`, { current: password, code })
      toast(t('Profile “{name}” removed', { name: profileName(target) }))
      onDone()
    } catch (x) { setErr((x as Error).message); setBusy(false) }
  }
  return (
    <Dialog title={t('Remove the administrator “{name}”?', { name: profileName(target) })} size="sm" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="danger" disabled={busy || !password || (!!user?.twoFactor && !code)} onClick={remove}>{t('Remove')}</Button></>}>
      <div className="form full">
        <p className="muted flush">{t("Their watch history is deleted too. This asks for your password, so that someone using a device you left signed in can't do it.")}</p>
        <YourPassword autoFocus password={password} code={code} setPassword={setPassword} setCode={setCode} />
        {err && <div className="error" role="alert">{err}</div>}
      </div>
    </Dialog>
  )
}

// Someone else's names, for whoever manages their account.
function NamesDialog({ user, onClose, onSaved }: { user: UserView; onClose: () => void; onSaved: () => void }) {
  const [display, setDisplay] = useState(user.displayName || '')
  const [username, setUsername] = useState(user.name)
  const [avatar, setAvatar] = useState(user.avatar || '')
  const [err, setErr] = useState('')
  const save = async () => {
    setErr('')
    try { await api.put(`/users/${user.id}/profile`, { displayName: display, username, avatar }); toast(t('Saved'), { tone: 'ok' }); onSaved() } catch (e) { setErr((e as Error).message) }
  }
  return (
    <Dialog title={t('Name and picture for {name}', { name: profileName(user) })} onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" disabled={!username.trim()} onClick={save}>{t('Save')}</Button></>}>
      <div className="form full">
        <Field label={t('Profile name')} hint={t('What they’re called around the app.')}><Input autoFocus value={display} maxLength={60} placeholder={user.name} onChange={(e) => setDisplay(e.target.value)} /></Field>
        <Field label={t('Username')} hint={t('What they sign in with.')} error={err}><Input value={username} maxLength={40} onChange={(e) => setUsername(e.target.value)} /></Field>
        <AvatarPicker value={avatar} onChange={setAvatar} />
      </div>
    </Dialog>
  )
}

const passwordWord = (u: UserView) => u.kind === 'adult' ? t('Password') : t('Password or PIN')

// A new password for someone else, or a new PIN for a Kids or Guest profile, for whoever
// manages them. Passwords can't be read back, so this is the way out of a forgotten one.
function PasswordDialog({ user: target, onClose, onSaved }: { user: UserView; onClose: () => void; onSaved: () => void }) {
  const { user } = useApp()
  const adult = target.kind === 'adult'
  const [pw, setPw] = useState('')
  const [again, setAgain] = useState('')
  const [current, setCurrent] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const min = adult ? 8 : 4
  const bytes = new TextEncoder().encode(pw).length
  const hint = bytes > 72 ? t('That’s too long: 72 bytes at most.') : t('At least {count} characters.', { count: min })
  const mismatch = again !== '' && again !== pw
  const name = profileName(target)
  const save = async () => {
    setErr(''); setBusy(true)
    try {
      await api.put(`/users/${target.id}/password`, target.isAdmin ? { password: pw, current, code } : { password: pw })
      toast(adult ? t('New password saved for {name}. They’re signed out on their devices.', { name }) : t('New password or PIN saved for {name}. They’re signed out on their devices.', { name }), { tone: 'ok' })
      onSaved()
    } catch (e) { setErr((e as Error).message); setBusy(false) }
  }
  const remove = async () => {
    setErr(''); setBusy(true)
    try {
      await api.put(`/users/${target.id}/access`, { clearPassword: true })
      toast(t('{name} now signs in with a tap on your home network', { name }), { tone: 'ok' })
      onSaved()
    } catch (e) { setErr((e as Error).message); setBusy(false) }
  }
  const ready = [...pw].length >= min && bytes <= 72 && pw === again && (!target.isAdmin || (!!current && (!user?.twoFactor || !!code)))
  return (
    <Dialog title={adult ? t('A new password for {name}', { name }) : target.hasPassword ? t('A new password or PIN for {name}', { name }) : t('Add a password or PIN for {name}', { name })} size="sm" onClose={onClose}
      actions={<>
        {!adult && target.hasPassword && <Button variant="ghost" disabled={busy} onClick={remove}>{t('Remove it')}</Button>}
        <Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button>
        <Button variant="primary" disabled={busy || !ready} onClick={save}>{t('Save')}</Button>
      </>}>
      <form className="form full" onSubmit={(e) => { e.preventDefault(); if (ready && !busy) save() }}>
        <p className="muted flush">
          {adult ? t('Tell them what it is, and ask them to change it when they next sign in.') : t('They’ll need this to sign in.')}
          {' '}{t('The old one stops working, and they’re signed out on every device.')}
          {target.twoFactor && <>{' '}{t('Their two-factor sign-in stays on.')}</>}
        </p>
        <Field label={adult ? t('New password') : t('New password or PIN')} hint={hint}><Input autoFocus type="password" autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} /></Field>
        <Field label={t('The same again')} error={mismatch ? t('These two don’t match yet.') : undefined}><Input type="password" autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} /></Field>
        {target.isAdmin && <YourPassword password={current} code={code} setPassword={setCurrent} setCode={setCode} />}
        {err && <div className="error" role="alert">{err}</div>}
        <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
      </form>
    </Dialog>
  )
}

function summary(u: UserView): string {
  const a = u.access
  if (!a) return t('No limits')
  const parts = [tx(AGE_LEVELS.find((l) => l.age === a.maxAge)?.label) || t('Up to age {age}', { age: a.maxAge })]
  if (a.libraries?.length) parts.push(tn('{count} library', '{count} libraries', a.libraries.length))
  if (!a.liveTv) parts.push(t('no Live TV'))
  if (a.kinds && Object.keys(a.kinds).length) parts.push(t('ages by kind'))
  if (a.time && (a.time.schoolDayMinutes || a.time.weekendMinutes || a.time.from || a.time.schoolNightUntil || a.time.weekendUntil)) parts.push(t('screen time'))
  const off = (a.off || []).filter((f) => f !== 'editProfile')
  if (off.length) parts.push(tn('{count} thing turned off', '{count} things turned off', off.length))
  if (!u.hasPassword) parts.push(t('no password'))
  return parts.join(' · ')
}

// AddProfile creates an adult, Kids or Guest profile (adults: Kids only).
export function AddProfile({ admin, onClose, onDone, initialKind = admin ? 'adult' : 'kids' }: { admin: boolean; onClose: () => void; onDone: () => void; initialKind?: string }) {
  const [kind, setKind] = useState(initialKind)
  const [name, setName] = useState('')
  const [display, setDisplay] = useState('')
  const [pw, setPw] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)
  const [age, setAge] = useState(7)
  const [err, setErr] = useState('')
  const [current, setCurrent] = useState('')
  const [code, setCode] = useState('')
  const { user } = useApp()
  const needsPw = kind === 'adult'
  const newAdmin = kind === 'adult' && isAdmin
  const save = async () => {
    setErr('')
    try {
      const body: Record<string, unknown> = { name, displayName: display, password: pw, kind, isAdmin: kind === 'adult' && isAdmin }
      if (newAdmin) { body.current = current; body.code = code }
      if (kind !== 'adult') body.access = { maxAge: age, allowUnrated: false, liveTv: kind === 'kids', libraryChannels: kind === 'kids', tunerChannels: false }
      await api.post('/users', body)
      toast(t('Profile “{name}” added', { name: display || name }), { tone: 'ok' })
      onDone()
    } catch (e) { setErr((e as Error).message) }
  }
  return (
    <Dialog title={t('Add a profile')} onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" disabled={!name.trim() || (needsPw ? pw.length < 8 : pw.length > 0 && pw.length < 4) || (newAdmin && (!current || (!!user?.twoFactor && !code)))} onClick={save}>{t('Add profile')}</Button></>}>
        <div className="form full">
          {admin && (
            <Segmented big label={t('Kind of profile')} value={kind} onChange={(k) => { setKind(k); setAge(k === 'guest' ? 13 : 7) }}
              options={(['adult', 'kids', 'guest'] as const).map((k) => ({ value: k as string, label: tx(KIND_LABEL[k]) }))} />
          )}
          <p className="muted flush">
            {kind === 'adult' && t('A full account with its own password and watch history.')}
            {kind === 'kids' && t('Only shows titles up to the age you choose. A password or PIN is optional: without one, kids sign in with a tap on your home network.')}
            {kind === 'guest' && t('For visitors: limited titles, no settings. Only the administrator can change Guest limits.')}
          </p>
          {kind !== 'adult' && <LegalNote kind="ratings" />}
          <Field label={t('Username')} hint={t('What they sign in with. It has to be theirs alone.')}><Input autoFocus value={name} onChange={(e) => setName(e.target.value)} maxLength={40} /></Field>
          <Field label={t('Profile name (optional)')} hint={t('What they’re called around the app, if that’s different.')}><Input value={display} onChange={(e) => setDisplay(e.target.value)} maxLength={60} placeholder={name} /></Field>
          <Field label={needsPw ? t('Password') : t('Password or PIN (optional)')}><Input type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" /></Field>
          {kind === 'adult' && admin && <label className="check-row"><input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} /> {t('Administrator (can change everything)')}</label>}
          {newAdmin && <YourPassword password={current} code={code} setPassword={setCurrent} setCode={setCode} />}
          {kind !== 'adult' && <AgePicker value={age} onChange={setAge} />}
          {err && <div className="error" role="alert">{err}</div>}
        </div>
    </Dialog>
  )
}

export function AgePicker({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <fieldset className="age-picker">
      <legend>{t('What may they watch?')}</legend>
      {AGE_LEVELS.map((l) => (
        <label key={l.age} className={'age-opt' + (value === l.age ? ' on' : '')}>
          <input type="radio" name="age" checked={value === l.age} onChange={() => onChange(l.age)} />
          <b>{tx(l.label)}</b><span className="muted">{tx(l.hint)}</span>
        </label>
      ))}
    </fieldset>
  )
}

// LimitsEditor changes an account's limits.
function LimitsEditor({ user, admin, onClose, onSaved }: { user: UserView; admin: boolean; onClose: () => void; onSaved: () => void }) {
  const [a, setA] = useState<Access>(user.access || { maxAge: -1, allowUnrated: true, libraries: [], blockGenres: [], allowItems: [], blockItems: [], allowShows: [], blockShows: [], liveTv: true, libraryChannels: true, tunerChannels: true })
  const [libs, setLibs] = useState<Library[]>([])
  const [genres, setGenres] = useState<string[]>([])
  const [titles, setTitles] = useState<Record<string, string>>({})
  const [q, setQ] = useState('')
  const [found, setFound] = useState<{ key: string; id: number; show: boolean; title: string }[]>([])
  const [clearPw, setClearPw] = useState(false)
  const [tab, setTab] = useState<'watch' | 'do' | 'time'>('watch')
  const lists = useParentalLists()
  useEffect(() => {
    api.get<Library[]>('/libraries').then(setLibs).catch(() => {})
    Promise.all([api.get<string[]>('/genres'), api.get<string[]>('/shows/genres')]).then(([g1, g2]) => setGenres([...new Set([...g1, ...g2])].sort())).catch(() => {})
  }, [])
  // Names for the titles already allowed or blocked.
  useEffect(() => {
    const want: [string, string][] = [
      ...[...(a.allowItems || []), ...(a.blockItems || [])].map((id): [string, string] => ['i' + id, `/items/${id}`]),
      ...[...(a.allowShows || []), ...(a.blockShows || [])].map((id): [string, string] => ['s' + id, `/shows/${id}`]),
    ].filter(([k]) => !titles[k])
    want.forEach(([k, url]) => api.get<any>(url).then((x) => setTitles((prev) => ({ ...prev, [k]: x.title || x.show?.title || k }))).catch(() => {}))
  }, [a.allowItems, a.blockItems, a.allowShows, a.blockShows]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (q.trim().length < 2) { setFound([]); return }
    const timer = setTimeout(async () => {
      const [it, sh] = await Promise.all([
        api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}&limit=6&kind=`).catch(() => ({ items: [] })),
        api.get<{ shows: Show[] }>(`/shows?search=${encodeURIComponent(q)}&limit=6`).catch(() => ({ shows: [] })),
      ])
      setFound([...sh.shows.map((s) => ({ key: 's' + s.id, id: s.id, show: true, title: t('{title} (show)', { title: s.title }) })), ...it.items.filter((x) => x.showId == null).map((x) => ({ key: 'i' + x.id, id: x.id, show: false, title: x.title + (x.year ? ` (${x.year})` : '') }))])
    }, 250)
    return () => clearTimeout(timer)
  }, [q])
  const set = (p: Partial<Access>) => setA((x) => ({ ...x, ...p }))
  const toggleIn = (list: number[] | undefined, id: number) => (list || []).includes(id) ? (list || []).filter((x) => x !== id) : [...(list || []), id]
  const override = (f: { id: number; show: boolean; title: string; key: string }, allow: boolean) => {
    setTitles((prev) => ({ ...prev, [f.key]: f.title }))
    if (f.show) set({ allowShows: allow ? [...new Set([...(a.allowShows || []), f.id])] : (a.allowShows || []).filter((x) => x !== f.id), blockShows: !allow ? [...new Set([...(a.blockShows || []), f.id])] : (a.blockShows || []).filter((x) => x !== f.id) })
    else set({ allowItems: allow ? [...new Set([...(a.allowItems || []), f.id])] : (a.allowItems || []).filter((x) => x !== f.id), blockItems: !allow ? [...new Set([...(a.blockItems || []), f.id])] : (a.blockItems || []).filter((x) => x !== f.id) })
    setQ(''); setFound([])
  }
  const save = async () => {
    try { await api.put(`/users/${user.id}/access`, { access: a, clearPassword: clearPw }); toast(t('Limits saved')); onSaved() } catch (e: any) { toast(e.message) }
  }
  const chips = (ids: number[] | undefined, prefix: 'i' | 's', field: keyof Access) => (ids || []).map((id) => (
    <span key={prefix + id} className="chip tag">{titles[prefix + id] || '…'}<button type="button" aria-label={t('Remove {name}', { name: titles[prefix + id] || t('this title') })} onClick={() => set({ [field]: (ids || []).filter((x) => x !== id) } as Partial<Access>)}>×</button></span>
  ))
  return (
    <Dialog title={t('Limits for {name}', { name: profileName(user) })} size="wide" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" onClick={save}>{t('Save limits')}</Button></>}>
        <Tabs className="pc-tabs" label={t('Limits')} value={tab} onChange={setTab} panelId="limits-panel"
          items={[{ value: 'watch', label: t('What they see') }, { value: 'do', label: t('What they can do') }, { value: 'time', label: t('Screen time') }]} />
        <div className="limits" id="limits-panel" role="tabpanel">
          {tab === 'time' && <TimeRulesEditor access={a} set={set} />}
          {tab === 'do' && <>
            <FeatureSwitches access={a} set={set} kind={user.kind} />
            <h3>{t('Live TV')}</h3>
            <label className="check-row"><input type="checkbox" checked={a.liveTv} onChange={(e) => set({ liveTv: e.target.checked })} /> {t('Live TV')}</label>
            <label className="check-row indent"><input type="checkbox" disabled={!a.liveTv} checked={a.libraryChannels} onChange={(e) => set({ libraryChannels: e.target.checked })} /> {t("Channels made from this server's libraries")}</label>
            <label className="check-row indent"><input type="checkbox" disabled={!a.liveTv} checked={a.tunerChannels} onChange={(e) => set({ tunerChannels: e.target.checked })} /> {t('Antenna, cable and internet TV channels (tuners, IPTV)')}</label>
            {user.kind !== 'adult' && user.hasPassword && admin !== undefined && (
              <label className="check-row limits-clear"><input type="checkbox" checked={clearPw} onChange={(e) => setClearPw(e.target.checked)} /> {t('Remove the password (sign in with a tap on the home network)')}</label>
            )}
          </>}
          {tab === 'watch' && <>
          <AgePicker value={a.maxAge} onChange={(n) => set({ maxAge: n })} />
          {a.maxAge >= 0 && <label className="check-row"><input type="checkbox" checked={a.allowUnrated} onChange={(e) => set({ allowUnrated: e.target.checked })} /> {t('Also show titles without a rating')} <span className="muted">{t('(home videos, unmatched files)')}</span></label>}

          <h3>{t('A different age for some kinds')}</h3>
          <p className="muted">{t('Films, TV, games, music and books are rated by different boards. Each can have its own limit, shown with what it allows in each country’s ratings.')}</p>
          <KindLimits access={a} set={set} lists={lists} />

          <h3>{t('Libraries')}</h3>
          <p className="muted">{t('Leave all unticked to allow every library.')}</p>
          <div className="check-grid">
            {libs.map((l) => <label key={l.id} className="check-row"><input type="checkbox" checked={(a.libraries || []).includes(l.id)} onChange={() => set({ libraries: toggleIn(a.libraries, l.id) })} /> {l.name}</label>)}
          </div>

          {genres.length > 0 && <>
            <h3>{t('Hide genres')}</h3>
            <div className="check-grid">
              {genres.map((g) => <label key={g} className="check-row"><input type="checkbox" checked={(a.blockGenres || []).includes(g)} onChange={() => set({ blockGenres: (a.blockGenres || []).includes(g) ? a.blockGenres.filter((x) => x !== g) : [...(a.blockGenres || []), g] })} /> {g}</label>)}
            </div>
          </>}

          <h3>{t('Exceptions')}</h3>
          <p className="muted">{t('Always allow or never allow particular movies and shows, whatever their rating.')}</p>
          <input className="input" placeholder={t('Search a movie or show…')} aria-label={t('Search titles')} value={q} onChange={(e) => setQ(e.target.value)} />
          {found.length > 0 && (
            <div className="list limits-found">
              {found.map((f) => <div key={f.key} className="list-item"><div className="grow">{f.title}</div><button className="btn sm" onClick={() => override(f, true)}>{t('Always allow')}</button><button className="btn sm danger" onClick={() => override(f, false)}>{t('Never allow')}</button></div>)}
            </div>
          )}
          <div className="chips"><span className="muted">{t('Always:')}</span>{chips(a.allowShows, 's', 'allowShows')}{chips(a.allowItems, 'i', 'allowItems')}</div>
          <div className="chips"><span className="muted">{t('Never:')}</span>{chips(a.blockShows, 's', 'blockShows')}{chips(a.blockItems, 'i', 'blockItems')}</div>

          {genres.length > 0 && <>
            <h3>{t('Show only these genres')}</h3>
            <p className="muted">{t('Leave all unticked to show every genre that isn’t hidden above.')}</p>
            <div className="check-grid">
              {genres.map((g) => <label key={g} className="check-row"><input type="checkbox" checked={(a.onlyGenres || []).includes(g)} onChange={() => set({ onlyGenres: (a.onlyGenres || []).includes(g) ? (a.onlyGenres || []).filter((x) => x !== g) : [...(a.onlyGenres || []), g] })} /> {g}</label>)}
            </div>
          </>}

          <h3>{t('Collections')}</h3>
          <p className="muted">{t('Allow or hide everything in a collection, whatever its rating. A collection made from a rule can’t be used: it has no list of titles to go by.')}</p>
          <CollectionRules access={a} set={set} />
          <TagRules access={a} set={set} />

          <LegalNote kind="ratings" />
          </>}
        </div>
    </Dialog>
  )
}
