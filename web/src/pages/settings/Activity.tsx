// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect } from 'react'
import { clearTaskHistory, refreshActivity, setActivityHidden, useActivity } from '../../activity'
import { Button, LoadError, Select, ask, toast } from '../../ui'
import { useApp } from '../../state'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'
import { t, tk } from '../../i18n'

// Settings for the activity button. Administrators choose who gets it, and everyone it's
// offered to can hide it for themselves.
export default function ActivitySettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title={tk('Activity')} desc={t('The button in the top bar that shows library scans and other background work, with their progress and any errors.')} />
      <YourButton />
      {user?.isAdmin && <ServerChoices />}
    </>
  )
}

function YourButton() {
  const a = useActivity()
  const change = (on: boolean) => { setActivityHidden(!on).catch((e) => toast(e.message)) }
  if (!a.loaded) return null
  if (!a.access.visible) {
    return (
      <Section title={tk('On this account')}>
        <Row label={t('Activity button')} desc={a.access.enabled ? t('An administrator hasn’t turned this on for your account.') : t('The owner of this server has turned this off.')} />
      </Section>
    )
  }
  return (
    <Section title={tk('On this account')} desc={a.access.readOnly ? t('You see what’s going on, with no file names, no one else’s details and no controls.') : undefined}>
      <Row label={t('Show the activity button')} desc={t('Only changes what you see. The work carries on either way.')}>
        <Switch checked={!a.hidden} label={t('Show the activity button')} onChange={change} />
      </Row>
    </Section>
  )
}

function ServerChoices() {
  const st = useServerSettings()
  const { s, set, on, dirty, busy, discard } = st
  // Whatever was saved applies to this page's own button straight away.
  const save = async () => { await st.save(); refreshActivity() }
  useEffect(() => { refreshActivity() }, [])
  if (st.failed) return <LoadError what={t('These settings')} onRetry={st.retry} />
  if (!s) return null
  const clear = async () => {
    if (!await ask({ title: t('Clear the activity history?'), body: t('What happened stops being listed in the panel. WittyLog keeps its own record, and anything going on now carries on.'), confirm: t('Clear history'), danger: true })) return
    try { await clearTaskHistory(); toast(t('History cleared')) } catch (e) { toast((e as Error).message) }
  }
  const enabled = on('activityEnabled')
  return (
    <>
      <Section title={tk('On this server')} desc={t('For everyone on this server.')}>
        <Row label={t('Activity button')} desc={t('Off hides it from everyone, administrators too. Background work carries on, and is still in the log.')}>
          <Switch checked={enabled} label={t('Activity button')} onChange={(v) => set('activityEnabled', v ? '1' : '0')} />
        </Row>
        <Row dim={!enabled} label={t('Who else can see it')} desc={t('Administrators always can. Anyone else gets a read-only view of the libraries they can see: no file names or paths, no one else’s details, no error text and no controls.')}>
          <Select value={s.activityViewers || 'admins'} disabled={!enabled} onChange={(e) => set('activityViewers', e.target.value)} options={[
            { value: 'admins', label: t('Only administrators') },
            { value: 'adults', label: t('Adult profiles too') },
            { value: 'everyone', label: t('Everyone, including kids and guests') },
          ]} />
        </Row>
        <Row label={t('Who else can see statistics')} desc={t('For the WittyStats module and the apps. Administrators see everything. Anyone else sees the server’s name and how long it’s been up, their own streams, and the libraries they can see: never the machine itself.')}>
          <Select value={s.statsViewers || 'admins'} onChange={(e) => set('statsViewers', e.target.value)} options={[
            { value: 'admins', label: t('Only administrators') },
            { value: 'adults', label: t('Adult profiles too') },
            { value: 'everyone', label: t('Everyone, including kids and guests') },
          ]} />
        </Row>
      </Section>
      <Section title={tk('History')} desc={t('Finished work is kept so you can see what happened and try again. When either limit is reached, the oldest goes first.')}>
        <Row label={t('Keep at most')} desc={t('Between 10 and 5,000 tasks.')}>
          <input className="input" type="number" min={10} max={5000} value={s.activityKeepCount ?? '500'} onChange={(e) => set('activityKeepCount', e.target.value)} />
        </Row>
        <Row label={t('For no longer than')} desc={t('Between 1 and 365 days.')}>
          <input className="input" type="number" min={1} max={365} value={s.activityKeepDays ?? '30'} onChange={(e) => set('activityKeepDays', e.target.value)} />
        </Row>
        <Row label={t('Clear the history')} desc={t('Removes everything listed. Anything going on now carries on.')}>
          <Button variant="danger" size="sm" onClick={clear}>{t('Clear history')}</Button>
        </Row>
      </Section>
      <SaveBar dirty={dirty} busy={busy} save={save} discard={discard} />
    </>
  )
}
