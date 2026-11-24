// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { reportClient } from '../../../errorReport'
import { t } from '../../../i18n'
import { Button, Notice } from '../../../ui'

// A fence around the module store's pages. Whatever a catalogue says, and whatever goes wrong while
// one of these pages is drawn, it stops here: the rest of Settings, the menu and every other page keep
// working, and this part says so in plain words with a way to try again. What happened goes to the
// server's log, as for any page.
export default class StoreBoundary extends Component<{ children: ReactNode; what?: string }, { failed: boolean; tries: number }> {
  state = { failed: false, tries: 0 }

  static getDerivedStateFromError() { return { failed: true } }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    const e = error as Error | undefined
    reportClient({ level: 'error', kind: 'js', message: `module store: ${e?.message || String(error)}`, stack: [e?.stack, info.componentStack].filter(Boolean).join('\n') })
  }

  render() {
    // A new try draws the part again from the start, with what it fetches fetched again.
    if (!this.state.failed) return <StoreBoundaryKey key={this.state.tries}>{this.props.children}</StoreBoundaryKey>
    return (
      <div className="st-broken" role="alert">
        <Notice tone="danger" title={this.props.what || t('This part of the module store couldn’t be shown')}
          action={<Button size="sm" onClick={() => this.setState((s) => ({ failed: false, tries: s.tries + 1 }))}>{t('Try again')}</Button>}>
          {t('Your modules are fine and keep working: only this page had a problem. It has been noted in the server’s log. If it keeps happening, the store’s list may have something this version can’t read; check again later or update the server.')}
        </Notice>
      </div>
    )
  }
}

function StoreBoundaryKey({ children }: { children: ReactNode }) { return <>{children}</> }
