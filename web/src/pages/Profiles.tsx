import { useEffect, useState } from 'react'
import { api, type Access, type Item, type Library, type Show, type User, type UserView } from '../api'
import { IconTrash } from '../icons'
import { toast, useApp } from '../state'
import { Avatar } from './AuthPage'

// Settings → Users: your account, the profiles you may manage, and their
// limits. The admin manages everyone; adults manage Kids profiles unless the
// admin keeps that to themself; Kids and Guest profiles manage nothing.

export const AGE_LEVELS: { age: number; label: string; hint: string }[] = [
  { age: 0, label: 'Little kids', hint: 'G, U, TV-Y, TV-G' },
  { age: 7, label: 'Kids 7+', hint: 'adds TV-Y7' },
  { age: 10, label: 'Older kids 10+', hint: 'adds PG, TV-PG' },
  { age: 13, label: 'Teens 13+', hint: 'adds PG-13, 12A' },
  { age: 16, label: 'Older teens 16+', hint: 'adds TV-14, 15, FSK 16' },
  { age: -1, label: 'Everything', hint: 'no age limit' },
]

const KIND_LABEL: Record<string, string> = { adult: 'Adult', kids: 'Kids', guest: 'Guest' }

export default function Profiles() {
  const { user } = useApp()
  const [users, setUsers] = useState<UserView[]>([])
  const [mypw, setMypw] = useState('')
  const [curpw, setCurpw] = useState('')
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<UserView | null>(null)
  const load = () => api.get<UserView[]>('/users').then(setUsers).catch(() => setUsers([]))
  useEffect(() => { load() }, [])
  const changePw = async () => { try { await api.put(`/users/${user!.id}/password`, { password: mypw, current: curpw }); setMypw(''); setCurpw(''); toast('Password changed; other devices are signed out') } catch (e: any) { toast(e.message) } }
  const del = async (u: User) => {
    if (!confirm(`Remove the profile “${u.name}”? Their watch history is deleted too.`)) return
    try { await api.del(`/users/${u.id}`); load() } catch (e: any) { toast(e.message) }
  }
  const restricted = user?.kind !== 'adult'
  return (
    <>
      <div className="panel">
        <h2>Your account</h2>
        <div className="form">
          <div className="profile-me"><Avatar name={user?.name || '?'} /> <b>{user?.name}</b> <span className="pill">{user?.isAdmin ? 'Administrator' : KIND_LABEL[user?.kind || 'adult']}</span></div>
          {restricted
            ? <p className="muted">This profile's password and limits are managed by the adults of this server.</p>
            : <form className="field-row" onSubmit={(e) => { e.preventDefault(); changePw() }}>
                <input className="input" type="password" autoComplete="current-password" aria-label="Current password" placeholder="Current password" value={curpw} onChange={(e) => setCurpw(e.target.value)} />
                <input className="input" type="password" autoComplete="new-password" aria-label="New password" placeholder="New password" value={mypw} onChange={(e) => setMypw(e.target.value)} />
                <button className="btn" disabled={mypw.length < 8 || !curpw}>Change password</button>
              </form>}
        </div>
      </div>

      {!restricted && users.length > 0 && (
        <div className="panel">
          <h2>{user?.isAdmin ? 'Everyone on this server' : 'Kids profiles you manage'}</h2>
          <div className="list">
            {users.map((u) => (
              <div className="list-item" key={u.id}>
                <Avatar name={u.name} />
                <div className="grow">
                  <b>{u.name}</b> <span className={'pill kind-' + u.kind}>{u.isAdmin ? 'Admin' : KIND_LABEL[u.kind]}</span>
                  <div className="sub">{u.isAdmin ? 'No limits' : summary(u)}</div>
                </div>
                {!u.isAdmin && u.id !== user!.id && <button className="btn sm" onClick={() => setEditing(u)}>Limits</button>}
                {u.id !== user!.id && <button className="btn sm danger" onClick={() => del(u)} aria-label={`Remove ${u.name}`}><IconTrash /></button>}
              </div>
            ))}
          </div>
          <div className="panel-actions"><button className="btn primary" onClick={() => setAdding(true)}>Add profile</button></div>
        </div>
      )}
      {!restricted && users.length === 0 && !user?.isAdmin && (
        <div className="panel"><h2>Kids profiles</h2><button className="btn primary" onClick={() => setAdding(true)}>Add a Kids profile</button></div>
      )}
      {adding && <AddProfile admin={!!user?.isAdmin} onClose={() => setAdding(false)} onDone={() => { setAdding(false); load() }} />}
      {editing && <LimitsEditor user={editing} admin={!!user?.isAdmin} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load() }} />}
    </>
  )
}

