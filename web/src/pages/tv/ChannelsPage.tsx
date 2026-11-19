// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes } from 'react-router-dom'
import { api, fmtRuntime, mediaUrl } from '../../api'
import { BackLink, EmptyState } from '../../components/common'
import BrandLogo from '../../components/BrandLogo'
import { IconEdit, IconPlus, IconRefresh, IconSettings, IconTrash, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import { ask } from '../../ui'
import { PageHead, Row, Section, Switch } from '../settings/ui'
import ChannelEditor from './ChannelEditor'
import { brandOf, forgetLineup, type Channel, type Suggestion } from './tvApi'
import './tvsettings.css'
import { t, tk, tn } from '../../i18n'

// WittyTV's channels from your library, on a page of their own: the list, the
// channels it suggests, and an editor per channel at /tv/channels/<id>. Tuners,
// guide data and the options stay in Settings, Channels & tuners.

export default function ChannelsPage() {
  const { user } = useApp()
  if (!user?.isAdmin) {
    return <EmptyState icon={<IconTv />} title={t('Only an administrator can change channels')} text={t('Ask whoever runs this server to add or change a channel.')} />
  }
  return (
    <div className="tvc-page set-main">
      <Routes>
        <Route index element={<ChannelList />} />
        <Route path="new" element={<ChannelEditor />} />
        <Route path=":id" element={<ChannelEditor />} />
        <Route path="*" element={<Navigate to="/tv/channels" replace />} />
      </Routes>
    </div>
  )
}

const GROUP_NAMES: Record<Suggestion['group'], string> = { network: tk('TV networks'), era: tk('Decades'), genre: tk('Genres'), studio: tk('Studios'), collection: tk('Your collections'), show: tk('Marathons'), classic: tk('Classics') }

function ChannelList() {
  const [list, setList] = useState<Channel[] | null>(null)
  const [sug, setSug] = useState<Suggestion[] | null>(null)
  const [busy, setBusy] = useState('')
  const [facts, setFacts] = useState<{ running: boolean; total: number; done: number; found: number } | null>(null)
  const load = () => {
    api.get<Channel[]>('/channels').then(setList).catch((e) => toast(e.message))
    api.get<Suggestion[]>('/channels/suggestions').then(setSug).catch(() => setSug([]))
  }
  useEffect(load, [])
  useEffect(() => {
    if (!facts?.running) return
    const timer = setTimeout(() => api.get<typeof facts>('/channels/networks/refresh').then((f) => { setFacts(f); if (f && !f.running) { toast(tn('Network facts filled in for {count} show', 'Network facts filled in for {count} shows', f.found)); load() } }).catch(() => {}), 2000)
    return () => clearTimeout(timer)
  }, [facts])
  const add = async (s: Suggestion) => {
    setBusy(s.key)
    try {
      await api.post('/channels', { name: s.name, description: s.description, definition: s.definition })
      forgetLineup()
      toast(t('{name} is on the air', { name: s.name }))
      load()
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const remove = async (c: Channel) => {
    if (!await ask({ title: t('Delete the channel “{name}”?', { name: c.name }), body: t('Your files aren’t touched.'), confirm: t('Delete channel'), danger: true })) return
    try { await api.del(`/channels/${c.id}`); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const setOn = async (c: Channel, enabled: boolean) => {
    try { await api.put(`/channels/${c.id}`, { enabled }); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const addAll = async (items: Suggestion[]) => {
    setBusy('all')
    let made = 0
    try {
      for (const s of items) {
        await api.post('/channels', { name: s.name, description: s.description, definition: s.definition })
        made++
      }
      forgetLineup()
      toast(tn('{count} channel is on the air', '{count} channels are on the air', made))
    } catch (e: any) { toast(e.message) } finally { setBusy(''); load() }
  }
  const groups = (['classic', 'genre', 'studio', 'collection', 'era', 'network', 'show'] as const).map((g) => ({ g, items: (sug || []).filter((s) => s.group === g && !s.exists) })).filter((x) => x.items.length)
  return (
    <>
      <div className="tvc-back"><BackLink to="/tv" label={t('Guide')} /></div>
      <PageHead title={t('Channels')} desc={t('Always-on TV channels made from your library. They run all the time, like real TV: tune in and you join the show part-way through. FFmpeg only runs while someone is watching.')}
        actions={<Link className="btn primary" to="new"><IconPlus /> {t('New channel')}</Link>} />
      <Section title={tk('Your channels')} desc={list && list.length > 0 ? tn('{count} channel, {on} on the air.', '{count} channels, {on} on the air.', list.length, { on: list.filter((c) => c.enabled).length }) : undefined}>
        {!list && <div className="center"><div className="spinner" /></div>}
        {list && list.length === 0 && <p className="muted">{t('No channels from your library yet. Press New channel, or add one of those below with one click.')}</p>}
        {list && list.length > 0 && (
          <ul className="tvs-list" aria-label={t('Your channels')}>
            {list.map((c) => (
              <li key={c.id} className="tvs-item">
                <span className="tvs-num">{c.number}</span>
                {c.logo ? <img className="tvs-logo" src={mediaUrl(`/api/channels/${c.id}/logo?v=${c.logo}`)} alt="" /> : brandOf(c.definition, c.look) ? <span className="tvs-logo"><BrandLogo kind={brandOf(c.definition, c.look)!.kind} name={brandOf(c.definition, c.look)!.name} label={false} /></span> : null}
                <div className="tvs-item-text"><b>{c.name}</b><span>{c.description || ({ simple: t('Simple'), rules: t('Rules'), timetable: t('Timetable') })[c.definition.mode]}</span></div>
                <Switch checked={c.enabled} label={t('{name} is on the air', { name: c.name })} onChange={(v) => void setOn(c, v)} />
                <Link className="btn sm ghost" to={String(c.id)} aria-label={t('Edit {name}', { name: c.name })}><IconEdit /> {t('Edit')}</Link>
                <button className="btn sm ghost" onClick={() => void remove(c)} aria-label={t('Delete {name}', { name: c.name })}><IconTrash /></button>
              </li>
            ))}
          </ul>
        )}
      </Section>
      {sug && sug.length > 0 && groups.length > 0 && (
        <Section title={tk('Add with one click')} desc={t('Channels your library can fill. Add puts one on the air; Change first opens it in the editor.')}>
          {groups.map(({ g, items }) => (
            <div key={g} className="tvs-sug-group" role="group" aria-label={t(GROUP_NAMES[g])}>
              <div className="tvs-sug-head"><h3 className="tvs-h4">{t(GROUP_NAMES[g])}</h3>{items.length > 1 && <button className="btn sm ghost" disabled={!!busy} onClick={() => void addAll(items)}><IconPlus /> {t('Add all {count}', { count: items.length })}</button>}</div>
              <div className="tvs-sugs">
                {items.map((s) => (
                  <div key={s.key} className="tvs-sug">
                    {brandOf(s.definition) ? <b className="tvs-sug-name"><BrandLogo kind={brandOf(s.definition)!.kind} name={brandOf(s.definition)!.name} label={false} className="tvs-sug-logo" /><span>{s.name}</span></b> : <b>{s.name}</b>}
                    <span>{s.description}</span>
                    <span className="tvs-dim">{tn('{count} title', '{count} titles', s.stats.titles)} · {fmtRuntime(s.stats.seconds)}</span>
                    <div className="tvs-sug-actions">
                      <button className="btn sm primary" disabled={busy === s.key} onClick={() => void add(s)} aria-label={t('Add {name}', { name: s.name })}><IconPlus /> {t('Add')}</button>
                      <Link className="btn sm ghost" to="new" state={{ suggestion: s }} aria-label={t('Change {name} first', { name: s.name })}>{t('Change first')}</Link>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </Section>
      )}
      {sug && sug.length === 0 && <p className="muted">{t('Nothing to suggest yet. Once a TV or film library has been scanned, channels you can add with one click appear here.')}</p>}
      <Section title={tk('More')}>
        <Row label={t('Network facts')} desc={t("Fills in the network and the original day and time of shows that don't have them yet, by asking your metadata services. Files are not touched. Network names are plain facts from metadata: nothing here is affiliated with, or endorsed by, any network.")}>
          <button className="btn sm" disabled={facts?.running} onClick={() => api.post<typeof facts>('/channels/networks/refresh').then(setFacts).catch((e) => toast(e.message))}>
            <IconRefresh /> {facts?.running ? t('Asking… {done} of {total}', { done: facts.done, total: facts.total }) : t('Fill in missing facts')}
          </button>
        </Row>
        <Row label={t('Tuners, IPTV and guide data')} desc={t("Live TV sources, their channels, XMLTV listings and WittyTV's options are in Settings.")}>
          <Link className="btn sm" to="/settings/channels"><IconSettings /> {t('Open Channels & tuners')}</Link>
        </Row>
      </Section>
    </>
  )
}
