import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtTime } from '../../api'
import { canPost, canReact, creatorArt, fmtCount, setReaction, timeAgo, videoThumb, type Comment, type Creator, type VideoCard } from '../../gallery'
import { IconCheck, IconEdit, IconTrash } from '../../icons'
import { toast, useApp } from '../../state'

// What WittyShorts and WittyChannels share: a creator's picture, video tiles,
// like buttons, a subscribe button and comments.

export function Avatar({ id, name, has, size = 36, v }: { id: number; name: string; has: boolean; size?: number; v?: number }) {
  const [broken, setBroken] = useState(false)
  if (has && !broken) return <img className="cr-avatar" style={{ width: size, height: size }} src={creatorArt(id, 'avatar', v)} alt="" loading="lazy" onError={() => setBroken(true)} />
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) % 360
  return <span className="cr-avatar ph" style={{ width: size, height: size, fontSize: size * 0.45, background: `hsl(${h} 35% 36%)` }} aria-hidden="true">{name.trim().charAt(0).toUpperCase() || '?'}</span>
}

export const viewsOf = (v: VideoCard) => v.views + v.origViews
export const likesOf = (v: VideoCard) => v.likes + v.origLikes

// A video in a grid, wide or vertical by what the list holds.
export function VideoTile({ v, to, showCreator = true, onClick }: { v: VideoCard; to: string; showCreator?: boolean; onClick?: (e: React.MouseEvent) => void }) {
  const pct = !v.watched && v.durationSec > 0 && v.positionSec > 0 ? Math.min(100, (v.positionSec / v.durationSec) * 100) : 0
  return (
    <Link to={to} className={'cr-tile' + (v.vertical ? ' vertical' : '')} onClick={onClick} title={v.title}>
      <span className="cr-thumb">
        <img src={videoThumb(v.id, 480, v.updatedAt)} alt="" loading="lazy" decoding="async" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />
        {v.durationSec > 0 && <span className="cr-dur">{fmtTime(v.durationSec)}</span>}
        {v.watched && <span className="cr-seen" aria-label="Watched"><IconCheck size={14} /></span>}
        {pct > 0 && <span className="cr-prog"><i style={{ width: pct + '%' }} /></span>}
      </span>
      <span className="cr-meta">
        {showCreator && <Avatar id={v.creatorId} name={v.creator} has={v.hasAvatar} size={34} />}
        <span className="cr-text">
          <b>{v.title}</b>
          {showCreator && <span className="muted">{v.creator}</span>}
          <span className="muted">{[viewsOf(v) ? `${fmtCount(viewsOf(v))} views` : '', timeAgo(v.uploadDate)].filter(Boolean).join(' · ')}</span>
        </span>
      </span>
    </Link>
  )
}

// useVideos: a list that grows as the viewer reaches its end.
export function useVideoList(query: string, pageSize = 30) {
  const [items, setItems] = useState<VideoCard[] | null>(null)
  const [more, setMore] = useState(false)
  const busy = useRef(false)
  const offset = useRef(0)
  const load = useCallback(async (reset: boolean) => {
    if (busy.current && !reset) return
    busy.current = true
    if (reset) offset.current = 0
    try {
      const r = await api.get<{ videos: VideoCard[]; more: boolean }>(`/videos?${query}&limit=${pageSize}&offset=${offset.current}`)
      offset.current += r.videos.length
      setItems((x) => (reset || !x ? r.videos : [...x, ...r.videos]))
      setMore(r.more)
    } catch { setItems((x) => x || []) } finally { busy.current = false }
  }, [query, pageSize])
  useEffect(() => { setItems(null); load(true) }, [load])
  return { items, more, loadMore: () => load(false) }
}

// A button that loads the next page by itself when scrolled to, and by being
// pressed with a remote.
export function LoadMore({ more, onMore }: { more: boolean; onMore: () => void }) {
  const ref = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || !more) return
    const io = new IntersectionObserver((e) => { if (e[0].isIntersecting) onMore() }, { rootMargin: '600px' })
    io.observe(el)
    return () => io.disconnect()
  }, [more, onMore])
  if (!more) return null
  return <div className="cr-more"><button ref={ref} className="btn ghost" onClick={onMore}>Show more</button></div>
}

