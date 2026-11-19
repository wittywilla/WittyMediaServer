// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { Programme } from './tvApi'
import { fmtClock } from './tvApi'
import { t } from '../../i18n'

// "We'll be right back": the card over a channel's slate, when nothing is scheduled. The
// watch view and the guide's preview both show it, so neither looks broken while it's on.
export default function Slate({ name, next }: { name: string; next: Programme | null }) {
  return (
    <div className="ctv-slate" aria-live="polite">
      <div className="ctv-slate-card">
        <div className="ctv-slate-ch">{name}</div>
        <div className="ctv-slate-msg">{t("We'll be right back")}</div>
        {next && <div className="ctv-slate-next">{t('Coming up at {time}: {title}', { time: fmtClock(next.start), title: next.title })}</div>}
      </div>
    </div>
  )
}
