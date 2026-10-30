import { api, mediaUrl, serverBase, type Library } from './api'

// What the photo, vertical video and creator libraries share: their JSON types,
// the addresses of their pictures, and small helpers. The rules themselves live
// on the server, behind the JSON API (docs/PHOTOS.md, SHORTS.md, CHANNELS.md).

export type Photo = {
  id: number; libraryId: number; kind: 'photo' | 'video'; title: string; takenAt: number; width: number; height: number
  durationSec?: number; raw?: boolean; motion?: boolean; hasLocation?: boolean; favorite?: boolean; updatedAt: number
}
export type PhotoDetail = Photo & {
  fileName: string; folder: string; sizeBytes: number; make?: string; model?: string; lens?: string; fNumber?: number; exposure?: string
  iso?: number; focalMm?: number; lat?: number; lon?: number; alt?: number; albums: number[]; webSafe: boolean
}
export type PhotoMonth = { ym: number; count: number }
export type PhotoFolder = { folder: string; count: number; coverId: number }
export type PhotoCluster = { lat: number; lon: number; count: number; coverId: number; south: number; west: number; north: number; east: number }
export type PhotoAlbum = {
  id: number; libraryId: number; ownerId: number; owner: string; name: string; description: string; shared: boolean
  count: number; coverId: number; createdAt: number; updatedAt: number
}
export type ShareLink = {
  id: number; userId: number; owner?: string; kind: 'item' | 'album'; targetId: number; title: string; allowDownload: boolean
  expiresAt: number | null; views: number; createdAt: number; token?: string
}

export type VideoCard = {
  id: number; libraryId: number; title: string; durationSec: number; width: number; height: number; vertical: boolean
  creatorId: number; creator: string; hasAvatar: boolean; platform?: string; uploadDate: string; tags: string[]; playlist?: string
  views: number; origViews: number; likes: number; origLikes: number; comments: number; reaction: number
  positionSec: number; watched: boolean; updatedAt: number; reason?: string
}
export type VideoDetail = VideoCard & { description: string; sourceUrl?: string; genres: string[]; contentRating?: string; custom?: Record<string, unknown> }
export type Creator = {
  id: number; libraryId: number; name: string; handle?: string; description?: string; platform?: string; url?: string
  hasAvatar: boolean; hasBanner: boolean; videos: number; subscribed: boolean; subscribers: number; updatedAt: number; latestAt?: string
}
export type Comment = {
  id: number; itemId: number; parentId?: number; userId?: number; author: string; body: string; imported: boolean
  likes?: number; createdAt: number; editedAt?: number
}

export const photoThumb = (id: number, w: number, v?: number) => mediaUrl(`/api/items/${id}/image?w=${w}`) + (v ? `&v=${v}` : '')
export const photoFull = (id: number, v?: number) => mediaUrl(`/api/items/${id}/image`) + (v ? `&v=${v}` : '')
export const videoThumb = (id: number, w: number, v?: number) => mediaUrl(`/api/items/${id}/poster?w=${w}`) + (v ? `&v=${v}` : '')
export const creatorArt = (id: number, kind: 'avatar' | 'banner', v?: number) => mediaUrl(`/api/creators/${id}/art/${kind}`) + (v ? `&v=${v}` : '')

// Taken-at times are the clock on the wall where the picture was taken; they
// are shown as stored, whatever the viewer's time zone.
const UTC = { timeZone: 'UTC' } as const
export const monthName = (ym: number) => new Date(Date.UTC(Math.floor(ym / 100), ym % 100 - 1, 1)).toLocaleDateString([], { ...UTC, month: 'long', year: 'numeric' })
export const takenDate = (t: number) => new Date(t * 1000).toLocaleDateString([], { ...UTC, weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' })
export const takenTime = (t: number) => new Date(t * 1000).toLocaleTimeString([], { ...UTC, hour: '2-digit', minute: '2-digit' })
export const dayKey = (t: number) => Math.floor(t / 86400)

export function fmtSize(n: number) {
  if (n >= 1e9) return (n / 1e9).toFixed(1) + ' GB'
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB'
  return Math.max(1, Math.round(n / 1e3)) + ' KB'
}

// 1234 -> "1.2K". The count is what the source said, plus plays here.
export function fmtCount(n: number) {
  if (n >= 1e9) return (n / 1e9).toFixed(1).replace(/\.0$/, '') + 'B'
  if (n >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'K'
  return String(n)
}

export function timeAgo(date: string | number) {
  const t = typeof date === 'number' ? date * 1000 : Date.parse(date + 'T00:00:00Z')
  if (!t) return ''
  const days = Math.floor((Date.now() - t) / 86400000)
  if (days < 1) return 'today'
  if (days < 2) return 'yesterday'
  if (days < 7) return `${days} days ago`
  if (days < 30) return `${Math.floor(days / 7)} ${days < 14 ? 'week' : 'weeks'} ago`
  if (days < 365) return `${Math.floor(days / 30)} ${days < 60 ? 'month' : 'months'} ago`
  return `${Math.floor(days / 365)} ${days < 730 ? 'year' : 'years'} ago`
}

// One viewer action, kept in step with the server's canPost and canReact.
export const canPost = (u?: { isAdmin: boolean; kind: string } | null) => !!u && (u.isAdmin || u.kind === 'adult' || !u.kind)
export const canReact = (u?: { isAdmin: boolean; kind: string } | null) => !!u && (u.isAdmin || u.kind !== 'guest')

// Uploads go one file per request, so each has its own progress and limit.
export type UploadResult = { name: string; id?: number; error?: string }
export function uploadFile(lib: Library, file: File, onProgress: (done: number) => void): Promise<UploadResult> {
  return new Promise((resolve) => {
    const x = new XMLHttpRequest()
    const form = new FormData()
    form.append('file', file, file.name)
    x.open('POST', serverBase() + `/api/libraries/${lib.id}/upload`)
    const tok = localStorage.getItem('wms.token')
    if (tok) x.setRequestHeader('Authorization', 'Bearer ' + tok)
    x.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(e.loaded / e.total) }
    x.onerror = () => resolve({ name: file.name, error: "Can't reach the server" })
    x.onload = () => {
      try {
        const r = JSON.parse(x.responseText)
        if (x.status >= 400 && !r.files?.length) resolve({ name: file.name, error: r.error || x.statusText })
        else resolve(r.files?.[0] || { name: file.name, error: 'Not saved' })
      } catch { resolve({ name: file.name, error: x.statusText || 'Not saved' }) }
    }
    x.send(form)
  })
}

export const copyText = async (text: string) => {
  try { await navigator.clipboard.writeText(text); return true } catch { /* not a secure page */ }
  const t = document.createElement('textarea')
  t.value = text; t.style.position = 'fixed'; t.style.opacity = '0'
  document.body.appendChild(t); t.select()
  let ok = false
  try { ok = document.execCommand('copy') } catch { /* nothing more to try */ }
  t.remove()
  return ok
}

export const setReaction = (itemId: number, value: number) => api.put(`/items/${itemId}/reaction`, { value })
