// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api } from '../../../api'
import { IconRefresh } from '../../../icons'
import { Button } from '../../../ui'
import { Section } from '../ui'
import { date, t, tk } from '../../../i18n'
import { auditRows, auditWords, toDate, type AuditEntry } from './overrides'
import './overrides.css'

// The module audit: every change to which code runs on this server, who made it and when, newest
// first. Overrides, uploads, versions chosen by hand and refused passwords are all here.

const PAGE = 50
const MOST = 500

// stamp: anything that changes when the modules do, so a change made on this page shows here.
export function ModuleAudit({ nameOf, stamp }: { nameOf: (id: string) => string; stamp?: unknown }) {
  const [rows, setRows] = useState<AuditEntry[] | null>(null)
  const [limit, setLimit] = useState(PAGE)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const load = useCallback(async (n: number) => {
    setLoading(true)
    try { setRows(auditRows(await api.get<unknown>(`/modules/audit?limit=${n}`))); setErr('') } catch (e) { setErr(e instanceof Error ? e.message : String(e)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load(limit) }, [load, limit, stamp])
  const more = rows !== null && rows.length >= limit && limit < MOST
  return (
    <Section title={tk('Module audit log')} desc={t('Every change to which code runs on this server: installs past a rule, files uploaded, overrides removed, versions chosen by hand and passwords refused. Newest first.')}
      actions={<Button size="sm" variant="ghost" icon={<IconRefresh />} aria-busy={loading} onClick={() => load(limit)}>{loading ? t('Reading…') : t('Refresh')}</Button>}>
      {err && <p className="error" role="alert">{err}</p>}
      {rows === null && !err && <p className="ms-note" role="status">{t('Reading the log…')}</p>}
      {rows?.length === 0 && <p className="ms-note">{t('Nothing has been written to it yet.')}</p>}
      {!!rows?.length && (
        <ul className="au-list" aria-label={t('Module audit log, newest first')}>
          {rows.map((r, i) => {
            const d = toDate(r.at)
            const mod = r.module ? nameOf(r.module.replace(/^module /, '')) : ''
            return (
              <li key={i} className="au-row">
                {d ? <time dateTime={d.toISOString()}>{date(d, { dateStyle: 'medium', timeStyle: 'short' })}</time> : <span />}
                <span className="au-what">
                  <span><b>{auditWords(r.action)}</b>{mod && <>: {mod}</>}{r.version && <> {r.version}</>}{r.user && <span className="muted"> · {t('by {user}', { user: r.user })}</span>}</span>
                  {r.detail && <span className="au-detail">{r.detail}</span>}
                </span>
              </li>
            )
          })}
        </ul>
      )}
      {more && <div className="au-more"><Button size="sm" onClick={() => setLimit((n) => Math.min(MOST, n + PAGE))} aria-busy={loading}>{t('Show older entries')}</Button></div>}
    </Section>
  )
}
