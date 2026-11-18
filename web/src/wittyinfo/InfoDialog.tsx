// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { t } from '../i18n'
import { desktopOnlyWords, editorAllowed } from './desktopOnly'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Dialog, Select } from '../ui'
import InfoEditor from './InfoEditor'

// "Edit scenes and skips": the .wittyinfo editor in a dialog, from a film's or an episode's
// page (target item) or a show's (target show, with the whole show or one season's defaults).
// Preview opens the player a few seconds before the part, with the editor open there too.

export default function InfoDialog({ target, id, title, seasons, season: start, onClose, onChanged, song }: {
  target: 'item' | 'show' | 'album'; id: number; title: string; seasons?: number[]; season?: number | null; song?: boolean
  onClose: () => void; onChanged?: () => void
}) {
  const nav = useNavigate()
  const [season, setSeason] = useState<number | null>(start ?? null)
  const base = target === 'item' ? `/wittyinfo/item/${id}` : target === 'album' ? `/wittyinfo/album/${id}` : `/wittyinfo/show/${id}${season != null ? `?season=${season}` : ''}`
  const scope = target === 'album' ? 'album' : target === 'item' ? (song ? 'song' : 'item') : season != null ? 'season' : 'show'
  if (!editorAllowed()) return <Dialog title={t('Scenes and skips')} size="sm" onClose={onClose}><p>{desktopOnlyWords()}</p></Dialog>
  return (
    <Dialog title={target === 'album' ? t('Album details') : song ? t('Song details and lyrics') : target === 'item' ? t('Scenes and skips') : t('Skip defaults')} description={title} size="wide" className="wi-dialog" onClose={onClose}>
      {target === 'show' && (
        <div className="wi-scope">
          <label htmlFor="wi-scope-pick">{t('For')}</label>
          <Select id="wi-scope-pick" inputSize="sm" value={season == null ? '' : String(season)} onChange={(e) => setSeason(e.target.value === '' ? null : Number(e.target.value))}
            options={[{ value: '', label: t('Every episode of the show') }, ...(seasons || []).map((n) => ({ value: String(n), label: n === 0 ? t('Every special') : t('Every episode of season {n}', { n }) }))]} />
        </div>
      )}
      <InfoEditor key={base} base={base} scope={scope} onSaved={onChanged} songSrc={song ? `/api/items/${id}/stream` : undefined}
        onPreview={target === 'item' && !song ? (at) => nav(`/play/${id}?start=${Math.floor(at)}&wittyinfo=1`) : undefined} />
    </Dialog>
  )
}
