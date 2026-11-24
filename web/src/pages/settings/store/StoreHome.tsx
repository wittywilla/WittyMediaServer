// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { IconShield } from '../../../icons'
import { t, tk } from '../../../i18n'
import { FailureDialog, useStoreView } from './parts'
import { PageHead } from '../ui'
import { LinkButton, Spinner, Tabs } from '../../../ui'
import StoreBoundary from './StoreBoundary'

// The module store, at a glance: one page, three tabs.
//   Installed: what's on this server, what needs a look first, and what's new.
//   Browse:    everything the store offers, by what it's for, with whether each works here.
//   Updates:   how updates happen, the ones waiting, and what they bring.
// The tab is in the address (?tab=), so Back, a link from elsewhere and a shared address all land on
// the same view. Each tab sits in its own StoreBoundary: whatever a catalogue says, a problem stays in
// that tab, and the rest of the app carries on.

const Installed = lazy(() => import('../Modules'))
const Browse = lazy(() => import('./ModuleStore'))

export type StoreTab = 'installed' | 'browse' | 'updates'
const TABS: StoreTab[] = ['installed', 'browse', 'updates']

export default function StoreHome() {
  const [params, setParams] = useSearchParams()
  const store = useStoreView()
  const asked = params.get('tab') as StoreTab | null
  // A link from Add library (?add=<id>) or to a row (#id) is about one module: Installed finds it.
  const tab: StoreTab = asked && TABS.includes(asked) ? asked : 'installed'
  const updates = store.view?.updates || 0
  const installed = store.view?.items.filter((i) => i.status?.installed).length
  const choose = (v: StoreTab) => setParams(() => { const n = new URLSearchParams(); if (v !== 'installed') n.set('tab', v); return n })
  const items = [
    { value: 'installed' as const, label: installed ? t('Installed · {count}', { count: installed }) : t('Installed') },
    { value: 'browse' as const, label: t('Browse') },
    { value: 'updates' as const, label: updates ? t('Updates · {count}', { count: updates }) : t('Updates') },
  ]
  return (
    <div className="ms-page st-page st-home">
      <PageHead title={tk('Module store')} desc={<>{t('Add to what this server can do, keep it up to date, and take away what you don’t use. Everything here is signed, and nothing runs until you choose it.')} <Link to="/settings/module-settings">{t('Each module’s own settings are in Module settings.')}</Link></>}
        actions={<LinkButton size="sm" variant="ghost" to="/settings/modules/stores" icon={<IconShield />}>{t('Other stores')}</LinkButton>} />
      {/* The tabs are this page's own contents, so Settings' list of sections stays away. */}
      <nav data-own-toc className="st-tabs-nav" aria-label={t('Module store')}>
        <Tabs<StoreTab> className="st-tabs" value={tab} onChange={choose} label={t('Module store')} panelId="st-panel" items={items} />
      </nav>
      <div id="st-panel" role="tabpanel" aria-label={items.find((i) => i.value === tab)!.label}>
        <StoreBoundary key={tab}>
          <Suspense fallback={<Spinner />}>
            {tab === 'browse' ? <Browse store={store} /> : <Installed store={store} mode={tab} />}
          </Suspense>
        </StoreBoundary>
      </div>
      {store.checkFailure && <FailureDialog failure={store.checkFailure} name={t('The check')} repairRoute={store.view?.repairRoute} onClose={store.clearCheckFailure} />}
    </div>
  )
}

// The old address of Browse, kept so links and bookmarks still land in the right place.
export function OldBrowse() {
  const [params] = useSearchParams()
  const n = new URLSearchParams(params)
  n.delete('show')
  n.set('tab', params.get('show') === 'updates' ? 'updates' : params.get('show') === 'installed' ? 'installed' : 'browse')
  return <Navigate replace to={`/settings/modules?${n}`} />
}
