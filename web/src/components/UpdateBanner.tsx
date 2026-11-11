// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { toast, useApp } from '../state'
import { Notice, setUserPref, useUserPref } from '../ui'

// The update notice for admins at the top of Home and Settings, and what it and Settings, About
// know about a newer version. Kept small, as Home loads it: the notes are drawn in Settings.

export type Change = { version: string; notes: string }
export type UpdateInfo = { version: string; title: string; notes: string; url: string; published?: number; prerelease: boolean; signed: boolean; size: number; changes?: Change[] }
export type Installer = { name: string; size: number; phase?: 'downloading' | 'verifying' | 'ready' | 'failed'; done?: number; error?: string; path?: string; command?: string }
export type UpdateStatus = {
  current: string; latest?: UpdateInfo; checkedAt?: number; checkError?: string; withdrawn?: string
  phase: 'idle' | 'downloading' | 'verifying' | 'installing' | 'restarting' | 'failed'
  done?: number; total?: number; error?: string; canApply: boolean; reason?: string; page: string
  method?: 'apply' | 'installer' | 'docker'; installer?: Installer; image?: string
}

// The banner and the card ask for the same thing; one answer does for both for a minute.
let cached: { at: number; p: Promise<UpdateStatus> } | null = null
export function loadUpdateStatus(fresh = false): Promise<UpdateStatus> {
  if (fresh || !cached || Date.now() - cached.at > 60_000) {
    const p = api.get<UpdateStatus>('/update')
    cached = { at: Date.now(), p }
    p.catch(() => { if (cached?.p === p) cached = null })
  }
  return cached.p
}
export const forgetUpdateStatus = () => { cached = null }

// headline: the newest version's first change in a few words ("DVDs and Blu-rays in Firefox"),
// and how many more changes there are in every version since this one. The changelog leads each
// change with its point in bold.
export function headline(changes: Change[] | undefined): { first: string; more: number } {
  let first = '', count = 0
  for (const c of changes || []) {
    for (const line of c.notes.split('\n')) {
      const m = /^[-*+] (.*)$/.exec(line)
      if (!m) continue
      count++
      if (!first) {
        const bold = /^\*\*(.+?)\*\*/.exec(m[1])
        first = (bold ? bold[1] : m[1]).replace(/\*\*|__|`|\[([^\]]*)\]\([^)]*\)/g, '$1').trim().replace(/[.:]$/, '')
        if (first.length > 90) first = first.slice(0, 88).replace(/\s+\S*$/, '') + '…'
      }
    }
  }
  return { first, more: Math.max(0, count - (first ? 1 : 0)) }
}

// UpdateBanner: for admins only, until they choose Later for that version (kept with their
// account, so it follows them to other devices).
export function UpdateBanner() {
  const { update, user } = useApp()
  const [later] = useUserPref<string>('updateLater', '')
  const [st, setSt] = useState<UpdateStatus | null>(null)
  const nav = useNavigate()
  const show = !!update && !!user?.isAdmin && later !== update
  useEffect(() => { if (show) loadUpdateStatus().then(setSt).catch(() => { /* the dot on Settings still says so */ }) }, [show, update])
  if (!show) return null
  const h = headline(st?.latest?.changes)
  const go = (start: boolean) => nav('/settings/about#updates', { state: start ? { startUpdate: true } : undefined })
  return (
    <div className="upd-banner">
      <Notice tone="info" title={`WittyMediaServer ${update} is available`} action={
        <div className="upd-banner-actions">
          <button type="button" className="btn primary sm" onClick={() => go(true)}>{st?.method === 'docker' ? 'How to update' : 'Update'}</button>
          <button type="button" className="btn sm" onClick={() => go(false)}>What’s new</button>
          <button type="button" className="btn ghost sm" onClick={() => setUserPref('updateLater', update).catch(() => toast('Could not save that'))}>Later</button>
        </div>
      }>
        {st && <span>{h.first ? `${h.first}${h.more ? `, and ${h.more} more ${h.more === 1 ? 'change' : 'changes'}` : ''}.` : 'A newer version of the server is ready to install.'}</span>}
      </Notice>
    </div>
  )
}
