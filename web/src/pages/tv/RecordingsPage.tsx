// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { toast, useApp } from '../../state'
import { ask } from '../../ui'
import MoreInfo from './MoreInfo'
import { themeRoot, useLook } from './look'
import { fmtClock, fmtDay, type Recording, type RecordingRule } from './tvApi'
import { useFeatures } from '../../sdui/useSdui'
import RecordOptions from './RecordOptions'
import './tv.css'
import './tvthemes.css'
import './tvsets.css'
import { num, t, tk, tn, tx } from '../../i18n'

const STATUS: Record<Recording['status'], string> = { scheduled: tk('Scheduled'), recording: tk('Recording now'), done: tk('Done'), failed: tk('Failed'), cancelled: tk('Cancelled') }

function fmtSize(b?: number) {
  if (!b) return ''
  return b > 1e9 ? num(b / 1e9, { maximumFractionDigits: 1, minimumFractionDigits: 1 }) + ' GB' : num(Math.round(b / 1e6)) + ' MB'
}

// ", from 2 min early to 5 min late", only when there is any.
function padText(before?: number, after?: number): string {
  if (!before && !after) return ''
  if (before && after) return ' · ' + t('From {before} min early to {after} min late', { before, after })
  return ' · ' + (before ? t('From {before} min early', { before }) : t('To {after} min late', { after }))
}

// What a series records, in a line.
function ruleText(r: RecordingRule): string {
  const parts = [r.anyChannel ? t('Every airing, on any channel') : t('Every airing on {channel}', { channel: r.channel })]
  if (r.newOnly) parts.push(t('new episodes only'))
  if (r.keep) parts.push(tn('keeps the newest', 'keeps the newest {count}', r.keep))
  return parts.join(', ') + padText(r.padBefore, r.padAfter)
}

export default function RecordingsPage() {
  const { user } = useApp()
  const look = useLook()
  const [list, setList] = useState<Recording[] | null>(null)
  const [info, setInfo] = useState<Recording | null>(null)
  const [rules, setRules] = useState<RecordingRule[]>([])
  const [err, setErr] = useState('')
  const [editing, setEditing] = useState<RecordingRule | null>(null)
  // A server with tv-dvr-rules-2 has padding, new episodes only, any channel and keep the last few.
  const rules2 = !!useFeatures()?.has('tv-dvr-rules-2')
  const load = () => {
    api.get<Recording[]>('/recordings').then((l) => { setList(l); setErr('') }).catch((e) => setErr(e.message))
    api.get<RecordingRule[]>('/recording-rules').then(setRules).catch(() => {})
  }
  useEffect(() => { load(); const t = setInterval(load, 15000); return () => clearInterval(t) }, [])
  const remove = async (r: Recording, deleteFile: boolean) => {
    const finished = r.status === 'done' || r.status === 'failed'
    const yes = await ask(deleteFile
      ? { title: t('Delete the recording of “{title}”?', { title: r.title }), body: t('Its file goes too. This can’t be undone.'), confirm: t('Delete recording'), danger: true }
      : finished
        ? { title: t('Remove “{title}” from this list?', { title: r.title }), body: t('Any file it made stays where it is.'), confirm: t('Remove from list') }
        : { title: t('Cancel the recording of “{title}”?', { title: r.title }), confirm: t('Cancel recording'), cancel: t('Keep recording'), danger: true })
    if (!yes) return
    try { await api.del(`/recordings/${r.id}${deleteFile ? '?deleteFile=1' : ''}`); load() } catch (e: any) { toast(e.message) }
  }
  const removeRule = async (r: RecordingRule) => {
    try { await api.del(`/recording-rules/${r.id}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="ctv ctv-page ctv-list-page" {...themeRoot(look)}>
      <header className="ctv-list-head">
        <h1>{t('Recordings')}</h1>
        <Link className="ctv-btn sm" to="/tv">{t('Back to the guide')}</Link>
      </header>
      {err && <div className="ctv-err" role="alert">{err}</div>}
      {list && list.length === 0 && <p className="ctv-list-empty">{t('Nothing recorded yet. Highlight a show on a Live TV channel in the guide and press R.')}</p>}
      {list && list.length > 0 && (
        <ul className="ctv-list">
          {list.map((r) => (
            <li key={r.id} className={'ctv-rec ' + r.status}>
              <div className="ctv-rec-main">
                <b>{r.title}</b>{r.sub && <small>{r.sub}</small>}
                <span>{t('{day}, {start} to {end}', { day: fmtDay(r.start), start: fmtClock(r.start), end: fmtClock(r.stop) })} · {t('Channel {number} {name}', { number: r.number, name: r.channel })}{r.sizeBytes ? ' · ' + fmtSize(r.sizeBytes) : ''}{padText(r.padBefore, r.padAfter)}{r.ruleTitle ? ' · ' + t('Series: {title}', { title: r.ruleTitle }) : ''}</span>
                {r.error && <span className="ctv-rec-err" role="alert">{tx(r.error)}</span>}
                {r.conflict && r.status === 'scheduled' && <span className="ctv-rec-err">{t('No tuner will be free when this starts: another recording or a viewer has them all.')}</span>}
              </div>
              <span className="ctv-rec-status">{tx(STATUS[r.status]) || r.status}</span>
              {r.channelKey && <button className="ctv-btn sm" onClick={() => setInfo(r)} aria-haspopup="dialog" aria-label={t('More info about {title}', { title: r.title })}>{t('More info')}</button>}
              <button className="ctv-btn sm" onClick={() => void remove(r, false)}>{r.status === 'done' || r.status === 'failed' ? t('Remove') : t('Cancel')}</button>
              {user?.isAdmin && r.path && r.status !== 'recording' && r.status !== 'scheduled' && <button className="ctv-btn sm" onClick={() => void remove(r, true)}>{t('Delete file')}</button>}
            </li>
          ))}
        </ul>
      )}
      {rules.length > 0 && (
        <section>
          <h2 className="ctv-list-h2">{t('Series')}</h2>
          <ul className="ctv-list">
            {rules.map((r) => (
              <li key={r.id} className="ctv-rec">
                <div className="ctv-rec-main"><b>{r.title}</b><span>{ruleText(r)}</span></div>
                {rules2 && <button className="ctv-btn sm" onClick={() => setEditing(r)} aria-haspopup="dialog" aria-label={t('Change the series {title}', { title: r.title })}>{t('Change')}</button>}
                <button className="ctv-btn sm" onClick={() => void removeRule(r)} aria-label={t('Stop recording {title}', { title: r.title })}>{t('Stop')}</button>
              </li>
            ))}
          </ul>
        </section>
      )}
      {editing && <RecordOptions rule={editing} onClose={() => setEditing(null)} onSaved={load} />}
      {info && info.channelKey && <MoreInfo channelKey={info.channelKey} at={info.start} onClose={() => setInfo(null)} />}
      <p className="ctv-list-note">{t('Recordings are of Live TV channels only. Library channels play what is already in your library. Protected broadcasts are never recorded.')}</p>
    </div>
  )
}
