import { useState } from 'react'
import { api, getPrefs } from '../../../api'
import { IconShield } from '../../../icons'
import { toast } from '../../../state'
import { Row, Section } from '../ui'
import type { LogsView } from './types'

export const LEVELS: { v: string; label: string }[] = [
  { v: 'off', label: 'Off' },
  { v: 'error', label: 'Errors only' },
  { v: 'warn', label: 'Warnings and errors' },
  { v: 'info', label: 'Normal' },
  { v: 'debug', label: 'Everything' },
]

export function PrivacyNote() {
  return (
    <div className="lg-privacy" role="note">
      <IconShield />
      <p><b>Your logs stay with you.</b> They are written only to this server’s own disk. They are never uploaded, and nobody, including the people who make this software, sees them unless you choose to send a file yourself.</p>
    </div>
  )
}

// Each area's level takes effect at once, with no restart.
export default function WhatToLog({ view, onView }: { view: LogsView; onView: (v: LogsView) => void }) {
  const [busy, setBusy] = useState('')
  const change = async (id: string, label: string, level: string) => {
    setBusy(id)
    try {
      onView(await api.put<LogsView>('/logs', { levels: { [id]: level } }))
      toast(`${label}: ${LEVELS.find((l) => l.v === level)?.label || level}`)
      if (id === 'clients') void getPrefs(true) // this app stops or starts reporting
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  return (
    <>
      <PrivacyNote />
      <Section title="What to log" desc="How much each part of the server writes down. Changes work straight away, with no restart. Off still records crashes. Modules can have a level of their own, under Modules.">
        {view.categories.map((c) => (
          <Row key={c.id} label={c.label} desc={c.description}>
            <select className="select" value={view.levels[c.id] || c.default} disabled={busy === c.id} onChange={(e) => void change(c.id, c.label, e.target.value)}>
              {LEVELS.map((l) => <option key={l.v} value={l.v}>{l.label}{l.v === c.default ? ' (default)' : ''}</option>)}
            </select>
          </Row>
        ))}
      </Section>
    </>
  )
}
