import { useEffect } from 'react'
import { clearTaskHistory, refreshActivity, setActivityHidden, useActivity } from '../../activity'
import { Button, LoadError, Select, ask, toast } from '../../ui'
import { useApp } from '../../state'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'

// Settings for the activity button. Administrators choose who gets it, and everyone it's
// offered to can hide it for themselves.
export default function ActivitySettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Activity" desc="The button in the top bar that shows library scans and other background work, with their progress and any errors." />
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
      <Section title="On this account">
        <Row label="Activity button" desc={a.access.enabled ? 'An administrator hasn’t turned this on for your account.' : 'The owner of this server has turned this off.'} />
      </Section>
    )
  }
  return (
    <Section title="On this account" desc={a.access.readOnly ? 'You see what’s going on, with no file names, no one else’s details and no controls.' : undefined}>
      <Row label="Show the activity button" desc="Only changes what you see. The work carries on either way.">
        <Switch checked={!a.hidden} label="Show the activity button" onChange={change} />
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
  if (st.failed) return <LoadError what="These settings" onRetry={st.retry} />
  if (!s) return null
  const clear = async () => {
    if (!await ask({ title: 'Clear the activity history?', body: 'What happened stops being listed in the panel. WittyLog keeps its own record, and anything going on now carries on.', confirm: 'Clear history', danger: true })) return
    try { await clearTaskHistory(); toast('History cleared') } catch (e) { toast((e as Error).message) }
  }
  const enabled = on('activityEnabled')
  return (
    <>
      <Section title="On this server" desc="For everyone on this server.">
        <Row label="Activity button" desc="Off hides it from everyone, administrators too. Background work carries on, and is still in the log.">
          <Switch checked={enabled} label="Activity button" onChange={(v) => set('activityEnabled', v ? '1' : '0')} />
        </Row>
        <Row dim={!enabled} label="Who else can see it" desc="Administrators always can. Anyone else gets a read-only view of the libraries they can see: no file names or paths, no one else’s details, no error text and no controls.">
          <Select value={s.activityViewers || 'admins'} disabled={!enabled} onChange={(e) => set('activityViewers', e.target.value)} options={[
            { value: 'admins', label: 'Only administrators' },
            { value: 'adults', label: 'Adult profiles too' },
            { value: 'everyone', label: 'Everyone, including kids and guests' },
          ]} />
        </Row>
      </Section>
      <Section title="History" desc="Finished work is kept so you can see what happened and try again. When either limit is reached, the oldest goes first.">
        <Row label="Keep at most" desc="Between 10 and 5,000 tasks.">
          <input className="input" type="number" min={10} max={5000} value={s.activityKeepCount ?? '500'} onChange={(e) => set('activityKeepCount', e.target.value)} />
        </Row>
        <Row label="For no longer than" desc="Between 1 and 365 days.">
          <input className="input" type="number" min={1} max={365} value={s.activityKeepDays ?? '30'} onChange={(e) => set('activityKeepDays', e.target.value)} />
        </Row>
        <Row label="Clear the history" desc="Removes everything listed. Anything going on now carries on.">
          <Button variant="danger" size="sm" onClick={clear}>Clear history</Button>
        </Row>
      </Section>
      <SaveBar dirty={dirty} busy={busy} save={save} discard={discard} />
    </>
  )
}
