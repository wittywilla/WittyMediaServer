// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, profileName } from '../../api'
import { toast } from '../../state'
import { Row, Section, Switch } from './ui'

// Playing on other devices, and watch parties: the owner's switches for each, and which
// accounts may control devices signed in as someone else.

type Account = { id: number; name: string; displayName: string; kind: string; isAdmin: boolean; controlOthers: boolean }
const KIND: Record<string, string> = { kids: 'Kids profile', guest: 'Guest' }

function Accounts({ dim }: { dim: boolean }) {
  const [accounts, setAccounts] = useState<Account[] | null>(null)
  useEffect(() => { api.get<{ accounts: Account[] }>('/remote-control/accounts').then((r) => setAccounts(r.accounts)).catch(() => setAccounts([])) }, [])
  const change = async (a: Account, on: boolean) => {
    setAccounts((list) => list && list.map((x) => (x.id === a.id ? { ...x, controlOthers: on } : x)))
    try { await api.put(`/remote-control/accounts/${a.id}`, { controlOthers: on }) } catch (e: any) {
      setAccounts((list) => list && list.map((x) => (x.id === a.id ? { ...x, controlOthers: !on } : x)))
      toast(e.message)
    }
  }
  return (
    <>
      {accounts?.map((a) => (
        <Row key={a.id} dim={dim} label={profileName(a)} desc={[a.isAdmin ? 'Administrator' : KIND[a.kind] || '', a.controlOthers ? 'Can control devices signed in as anyone' : 'Controls only their own devices'].filter(Boolean).join(' · ')}>
          <Switch label={`${profileName(a)} can control other people's devices`} checked={a.controlOthers} disabled={dim} onChange={(v) => change(a, v)} />
        </Row>
      ))}
    </>
  )
}

export default function RemoteControlSections({ s, set }: { s: Record<string, string>; set: (k: string, v: string) => void }) {
  // Unset on a server from before these existed: both start on.
  const remote = s.remoteControl !== '0', parties = s.syncplay !== '0'
  return (
    <>
      <Section title="Play on other devices" desc="Any signed-in device can be played on, and steered, from another: choose a film on a phone and have it start on the TV. Everyone controls devices signed in as their own account. The switches below let an account control devices signed in as anyone, such as the TV on a Kids profile; they take effect at once. Nobody can play something on another device that its account, or their own, isn't allowed to see.">
        <Row label="Allow playing on other devices">
          <Switch label="Allow playing on other devices" checked={remote} onChange={(v) => set('remoteControl', v ? '1' : '0')} />
        </Row>
        <Accounts dim={!remote} />
      </Section>
      <Section title="Watch parties" desc="People watch the same thing at the same moment, each on their own screen, with a six-letter code to join. Each person's own limits still decide what they can see. Parties end when the server restarts.">
        <Row label="Allow watch parties">
          <Switch label="Allow watch parties" checked={parties} onChange={(v) => set('syncplay', v ? '1' : '0')} />
        </Row>
        <Row label="Who can start one" desc="Anyone signed in can join one they have the code for." dim={!parties}>
          <select className="select" value={s.syncplayCreate || 'everyone'} disabled={!parties} onChange={(e) => set('syncplayCreate', e.target.value)}>
            <option value="everyone">Everyone</option><option value="adults">Adults</option><option value="admins">Only administrators</option>
          </select>
        </Row>
      </Section>
    </>
  )
}
