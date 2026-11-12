// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The editor's requests (docs/SDUI.md, "The editor's contract"). Everything the editor
// changes goes to the draft, which nobody sees until it is published; only publishing,
// discarding and going back to an old revision touch what everyone gets.
//
// Every write says which version it was made against (If-Match). The draft's ETag (the
// published configuration's while there is no draft) comes with every answer about the
// draft and is kept here, so two pages of the editor never trip over each other.

import { ApiError, getToken, serverBase } from '../../../api'
import { sduiAsk, SDUI_BASE } from '../../../sdui/client'
import type { Catalogue, Change, Issue, Navigation, Page, PageEntry, Preset } from './editorModel.ts'

export type Config = { version: number; revision: number; updatedAt?: string; updatedBy?: string; pages?: Record<string, Page>; navigation?: Navigation; etag?: string }
export type Revision = { revision: number; updatedAt?: string; updatedBy?: string; etag: string; pages: string[] | null; size: number; current: boolean }
export type PageAnswer = { etag: string; page: Page; source: string; issues: Issue[]; builtin?: Page }
export type NavAnswer = { etag: string; navigation: Navigation; source: string; builtin: Navigation }
export type Checked = { ok: boolean; issues: Issue[] }
export type DryRun = Checked & { changes: Change[] }

const enc = encodeURIComponent
const DRAFT = 'target=draft'

// The ETag a change to the draft is made against.
let draftTag = ''
const keep = (etag: string | undefined | null) => { if (etag) draftTag = etag }

// conflict: someone saved in between (412). issuesOf: what a refused save says is wrong (400).
export const isConflict = (e: unknown) => e instanceof ApiError && e.status === 412
export const issuesOf = (e: unknown): Issue[] | null => (e instanceof ApiError && e.status === 400 && Array.isArray(e.data?.issues) ? e.data.issues as Issue[] : null)
export const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

let catalogue: { key: string; answer: Promise<Catalogue> } | null = null

// The catalogue, asked once per server and sign-in: everything the forms are built from.
export function getCatalogue(): Promise<Catalogue> {
  const key = serverBase() + ' ' + getToken()
  if (catalogue?.key !== key) {
    const answer = sduiAsk<Catalogue>('GET', `${SDUI_BASE}/catalog`).then((r) => {
      if (!r.data) throw new ApiError(0, 'The server sent no catalogue.')
      return r.data
    })
    answer.catch(() => { if (catalogue?.key === key) catalogue = null })
    catalogue = { key, answer }
  }
  return catalogue.answer
}

// ---------------------------------------------------------------- reading

export async function listPages(draft = true): Promise<{ etag: string; draft: boolean; pages: PageEntry[] }> {
  const r = await sduiAsk<{ etag: string; draft: boolean; pages: PageEntry[] }>('GET', `${SDUI_BASE}/config/pages${draft ? '?' + DRAFT : ''}`)
  const d = r.data || { etag: '', draft: false, pages: [] }
  if (draft) keep(d.etag)
  return d
}

export async function getPage(id: string, draft = true): Promise<PageAnswer> {
  const r = await sduiAsk<PageAnswer>('GET', `${SDUI_BASE}/config/pages/${enc(id)}${draft ? '?' + DRAFT : ''}`)
  if (!r.data) throw new ApiError(0, 'The server sent no page.')
  if (draft) keep(r.data.etag)
  return r.data
}

export async function getNavigation(draft = true): Promise<NavAnswer> {
  const r = await sduiAsk<NavAnswer>('GET', `${SDUI_BASE}/config/navigation${draft ? '?' + DRAFT : ''}`)
  if (!r.data) throw new ApiError(0, 'The server sent no menu.')
  if (draft) keep(r.data.etag)
  return r.data
}

// Layout options (docs/SDUI.md, "Layout options"): which one each form factor starts with
// (`layouts.default`), which profiles may choose (`layouts.allowed`), the built-in starting
// ones, and the options themselves.
export type LayoutOption = { id: string; label: string; description?: string; icon?: string; nav: Record<string, string>; overlay?: boolean }
export type LayoutPolicy = { default?: Record<string, string>; allowed?: string[] }
export type LayoutsAnswer = { etag: string; layouts: LayoutPolicy; builtin: Record<string, string>; options: LayoutOption[] }
export async function getLayouts(draft = true): Promise<LayoutsAnswer> {
  const r = await sduiAsk<LayoutsAnswer>('GET', `${SDUI_BASE}/config/layouts${draft ? '?' + DRAFT : ''}`)
  if (!r.data) throw new ApiError(0, 'The server sent no layouts.')
  if (draft) keep(r.data.etag)
  return r.data
}

export async function getPresets(): Promise<Preset[]> {
  return (await sduiAsk<{ presets: Preset[] }>('GET', `${SDUI_BASE}/presets`)).data?.presets || []
}

// What differs between two revisions: a number, "current" or "draft". null when one of them isn't kept.
export async function getDiff(from: string | number, to: string | number): Promise<Change[] | null> {
  try {
    return (await sduiAsk<{ changes: Change[] }>('GET', `${SDUI_BASE}/config/diff?from=${enc(String(from))}&to=${enc(String(to))}`)).data?.changes || []
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null
    throw e
  }
}

