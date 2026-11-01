import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { IconHelp } from '../icons'
import { settingsHelp } from '../help/guides'

// The way into a guide from the page it's about. `to` is a guide and perhaps a part of it,
// such as "module-store#what-limits-a-module"; internal/docscheck checks each one leads somewhere.
export function HelpLink({ to, children }: { to: string; children?: ReactNode }) {
  return <Link to={`/help/${to}`}>{children ?? 'Help'}</Link>
}

// At the top of a settings page, on every settings page.
export function SettingsHelp({ page }: { page: string | undefined }) {
  const to = page ? settingsHelp[page] : undefined
  if (!to) return null
  return (
    <div className="set-help">
      <Link to={`/help/${to}`} className="help-link"><IconHelp size={16} /><span>Help with this page</span></Link>
    </div>
  )
}
