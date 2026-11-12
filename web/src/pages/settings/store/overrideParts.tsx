// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState, type ReactNode } from 'react'
import { api, fmtDateTime } from '../../../api'
import { IconShield } from '../../../icons'
import type { ModuleReport, ModuleStatus } from '../../../moduleStore'
import { toast } from '../../../state'
import { ask, Button, Notice } from '../../../ui'
import { OverrideDialog, type OverrideBody } from './OverrideDialog'
import {
  overrideLines, releaseStatus, STATUS_WORDS, statusWarns, toDate, uiLines, type NeedsOverride, type OverrideRecord, type UiSummary,
} from './overrides'
import './overrides.css'

// What the store and the Modules page both show about overrides and release status.

function Tag({ tone, icon, children }: { tone?: 'ok' | 'warn' | 'bad' | 'new' | 'off'; icon?: ReactNode; children: ReactNode }) {
  return <span className={'ms-tag' + (tone ? ' ' + tone : '')}>{icon}{children}</span>
}

// The labels for a module installed past a rule, and for a release that isn't stable.
export function OverrideTags({ override, status }: { override?: OverrideRecord | null; status?: string }) {
  const st = releaseStatus(status)
  const words = STATUS_WORDS[st]
  return (
    <>
      {override && <Tag tone="bad" icon={<IconShield size={12} />}>Overridden</Tag>}
      {st !== 'stable' && <Tag tone={words.tone}>{words.title}</Tag>}
    </>
  )
}

const SOURCE_WORDS: Record<string, string> = { catalogue: 'from the official catalogue', upload: 'from an uploaded file', store: 'from another store' }

// The override on a module: what was bypassed, who said yes and when, what it was given, and the
// way back. Removing it goes back to the last version that needed none, or takes the module off.
export function OverrideRecordBox({ id, name, record, onRemoved, disabled }: { id: string; name: string; record: OverrideRecord; onRemoved: (r?: ModuleReport) => void; disabled?: boolean }) {
  const [working, setWorking] = useState(false)
  const when = toDate(record.at)
  const lines = overrideLines(record)
  const serious = lines.some((l) => ['unsigned', 'untrusted-key', 'custom', 'weak-sandbox'].includes(l.kind))
  const remove = async () => {
    if (!await ask({
      title: `Remove the override on ${name}?`, confirm: 'Remove override', danger: true,
      body: `${name} goes back to the last version on this server that needed no override. If there isn’t one, ${name} is taken off this server. Your files and what you’ve watched, read or played stay as they are.`,
    })) return
    setWorking(true)
    try {
      const r = await api.del<ModuleReport | ModuleStatus>(`/modules/${encodeURIComponent(id)}/override`)
      toast(`The override on ${name} was removed`)
      onRemoved(r && 'modules' in (r as ModuleReport) ? r as ModuleReport : undefined)
    } catch (e) { toast(e instanceof Error ? e.message : String(e)) } finally { setWorking(false) }
  }
  return (
    <section className={'ov-record' + (serious ? ' danger' : '')} aria-label={`Override on ${name}`}>
      <h3><IconShield size={18} /> Overridden</h3>
      <p>{name} was installed past {lines.length === 1 ? 'a rule' : `${lines.length} rules`} that would normally stop it. It stays on this version: automatic updates leave it alone until the override is removed or you choose a version that needs none.</p>
      <ul>{lines.map((l) => <li key={l.kind}><b>{l.title}</b>{l.reason ? `: ${l.reason}` : ''}</li>)}</ul>
      <dl className="ms-facts">
        {(record.by || when) && <div className="wide"><dt>Accepted</dt><dd>{record.by ? `by ${record.by}` : ''}{record.by && when ? ', ' : ''}{when ? <time dateTime={when.toISOString()}>{fmtDateTime(when.getTime())}</time> : ''}</dd></div>}
        {record.version && <div><dt>Version</dt><dd>{record.version}</dd></div>}
        {record.source && <div><dt>Came</dt><dd>{SOURCE_WORDS[record.source] || record.source}</dd></div>}
        {record.fingerprint && <div className="wide"><dt>Key fingerprint</dt><dd className="mono">{record.fingerprint}</dd></div>}
        <div className="wide"><dt>Permissions given</dt><dd>{record.grants?.length ? record.grants.join(', ') : serious ? 'None: it runs with no permissions, in the strictest sandbox' : 'As the module’s official rules allow'}</dd></div>
      </dl>
      <div><Button size="sm" variant="danger" busy={working} disabled={disabled} onClick={remove}>Remove override</Button></div>
    </section>
  )
}

