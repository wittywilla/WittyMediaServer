// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { reportClient } from '../errorReport'
import { t } from '../i18n'
import { IconRefresh } from '../icons'

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
      <main className="center">
        <div className="empty" role="alert">
          <span className="empty-ic" aria-hidden="true"><IconRefresh /></span>
          <h1 style={{ margin: '0 0 var(--sp-1)', fontSize: 'var(--text-lg)', color: 'var(--fg)' }}>{t('This page ran into a problem')}</h1>
          <p>{t("Reloading usually fixes it. What happened has been noted in your server's log, and nothing was sent anywhere else.")}</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'center', gap: 'var(--sp-2)' }}>
            <button type="button" className="btn primary" onClick={() => window.location.reload()}>{t('Reload')}</button>
            <button type="button" className="btn ghost" onClick={() => window.location.assign('/')}>{t('Go to Home')}</button>
            <a className="btn ghost" href="/help/troubleshooting">{t('Troubleshooting')}</a>
          </div>
          <p style={{ marginTop: 'var(--sp-3)' }}>{t('Looking after this server, and it keeps happening?')} <a href="/settings/logs?tab=bundle">{t('Download the logs, sanitized for sharing, for whoever is helping you.')}</a></p>
        </div>
      </main>
    )
  }
}
