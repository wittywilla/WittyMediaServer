// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { reportClient } from '../errorReport'
import { t } from '../i18n'
import { StandaloneCard } from './StandaloneCard'

// The last line of defence: an error while drawing a page would otherwise leave
// a blank screen. It's reported to the server's log and this offers a way back.
// The router may be what broke, so the buttons don't rely on it.
export default class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() { return { failed: true } }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    const e = error as Error | undefined
    reportClient({ level: 'error', kind: 'js', message: e?.message || String(error), stack: [e?.stack, info.componentStack].filter(Boolean).join('\n') })
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <StandaloneCard foot={<>{t('Looking after this server, and it keeps happening?')} <a href="/settings/logs?tab=bundle">{t('Download the logs, sanitized for sharing, for whoever is helping you.')}</a></>}>
        <div role="alert">
          <h1>{t('This page ran into a problem')}</h1>
          <p className="sa-lead">{t("Reloading usually fixes it. What happened has been noted in your server's log, and nothing was sent anywhere else.")}</p>
        </div>
        <div className="sa-actions">
          <button type="button" className="btn primary" onClick={() => window.location.reload()}>{t('Reload')}</button>
          <div className="sa-links">
            <button type="button" className="sa-text-btn" onClick={() => window.location.assign('/')}>{t('Go to Home')}</button>
            <a className="sa-text-btn" href="/help/troubleshooting">{t('Troubleshooting')}</a>
          </div>
        </div>
      </StandaloneCard>
    )
  }
}
