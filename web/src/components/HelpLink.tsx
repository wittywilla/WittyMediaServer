import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { IconHelp } from '../icons'
import { settingsHelp } from '../help/settingsHelp'
import './helplink.css'

// The ways into a guide from the page it's about. `to` is a guide and perhaps a part of it, such
// as "module-store#what-limits-a-module"; internal/docscheck checks that each leads somewhere.

// A link in a sentence. In the middle of something that would be lost by leaving (setup, a
// dialog), newTab keeps that page where it is.
export function HelpLink({ to, newTab, children }: { to: string; newTab?: boolean; children: ReactNode }) {
  if (newTab) return <Link className="help-inline" to={`/help/${to}`} target="_blank" rel="noopener">{children}<span className="sr-only"> (opens in a new tab)</span></Link>
  return <Link className="help-inline" to={`/help/${to}`}>{children}</Link>
}

// A quiet link with its own icon, for a page's toolbar or header.
export function PageHelp({ to, children }: { to: string; children?: ReactNode }) {
  return <Link to={`/help/${to}`} className="help-link"><IconHelp size={16} /><span>{children ?? 'Help'}</span></Link>
}

// At the top of every settings page.
export function SettingsHelp({ page }: { page: string | undefined }) {
  const to = page ? settingsHelp[page] : undefined
  return to ? <div className="set-help"><PageHelp to={to}>Help with this page</PageHelp></div> : null
}