export function LikeButtons({ v, onChange }: { v: VideoCard; onChange: (reaction: number) => void }) {
  const { user } = useApp()
  const set = async (value: number) => {
    if (!canReact(user)) return
    const next = v.reaction === value ? 0 : value
    onChange(next)
    try { await setReaction(v.id, next) } catch { onChange(v.reaction); toast("Couldn't save that") }
  }
  const likes = likesOf(v) + (v.reaction > 0 ? 0 : 0)
  return (
    <div className="seg cr-likes" role="group" aria-label="Rate this video">
      <button className={v.reaction > 0 ? 'active' : ''} aria-pressed={v.reaction > 0} disabled={!canReact(user)} onClick={() => set(1)}>♥ Like{likes ? ` · ${fmtCount(likes)}` : ''}</button>
      <button className={v.reaction < 0 ? 'active' : ''} aria-pressed={v.reaction < 0} disabled={!canReact(user)} aria-label="Not for me" title="Show less like this" onClick={() => set(-1)}>✕</button>
    </div>
  )
}

export function SubscribeButton({ c, onChange }: { c: Pick<Creator, 'id' | 'subscribed' | 'name'>; onChange: (on: boolean) => void }) {
  const { user } = useApp()
  if (!canReact(user)) return null
  const toggle = async () => {
    const on = !c.subscribed
    onChange(on)
    try { await api.put(`/creators/${c.id}/subscribe`, { on }) } catch { onChange(!on); toast("Couldn't save that") }
  }
  return <button className={'btn sm ' + (c.subscribed ? 'ghost' : 'primary')} aria-pressed={c.subscribed} onClick={toggle}>{c.subscribed ? 'Subscribed' : 'Subscribe'}</button>
}

// Comments: those of profiles on this server first, then those saved with an archive.
export function Comments({ itemId, count }: { itemId: number; count?: number }) {
  const { user } = useApp()
  const [list, setList] = useState<Comment[] | null>(null)
  const [total, setTotal] = useState(count ?? 0)
  const [body, setBody] = useState('')
  const [reply, setReply] = useState<Comment | null>(null)
  const [editing, setEditing] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const load = useCallback(() => {
    api.get<{ comments: Comment[]; total: number }>(`/items/${itemId}/comments?limit=100`).then((r) => { setList(r.comments); setTotal(r.total) }).catch(() => setList([]))
  }, [itemId])
  useEffect(load, [load])
  const send = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!body.trim()) return
    setBusy(true)
    try {
      if (editing) { await api.put(`/comments/${editing}`, { body }); setEditing(null) }
      else await api.post(`/items/${itemId}/comments`, { body, parentId: reply?.id })
      setBody(''); setReply(null); load()
    } catch (x: any) { toast(x.message) } finally { setBusy(false) }
  }
  const remove = async (c: Comment) => {
    if (!confirm('Delete this comment?')) return
    try { await api.del(`/comments/${c.id}`); load() } catch (x: any) { toast(x.message) }
  }
  const top = (list || []).filter((c) => !c.parentId)
  const repliesOf = (id: number) => (list || []).filter((c) => c.parentId === id)
  const row = (c: Comment) => (
    <li key={c.id} className="cr-comment">
      <div className="cr-comment-head"><b>{c.author || 'Someone'}</b><span className="muted">{c.createdAt ? timeAgo(c.createdAt) : ''}{c.editedAt ? ' · edited' : ''}{c.imported ? ' · from the archive' : ''}{c.likes ? ` · ♥ ${fmtCount(c.likes)}` : ''}</span></div>
      <p>{c.body}</p>
      <div className="cr-comment-acts">
        {canPost(user) && !c.imported && <button className="btn sm ghost" onClick={() => { setReply(c); setEditing(null) }}>Reply</button>}
        {!c.imported && c.userId === user?.id && <button className="btn sm ghost" aria-label="Edit comment" onClick={() => { setEditing(c.id); setBody(c.body); setReply(null) }}><IconEdit /></button>}
        {(c.userId === user?.id || user?.isAdmin) && <button className="btn sm ghost" aria-label="Delete comment" onClick={() => remove(c)}><IconTrash /></button>}
      </div>
      {repliesOf(c.id).length > 0 && <ul className="cr-replies">{repliesOf(c.id).map(row)}</ul>}
    </li>
  )
  return (
    <section className="cr-comments" aria-label="Comments">
      <h2>{total} {total === 1 ? 'comment' : 'comments'}</h2>
      {canPost(user) && (
        <form className="cr-comment-form" onSubmit={send}>
          {reply && <div className="muted cr-replying">Replying to {reply.author} <button type="button" className="btn sm ghost" onClick={() => setReply(null)}>Cancel</button></div>}
          <textarea className="input" rows={2} maxLength={2000} placeholder={editing ? 'Edit your comment' : 'Add a comment'} aria-label="Your comment" value={body} onChange={(e) => setBody(e.target.value)} />
          <div className="cr-comment-send">
            {editing && <button type="button" className="btn sm ghost" onClick={() => { setEditing(null); setBody('') }}>Cancel</button>}
            <button className="btn sm primary" disabled={busy || !body.trim()}>{editing ? 'Save' : 'Comment'}</button>
          </div>
        </form>
      )}
      {list === null ? null : top.length === 0 ? <p className="muted">No comments yet.</p> : <ul className="cr-comment-list">{top.map(row)}</ul>}
    </section>
  )
}

