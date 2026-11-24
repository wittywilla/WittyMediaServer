// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { EmptyState } from './common'
import { LinkButton } from '../ui'
import { t } from '../i18n'

// The page for an address with nothing at it: the app's own, or a module's page the module
// doesn't have.
export function PageNotFound() {
  return <><h1 className="sr-only">{t('Page not found')}</h1><EmptyState title={t('There’s no page here')} text={t('The address may have changed, or it may be mistyped.')} action={<LinkButton variant="primary" to="/">{t('Go to Home')}</LinkButton>} /></>
}
