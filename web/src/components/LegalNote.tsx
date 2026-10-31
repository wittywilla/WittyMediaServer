import { Link } from 'react-router-dom'
import notices from '../../../internal/legal/notices.json'
import { useApp } from '../state'

// The short legal notices, in one place. The words live in
// internal/legal/notices.json (the server gives the same words to native apps), so
// a screen only says which notice it needs. docs/LEGAL-NOTICES-CHECKLIST.md
// says which one that is:
//
//   <LegalNote kind="webResults" />

export type LegalKind = keyof typeof notices.notices

// A screen can be seen signed out (a shared link, the setup wizard), where Settings can't be opened.
export function useLegalPath(section = '') {
  const { user } = useApp()
  return (user ? '/settings/legal' : '/legal') + (section ? '#' + section : '')
}

// The first-run acknowledgement's words.
export function legalAckText() { return notices.ack }

// newTab: for screens someone shouldn't be taken away from, like the setup wizard.
export default function LegalNote({ kind, className, newTab }: { kind: LegalKind; className?: string; newTab?: boolean }) {
  const n = notices.notices[kind]
  const to = useLegalPath(n.section)
  return (
    <p className={'legal-note' + (className ? ' ' + className : '')} role="note" aria-label="Legal notice" data-legal={kind}>
      {n.text}{' '}
      <Link to={to} target={newTab ? '_blank' : undefined} aria-label={'Read the legal notes on this' + (newTab ? ' (opens in a new tab)' : '')}>{notices.linkText}</Link>
    </p>
  )
}
