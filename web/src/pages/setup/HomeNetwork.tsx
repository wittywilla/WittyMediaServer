// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { toast } from '../../state'
import { api } from '../../api'
import { t } from '../../i18n'
import { TickRow } from './parts'

// The home network, in the Ready step's Customize: whether apps at home find this server by themselves (on unless
// someone says otherwise), its address, and the number at the end of it (the port). Each is saved as it changes.
export function Network() {
  const net = useNetwork()
  const [find, setFind] = useState(true)
  const [lan, setLan] = useState<string[]>([])
  useEffect(() => {
    api.get<Record<string, string>>('/settings').then((v) => setFind(v.lanDiscovery !== '0')).catch(() => {})
    api.get<{ lanUrls?: string[] }>('/system').then((s) => setLan(s.lanUrls || [])).catch(() => {})
  }, [])
  // An address another device at home can open. This one may only be "localhost".
  const here = /^(localhost|127\.|\[::1\])/.test(location.hostname) || location.hostname.endsWith('.localhost') ? '' : location.origin
  const addrs = here ? [here] : lan
  const change = (v: boolean) => {
    setFind(v)
    api.put('/settings', { lanDiscovery: v ? '1' : '0' }).catch(() => { setFind(!v); toast(t('That couldn’t be saved. You can change it later in Settings, Network.'), { tone: 'danger' }) })
  }
  return (
    <div>
      <TickRow kind="switch" checked={find} onChange={change} title={t('Find this server automatically')} text={t('Apps on your home network find it by its name, with no address to type.')} />
      {addrs.length > 0
        ? <p className="su-addr su-gap">{t('Other devices at home open:')} {addrs.slice(0, 2).map((a) => <code key={a}>{a}</code>)}</p>
        : <p className="muted su-gap">{t('Right now it answers only on this computer. Settings, Network says how to let other devices at home reach it.')}</p>}
      {net.view
        ? <div className="su-gap"><PortSetting view={net.view} onSaved={net.setView} compact /></div>
        : <p className="muted">{net.failed ? t('The port couldn’t be read. It is under Settings, Network.') : t('Reading the port…')}</p>}
    </div>
  )
}
