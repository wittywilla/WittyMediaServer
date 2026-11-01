import { useEffect, useRef, useState, type ReactNode } from 'react'
import { api, fmtTime, type Item } from '../api'
import { IconMute, IconPlus, IconVolume } from '../icons'
import { toast } from '../state'
import { IconAddList, IconHeart, IconHeartOn } from './icons'
import type { PlayerView } from './types'

// Pieces the bar and the full player share.

// A button with something that opens from it. Escape and a press elsewhere close it, and
// Escape stops there: it shouldn't also close the player underneath.
export function Pop({ label, icon, children, className, on, up }: { label: string; icon: ReactNode; children: (close: () => void) => ReactNode; className?: string; on?: boolean; up?: boolean }) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const out = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' && e.key !== 'GoBack' && e.key !== 'BrowserBack') return
      e.preventDefault()
      e.stopImmediatePropagation()
      setOpen(false)
      box.current?.querySelector<HTMLElement>(':scope > button')?.focus()
    }
    window.addEventListener('pointerdown', out)
    window.addEventListener('keydown', esc, true)
    return () => { window.removeEventListener('pointerdown', out); window.removeEventListener('keydown', esc, true) }
  }, [open])
  return (
    <div className={'mp-pop' + (up ? ' up' : '')} ref={box}>
      <button type="button" className={'mp-ibtn ' + (className || '') + (on ? ' on' : '')} aria-label={label} title={label} aria-haspopup="true" aria-expanded={open} onClick={() => setOpen(!open)}>{icon}</button>
      {open && <div className="mp-pop-body" role="group" aria-label={label}>{children(() => setOpen(false))}</div>}
    </div>
  )
}

export function Volume({ p, up }: { p: PlayerView; up?: boolean }) {
  const { volume, muted } = p.sound
  const shown = muted ? 0 : volume
  return (
    <Pop label="Volume" up={up} icon={shown === 0 ? <IconMute /> : <IconVolume />}>
      {() => (
        <div className="mp-volume">
          <button type="button" className="mp-ibtn" aria-pressed={muted} aria-label={muted ? 'Turn the sound back on' : 'Mute'} onClick={() => p.setSound({ muted: !muted })}>{shown === 0 ? <IconMute /> : <IconVolume />}</button>
          <input className="range" type="range" autoFocus min={0} max={100} step={2} value={Math.round(shown * 100)} aria-label="Volume" aria-valuetext={`${Math.round(shown * 100)}%`}
            style={{ '--pct': shown * 100 + '%' } as React.CSSProperties} onChange={(e) => p.setSound({ volume: Number(e.target.value) / 100, muted: false })} />
        </div>
      )}
    </Pop>
  )
}

export function Like({ p }: { p: PlayerView }) {
  if (!p.current || p.live) return null
  const id = p.current.id
  return <button type="button" className={'mp-ibtn' + (p.liked ? ' liked' : '')} aria-pressed={p.liked} aria-label={p.liked ? 'Remove from liked songs' : 'Add to liked songs'} title={p.liked ? 'Liked' : 'Like'} onClick={() => p.toggleLike(id)}>{p.liked ? <IconHeartOn /> : <IconHeart />}</button>
}

type Playlist = { id: number; name: string; kind: string; system?: string }

// Playlists are collections, which also hold rules and any other kind of media, so this is
// one list across the app and a playlist made here is on the Collections page too.
export function AddToPlaylist({ items, up, label = 'Add to a playlist' }: { items: Item[]; up?: boolean; label?: string }) {
  return (
    <Pop label={label} up={up} icon={<IconAddList />}>
      {(close) => <PlaylistPicker items={items} onDone={close} />}
    </Pop>
  )
}

export function PlaylistPicker({ items, onDone }: { items: Item[]; onDone: () => void }) {
  const [lists, setLists] = useState<Playlist[] | null>(null)
  const [making, setMaking] = useState(false)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Playlist[]>('/collections').then((l) => setLists(l.filter((c) => c.kind === 'manual' && !c.system))).catch(() => setLists([])) }, [])
  const add = async (id: number, called: string) => {
    setBusy(true)
    try {
      await api.post(`/collections/${id}/items`, { itemIds: items.map((i) => i.id) })
      toast(`Added to ${called}`)
      onDone()
    } catch (e) { toast((e as Error).message) }
    setBusy(false)
  }
  const make = async () => {
    const n = name.trim()
    if (!n) return
    setBusy(true)
    try {
      const c = await api.post<Playlist>('/collections', { name: n, kind: 'manual' })
      await add(c.id, n)
    } catch (e) { toast((e as Error).message); setBusy(false) }
  }
  if (making) {
    return (
      <form className="mp-newlist" onSubmit={(e) => { e.preventDefault(); void make() }}>
        <input className="input" autoFocus aria-label="Name of the new playlist" placeholder="Name" maxLength={120} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn sm primary" disabled={busy || !name.trim()}>Make it</button>
      </form>
    )
  }
  return (
    <div className="mp-menu" role="menu">
      <button type="button" role="menuitem" autoFocus onClick={() => setMaking(true)}><IconPlus size={18} /> New playlist</button>
      {!lists && <span className="muted mp-menu-note">Loading…</span>}
      {lists?.map((l) => <button type="button" role="menuitem" key={l.id} disabled={busy} onClick={() => void add(l.id, l.name)}>{l.name}</button>)}
    </div>
  )
}

// The position, with a name for screen readers that says it as time.
export function Seek({ p, slim }: { p: PlayerView; slim?: boolean }) {
  const { time, duration: dur } = p
  const pct = dur > 0 ? Math.min(100, (time / dur) * 100) : 0
  return (
    <input className={'range' + (slim ? ' mp-seek-slim' : '')} type="range" min={0} max={Math.max(1, dur)} step={1} value={Math.min(time, dur)} aria-label="Position" aria-valuetext={`${fmtTime(time)} of ${fmtTime(dur)}`}
      style={{ '--pct': pct + '%' } as React.CSSProperties} onChange={(e) => p.seek(Number(e.target.value))} />
  )
}
