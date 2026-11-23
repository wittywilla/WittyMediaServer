// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { IconCheck } from '../icons'
import { Button } from '../ui'
import { t } from '../i18n'

// A key for an outside service (TMDB, TheTVDB, Musixmatch) that is already saved on the server. The server never sends
// a key back (GET /settings says <name>Set: "1" instead), so a field here could only be empty or full of dots, and either
// looks like being asked again (#534). It says the key is saved, with a button to change it; every page that takes a key
// shows this once one is saved, so a key given in setup is never asked for again.
export function SavedKey({ name, onChange, disabled }: { name: string; onChange: () => void; disabled?: boolean }) {
  return (
    <div className="saved-key" role="group" aria-label={t('{name} key', { name })}>
      <span className="saved-key-mark"><IconCheck size={16} /> {t('Key saved')}</span>
      <Button size="sm" disabled={disabled} onClick={onChange} aria-label={t('Change the {name} key', { name })}>{t('Change')}</Button>
    </div>
  )
}

// Whether the server has a key saved, from GET /settings: a secret comes back as dots, with <name>Set beside it.
export const keySaved = (s: Record<string, string> | null | undefined, key: string) => s?.[key + 'Set'] === '1'

// A settings page's field for a secret (useServerSettings' draft): shown as saved until Change; then an empty field,
// and while it's empty the draft keeps the dots, so Save changes leaves the saved key as it is. Saved or discarded, it
// goes back to saying it's saved.
export function SavedKeyField({ name, saved, value, onChange, placeholder, label, disabled }: {
  name: string; saved: boolean; value: string; onChange: (v: string) => void; placeholder?: string; label: string; disabled?: boolean
}) {
  const [changing, setChanging] = useState(false)
  const [first] = useState(value)
  const masked = value.includes('•')
  const emptied = useRef(false)
  useEffect(() => { if (masked && !emptied.current) setChanging(false); emptied.current = false }, [value]) // eslint-disable-line react-hooks/exhaustive-deps
  if (saved && !changing) return <SavedKey name={name} disabled={disabled} onChange={() => setChanging(true)} />
  return (
    <div className="saved-key-field">
      <input className="input" disabled={disabled} placeholder={placeholder} value={masked ? '' : value} spellCheck={false} autoComplete="off" aria-label={label} autoFocus={changing}
        onChange={(e) => { const v = e.target.value; if (!v && first.includes('•')) { emptied.current = true; onChange(first) } else onChange(v) }} />
      {saved && <Button size="sm" variant="ghost" onClick={() => { onChange(first); setChanging(false) }}>{t('Keep the saved key')}</Button>}
    </div>
  )
}
