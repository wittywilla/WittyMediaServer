// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Asking the server for server-driven pages (docs/SDUI.md, "HTTP API"): a page is kept
// with its ETag, asked for again through the diff endpoint, and its sections' further
// pages of items are fetched once each. Nothing here draws anything.

import { ApiError, getToken, serverBase } from '../api'
import { applyPatches } from './patch'
import { query, type Device } from './device'
import { VERSION, type Document, type Navigation, type Patch, type Personal, type SectionItems } from './types'

type Answer<T> = { status: number; etag: string; data: T | null }

async function ask<T>(method: string, path: string, opts: { body?: unknown; headers?: Record<string, string>; signal?: AbortSignal } = {}): Promise<Answer<T>> {
  const headers: Record<string, string> = { ...(opts.headers || {}) }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  let res: Response
  try {
    res = await fetch(serverBase() + path, { method, headers, body: opts.body === undefined ? undefined : JSON.stringify(opts.body), credentials: 'same-origin', signal: opts.signal })
  } catch (e) {
    if (opts.signal?.aborted) throw e
    throw new ApiError(0, "Can't reach your server. Check that WittyMediaServer is running and that this device can reach it.")
  }
  if (res.status === 304) return { status: 304, etag: res.headers.get('ETag') || '', data: null }
  const text = await res.text()
  let data: unknown = null
  try { data = text ? JSON.parse(text) : null } catch { data = { error: text } }
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string } | null)?.error || res.statusText, data)
  return { status: res.status, etag: res.headers.get('ETag') || '', data: data as T }
}

const BASE = '/api/v1/sdui'

// ---------------------------------------------------------------- features

let asked: { key: string; answer: Promise<Set<string>> } | null = null

// What this server can do, by name (/capabilities). Asked once per server and sign-in.
export function serverFeatures(): Promise<Set<string>> {
  const key = serverBase() + ' ' + getToken()
  if (asked?.key !== key) {
    const answer = ask<{ features?: string[] }>('GET', '/api/v1/capabilities').then((r) => new Set(r.data?.features || [])).catch(() => { if (asked?.key === key) asked = null; return new Set<string>() })
    asked = { key, answer }
  }
  return asked.answer
}

// ---------------------------------------------------------------- preview

// An administrator looking at the draft: every page and the menu are asked for with the
// token, and say they came from the draft.
let preview = ''
const previewListeners = new Set<() => void>()
export const getPreview = () => preview
export function setPreview(token: string) { preview = token; pages.clear(); nav = null; previewListeners.forEach((l) => l()); bump() }
export const onPreview = (l: () => void) => { previewListeners.add(l); return () => { previewListeners.delete(l) } }

// ---------------------------------------------------------------- pages

type Kept = { doc: Document; etag: string; at: number }
const pages = new Map<string, Kept>()
const MAX_KEPT = 24

// Something happened that pages may show (a title marked watched, a new arrangement):
// what is kept is asked for again the next time it is wanted.
let generation = 0
const changeListeners = new Set<() => void>()
export function bump() { generation++; for (const k of pages.values()) k.at = 0; changeListeners.forEach((l) => l()) }
export const onChange = (l: () => void) => { changeListeners.add(l); return () => { changeListeners.delete(l) } }
export const forgetPages = () => { pages.clear(); nav = null }

function pageQuery(device: Device, params: Record<string, string>, over?: { preview?: string }): string {
  return query(device, { ...params, preview: over?.preview ?? (preview || undefined) })
}

export const pageKey = (page: string, device: Device, params: Record<string, string>, over?: { preview?: string }) => page + '?' + pageQuery(device, params, over)

// kept: the page as it was last drawn, to show at once while it is asked for again.
export function kept(page: string, device: Device, params: Record<string, string>, over?: { preview?: string }): Document | null {
  return pages.get(getToken() + ' ' + pageKey(page, device, params, over))?.doc || null
}

function keep(key: string, doc: Document, etag: string) {
  if (doc.version !== VERSION) throw new ApiError(0, 'This server sends pages in a form this app doesn’t know. Updating the app should fix it.')
  pages.delete(key)
  pages.set(key, { doc, etag: etag || doc.meta?.etag || '', at: Date.now() })
  while (pages.size > MAX_KEPT) pages.delete(pages.keys().next().value as string)
  return doc
}