// A release that isn't stable says so, with its publisher's note, where it can't be missed.
export function StatusNotice({ status, note, name, installed }: { status?: string; note?: string; name: string; installed?: boolean }) {
  const st = releaseStatus(status)
  if (st === 'stable') return null
  if (!statusWarns(st)) return note ? <Notice title={`${name} is in beta`}>{note}</Notice> : null
  return (
    <Notice tone="warn" title={st === 'incomplete' ? `${name} isn’t finished` : `${name} is experimental`}>
      {note || (st === 'incomplete' ? 'Parts of it are missing or don’t work yet.' : 'It may change a lot or break.')} {installed ? 'Its publisher says so; it stays on this version until you choose another.' : 'Installing it needs an override.'}
    </Notice>
  )
}

// What a module's SDUI adds to the app: its pages, its menu entries and whether it has settings.
export function WhatItAdds({ ui, name }: { ui?: UiSummary | null; name: string }) {
  const l = uiLines(ui)
  // An empty summary only says its server-driven screens add nothing; it may still have a page of
  // its own, so nothing is claimed either way.
  if (!l || l.empty) return null
  return (
    <section className="st-mod-sec" aria-labelledby="st-adds">
      <h2 id="st-adds">What it adds</h2>
      {(
        <ul className="ov-adds" aria-label={`What ${name} adds`}>
          {l.pages.length > 0 && <li><b>{l.pages.length === 1 ? 'A page' : `${l.pages.length} pages`}:</b> {l.pages.join(', ')}</li>}
          {l.navigation.length > 0 && <li><b>In the menu:</b> {l.navigation.join(', ')}</li>}
          <li><b>Settings:</b> {l.settings ? 'its own page in Settings' : 'none of its own'}</li>
        </ul>
      )}
    </section>
  )
}

// A release downloaded and checked, waiting for someone to accept what it needs.
export function PendingNotice({ m, onGo, onDropped }: { m: ModuleStatus; onGo: () => void; onDropped: (r?: ModuleReport) => void }) {
  const p = m.pending
  if (!p) return null
  const drop = async () => {
    try {
      const r = await api.del<ModuleReport>(`/modules/${encodeURIComponent(m.id)}/pending`)
      toast(`${m.name} ${p.version} was dropped`)
      onDropped(r && 'modules' in r ? r : undefined)
    } catch (e) { toast(e instanceof Error ? e.message : String(e)) }
  }
  return (
    <Notice tone="warn" title={`${m.name} ${p.version}: downloaded, needs your go-ahead`}
      action={<span className="ms-actions"><Button size="sm" variant="primary" onClick={onGo}>Review…</Button><Button size="sm" variant="ghost" onClick={drop}>Drop it</Button></span>}>
      It was downloaded and its signature checked, and then it turned out to need an override. Nothing changed: the version in use keeps running.
    </Notice>
  )
}

// One override dialog for a page, opened with what the server asked for and how to send it.
export type OverrideAsk = { id: string; name: string; need: NeedsOverride; permissionWords?: Record<string, string>; send: (body: OverrideBody) => Promise<void> }
export function useOverrideDialog() {
  const [ask, setAsk] = useState<OverrideAsk | null>(null)
  const dialog = ask && <OverrideDialog id={ask.id} name={ask.name} need={ask.need} permissionWords={ask.permissionWords} onSubmit={ask.send} onClose={() => setAsk(null)} />
  return { open: setAsk, dialog }
}