function summary(u: UserView): string {
  const a = u.access
  if (!a) return 'No limits'
  const parts = [AGE_LEVELS.find((l) => l.age === a.maxAge)?.label || `Up to age ${a.maxAge}`]
  if (a.libraries?.length) parts.push(`${a.libraries.length} ${a.libraries.length === 1 ? 'library' : 'libraries'}`)
  if (!a.liveTv) parts.push('no Live TV')
  if (!u.hasPassword) parts.push('no password')
  return parts.join(' · ')
}

// AddProfile creates an adult, Kids or Guest profile (adults: Kids only).
export function AddProfile({ admin, onClose, onDone, initialKind = admin ? 'adult' : 'kids' }: { admin: boolean; onClose: () => void; onDone: () => void; initialKind?: string }) {
  const [kind, setKind] = useState(initialKind)
  const [name, setName] = useState('')
  const [pw, setPw] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)
  const [age, setAge] = useState(7)
  const [err, setErr] = useState('')
  const needsPw = kind === 'adult'
  const save = async () => {
    setErr('')
    try {
      const body: any = { name, password: pw, kind, isAdmin: kind === 'adult' && isAdmin }
      if (kind !== 'adult') body.access = { maxAge: age, allowUnrated: false, liveTv: kind === 'kids', libraryChannels: kind === 'kids', tunerChannels: false }
      await api.post('/users', body)
      toast(`Profile “${name}” added`)
      onDone()
    } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-label="Add a profile" onClick={(e) => e.stopPropagation()}>
        <h2>Add a profile</h2>
        <div className="form full">
          {admin && (
            <div className="seg big" role="radiogroup" aria-label="Kind of profile">
              {(['adult', 'kids', 'guest'] as const).map((k) => <button key={k} type="button" role="radio" aria-checked={kind === k} className={kind === k ? 'active' : ''} onClick={() => { setKind(k); setAge(k === 'guest' ? 13 : 7) }}>{KIND_LABEL[k]}</button>)}
            </div>
          )}
          <p className="muted" style={{ margin: 0 }}>
            {kind === 'adult' && 'A full account with its own password and watch history.'}
            {kind === 'kids' && 'Only shows titles up to the age you choose. A password or PIN is optional: without one, kids sign in with a tap on your home network.'}
            {kind === 'guest' && 'For visitors: limited titles, no settings. Only the administrator can change Guest limits.'}
          </p>
          <label>Name<input className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} maxLength={40} /></label>
          <label>{needsPw ? 'Password' : 'Password or PIN (optional)'}<input className="input" type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" /></label>
          {kind === 'adult' && admin && <label className="check-row"><input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} /> Administrator (can change everything)</label>}
          {kind !== 'adult' && <AgePicker value={age} onChange={setAge} />}
          {err && <div className="error" role="alert">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" disabled={!name.trim() || (needsPw ? pw.length < 8 : pw.length > 0 && pw.length < 4)} onClick={save}>Add profile</button></div>
      </div>
    </div>
  )
}

