// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { type ShareLink } from '../../gallery'
import { date, t, tk, tn, bidi } from '../../i18n'
import { toast, useApp } from '../../state'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'
import LegalNote from '../../components/LegalNote'
import { ask } from '../../ui/Overlay'
import RemoteControlSections from './RemoteControl'

// Uploads, links for people without an account, and the place kept for the local AI. (The
// photo map's online map is WittyPhotos' own setting, under Modules.)

function ServerSharing() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return st.failed ? <LoadError what={t('This page')} onRetry={st.retry} /> : null
  return (
    <>
      <Section title={tk('Uploads')} desc={t("Adults can add pictures and videos to Photos and Vertical videos libraries from their devices. Files are saved in an Uploads folder inside the library's first folder and never replace anything.")}>
        <Row label={t('Allow uploads')}>
          <Switch label={t('Allow uploads')} checked={on('uploads')} onChange={(v) => set('uploads', v ? '1' : '0')} />
        </Row>
        <Row label={t('Largest upload')} desc={t('For one file.')} dim={!on('uploads')}>
          <select className="select" value={s.uploadMaxMb || '4096'} disabled={!on('uploads')} onChange={(e) => set('uploadMaxMb', e.target.value)}>
            <option value="512">512 MB</option><option value="2048">2 GB</option><option value="4096">4 GB</option><option value="16384">16 GB</option><option value="65536">64 GB</option>
          </select>
        </Row>
        <Row label={t('Room for each person')} desc={t('What one account can upload in all, across galleries. Administrators have no limit. Uploads also stop when under 2 GB is left on the disk.')} dim={!on('uploads')}>
          <select className="select" value={s.uploadAccountMb || '102400'} disabled={!on('uploads')} onChange={(e) => set('uploadAccountMb', e.target.value)}>
            <option value="10240">10 GB</option><option value="102400">100 GB</option><option value="1048576">1 TB</option><option value="0">{t('No limit')}</option>
          </select>
        </Row>
      </Section>
      <Section title={tk('Links for people without an account')} desc={t("Adults can make a link to one picture or video, or to an album's pictures. Whoever has the link sees that and nothing else, without signing in. It only works where this server can be reached: on your home network until you set up remote access.")}>
        <Row label={t('Allow links')} desc={t('Turning this off stops every existing link too.')}>
          <Switch label={t('Allow links')} checked={on('shareLinks')} onChange={(v) => set('shareLinks', v ? '1' : '0')} />
        </Row>
      </Section>
      <RemoteControlSections s={s} set={set} />
      <Section title={tk('Local AI')} desc={t('Planned, not available yet.')}>
        <Row label={t('People and places in photos')} desc={t("Will recognise faces and places on this machine, with nothing sent anywhere. It doesn't exist yet, so this does nothing and nothing looks at your pictures.")} dim>
          <Switch label={t('People and places in photos')} checked={false} disabled onChange={() => {}} />
        </Row>
      </Section>
      <SaveBar {...st} />
    </>
  )
}

function Links() {
  const { user } = useApp()
  const [links, setLinks] = useState<ShareLink[] | null>(null)
  const [all, setAll] = useState(false)
  const load = () => api.get<ShareLink[]>(`/shares${all ? '?all=1' : ''}`).then(setLinks).catch(() => setLinks([]))
  useEffect(() => { load() }, [all]) // eslint-disable-line react-hooks/exhaustive-deps
  const end = async (l: ShareLink) => {
    if (!await ask({ title: t('End the link to “{title}”?', { title: l.title }), body: t('Anyone who has it will no longer be able to open it.'), confirm: t('End link'), danger: true })) return
    try { await api.del(`/shares/${l.id}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title={tk('Your links')} desc={t("Links stay valid until their end date, or until you end them here. A link itself can't be shown again: the server only keeps a fingerprint of it.")}
      actions={user?.isAdmin ? <label className="set-check"><input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} /> {t("Everyone's")}</label> : undefined}>
      {links && !links.length && <p className="muted" style={{ padding: '4px 0' }}>{t('No links yet. Make one with Share on a picture, video or album.')}</p>}
      {links?.map((l) => (
        <Row key={l.id} label={l.title} desc={[l.kind === 'photos' ? t('Pictures') : t('Single item'), l.owner && all ? t('by {name}', { name: l.owner }) : '', tn('opened {count} time', 'opened {count} times', l.views), l.expiresAt ? t('until {date}', { date: date(l.expiresAt * 1000, { dateStyle: 'short' }) }) : t('no end date'), l.allowDownload ? t('downloads allowed') : ''].filter(Boolean).map(bidi).join(' · ')}>
          <button className="btn sm ghost" onClick={() => end(l)}>{t('End link')}</button>
        </Row>
      ))}
    </Section>
  )
}

export default function SharingSettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title={tk('Sharing & photos')} desc={t('Who can add to this server, and what can be seen from outside it.')} />
      {user?.isAdmin && <ServerSharing />}
      <Links />
      <LegalNote kind="sharing" className="set-legal" />
    </>
  )
}