// Custom metadata for a video: where it came from, tags and any fields of your own.
export function EditVideoDialog({ v, onClose, onSaved }: { v: { id: number; title: string; platform?: string; sourceUrl?: string; uploadDate: string; tags: string[]; creator: string; playlist?: string; description?: string; custom?: Record<string, unknown>; contentRating?: string }; onClose: () => void; onSaved: () => void }) {
  const [title, setTitle] = useState(v.title)
  const [description, setDescription] = useState(v.description || '')
  const [rating, setRating] = useState(v.contentRating || '')
  const [platform, setPlatform] = useState(v.platform || '')
  const [source, setSource] = useState(v.sourceUrl || '')
  const [date, setDate] = useState(v.uploadDate || '')
  const [tags, setTags] = useState(v.tags.join(', '))
  const [creator, setCreator] = useState(v.creator)
  const [playlist, setPlaylist] = useState(v.playlist || '')
  const [custom, setCustom] = useState(Object.entries(v.custom || {}).map(([k, x]) => `${k}: ${String(x)}`).join('\n'))
  const [err, setErr] = useState('')
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setErr('')
    const fields: Record<string, string> = {}
    for (const line of custom.split('\n')) { const i = line.indexOf(':'); if (i > 0) fields[line.slice(0, i).trim()] = line.slice(i + 1).trim() }
    try {
      await api.put(`/items/${v.id}`, { title, overview: description, contentRating: rating })
      await api.put(`/videos/${v.id}`, { platform, sourceUrl: source, uploadDate: date, tags: tags.split(',').map((t) => t.trim()).filter(Boolean), creator, playlist, custom: fields })
      onSaved()
    } catch (x: any) { setErr(x.message) }
  }
  const field = (label: string, value: string, set: (s: string) => void, extra?: Partial<React.InputHTMLAttributes<HTMLInputElement>>) => (
    <label className="oobe-field">{label}<input className="input" value={value} onChange={(e) => set(e.target.value)} {...extra} /></label>
  )
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal wide cr-edit" role="dialog" aria-modal="true" aria-labelledby="ev-h" onClick={(e) => e.stopPropagation()} onSubmit={save}>
        <h2 id="ev-h">Edit details</h2>
        <p className="muted">Saved on this server; the video and its files stay as they are.</p>
        {field('Title', title, setTitle, { required: true })}
        <label className="oobe-field">Description<textarea className="input" rows={3} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
        <div className="cr-edit-row">
          {field('Creator', creator, setCreator, { required: true })}
          {field('Platform of origin', platform, setPlatform, { placeholder: 'e.g. the site it came from' })}
          {field('Date', date, setDate, { placeholder: '2024-05-31' })}
        </div>
        {field('Source address', source, setSource, { placeholder: 'https://…', type: 'url' })}
        <div className="cr-edit-row">
          {field('Tags, separated by commas', tags, setTags)}
          {field('Playlist', playlist, setPlaylist)}
          {field('Content rating', rating, setRating, { placeholder: 'e.g. PG, TV-Y7, 16+' })}
        </div>
        <label className="oobe-field">Your own fields, one per line (name: value)<textarea className="input" rows={3} value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="Recorded by: Sam" /></label>
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </div>
  )
}