// fetchPage: the page as it is now. With one kept, the diff endpoint says what changed
// (nothing, some patches, or the whole document); a patch that can't be applied drops
// the kept tree and asks for the page.
export async function fetchPage(page: string, device: Device, params: Record<string, string>, opts: { signal?: AbortSignal; diff?: boolean; preview?: string } = {}): Promise<Document> {
  const q = pageQuery(device, params, opts)
  const key = getToken() + ' ' + page + '?' + q
  const path = `${BASE}/pages/${encodeURIComponent(page)}`
  const had = pages.get(key)
  if (had?.etag && opts.diff !== false) {
    try {
      const r = await ask<{ etag: string; patches?: Patch[]; document?: Document }>('GET', `${path}/diff?${q}&since=${encodeURIComponent(had.etag)}`, { signal: opts.signal })
      if (r.status === 304 || !r.data) { had.at = Date.now(); return had.doc }
      if (r.data.document) return keep(key, r.data.document, r.data.etag)
      const next = applyPatches(had.doc, r.data.patches || [])
      if (next) { next.meta = { ...next.meta, etag: r.data.etag }; return keep(key, next, r.data.etag) }
    } catch (e) {
      if (opts.signal?.aborted) throw e
      if (e instanceof ApiError && (e.status === 404 || e.status === 403 || e.status === 0)) throw e
      // Anything else: the whole page, below.
    }
    pages.delete(key)
  }
  const r = await ask<Document>('GET', `${path}?${q}`, { signal: opts.signal, headers: had?.etag ? { 'If-None-Match': had.etag } : undefined })
  if (r.status === 304 && had) { pages.set(key, had); had.at = Date.now(); return had.doc }
  if (!r.data) throw new ApiError(0, 'The server sent no page.')
  return keep(key, r.data, r.etag)
}

// update: the page as the renderer has it now (more items arrived), kept for next time.
export function remember(page: string, device: Device, params: Record<string, string>, doc: Document) {
  const key = getToken() + ' ' + pageKey(page, device, params)
  const had = pages.get(key)
  // Its ETag no longer names this tree, so the next ask is for the whole page.
  if (had) pages.set(key, { doc, etag: '', at: had.at })
}

// ---------------------------------------------------------------- a section's items

const inFlight = new Map<string, Promise<SectionItems>>()

// fetchItems: one page of a section's items. The address is the server's (props.more.url,
// a season's url); the device is added to it. The same cursor is never asked twice at once.
export function fetchItems(url: string, device: Device): Promise<SectionItems> {
  if (!url.startsWith('/api/')) return Promise.reject(new ApiError(0, 'That isn’t an address on this server.'))
  const [path, had = ''] = url.split('?')
  const q = new URLSearchParams(had)
  for (const [k, v] of new URLSearchParams(pageQuery(device, {}))) if (!q.has(k)) q.set(k, v)
  const full = path + '?' + q.toString()
  const key = getToken() + ' ' + full
  const going = inFlight.get(key)
  if (going) return going
  const p = ask<SectionItems>('GET', full).then((r) => r.data || { version: 1, section: '' }).finally(() => inFlight.delete(key))
  inFlight.set(key, p)
  return p
}

// ---------------------------------------------------------------- navigation

let nav: { key: string; etag: string; data: Navigation } | null = null

export const keptNavigation = (device: Device): Navigation | null => (nav && nav.key === navKey(device) ? nav.data : null)
const navKey = (device: Device) => getToken() + ' ' + serverBase() + ' ' + query(device, { preview: preview || undefined })

export async function fetchNavigation(device: Device): Promise<Navigation> {
  const key = navKey(device)
  const had = nav?.key === key ? nav : null
  const r = await ask<Navigation>('GET', `${BASE}/navigation?${query(device, { preview: preview || undefined })}`, { headers: had?.etag ? { 'If-None-Match': had.etag } : undefined })
  if (r.status === 304 && had) return had.data
  if (!r.data) throw new ApiError(0, 'The server sent no menu.')
  nav = { key, etag: r.etag || r.data.etag || '', data: r.data }
  return r.data
}

// ---------------------------------------------------------------- personal

export type PersonalAnswer = { editable: boolean; personal?: Personal }

const personalPath = (url: string) => (url.startsWith('/api/') ? url : '')

export async function getPersonal(url: string): Promise<PersonalAnswer> {
  const r = await ask<PersonalAnswer>('GET', personalPath(url))
  return r.data || { editable: false }
}

export async function putPersonal(url: string, p: Personal): Promise<void> {
  await ask('PUT', personalPath(url), { body: p })
  bump(); nav = null
}

export async function deletePersonal(url: string): Promise<void> {
  await ask('DELETE', personalPath(url))
  bump(); nav = null
}

export const NAV_PERSONAL = `${BASE}/navigation/personal`
export const pagePersonal = (page: string) => `${BASE}/pages/${encodeURIComponent(page)}/personal`
export { ask as sduiAsk, BASE as SDUI_BASE }