export async function getHistory(): Promise<{ current: number; etag: string; keep: number; revisions: Revision[] }> {
  return (await sduiAsk<{ current: number; etag: string; keep: number; revisions: Revision[] }>('GET', `${SDUI_BASE}/config/history`)).data || { current: 0, etag: '', keep: 0, revisions: [] }
}

export async function validatePage(page: Page, signal?: AbortSignal): Promise<Checked> {
  return (await sduiAsk<Checked>('POST', `${SDUI_BASE}/validate`, { body: { page }, signal })).data || { ok: true, issues: [] }
}

export async function validateNavigation(navigation: Navigation, signal?: AbortSignal): Promise<Checked> {
  return (await sduiAsk<Checked>('POST', `${SDUI_BASE}/validate`, { body: { navigation }, signal })).data || { ok: true, issues: [] }
}

// ---------------------------------------------------------------- writing to the draft

// One change to the draft, made against the version last read. A server that wants the
// version and wasn't given it (428) is asked again with a fresh one. When someone else
// saved in between (412), onConflict decides: true reads the new version and makes the
// change again on top of it, false gives up.
async function toDraft<T>(method: string, path: string, body: unknown, onConflict?: () => Promise<boolean>): Promise<T> {
  const send = () => sduiAsk<T>(method, path + (path.includes('?') ? '&' : '?') + DRAFT, { body, headers: draftTag ? { 'If-Match': draftTag } : undefined })
  let r
  try {
    r = await send()
  } catch (e) {
    const status = e instanceof ApiError ? e.status : 0
    if (status !== 428 && !(status === 412 && onConflict && await onConflict())) throw e
    await listPages(true) // reads the version there is now
    r = await send()
  }
  keep(r.etag || (r.data as { etag?: string } | null)?.etag)
  return r.data as T
}

export const savePage = (page: Page, onConflict?: () => Promise<boolean>) => toDraft<Config>('PUT', `${SDUI_BASE}/config/pages/${enc(page.id)}`, page, onConflict)
// A built-in page goes back to how it ships; a page of the administrator's own is gone.
export const removePage = (id: string, onConflict?: () => Promise<boolean>) => toDraft<Config>('DELETE', `${SDUI_BASE}/config/pages/${enc(id)}`, undefined, onConflict)
export const saveNavigation = (nav: Navigation, onConflict?: () => Promise<boolean>) => toDraft<Config>('PUT', `${SDUI_BASE}/config/navigation`, nav, onConflict)
export const saveLayouts = (policy: LayoutPolicy, onConflict?: () => Promise<boolean>) => toDraft<Config>('PUT', `${SDUI_BASE}/config/layouts`, policy, onConflict)
export const resetNavigation = (onConflict?: () => Promise<boolean>) => toDraft<Config>('DELETE', `${SDUI_BASE}/config/navigation`, undefined, onConflict)
// A page preset lands under the preset's own page ID.
export const applyPreset = (id: string, onConflict?: () => Promise<boolean>) => toDraft<Config>('POST', `${SDUI_BASE}/presets/${enc(id)}/apply`, undefined, onConflict)
export const importFile = (file: unknown, theme: boolean, onConflict?: () => Promise<boolean>) => toDraft<Config>('POST', `${SDUI_BASE}/import${theme ? '?theme=1' : ''}`, file, onConflict)

export async function importDryRun(file: unknown, theme: boolean): Promise<DryRun> {
  return (await sduiAsk<DryRun>('POST', `${SDUI_BASE}/import?dryRun=1&${DRAFT}${theme ? '&theme=1' : ''}`, { body: file })).data || { ok: false, issues: [], changes: [] }
}

// ---------------------------------------------------------------- the draft and what everyone gets

export async function previewToken(): Promise<string> {
  return (await sduiAsk<{ token: string }>('POST', `${SDUI_BASE}/draft/preview`)).data?.token || ''
}

// Publishing sends the draft's own ETag, so a draft someone else changed since it was looked at isn't published unseen.
export async function publishDraft(): Promise<Config> {
  const r = await sduiAsk<Config>('POST', `${SDUI_BASE}/draft/publish`, { headers: draftTag ? { 'If-Match': draftTag } : undefined })
  draftTag = r.etag || ''
  return r.data as Config
}

export async function discardDraft(): Promise<void> {
  await sduiAsk('DELETE', `${SDUI_BASE}/draft`)
  draftTag = ''
}

// Going back changes what everyone gets, so it is made against the published version the history was read with.
export async function rollback(revision: number, etag: string): Promise<Config> {
  const send = (tag: string) => sduiAsk<Config>('POST', `${SDUI_BASE}/config/rollback/${revision}`, { headers: tag ? { 'If-Match': tag } : undefined })
  try {
    return (await send(etag)).data as Config
  } catch (e) {
    if (!(e instanceof ApiError) || e.status !== 428) throw e
    return (await send((await getHistory()).etag)).data as Config
  }
}

// ---------------------------------------------------------------- files

export async function getExport(page?: string, draft = true): Promise<unknown> {
  const q = [page ? 'page=' + enc(page) : '', draft ? DRAFT : ''].filter(Boolean).join('&')
  return (await sduiAsk<unknown>('GET', `${SDUI_BASE}/export${q ? '?' + q : ''}`)).data
}
