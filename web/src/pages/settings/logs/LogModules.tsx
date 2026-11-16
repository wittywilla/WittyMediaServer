// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../../api'
import { IconClose, IconPlug } from '../../../icons'
import { toast } from '../../../state'
import { Row, Section, Switch } from '../ui'
import type { LogModule, LogsView } from './types'
import { LEVELS } from './WhatToLog'
import { list, t, tn, tx } from '../../../i18n'

function recent(m: LogModule): string {
  const bad = [m.errors > 0 && tn('{count} error', '{count} errors', m.errors), m.warnings > 0 && tn('{count} warning', '{count} warnings', m.warnings)].filter(Boolean) as string[]
  if (bad.length) return t('Recently: {what}', { what: list(bad) })
  return m.lines > 0 ? tn('Recently: {count} line, no problems', 'Recently: {count} lines, no problems', m.lines) : t('Nothing logged recently')
}

function ModuleCard({ m, busy, change }: { m: LogModule; busy: boolean; change: (id: string, body: { level?: string; topics?: Record<string, boolean> }, said: string) => void }) {
  const [open, setOpen] = useState(false)
  const on = m.topics.filter((t) => t.on).length
  return (
    <li className={'lg-mod' + (m.missing ? ' gone' : '')}>
      <div className="lg-mod-top">
        <div className="lg-file-text">
          <b>
            {m.name}{m.version && <span className="lg-mod-ver"> {m.version}</span>}
            {m.new && <span className="set-badge lg-new-mod">{t('New')}</span>}
            {m.missing && <span className="lg-gone-tag">{t('No longer installed')}</span>}
          </b>
          <span>{recent(m)}</span>
        </div>
        <select className="select" aria-label={t('How much {module} logs', { module: m.name })} value={m.level} disabled={busy}
          onChange={(e) => change(m.id, { level: e.target.value }, e.target.value ? tx(LEVELS.find((l) => l.v === e.target.value)?.label) || e.target.value : t('Same as its area'))}>
          <option value="">{t('Same as its area')}</option>
          {LEVELS.map((l) => <option key={l.v} value={l.v}>{tx(l.label)}</option>)}
        </select>
      </div>
      <div className="lg-actions">
        <Link className="btn sm ghost" to={`?module=${encodeURIComponent(m.id)}`}>{t('Show what this module logged')}</Link>
        {m.topics.length > 0 && (
          <button type="button" className="btn sm ghost" aria-expanded={open} onClick={() => setOpen(!open)}>
            {on > 0 ? tn('Advanced ({count} on)', 'Advanced ({count} on)', on) : t('Advanced')}
          </button>
        )}
      </div>
      {open && (
        <div className="lg-topics">
          {m.topics.map((tp) => (
            <Row key={tp.id} label={tx(tp.label)} desc={tx(tp.description)}>
              <Switch label={`${m.name}: ${tx(tp.label)}`} checked={tp.on} disabled={busy} onChange={(v) => change(m.id, { topics: { [tp.id]: v } }, v ? t('{topic} on', { topic: tx(tp.label) }) : t('{topic} off', { topic: tx(tp.label) }))} />
            </Row>
          ))}
        </div>
      )}
    </li>
  )
}

// Each module's own logging: a level, and for those that have them, kinds of
// detail that can be turned on without making the rest of it detailed too.
export default function LogModules({ view, onView }: { view: LogsView; onView: (v: LogsView) => void }) {
  const [busy, setBusy] = useState('')
  const fresh = view.modules.filter((m) => m.new)
  const change = async (id: string, body: { level?: string; topics?: Record<string, boolean> }, said: string) => {
    setBusy(id)
    try {
      onView(await api.put<LogsView>('/logs', { modules: { [id]: body } }))
      toast(t('{module}: {change}', { module: view.modules.find((m) => m.id === id)?.name || id, change: said }))
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const dismiss = async () => {
    try { onView(await api.put<LogsView>('/logs', { modulesSeen: true })) } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title={t('Modules')} desc={t('How much each module writes down. A module follows the level of its area unless you choose otherwise. Changes work straight away, with no restart.')}>
      {fresh.length > 0 && (
        <div className="lg-mod-notice" role="status">
          <span>{fresh.length === 1
            ? t('A new module was found: {name}. Its logging is ready below.', { name: fresh[0].name })
            : t('New modules were found: {names}. Their logging is ready below.', { names: list(fresh.map((m) => m.name)) })}</span>
          <button type="button" className="btn sm ghost" onClick={() => void dismiss()}><IconClose /> {t('Dismiss')}</button>
        </div>
      )}
      {view.modules.length === 0
        ? <div className="set-empty"><IconPlug size={28} /><b>{t('No modules found')}</b><p>{t('Modules appear here as soon as the server notices them.')}</p></div>
        : <ul className="lg-mods">{view.modules.map((m) => <ModuleCard key={m.id} m={m} busy={busy === m.id} change={(id, b, s) => void change(id, b, s)} />)}</ul>}
    </Section>
  )
}
