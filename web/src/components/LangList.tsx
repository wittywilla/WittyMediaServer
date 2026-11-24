// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { langName } from '../api'
import { list, t, tn } from '../i18n'

// The languages of a title's sound tracks or subtitles, on its detail page. A film can carry 35 subtitle languages:
// as one run-on line they were a wall of words (#634). The first few show, then "+29 more", which shows the rest.
const SHOWN = 6

export function LangList({ codes }: { codes: string[] }) {
  const [all, setAll] = useState(false)
  const names = [...new Set(codes.map(langName))]
  const cut = !all && names.length > SHOWN + 1
  const shown = cut ? names.slice(0, SHOWN) : names
  return (
    <>
      {cut ? shown.join(', ') : list(shown)}
      {cut && <>{' '}<button type="button" className="lang-more" aria-expanded={false} onClick={() => setAll(true)}
        aria-label={tn('Show {count} more language', 'Show {count} more languages', names.length - SHOWN)}>{t('+{count} more', { count: names.length - SHOWN })}</button></>}
    </>
  )
}
