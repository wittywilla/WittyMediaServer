import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../../api'
import { IconClose, IconPlug } from '../../../icons'
import { toast } from '../../../state'
import { Row, Section, Switch } from '../ui'
import type { LogModule, LogsView } from './types'
import { LEVELS } from './WhatToLog'

const plural = (n: number, one: string) => `${n.toLocaleString()} ${one}${n === 1 ? '' : 's'}`

function recent(m: LogModule): string {
  const bad = [m.errors > 0 && plural(m.errors, 'error'), m.warnings > 0 && plural(m.warnings, 'warning')].filter(Boolean).join(' and ')
  if (bad) return `Recently: ${bad}`
  return m.lines > 0 ? `Recently: ${plural(m.lines, 'line')}, no problems` : 'Nothing logged recently'
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
            {m.new && <span className="set-badge lg-new-mod">New</span>}
            {m.missing && <span className="lg-gone-tag">No longer installed</span>}
          </b>
          <span>{recent(m)}</span>
        </div>
        <select className="select" aria-label={`How much ${m.name} logs`} value={m.level} disabled={busy}
          onChange={(e) => change(m.id, { level: e.target.value }, e.target.value ? LEVELS.find((l) => l.v === e.target.value)?.label || e.target.value : 'Same as its area')}>
          <option value="">Same as its area</option>
          {LEVELS.map((l) => <option key={l.v} value={l.v}>{l.label}</option>)}
        </select>
      </div>
      <div className="lg-actions">
        <Link className="btn sm ghost" to={`?module=${encodeURIComponent(m.id)}`}>Show what this module logged</Link>
        {m.topics.length > 0 && (
          <button type="button" className="btn sm ghost" aria-expanded={open} onClick={() => setOpen(!open)}>
            Advanced{on > 0 ? ` (${on} on)` : ''}
          </button>
        )}
      </div>
      {open && (
        <div className="lg-topics">
          {m.topics.map((t) => (
            <Row key={t.id} label={t.label} desc={t.description}>
              <Switch label={`${m.name}: ${t.label}`} checked={t.on} disabled={busy} onChange={(v) => change(m.id, { topics: { [t.id]: v } }, `${t.label} ${v ? 'on' : 'off'}`)} />
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
      toast(`${view.modules.find((m) => m.id === id)?.name || id}: ${said}`)
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const dismiss = async () => {
    try { onView(await api.put<LogsView>('/logs', { modulesSeen: true })) } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Modules" desc="How much each module writes down. A module follows the level of its area unless you choose otherwise. Changes work straight away, with no restart.">
      {fresh.length > 0 && (
        <div className="lg-mod-notice" role="status">
          <span>{fresh.length === 1
            ? `A new module was found: ${fresh[0].name}. Its logging is ready below.`
            : `New modules were found: ${fresh.map((m) => m.name).join(', ')}. Their logging is ready below.`}</span>
          <button type="button" className="btn sm ghost" onClick={() => void dismiss()}><IconClose /> Dismiss</button>
        </div>
      )}
      {view.modules.length === 0
        ? <div className="set-empty"><IconPlug size={28} /><b>No modules found</b><p>Modules appear here as soon as the server notices them.</p></div>
        : <ul className="lg-mods">{view.modules.map((m) => <ModuleCard key={m.id} m={m} busy={busy === m.id} change={(id, b, s) => void change(id, b, s)} />)}</ul>}
    </Section>
  )
}