export function AgePicker({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <fieldset className="age-picker">
      <legend>What may they watch?</legend>
      {AGE_LEVELS.map((l) => (
        <label key={l.age} className={'age-opt' + (value === l.age ? ' on' : '')}>
          <input type="radio" name="age" checked={value === l.age} onChange={() => onChange(l.age)} />
          <b>{l.label}</b><span className="muted">{l.hint}</span>
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
    want.forEach(([k, url]) => api.get<any>(url).then((x) => setTitles((t) => ({ ...t, [k]: x.title || x.show?.title || k }))).catch(() => {}))
  }, [a.allowItems, a.blockItems, a.allowShows, a.blockShows]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (q.trim().length < 2) { setFound([]); return }
    const t = setTimeout(async () => {
      const [it, sh] = await Promise.all([
        api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}&limit=6&kind=`).catch(() => ({ items: [] })),
        api.get<{ shows: Show[] }>(`/shows?search=${encodeURIComponent(q)}&limit=6`).catch(() => ({ shows: [] })),
      ])
      setFound([...sh.shows.map((s) => ({ key: 's' + s.id, id: s.id, show: true, title: s.title + ' (show)' })), ...it.items.filter((x) => x.showId == null).map((x) => ({ key: 'i' + x.id, id: x.id, show: false, title: x.title + (x.year ? ` (${x.year})` : '') }))])
    }, 250)
    return () => clearTimeout(t)
  }, [q])
  const set = (p: Partial<Access>) => setA((x) => ({ ...x, ...p }))
  const toggleIn = (list: number[] | undefined, id: number) => (list || []).includes(id) ? (list || []).filter((x) => x !== id) : [...(list || []), id]
  const override = (f: { id: number; show: boolean; title: string; key: string }, allow: boolean) => {
    setTitles((t) => ({ ...t, [f.key]: f.title }))
    if (f.show) set({ allowShows: allow ? [...new Set([...(a.allowShows || []), f.id])] : (a.allowShows || []).filter((x) => x !== f.id), blockShows: !allow ? [...new Set([...(a.blockShows || []), f.id])] : (a.blockShows || []).filter((x) => x !== f.id) })
    else set({ allowItems: allow ? [...new Set([...(a.allowItems || []), f.id])] : (a.allowItems || []).filter((x) => x !== f.id), blockItems: !allow ? [...new Set([...(a.blockItems || []), f.id])] : (a.blockItems || []).filter((x) => x !== f.id) })
    setQ(''); setFound([])
  }
  const save = async () => {
    try { await api.put(`/users/${user.id}/access`, { access: a, clearPassword: clearPw }); toast('Limits saved'); onSaved() } catch (e: any) { toast(e.message) }
  }
  const chips = (ids: number[] | undefined, prefix: 'i' | 's', field: keyof Access) => (ids || []).map((id) => (
    <span key={prefix + id} className="chip tag">{titles[prefix + id] || '…'}<button type="button" aria-label="Remove" onClick={() => set({ [field]: (ids || []).filter((x) => x !== id) } as Partial<Access>)}>×</button></span>
  ))
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide" role="dialog" aria-label={`Limits for ${user.name}`} onClick={(e) => e.stopPropagation()}>
        <h2>Limits for {user.name}</h2>
        <div className="limits">
          <AgePicker value={a.maxAge} onChange={(n) => set({ maxAge: n })} />
          {a.maxAge >= 0 && <label className="check-row"><input type="checkbox" checked={a.allowUnrated} onChange={(e) => set({ allowUnrated: e.target.checked })} /> Also show titles without a rating <span className="muted">(home videos, unmatched files)</span></label>}

          <h3>Libraries</h3>
          <p className="muted">Leave all unticked to allow every library.</p>
          <div className="check-grid">
            {libs.map((l) => <label key={l.id} className="check-row"><input type="checkbox" checked={(a.libraries || []).includes(l.id)} onChange={() => set({ libraries: toggleIn(a.libraries, l.id) })} /> {l.name}</label>)}
          </div>

          {genres.length > 0 && <>
            <h3>Hide genres</h3>
            <div className="check-grid">
              {genres.map((g) => <label key={g} className="check-row"><input type="checkbox" checked={(a.blockGenres || []).includes(g)} onChange={() => set({ blockGenres: (a.blockGenres || []).includes(g) ? a.blockGenres.filter((x) => x !== g) : [...(a.blockGenres || []), g] })} /> {g}</label>)}
            </div>
          </>}

          <h3>Exceptions</h3>
          <p className="muted">Always allow or never allow particular movies and shows, whatever their rating.</p>
          <input className="input" placeholder="Search a movie or show…" aria-label="Search titles" value={q} onChange={(e) => setQ(e.target.value)} />
          {found.length > 0 && (
            <div className="list" style={{ marginTop: 6 }}>
              {found.map((f) => <div key={f.key} className="list-item"><div className="grow">{f.title}</div><button className="btn sm" onClick={() => override(f, true)}>Always allow</button><button className="btn sm danger" onClick={() => override(f, false)}>Never allow</button></div>)}
            </div>
          )}
          <div className="chips"><span className="muted">Always:</span>{chips(a.allowShows, 's', 'allowShows')}{chips(a.allowItems, 'i', 'allowItems')}</div>
          <div className="chips"><span className="muted">Never:</span>{chips(a.blockShows, 's', 'blockShows')}{chips(a.blockItems, 'i', 'blockItems')}</div>

          <h3>Live TV</h3>
          <label className="check-row"><input type="checkbox" checked={a.liveTv} onChange={(e) => set({ liveTv: e.target.checked })} /> Live TV</label>
          <label className="check-row indent"><input type="checkbox" disabled={!a.liveTv} checked={a.libraryChannels} onChange={(e) => set({ libraryChannels: e.target.checked })} /> Channels made from this server's libraries</label>
          <label className="check-row indent"><input type="checkbox" disabled={!a.liveTv} checked={a.tunerChannels} onChange={(e) => set({ tunerChannels: e.target.checked })} /> Antenna, cable and internet TV channels (tuners, IPTV)</label>

          {user.kind !== 'adult' && user.hasPassword && admin !== undefined && (
            <label className="check-row" style={{ marginTop: 12 }}><input type="checkbox" checked={clearPw} onChange={(e) => setClearPw(e.target.checked)} /> Remove the password (sign in with a tap on the home network)</label>
          )}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save limits</button></div>
      </div>
    </div>
  )
}
