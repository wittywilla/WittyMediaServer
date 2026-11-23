// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { legend } from './inputModel'
import { useInput } from './useInput'
import { HELP_EVENT } from './KeyHelp.event'
import { useTvMode } from '../tvmode/mode'
import { IconInfo } from '../icons'
import { t, tx } from '../i18n'

// The key legend: what each key, button or remote key does here, drawn for the input in use and hidden when there
// is none (touch only). It sits in a screen's Dock. docs/plans/DESIGN-2.md section 3.
//
// TV mode (the owner's rule): a computer's keyboard shortcuts are never shown on a television, and the remote's or a
// pad's keys aren't a bar along the screen either. They are in the Keys dialog, opened by one button (KeyInfoButton)
// a remote reaches; Back closes it. The keys themselves keep working.
export function KeyLegend({ scope = 'screen', className }: { scope?: 'screen' | 'list'; className?: string }) {
  const { kind, family } = useInput()
  const tv = useTvMode()
  if (tv) return <KeyInfoButton className={className} />
  const items = legend(kind, family, { scope })
  if (!items.length) return null
  return (
    <ul className={'key-legend' + (className ? ' ' + className : '')} aria-label={t('Keys')} data-nav-skip>
      {items.map((i) => (
        <li key={i.action}><kbd>{tx(i.glyph)}</kbd><span>{tx(i.label)}</span></li>
      ))}
    </ul>
  )
}

// The one button a TV shows for the keys: it opens the Keys dialog (ui/KeyHelp.tsx) with the remote's or the pad's.
export function KeyInfoButton({ className }: { className?: string }) {
  return (
    <button type="button" className={'key-info-btn' + (className ? ' ' + className : '')} onClick={() => window.dispatchEvent(new Event(HELP_EVENT))} aria-haspopup="dialog">
      <IconInfo size={20} /><span>{t('Keys')}</span>
    </button>
  )
}
