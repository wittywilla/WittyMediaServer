// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { t } from '../i18n'

// Tip is a small "?" that explains a setting on hover, focus or tap.
export default function Tip({ children, wide }: { children: React.ReactNode; wide?: boolean }) {
  return (
    <span className={'tip' + (wide ? ' wide' : '')} tabIndex={0} role="button" aria-label={t('More information')} onClick={(e) => e.preventDefault()}>
      ?
      <span className="tip-body" role="tooltip">{children}</span>
    </span>
  )
}
