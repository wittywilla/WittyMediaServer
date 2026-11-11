// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import './diagnoser.css'

// One thing a check found, in plain words, for WittyDiagnoser and Repair alike. The word
// says how much it matters, so colour is never alone in saying it.

export type Severity = 'ok' | 'info' | 'warn' | 'problem'
export type Finding = { severity: Severity; title: string; what?: string; fix?: string; details?: string[]; names?: string[]; more?: number; target?: string }

export const SEVERITY_WORDS: Record<Severity, string> = { problem: 'Problem', warn: 'Warning', info: 'Note', ok: 'Fine' }

export default function FindingCard({ f, fixLabel = 'What to do', badge, children, whole }: { f: Finding; fixLabel?: string; badge?: ReactNode; children?: ReactNode; whole?: boolean }) {
  const lines = [...(f.details || []), ...(f.names || [])]
  const list = (
    <>
      <ul>{lines.map((l, i) => <li key={i}>{l}</li>)}</ul>
      {!!f.more && <p className="set-row-desc">…and {f.more.toLocaleString()} more.</p>}
    </>
  )
  return (
    <article className={'dgn-card sev-' + f.severity}>
      <div className="dgn-card-head">
        <span className="dgn-sev">{SEVERITY_WORDS[f.severity]}</span>
        {f.target && <span className="dgn-target">{f.target}</span>}
        {badge}
      </div>
      <h3>{f.title}</h3>
      {f.what && <p>{f.what}</p>}
      {f.fix && <p><b>{fixLabel}:</b> {f.fix}</p>}
      {lines.length > 0 && (whole ? list : (
        <details>
          <summary>Details</summary>
          {list}
        </details>
      ))}
      {children}
    </article>
  )
}
